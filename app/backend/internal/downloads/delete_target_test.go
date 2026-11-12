package downloads

import (
	"strings"
	"testing"
)

// The shared rules read a Downloader's paths with either separator, the way
// the Downloader builds them.
func TestSafeDeleteTargetBackslashPaths(t *testing.T) {
	ask := func(content, name string, others ...string) string {
		_, why := safeDeleteTarget(deleteAsk{client: "aria2", content: content, name: name, others: others}, nil)
		return why
	}
	if why := ask(`C:\dl\tv\a.mkv`, "a.mkv"); strings.HasPrefix(why, `C:\dl\tv\a.mkv is not named like the torrent`) && !strings.Contains(why, "here") {
		t.Errorf("name check: %s", why)
	}
	if why := ask(`C:\dl\tv\b`, "a.mkv"); !strings.Contains(why, "is not named like the torrent") {
		t.Errorf("wrong name passed: %s", why)
	}
	for _, other := range []string{`C:\dl\tv\Show\ep.mkv`, `C:\dl\tv`, `C:\dl\tv\Show`} {
		if why := ask(`C:\dl\tv\Show`, "Show", other); !strings.Contains(why, "another torrent") {
			t.Errorf("overlap with %s: %s", other, why)
		}
	}
	if why := ask(`C:\dl\tv\Show`, "Show", `C:\dl\tv\Showtime`); strings.Contains(why, "another torrent") {
		t.Errorf("a sibling counted as overlap: %s", why)
	}
	// A slash path keeps its rules: a backslash is part of a name there.
	if why := ask(`/dl/tv/a\b`, `b`); !strings.Contains(why, "is not named like the torrent") {
		t.Errorf("slash path split on a backslash: %s", why)
	}
}
