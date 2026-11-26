package errs

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxStackDepth = 32

// Error is a structured error created by this package: a message, an
// optional cause, slog attributes, the creation time and a stack trace. Use
// From or [errors.AsType] to obtain it from an error chain. An *Error is
// immutable and safe for concurrent use as long as its attribute values are.
type Error struct {
	msg   string
	cause error
	attrs []slog.Attr
	time  time.Time
	stack []uintptr

	// The formatted stack trace is built on first use and cached, so logging
	// the same error repeatedly formats its stack only once.
	traceOnce sync.Once
	trace     string
}

// maxInlineChain is the chain depth that chain walks handle without a heap
// allocation; deeper chains still work, they just allocate.
const maxInlineChain = 8

// New returns an error with the given message and attributes.
func New(msg string, attrs ...slog.Attr) error {
	return newError(msg, nil, attrs)
}

// Errorf returns an error formatted with [fmt.Errorf]. The %w verb is supported
// and keeps the error chain. Use Wrap to add attributes.
func Errorf(format string, args ...any) error {
	return newError("", fmt.Errorf(format, args...), nil)
}

// Wrap returns an error that wraps err with a message, attributes and a stack
// trace, printing as "msg: err". An empty msg prints err's message alone.
// Wrap returns nil when err is nil, so it can wrap a call's result directly.
func Wrap(err error, msg string, attrs ...slog.Attr) error {
	if err == nil {
		return nil
	}
	return newError(msg, err, attrs)
}

// Recover calls fn and converts a panic into an error carrying attrs and the
// stack of the panic. A panic value that is an error stays in the chain for
// [errors.Is] and [errors.As]. Recover returns nil when fn returns normally.
func Recover(fn func(), attrs ...slog.Attr) (err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		cause, ok := r.(error)
		if !ok {
			cause = fmt.Errorf("%v", r)
		}
		err = newError("panic", cause, attrs)
	}()
	fn()
	return nil
}

// newError must be called directly by the exported function the user called,
// so that the captured stack starts at the user's frame.
func newError(msg string, cause error, attrs []slog.Attr) *Error {
	// Capture into a stack-allocated array and keep only the frames that
	// exist: typical stacks are 5-15 frames deep, so this allocates far less
	// than a full maxStackDepth slice. Skip runtime.Callers, newError and the
	// exported caller.
	var pcs [maxStackDepth]uintptr
	n := runtime.Callers(3, pcs[:])
	return &Error{
		msg:   msg,
		cause: cause,
		attrs: slices.Clone(attrs),
		time:  time.Now(),
		stack: slices.Clone(pcs[:n]),
	}
}

// From returns the outermost *Error in the chain of err.
func From(err error) (*Error, bool) {
	return errors.AsType[*Error](err)
}

// Error returns "msg: cause", or whichever of the two is present.
func (e *Error) Error() string {
	switch {
	case e.cause == nil:
		return e.msg
	case e.msg == "":
		return e.cause.Error()
	default:
		return e.msg + ": " + e.cause.Error()
	}
}

// Unwrap returns the wrapped cause, or nil.
func (e *Error) Unwrap() error {
	return e.cause
}

// appendChain appends e and every *Error found by unwrapping, outermost
// first. Callers pass a stack-allocated buffer, so walking a typical chain
// does not allocate.
func (e *Error) appendChain(dst []*Error) []*Error {
	dst = append(dst, e)
	for cur := e; ; {
		next, ok := errors.AsType[*Error](cur.cause)
		if !ok {
			return dst
		}
		dst = append(dst, next)
		cur = next
	}
}

func (e *Error) innermost() *Error {
	cur := e
	for {
		next, ok := errors.AsType[*Error](cur.cause)
		if !ok {
			return cur
		}
		cur = next
	}
}

// Attrs returns the attributes of the whole chain, merged by key, with their
// values resolved. Keys keep the position of their first appearance,
// outermost first; for a repeated key the innermost value wins, because it
// is closest to where the error happened.
func (e *Error) Attrs() []slog.Attr {
	var buf [maxInlineChain]*Error
	chain := e.appendChain(buf[:0])
	total := 0
	for _, c := range chain {
		total += len(c.attrs)
	}
	if total == 0 {
		return nil
	}
	merged := make([]slog.Attr, 0, total)
	for _, c := range chain {
		for _, a := range c.attrs {
			a.Value = a.Value.Resolve()
			// Errors carry few attributes, so a linear scan beats a map.
			if i := slices.IndexFunc(merged, func(m slog.Attr) bool { return m.Key == a.Key }); i >= 0 {
				merged[i] = a
				continue
			}
			merged = append(merged, a)
		}
	}
	return merged
}

// lookup returns the innermost attribute with the given key.
func (e *Error) lookup(key string) (slog.Attr, bool) {
	var buf [maxInlineChain]*Error
	for _, c := range slices.Backward(e.appendChain(buf[:0])) {
		for _, a := range slices.Backward(c.attrs) {
			if a.Key == key {
				return a, true
			}
		}
	}
	return slog.Attr{}, false
}

// Time returns when the innermost error of the chain was created.
func (e *Error) Time() time.Time {
	return e.innermost().time
}

// Stack returns the frames of the stack captured by the innermost error of
// the chain, starting at the frame that created it.
func (e *Error) Stack() iter.Seq[runtime.Frame] {
	pcs := e.innermost().stack
	return func(yield func(runtime.Frame) bool) {
		frames := runtime.CallersFrames(pcs)
		for {
			f, more := frames.Next()
			if f.Function != "" && !yield(f) {
				return
			}
			if !more {
				return
			}
		}
	}
}

// stackTrace formats Stack as one "function\n\tfile:line" entry per frame.
// The result is cached on the innermost error.
func (e *Error) stackTrace() string {
	in := e.innermost()
	in.traceOnce.Do(func() {
		var sb strings.Builder
		sb.Grow(len(in.stack) * 96) // a typical frame is 60-120 bytes
		for f := range in.Stack() {
			sb.WriteString(f.Function)
			sb.WriteString("\n\t")
			sb.WriteString(f.File)
			sb.WriteByte(':')
			sb.WriteString(strconv.Itoa(f.Line))
			sb.WriteByte('\n')
		}
		in.trace = sb.String()
	})
	return in.trace
}

// Format implements [fmt.Formatter]. %s and %v print the message, %q prints it
// quoted, and %+v also prints the attributes and the stack trace.
func (e *Error) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		io.WriteString(s, e.Error())
		if s.Flag('+') {
			for _, a := range e.Attrs() {
				fmt.Fprintf(s, "\n%s: %v", a.Key, a.Value)
			}
			io.WriteString(s, "\n--- stack ---\n")
			io.WriteString(s, e.stackTrace())
		}
	case 's':
		io.WriteString(s, e.Error())
	case 'q':
		fmt.Fprintf(s, "%q", e.Error())
	default:
		fmt.Fprintf(s, "%%!%c(*errs.Error=%s)", verb, e.Error())
	}
}

// LogValue implements [slog.LogValuer]. It returns a group with the message,
// the merged attributes, the time and the stack trace.
func (e *Error) LogValue() slog.Value {
	merged := e.Attrs()
	attrs := make([]slog.Attr, 0, len(merged)+3)
	attrs = append(attrs, slog.String("message", e.Error()))
	attrs = append(attrs, merged...)
	attrs = append(attrs,
		slog.Time("time", e.Time()),
		slog.String("stacktrace", e.stackTrace()),
	)
	return slog.GroupValue(attrs...)
}
