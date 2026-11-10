package alerts

import "testing"

func TestSayFallsBackToEnglish(t *testing.T) {
	if got := say("fr", MomentImport); got != "Dans la bibliothèque" {
		t.Fatalf("fr: %q", got)
	}
	if got := say("", "tracks", "n", "4"); got != "4 tracks" {
		t.Fatalf("empty lang: %q", got)
	}
	if got := say("xx", MomentGrab); got != "Sent to download" {
		t.Fatalf("unknown lang: %q", got)
	}
	if got := say("pt-BR", "episode_range", "from", "S01E01", "to", "S01E03", "n", "3"); got != "S01E01 a S01E03 (3 episódios)" {
		t.Fatalf("pt-br: %q", got)
	}
	for l, m := range words {
		for k := range m {
			if _, ok := words["en"][k]; !ok {
				t.Errorf("%s has %q, English does not", l, k)
			}
		}
		if len(m) != len(words["en"]) {
			t.Errorf("%s has %d lines, English %d", l, len(m), len(words["en"]))
		}
	}
}
