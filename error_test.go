package errs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"strings"
	"testing"
)

func TestWrapNil(t *testing.T) {
	if Wrap(nil, "x") != nil || Wrap(nil, "", Code.Attr("c")) != nil {
		t.Fatal("wrapping nil must return nil")
	}
}

func TestMessagesAndChain(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{name: "new", err: New("boom"), expected: "boom"},
		{name: "errorf keeps %w", err: Errorf("open: %w", fs.ErrNotExist), expected: "open: file does not exist"},
		{name: "wrap with message", err: Wrap(Errorf("open: %w", fs.ErrNotExist), "load cfg"), expected: "load cfg: open: file does not exist"},
		{name: "wrap without message", err: Wrap(fs.ErrNotExist, ""), expected: "file does not exist"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("got %q, expected %q", got, tt.expected)
			}
		})
	}
	if err := Wrap(Errorf("open: %w", fs.ErrNotExist), "load"); !errors.Is(err, fs.ErrNotExist) {
		t.Error("the chain must reach the wrapped sentinel")
	}
	if e, _ := From(New("x")); e.Unwrap() != nil {
		t.Error("New must not have a cause")
	}
}

func TestAttrsMergeAcrossChain(t *testing.T) {
	inner := New("x", slog.String("a", "inner"), slog.String("b", "inner"), userID.Attr(1))
	outer := Wrap(inner, "y", slog.String("c", "outer"), slog.String("a", "outer"))

	e, ok := From(outer)
	if !ok {
		t.Fatal("From failed")
	}
	var got []string
	for _, a := range e.Attrs() {
		got = append(got, a.String())
	}
	expected := "c=outer a=inner b=inner user_id=1"
	if strings.Join(got, " ") != expected {
		t.Errorf("Attrs = %v, expected %s", got, expected)
	}

	innerErr, _ := From(inner)
	if !e.Time().Equal(innerErr.Time()) {
		t.Error("Time must come from the innermost error")
	}
}

func TestAttrsAreCopied(t *testing.T) {
	attrs := []slog.Attr{slog.String("k", "v")}
	err := New("x", attrs...)
	attrs[0] = slog.String("k", "changed")
	if v, _ := Key[string]("k").From(err); v != "v" {
		t.Errorf("New must copy its attributes, got %q", v)
	}
}

func TestStackStartsAtCaller(t *testing.T) {
	for name, err := range map[string]error{
		"New":    New("x"),
		"Errorf": Errorf("x"),
		"Wrap":   Wrap(errors.New("x"), "y"),
	} {
		e, _ := From(err)
		for f := range e.Stack() {
			if !strings.HasSuffix(f.Function, "TestStackStartsAtCaller") {
				t.Errorf("%s: first frame = %q", name, f.Function)
			}
			break
		}
	}
}

func TestFormat(t *testing.T) {
	err := New("boom", Code.Attr("c1"), slog.String("k", "v"))
	if got := fmt.Sprintf("%v|%s|%q|%d", err, err, err, err); got != `boom|boom|"boom"|%!d(*errs.Error=boom)` {
		t.Errorf("format = %q", got)
	}
	verbose := fmt.Sprintf("%+v", err)
	for _, want := range []string{"boom", "\ncode: c1", "\nk: v", "--- stack ---", "TestFormat"} {
		if !strings.Contains(verbose, want) {
			t.Errorf("%%+v missing %q in:\n%s", want, verbose)
		}
	}
}

func TestLogValue(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	logger.Error("failed", "err", Wrap(New("boom", Code.Attr("c1")), "auth", userID.Attr(7)))
	out := buf.String()
	for _, want := range []string{`err.message="auth: boom"`, "err.user_id=7", "err.code=c1", "err.time=", "err.stacktrace="} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q in %s", want, out)
		}
	}
}

func TestRecover(t *testing.T) {
	if err := Recover(func() {}); err != nil {
		t.Fatalf("Recover without panic = %v", err)
	}
	err := Recover(func() { panic(fs.ErrClosed) }, Code.Attr("PANIC"))
	if !errors.Is(err, fs.ErrClosed) || err.Error() != "panic: file already closed" {
		t.Fatalf("Recover(error) = %v", err)
	}
	if code, _ := Code.From(err); code != "PANIC" {
		t.Errorf("Recover attributes missing: %q", code)
	}
	if !strings.Contains(fmt.Sprintf("%+v", err), "TestRecover") {
		t.Errorf("Recover stack missing the panicking frame: %+v", err)
	}
	if err := Recover(func() { panic("oops") }); err == nil || err.Error() != "panic: oops" {
		t.Fatalf("Recover(string) = %v", err)
	}
}

var sinkError error

func BenchmarkCreate(b *testing.B) {
	base := errors.New("not found")
	b.Run("errors.New", func(b *testing.B) {
		for b.Loop() {
			sinkError = errors.New("not found")
		}
	})
	b.Run("fmt.Errorf", func(b *testing.B) {
		for b.Loop() {
			sinkError = fmt.Errorf("find user %d: %w", 42, base)
		}
	})
	b.Run("New", func(b *testing.B) {
		for b.Loop() {
			sinkError = New("not found")
		}
	})
	b.Run("Wrap+2attrs", func(b *testing.B) {
		for b.Loop() {
			sinkError = Wrap(base, "find user", Code.Attr("USER_NOT_FOUND"), userID.Attr(42))
		}
	})
}

func BenchmarkInspect(b *testing.B) {
	err := Wrap(Wrap(New("db", Code.Attr("X"), userID.Attr(1)), "repo"), "handler", slog.String("route", "/u"))
	b.Run("Key.From", func(b *testing.B) {
		for b.Loop() {
			_, _ = userID.From(err)
		}
	})
	b.Run("Attrs", func(b *testing.B) {
		e, _ := From(err)
		for b.Loop() {
			_ = e.Attrs()
		}
	})
	b.Run("Error", func(b *testing.B) {
		for b.Loop() {
			_ = err.Error()
		}
	})
}

func BenchmarkLog(b *testing.B) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	err := Wrap(New("db", Code.Attr("X")), "handler", userID.Attr(42))
	for b.Loop() {
		logger.Error("request failed", "err", err)
	}
}

func TestAttrsWithoutAttributes(t *testing.T) {
	e, _ := From(Wrap(New("x"), "y"))
	if attrs := e.Attrs(); attrs != nil {
		t.Fatalf("Attrs = %v, want nil", attrs)
	}
}
