package errs

import "log/slog"

// Key names an error attribute whose value has type T. It creates the
// attribute with Attr and reads it back from any error chain with From,
// without type assertions:
//
//	const UserID errs.Key[int] = "user_id"
//
//	err := errs.New("user not found", UserID.Attr(42))
//	id, ok := UserID.From(err) // 42, true
type Key[T any] string

const (
	// Code is the machine-readable error code, such as "USER_NOT_FOUND".
	Code Key[string] = "code"

	// Public is a message that is safe to show to end users, unlike the
	// error message itself.
	Public Key[string] = "public"
)

// typed keeps the original Go value of an attribute created by [Key.Attr], so
// that [Key.From] returns it unchanged. slog itself would turn an int into an
// int64, for example. Handlers resolve it to the plain value when logging.
type typed[T any] struct{ v T }

func (t typed[T]) LogValue() slog.Value { return slog.AnyValue(t.v) }

// Attr returns the attribute k = v.
func (k Key[T]) Attr(v T) slog.Attr {
	return slog.Any(string(k), typed[T]{v})
}

// From returns the value of the innermost attribute named k in the chain of
// err. For an attribute that was not created by [Key.Attr], such as one made
// with [slog.Int], the value must have the type that [slog.Value.Any] returns,
// which is int64 for [slog.Int].
func (k Key[T]) From(err error) (T, bool) {
	var zero T
	e, ok := From(err)
	if !ok {
		return zero, false
	}
	a, ok := e.lookup(string(k))
	if !ok {
		return zero, false
	}
	if t, ok := a.Value.Any().(typed[T]); ok {
		return t.v, true
	}
	v, ok := a.Value.Resolve().Any().(T)
	return v, ok
}
