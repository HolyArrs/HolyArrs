package alerts

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

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

// Every language keeps English's {slots}, so a translated alert never drops a
// number or a name; a language not listed (or a line it lacks) sends English.
func TestWordsKeepEnglishSlots(t *testing.T) {
	slots := func(s string) string {
		out := slotRe.FindAllString(s, -1)
		sort.Strings(out)
		return strings.Join(out, " ")
	}
	for l, m := range words {
		for k, line := range m {
			if line == "" {
				t.Errorf("%s %s is empty", l, k)
			}
			if got, want := slots(line), slots(words["en"][k]); got != want {
				t.Errorf("%s %s: slots %q, English %q", l, k, got, want)
			}
		}
	}
	// Languages the app does not plan to offer, so this still holds once sv, he, pt-PT and zh-TW get alert words.
	for _, l := range []string{"ur", "sw", "es-419", "xx"} {
		if got := say(l, "in_library", "name", "Dune"); got != "Dune is in the Library" {
			t.Errorf("%s: %q, want English", l, got)
		}
	}
	for _, moment := range []string{MomentGrab, MomentImport, MomentRename, MomentAdd, MomentDelete, MomentDecision, MomentSubtitle} {
		if words["en"][moment] == "" {
			t.Errorf("no English line for %s", moment)
		}
	}
}

var slotRe = regexp.MustCompile(`\{\w+\}`)
