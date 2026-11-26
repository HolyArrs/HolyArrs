package importer

import "testing"

// Files held for Review before import (#611): play length, video track, first audio.
func TestHeldWhy(t *testing.T) {
	f := func(kv ...any) map[string]any {
		m := map[string]any{"container": "matroska", "video_tracks": 1}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i].(string)] = kv[i+1]
		}
		return m
	}
	cases := []struct {
		name     string
		facts    map[string]any
		minutes  int
		original string
		wanted   []string
		held     bool
	}{
		{"right length", f("runtime_seconds", 96*60), 96, "en", nil, false},
		{"half length fake (tydude repro)", f("runtime_seconds", 2880), 96, "en", nil, true},
		{"99 min as a 110 min film: inside 15%", f("runtime_seconds", 99*60), 110, "en", nil, false},
		{"99 min Russian film as The Book Thief (131 min)", f("runtime_seconds", 99*60), 131, "en", nil, true},
		{"short film, 12 min floor", f("runtime_seconds", 30*60), 40, "en", nil, false},
		{"Talk to Me matched to a 21 min short", f("runtime_seconds", 95*60), 21, "en", nil, true},
		{"Title under 20 min: skipped", f("runtime_seconds", 95*60), 15, "en", nil, false},
		{"long extended cut (+45%)", f("runtime_seconds", 145*60), 100, "en", nil, false},
		{"too long", f("runtime_seconds", 180*60), 100, "en", nil, true},
		{"extended cut 20 min longer", f("runtime_seconds", 120*60), 100, "en", nil, false},
		{"runtime unknown", f(), 96, "en", nil, false},
		{"title runtime unknown", f("runtime_seconds", 2880), 0, "en", nil, false},
		{"no video track", f("video_tracks", 0, "runtime_seconds", 96*60), 96, "en", nil, true},
		{"unreadable container: no verdict", map[string]any{"video_tracks": 0}, 96, "en", nil, false},
		{"Korean audio first on an English film", f("runtime_seconds", 125*60, "first_audio_language", "ko"), 124, "en", nil, true},
		{"original language first", f("runtime_seconds", 125*60, "first_audio_language", "ko"), 124, "ko", nil, false},
		{"wanted dub first", f("runtime_seconds", 125*60, "first_audio_language", "de"), 124, "en", []string{"de"}, false},
		{"untagged first audio", f("runtime_seconds", 125*60), 124, "en", nil, false},
		{"title language unknown", f("first_audio_language", "ko"), 124, "", nil, false},
	}
	for _, c := range cases {
		if got := heldWhy(c.facts, c.minutes, c.original, c.wanted); (got != "") != c.held {
			t.Errorf("%s: heldWhy = %q, held want %v", c.name, got, c.held)
		}
	}
}
