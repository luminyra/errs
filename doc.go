// Package errs builds structured errors for Go 1.27 on top of [log/slog]. An
// [Error] carries a message, an optional cause, slog attributes, its creation
// time and a stack trace.
//
// # Attributes
//
// Attributes are plain [log/slog.Attr] values, so anything slog can log can
// be attached with [New] or [Wrap]. A typed [Key] creates an attribute and
// reads it back from any error chain without type assertions, and [Code] and
// [Public] are predefined keys for a machine-readable error code and a
// message that is safe to show to end users:
//
//	const UserID errs.Key[int] = "user_id"
//
//	err := errs.New("user not found", errs.Code.Attr("USER_NOT_FOUND"), UserID.Attr(id))
//	err = errs.Wrap(err, "get profile", slog.String("route", "/users/{id}"))
//
//	code, _ := errs.Code.From(err) // "USER_NOT_FOUND"
//	id, _ := UserID.From(err)      // an int, no type assertion
//
// When errors from this package wrap each other, [Error.Attrs] merges the
// attributes of the whole chain and [Key.From] returns the innermost value,
// because it is closest to where the error happened.
//
// # Inspection and logging
//
// Errors support [errors.Is] and [errors.As] through Unwrap; [From] returns
// the outermost [Error] of a chain. %+v prints the attributes and the stack
// trace, [Error.Stack] iterates over the stack frames, and Error implements
// [log/slog.LogValuer], so logging an error expands it into a group.
// [Recover] turns a panic into an error that keeps the panicking stack.
//
// The package is inspired by samber/oops and pkg/errors.
package errs
