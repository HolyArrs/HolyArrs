package main

import (
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

func TestJobTimingLogsOncePerMinute(t *testing.T) {
	now := time.Unix(1000, 0)
	var lines []string
	j := &jobTiming{now: func() time.Time { return now }, logf: func(_ string, a ...any) { lines = append(lines, a[0].(string)) }}
	tr := &imgstore.Trace{}
	tr.D[imgstore.PhaseBig] = 600 * time.Millisecond
	j.add("img", "ok", time.Second, tr)
	now = now.Add(30 * time.Second)
	j.add("big", "later", 3*time.Second, nil)
	if len(lines) != 0 {
		t.Fatalf("logged early: %v", lines)
	}
	now = now.Add(31 * time.Second)
	j.add("img", "ok", time.Second, nil)
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %v", lines)
	}
	l := lines[0]
	for _, want := range []string{"images jobs: 3 in 1m1s", "[big=1 img=2]", "[later=1 ok=2]", "total=1666", "max=3000", "big=200"} {
		if !strings.Contains(l, want) {
			t.Errorf("line %q lacks %q", l, want)
		}
	}
}
