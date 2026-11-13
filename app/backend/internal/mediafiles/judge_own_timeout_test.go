package mediafiles

import (
	"strings"
	"testing"
	"time"
)

// TestSpecTimeoutRunsOnce: a pattern that ran out of time is not run again,
// so an imported trait with a slow pattern costs its timeout once, not on
// every Release judged.
func TestSpecTimeoutRunsOnce(t *testing.T) {
	re, err := CompileSpecPattern(`^(a+)+$`)
	if err != nil {
		t.Fatal(err)
	}
	s := ownSpec{OwnSpec: OwnSpec{Name: "slow", Kind: "name", Value: `^(a+)+$`}, re: re}
	text := strings.Repeat("a", 40) + "!"
	if s.match("slow-trait-test", text) {
		t.Fatal("a timed-out pattern fits")
	}
	start := time.Now()
	for range 5 {
		if s.match("slow-trait-test", text) {
			t.Fatal("a timed-out pattern fits")
		}
	}
	if d := time.Since(start); d > specTimeout {
		t.Fatalf("the timed-out pattern ran again: 5 matches took %s", d)
	}
}
