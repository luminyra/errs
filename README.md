# errs

[![Go Reference](https://pkg.go.dev/badge/github.com/luminyra/errs.svg)](https://pkg.go.dev/github.com/luminyra/errs)
[![CI](https://github.com/luminyra/errs/actions/workflows/ci.yml/badge.svg)](https://github.com/luminyra/errs/actions/workflows/ci.yml)

Structured errors for Go 1.27 built on `log/slog`: attach `slog.Attr` values, read them back with typed keys, and log the whole chain with a stack trace.

```go
const UserID errs.Key[int] = "user_id"

err := errs.Wrap(sql.ErrNoRows, "find user",
	errs.Code.Attr("USER_NOT_FOUND"),
	UserID.Attr(42),
)

id, _ := UserID.From(err)                // 42, an int
code, _ := errs.Code.From(err)           // "USER_NOT_FOUND"
slog.Error("request failed", "err", err) // expands into a group with a stack trace
```

## Install

```sh
go get github.com/luminyra/errs
```

Requires Go 1.27 or later. The module has no dependencies.

## Highlights

- **slog attributes.** Errors carry plain `slog.Attr` values, so anything slog can log can be attached, and logging an error expands it into a group.
- **Typed keys.** `errs.Key[T]` writes an attribute and reads it back from any error chain as a `T`. `errs.Code` and `errs.Public` are predefined keys for an error code and a user-facing message.
- **Chain aware.** Attributes are merged across wrapped errors, and the innermost value wins because it is closest to the failure. `errors.Is` and `errors.As` work through `Unwrap`.
- **Stack traces.** Each error records where it was created. `%+v` prints the attributes and the stack, and `Error.Stack` iterates over the frames.
- **Panic recovery.** `errs.Recover` turns a panic into an error that keeps the panicking stack.

## Usage

```go
func handleGetUser(w http.ResponseWriter, r *http.Request) {
	err := errs.Wrap(loadProfile(r), "get profile", slog.String("route", r.URL.Path))
	if err == nil {
		return
	}
	msg, ok := errs.Public.From(err)
	if !ok {
		msg = "internal error"
	}
	http.Error(w, msg, http.StatusInternalServerError)
	slog.ErrorContext(r.Context(), "request failed", "err", err)
}
```

## Examples

Runnable programs live in [`examples/`](examples):

| Example | Shows |
| --- | --- |
| [`basic`](examples/basic/main.go) | attributes across layers, typed keys, merged attributes |
| [`logging`](examples/logging/main.go) | panic recovery and slog output |

```sh
go run ./examples/basic
```

## Design

errs is inspired by [samber/oops](https://github.com/samber/oops) and [pkg/errors](https://github.com/pkg/errors). Instead of a builder with a fixed set of attributes, it reuses slog's attribute model and adds typed keys, so errors and logs share one vocabulary.

## License

[MIT](LICENSE)
