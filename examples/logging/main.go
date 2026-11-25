// Command logging turns a panic into an error with errs and shows how the
// error's attributes appear in slog output.
package main

import (
	"log/slog"
	"os"

	"github.com/luminyra/errs"
)

func main() {
	// Drop the time and the stack trace to keep the example output short and
	// stable; real programs usually keep both.
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey || a.Key == "stacktrace" {
				return slog.Attr{}
			}
			return a
		},
	}))

	// region: recover
	// Recover calls the function and turns a panic into an error with
	// attributes and the stack of the panic; the program keeps running.
	err := errs.Recover(func() {
		var counts map[string]int
		counts["alice"]++ // writing to a nil map panics
	}, errs.Code.Attr("JOB_PANIC"), slog.String("job", "sync-users"))
	// endregion

	// region: log
	// *errs.Error implements slog.LogValuer, so logging it expands it into a
	// group.
	logger.Error("job failed", "err", err)
	// endregion
}
