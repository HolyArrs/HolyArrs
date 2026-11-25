package webserver

import (
	"context"
	"testing"
)

func TestOutsideName(t *testing.T) {
	for in, want := range map[string]string{
		"maria⁣12": "maria (deleted)",
		"maria":    "maria",
		"":         "",
		"a⁣b":      "a⁣b",
	} {
		if got := OutsideName(in); got != want {
			t.Errorf("OutsideName(%q) = %q, want %q", in, got, want)
		}
	}
	app := ContextWith(context.Background(), Caller{UserID: 1})
	key := ContextWith(context.Background(), Caller{UserID: 1, KeyID: 3})
	if got := NameFor(app, "maria⁣12"); got != "maria⁣12" {
		t.Errorf("the app lost the stored name: %q", got)
	}
	if got := NameFor(key, "maria⁣12"); got != "maria (deleted)" {
		t.Errorf("a key holder got %q", got)
	}
}
