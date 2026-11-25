package errs

import (
	"errors"
	"log/slog"
	"testing"
)

const userID Key[int] = "user_id"

func TestKeys(t *testing.T) {
	inner := New("missing row", Code.Attr("NOT_FOUND"), userID.Attr(1), slog.String("table", "users"))
	outer := Wrap(inner, "get user", userID.Attr(2), Public.Attr("user not found"), slog.Int("attempt", 3))

	if code, ok := Code.From(outer); !ok || code != "NOT_FOUND" {
		t.Errorf("Code = %q, %t", code, ok)
	}
	if msg, ok := Public.From(outer); !ok || msg != "user not found" {
		t.Errorf("Public = %q, %t", msg, ok)
	}
	if id, ok := userID.From(outer); !ok || id != 1 {
		t.Errorf("user_id must resolve to the innermost value: %d, %t", id, ok)
	}
	if table, ok := Key[string]("table").From(outer); !ok || table != "users" {
		t.Errorf("plain slog.String attribute = %q, %t", table, ok)
	}
	if n, ok := Key[int64]("attempt").From(outer); !ok || n != 3 {
		t.Errorf("plain slog.Int attribute as int64 = %d, %t", n, ok)
	}
	if _, ok := Key[int]("attempt").From(outer); ok {
		t.Error("slog.Int must not match Key[int]")
	}
	if _, ok := Key[string]("missing").From(outer); ok {
		t.Error("missing key found")
	}
	if _, ok := Code.From(errors.New("plain")); ok {
		t.Error("Key.From must report false for errors from other packages")
	}
}
