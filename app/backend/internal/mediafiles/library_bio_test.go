package mediafiles

import (
	"strings"
	"testing"
)

func TestShortBio(t *testing.T) {
	t.Parallel()
	if got := shortBio("  Short   bio. "); got != "Short bio." {
		t.Fatalf("short: %q", got)
	}
	long := strings.Repeat("Word word word word word word word word word. ", 12)
	got := shortBio(long)
	if len(got) > libraryBioMax || !strings.HasSuffix(got, ".") {
		t.Fatalf("sentences: %d %q", len(got), got)
	}
	one := strings.Repeat("wörd ", 100)
	got = shortBio(one)
	if len(got) > libraryBioMax+len("…") || !strings.HasSuffix(got, "…") || strings.Contains(got, "\uFFFD") {
		t.Fatalf("one sentence: %q", got)
	}
}
