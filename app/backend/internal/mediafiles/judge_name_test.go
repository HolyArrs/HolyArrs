package mediafiles

import "testing"

func TestJudgeNameAddsInspectedResolution(t *testing.T) {
	info := map[string]any{"resolution": "1080p"}
	if got := JudgeName("The Rookie - S01E02 - Title", info); got != "The Rookie - S01E02 - Title 1080p" {
		t.Fatalf("no resolution in the name: got %q", got)
	}
	if got := JudgeName("The Matrix (1999) Bluray-720p", info); got != "The Matrix (1999) Bluray-720p" {
		t.Fatalf("the name's own resolution wins: got %q", got)
	}
	if got := JudgeName("Show - S01E02 - Title", nil); got != "Show - S01E02 - Title" {
		t.Fatalf("no inspection: got %q", got)
	}
	if got := JudgeName("Show - S01E02 - Title", map[string]any{"resolution": "odd"}); got != "Show - S01E02 - Title" {
		t.Fatalf("odd inspection value: got %q", got)
	}
}
