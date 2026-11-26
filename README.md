<img align="right" src="https://images.luminyra.dev/luminyra/github/errs/logo.webp" alt="A cracked terracotta seal wrapped in parchment with a colored context tab." width="96" height="96">

# errs

[![Go Reference](https://pkg.go.dev/badge/github.com/luminyra/errs.svg)](https://pkg.go.dev/github.com/luminyra/errs)
[![CI](https://github.com/luminyra/errs/actions/workflows/ci.yml/badge.svg)](https://github.com/luminyra/errs/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/luminyra/errs)](https://goreportcard.com/report/github.com/luminyra/errs)
[![Go 1.27+](https://img.shields.io/badge/Go-1.27%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/doc/devel/release)

errs gives Go errors the context you need when something goes wrong in production: an error code for your API, key-value details such as the user or order involved, a message that is safe to show to users, and a stack trace, all of which appear automatically when you log the error with `log/slog`.

The errors stay ordinary Go errors: `errors.Is`, `errors.As` and `%w` keep working, so you can adopt errs one function at a time.

- **slog attributes.** Details are plain `slog.Attr` values, so anything slog can log can be attached, and logging an error expands it into a structured group.
- **Typed keys.** `errs.Key[T]` attaches a value and reads it back with its type, with no type assertions and no stringly typed maps.
- **Codes and public messages.** `errs.Code` and `errs.Public` separate what your API returns from what your logs record.
- **Stack traces and panics.** Every error records where it was created, and `errs.Recover` turns a panic into an error.
- **Cheap to inspect and log.** Reading an attribute back does not allocate, and a stack trace is formatted once per error, however often it is logged.

<p align="center">
  <img src="https://images.luminyra.dev/luminyra/github/errs/cover.webp" alt="A cracked wax seal wrapped in parchment and tagged for context." width="640">
</p>

## Contents

- [When to use it](#when-to-use-it) · [Install](#install) · [Getting started](#getting-started)
- [Common tasks](#common-tasks) · [How it works](#how-it-works) · [Performance](#performance) · [API at a glance](#api-at-a-glance)

## When to use it

errs is meant for applications and services: HTTP and gRPC handlers, workers and jobs, where an error eventually reaches a log line or an API response and you want it to explain itself.

In a reusable library, prefer plain sentinel errors (`errors.New`) and `fmt.Errorf("...: %w", err)`, and let the application decide how much context to add. On paths that create errors in a tight loop, keep in mind that capturing a stack trace costs more than `errors.New`.

## Install

```sh
go get github.com/luminyra/errs
```

errs requires Go 1.27 or later and has no dependencies.

## Getting started

Copy the program into `main.go` and run it with `go run .`:

```go
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/luminyra/errs"
)

var ErrNotFound = errors.New("not found")

// UserID is a typed attribute key: it stores an int and reads back an int.
const UserID errs.Key[int] = "user_id"

func findUser(id int) error {
	// Pretend the database had no row for id.
	return errs.Wrap(ErrNotFound, "find user",
		errs.Code.Attr("USER_NOT_FOUND"),
		errs.Public.Attr("We could not find that user."),
		UserID.Attr(id),
	)
}

func main() {
	err := findUser(42)

	// The error still works with the standard errors package...
	fmt.Println(err, "| is ErrNotFound:", errors.Is(err, ErrNotFound))

	// ...and carries values you can read back with their type.
	code, _ := errs.Code.From(err)
	message, _ := errs.Public.From(err)
	id, _ := UserID.From(err)
	fmt.Println(code, "|", message, "| user", id)

	// Logging it with slog expands the attributes (and a stack trace,
	// dropped here to keep the output short).
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey || a.Key == "stacktrace" {
				return slog.Attr{}
			}
			return a
		},
	}))
	logger.Error("request failed", "err", err)
}
```

```text
find user: not found | is ErrNotFound: true
USER_NOT_FOUND | We could not find that user. | user 42
level=ERROR msg="request failed" err.message="find user: not found" err.code=USER_NOT_FOUND err.public="We could not find that user." err.user_id=42
```

<p align="center">
  <a href="https://images.luminyra.dev/luminyra/github/errs/demo.webp"><img src="https://images.luminyra.dev/luminyra/github/errs/demo.webp" alt="Wrap ErrNotFound with a code and a typed user ID, read the attributes or log them with slog, and still match the cause with errors.Is." width="760"></a>
</p>

## Common tasks

### Add context while an error travels up

Each layer wraps the error with a short message and the details it knows about. `Wrap` returns `nil` for a `nil` error, so it can wrap a call's result directly:

```go
func (s *Service) Profile(ctx context.Context, id int) (*Profile, error) {
	user, err := s.users.Find(ctx, id)
	if err != nil {
		return nil, errs.Wrap(err, "load profile", UserID.Attr(id))
	}
	return newProfile(user), nil
}
```

`errs.New(msg, attrs...)` creates a new error, and `errs.Errorf` works like `fmt.Errorf`, including `%w`.

### Attach any detail

Any `slog.Attr` works, including groups:

```go
err = errs.Wrap(err, "charge card",
	slog.String("order_id", orderID),
	slog.Group("amount", slog.Int("cents", cents), slog.String("currency", "EUR")),
)
```

Declare a typed key with `errs.Key[T]` for values you read back in code, such as IDs that decide a response.

### Turn an error into an API response

```go
code, ok := errs.Code.From(err)
if !ok {
	code = "INTERNAL"
}
message, ok := errs.Public.From(err)
if !ok {
	message = "Something went wrong."
}
writeJSON(w, statusFor(code), map[string]string{"code": code, "message": message})
```

`Public` keeps user-facing text separate from the error message, which may contain internal details.

### Inspect the whole chain

When errors from this package wrap each other, the innermost value of a key wins, because it is closest to where the failure happened:

```go
if e, ok := errs.From(err); ok {
	for _, attr := range e.Attrs() { // merged across the chain
		fmt.Println(attr)
	}
	fmt.Printf("%+v\n", e) // message, attributes and stack trace
}
```

`errs.From` is shorthand for `errors.AsType[*errs.Error](err)`.

### Recover from panics

```go
err := errs.Recover(func() { runJob(job) }, slog.String("job", job.Name))
if err != nil {
	slog.Error("job crashed", "err", err) // includes the stack of the panic
}
```

A panic value that is an `error` stays in the chain, so `errors.Is` still matches it.

## How it works

- An `*errs.Error` holds a message, an optional cause, attributes, its creation time and a stack trace captured when it was created. It is immutable and safe to share between goroutines.
- `Error()` returns `"message: cause"`. `Unwrap` returns the cause, so standard error inspection sees through it.
- `Key.Attr` keeps the original Go type of a value, so `Key[int]` reads back an `int`. For attributes created with slog functions, `Key.From` returns the type that `slog.Value.Any` reports, which is `int64` for `slog.Int`.
- `LogValue` resolves the error into a group with `message`, the merged attributes, `time` and `stacktrace`. Any slog handler, including JSON handlers, renders it.

## Performance

Measured with the benchmarks in this repository (`make bench`; Go 1.27.4, linux/arm64, 5 CPUs, median of 6 runs). Run them on your hardware for numbers that matter to you.

| Operation | Time | Allocations |
| --- | --- | --- |
| `errors.New` (for comparison) | 14 ns | 1 |
| `fmt.Errorf` with `%w` (for comparison) | 65 ns | 2 |
| `errs.New`, including the stack trace | 191 ns | 2 |
| `errs.Wrap` with two attributes | 227 ns | 4 |
| `Key.From` on a three-level chain | 19 ns | 0 |
| Logging an error with `slog.NewJSONHandler` | 0.8 µs | 4 |

Capturing the stack trace is most of the cost of creating an error, so create errs errors on failure paths, not in loops that fail as part of normal operation. The stack is captured into a fixed buffer and copied at its real depth, and the formatted trace is cached on the error.

## API at a glance

| Task | API |
| --- | --- |
| Create and wrap | `New`, `Errorf`, `Wrap`, `Recover` |
| Attach and read typed values | `Key[T]`, `Key.Attr`, `Key.From`, `Code`, `Public` |
| Inspect | `From`, `Error.Attrs`, `Error.Time`, `Error.Stack`, `%+v` |
| Log | `Error.LogValue` (used by slog automatically) |

The full reference with runnable examples is on [pkg.go.dev](https://pkg.go.dev/github.com/luminyra/errs). Complete programs live in [`examples/`](examples): [`basic`](examples/basic/main.go) and [`logging`](examples/logging/main.go).

## Versioning

errs follows [semantic versioning](https://semver.org). Until v1.0.0, a minor release may change the API; every change is listed in the [changelog](CHANGELOG.md). It supports the two most recent Go releases, starting with Go 1.27.

## Contributing

Questions, bug reports and pull requests are welcome. See the [contributing guide](https://github.com/luminyra/.github/blob/main/CONTRIBUTING.md), and report security issues as described in the [security policy](https://github.com/luminyra/.github/blob/main/SECURITY.md).

errs was inspired by [samber/oops](https://github.com/samber/oops) and [pkg/errors](https://github.com/pkg/errors). It is released under the [MIT License](LICENSE).
