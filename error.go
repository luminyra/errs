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
}

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
	pcs := make([]uintptr, maxStackDepth)
	// Skip runtime.Callers, newError and the exported caller.
	n := runtime.Callers(3, pcs)
	return &Error{
		msg:   msg,
		cause: cause,
		attrs: slices.Clone(attrs),
		time:  time.Now(),
		stack: pcs[:n:n],
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

// chain returns e followed by every *Error found by unwrapping, outermost
// first.
func (e *Error) chain() []*Error {
	result := []*Error{e}
	for cur := e; ; {
		next, ok := errors.AsType[*Error](cur.cause)
		if !ok {
			return result
		}
		result = append(result, next)
		cur = next
	}
}

func (e *Error) innermost() *Error {
	c := e.chain()
	return c[len(c)-1]
}

// Attrs returns the attributes of the whole chain, merged by key, with their
// values resolved. Keys keep the position of their first appearance,
// outermost first; for a repeated key the innermost value wins, because it
// is closest to where the error happened.
func (e *Error) Attrs() []slog.Attr {
	var merged []slog.Attr
	index := make(map[string]int)
	for _, c := range e.chain() {
		for _, a := range c.attrs {
			a.Value = a.Value.Resolve()
			if i, ok := index[a.Key]; ok {
				merged[i] = a
				continue
			}
			index[a.Key] = len(merged)
			merged = append(merged, a)
		}
	}
	return merged
}

// lookup returns the innermost attribute with the given key.
func (e *Error) lookup(key string) (slog.Attr, bool) {
	for _, c := range slices.Backward(e.chain()) {
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
func (e *Error) stackTrace() string {
	var sb strings.Builder
	for f := range e.Stack() {
		sb.WriteString(f.Function)
		sb.WriteString("\n\t")
		sb.WriteString(f.File)
		sb.WriteByte(':')
		sb.WriteString(strconv.Itoa(f.Line))
		sb.WriteByte('\n')
	}
	return sb.String()
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
	attrs := []slog.Attr{slog.String("message", e.Error())}
	attrs = append(attrs, e.Attrs()...)
	attrs = append(attrs,
		slog.Time("time", e.Time()),
		slog.String("stacktrace", e.stackTrace()),
	)
	return slog.GroupValue(attrs...)
}
