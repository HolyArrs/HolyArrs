package sources

import (
	"context"
	"errors"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A Release found by a season search matched no Title when its name is the
// show's plus only a site's lead word, a release tag or its group. "Pick
// anyway" still sends it to the episodes its numbers name.
func TestChoose_LooseNameFromSeasonSearch(t *testing.T) {
	all := []string{"S01E01", "S01E02", "S01E03"}
	for _, c := range []struct {
		name string
		want []string
	}{
		{"Watch Copper Valley S01 1080p WEB-DL H.264-QUILL", all},
		{"Free Download Copper Valley S01 1080p WEB-DL H.264-QUILL", all},
		{"Watch.Copper.Valley.S01E02.720p.HDTV.x264-QUILL", []string{"S01E02"}},
		{"Copper Valley Complete S01 1080p WEB-DL H.264-QUILL", all},
	} {
		f, sr, rel := seasonSearchRel(t, c.name, nil)
		if len(rel.MatchedSlotIDs) != 0 {
			t.Fatalf("%s matched %v; the case needs a name that matches none", c.name, rel.MatchedSlotIDs)
		}
		ctx := context.Background()
		var nf *NeedsForceError
		if _, err := sr.Choose(ctx, rel.ID, 0, false, domain.System); !errors.As(err, &nf) || !strings.Contains(strings.Join(nf.Reasons, " "), "loosely") || !strings.Contains(strings.Join(nf.Reasons, " "), c.name) {
			t.Fatalf("%s without force: %v, want needs force naming the release and saying the name only loosely matched", c.name, err)
		}
		ch, err := sr.Choose(ctx, rel.ID, 0, true, domain.System)
		if err != nil || !ch.Forced {
			t.Fatalf("%s forced: %+v %v", c.name, ch, err)
		}
		for _, l := range all {
			chosen := f.state(t, l) == mediafiles.SlotChosen
			if want := slicesHas(c.want, l); chosen != want {
				t.Errorf("%s: %s chosen = %v, want %v", c.name, l, chosen, want)
			}
		}
		if st := f.state(t, "S02E01"); st == mediafiles.SlotChosen {
			t.Errorf("%s: S02E01 was chosen from a season 1 search", c.name)
		}
	}
}

func slicesHas(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// A name with another word beside the show's (a longer title, a country, a
// spin-off, a soundtrack), another season's numbers, or no season or episode
// at all is refused and not sent to the season searched.
func TestChoose_LooseNameRefusesOtherTitles(t *testing.T) {
	for _, name := range []string{
		"Copper Valley Chronicles S01 1080p",
		"Copper Valley Chronicles S01E02",
		"Copper.Valley.2.S01",
		"Copper Valley UK S01E01",
		"Copper Valley Soundtrack FLAC",
		"Copper Valley Documentary Making Of 1080p",
		"Copper Valley - Behind the scenes S01",
		"Copper Valley 1080p WEB-DL",
		"Watch Copper Valley QUILL [WEB DL 1080p]",
		"Watch Copper Valleyview S01 1080p WEB-DL H.264-QUILL",
		"Watch Copper Big Valley S01 1080p WEB-DL H.264-QUILL",
		"Watch Copper Valley S02 1080p WEB-DL H.264-QUILL",
	} {
		_, sr, rel := seasonSearchRel(t, name, nil)
		if len(rel.MatchedSlotIDs) != 0 {
			t.Fatalf("%s matched %v, want none", name, rel.MatchedSlotIDs)
		}
		if _, err := sr.Choose(context.Background(), rel.ID, 0, true, domain.System); !errors.Is(err, ErrNoSlot) {
			t.Errorf("%s was forced from the season search: %v", name, err)
		}
	}
}

func TestLooseShowName(t *testing.T) {
	ref := func(n string, a ...string) mediafiles.TitleRef { return mediafiles.TitleRef{Name: n, Aliases: a} }
	cv := ref("Copper Valley")
	for _, c := range []struct {
		title, group string
		show         mediafiles.TitleRef
		others       []mediafiles.TitleRef
		want         bool
	}{
		{"Watch Copper Valley", "", cv, nil, true},
		{"Copper Valley Uncut", "", cv, nil, true},
		{"Copper Valley QUILL", "QUILL", cv, nil, true},
		{"Copper Valley QUILL", "", cv, nil, false},
		{"Copper Valley", "", cv, nil, false},
		{"Copper Valley Chronicles", "", cv, nil, false},
		{"Copper Valley UK", "", cv, nil, false},
		{"Watch Copper Valley Me", "", cv, nil, false},
		// a name of one word is never loosened
		{"Watch You", "", ref("You"), nil, false},
		{"You Me Her", "", ref("You"), nil, false},
		{"Lost in Space", "", ref("Lost"), nil, false},
		{"The Office UK", "", ref("The Office"), nil, false},
		// an alias counts
		{"Watch Valley of Copper", "", ref("Kupfer", "Valley of Copper"), nil, true},
		// the extra words, or the whole title, are another library title's
		{"Watch Copper Valley", "", cv, []mediafiles.TitleRef{ref("Watch")}, false},
		{"Copper Valley Complete", "", cv, []mediafiles.TitleRef{ref("Complete", "Other")}, false},
		{"Watch Copper Valley", "", cv, []mediafiles.TitleRef{ref("Other", "Watch Copper Valley")}, false},
	} {
		if got := looseShowName(c.title, c.group, c.show, c.others); got != c.want {
			t.Errorf("looseShowName(%q, group %q, %s, %d others) = %v, want %v", c.title, c.group, c.show.Name, len(c.others), got, c.want)
		}
	}
}
