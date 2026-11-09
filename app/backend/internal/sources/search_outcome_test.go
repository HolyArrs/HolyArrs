package sources

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNoAnswer_FailedSources(t *testing.T) {
	sk := []skippedSource{{ID: 3, Name: "Alpha", Why: KindBlocked}}
	reason, data, ok, failed := noAnswer([]string{"Alpha: refused"}, 0, sk)
	if !ok || !failed || reason != "no Source answered: Alpha: refused" {
		t.Fatalf("got %q %v", reason, ok)
	}
	if got := data["sources"].([]skippedSource); len(got) != 1 || got[0].ID != 3 {
		t.Fatalf("sources = %v", data["sources"])
	}
}

func TestNoAnswer_AllPaused(t *testing.T) {
	reason, data, ok, failed := noAnswer(nil, 0, []skippedSource{{ID: 1, Name: "A", Why: KindRateLimited}, {ID: 2, Name: "B", Why: "paused"}})
	if !ok || failed || !strings.Contains(reason, "A: slowed down") || !strings.Contains(reason, "B: paused") {
		t.Fatalf("got %q %v", reason, ok)
	}
	if len(data["sources"].([]skippedSource)) != 2 {
		t.Fatal(data)
	}
}

func TestNoAnswer_SomeAnsweredOrNoneTried(t *testing.T) {
	if _, _, ok, _ := noAnswer([]string{"A: x"}, 1, []skippedSource{{ID: 1}}); ok {
		t.Fatal("a Source answered")
	}
	if _, _, ok, _ := noAnswer(nil, 0, nil); ok {
		t.Fatal("no Source was tried")
	}
}

func TestFailWhy_KindNames(t *testing.T) {
	if got := failWhy(fmt.Errorf("wrap: %w", &Error{Kind: KindBlocked})); got != KindBlocked {
		t.Fatal(got)
	}
	if got := failWhy(errors.New("boom")); got != KindDown {
		t.Fatal(got)
	}
}
