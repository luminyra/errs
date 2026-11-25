package errs_test

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"

	"github.com/luminyra/errs"
)

const UserID errs.Key[int] = "user_id"

func Example() {
	err := errs.Wrap(fs.ErrNotExist, "find user", errs.Code.Attr("USER_NOT_FOUND"), UserID.Attr(42))
	err = errs.Wrap(err, "get profile", slog.String("route", "/users/{id}"))

	code, _ := errs.Code.From(err)
	id, _ := UserID.From(err)
	fmt.Println(err)
	fmt.Println(errors.Is(err, fs.ErrNotExist), code, id)
	// Output:
	// get profile: find user: file does not exist
	// true USER_NOT_FOUND 42
}

func ExampleError_Attrs() {
	inner := errs.New("query failed", slog.String("table", "users"), slog.Int("attempt", 1))
	outer := errs.Wrap(inner, "load users", slog.Int("attempt", 3))

	if e, ok := errs.From(outer); ok {
		for _, a := range e.Attrs() {
			fmt.Println(a)
		}
	}
	// Output:
	// attempt=1
	// table=users
}

func ExampleError_LogValue() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey || a.Key == "stacktrace" {
				return slog.Attr{} // keep the example output stable
			}
			return a
		},
	}))

	logger.Error("request failed", "err", errs.New("timeout", errs.Code.Attr("UPSTREAM_TIMEOUT")))
	// Output: level=ERROR msg="request failed" err.message=timeout err.code=UPSTREAM_TIMEOUT
}

func ExampleRecover() {
	err := errs.Recover(func() {
		var counts map[string]int
		counts["alice"]++
	}, errs.Code.Attr("JOB_PANIC"))

	code, _ := errs.Code.From(err)
	fmt.Println(err, code)
	// Output: panic: assignment to entry in nil map JOB_PANIC
}
