package subtitles

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites/fakesite"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
)

const adSub = "1\n00:00:01,000 --> 00:00:02,000\nSubtitles by Someone\n\n2\n00:00:03,000 --> 00:00:04,000\n[door] Hello there\n\n"

// heldWith counts files in the holding folder holding text.
func heldWith(t *testing.T, f *fix, text string) int {
	n := 0
	_ = filepath.WalkDir(filepath.Join(f.lib, ".holyarrs-replaced"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if c, _ := os.ReadFile(p); strings.Contains(string(c), text) {
				n++
			}
		}
		return nil
	})
	return n
}

// A fetched subtitle loses its ad lines on save with its original held, and
// Clean again tidies the one on disk with steps turned on later.
func TestService_CleanOnSaveAndAgain(t *testing.T) {
	f := newFix(t)
	site := &fakesite.Site{Subs: []fakesite.Sub{
		{ID: "ok", Kind: "movie", Language: "en", Title: "The Lantern", Year: 2010, Release: "The.Lantern.2010.1080p.BluRay.x264-GRP", Format: "srt", Text: adSub},
	}}
	s, _ := svc(t, f, site)
	ctx := context.Background()
	wishAll(t, f, Want{Language: "en"})
	res, _ := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System)
	if res.Outcomes[0].Result != "saved" {
		t.Fatalf("%+v", res.Outcomes)
	}
	rel := res.Outcomes[0].Path
	b, _ := os.ReadFile(f.abs(rel))
	if strings.Contains(string(b), "Subtitles by") || !strings.HasPrefix(string(b), "1\n00:00:03,000") || !strings.Contains(string(b), "[door]") {
		t.Fatalf("not cleaned on save: %q", b)
	}
	es := eventsOf(t, f.st, EventCleaned)
	if len(es) != 1 || !strings.Contains(es[0].Summary, "1 ad lines removed") || es[0].Data["original"] == nil {
		t.Fatalf("%+v", es)
	}
	if es[0].Data["path"] != rel { // History shows the file
		t.Fatalf("path %v, want %q", es[0].Data["path"], rel)
	}
	if heldWith(t, f, "Subtitles by Someone") != 1 {
		t.Fatal("original not held")
	}
	rows, _ := database.Select[Subtitle](ctx, f.st.Read(), "subtitles", "place = 'beside' AND state = 'present'")
	if ok, err := s.CleanAgain(ctx, rows[0].ID, domain.System); ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
	// English gets its own row; other languages keep the every-language row.
	set := ReadSettings(ctx, f.st.Read())
	set.Clean = map[string]subtext.Tidy{EveryLanguage: {Ads: true}, "EN": {Ads: true, HI: true}}
	if err := SaveSettings(ctx, f.st, set, domain.System); err != nil {
		t.Fatal(err)
	}
	got := ReadSettings(ctx, f.st.Read())
	if c := got.CleanFor("en-US"); !c.HI || !c.Ads || c.Tags {
		t.Fatalf("%+v", c)
	}
	if c := got.CleanFor("fr"); c.HI || !c.Ads {
		t.Fatalf("%+v", c)
	}
	n, err := s.CleanTitle(ctx, f.movie, domain.System)
	if err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	b, _ = os.ReadFile(f.abs(rel))
	if strings.Contains(string(b), "[door]") || !strings.Contains(string(b), "Hello there") {
		t.Fatalf("not cleaned again: %q", b)
	}
	if heldWith(t, f, "[door]") != 2 { // the fetched original and the text before Clean again
		t.Fatalf("held %d", heldWith(t, f, "[door]"))
	}
	if err := s.handleClean(ctx, &jobs.Run{}); err != nil {
		t.Fatal(err)
	}
	es = eventsOf(t, f.st, EventCleaned)
	if !strings.Contains(es[0].Summary+es[len(es)-1].Summary, "0 of 1 changed") {
		t.Fatalf("%+v", es)
	}
}

// A hearing-impaired or forced subtitle keeps its sound notes.
func TestStepsFor(t *testing.T) {
	set := DefaultSettings()
	set.Clean = map[string]subtext.Tidy{EveryLanguage: {Ads: true, HI: true, Caps: true}}
	if s := stepsFor(set, &Subtitle{Language: "en"}); !s.HI || !s.Caps {
		t.Fatalf("%+v", s)
	}
	if s := stepsFor(set, &Subtitle{Language: "en", HearingImpaired: true}); s.HI || !s.Ads {
		t.Fatalf("%+v", s)
	}
	if s := stepsFor(set, &Subtitle{Language: "en", Forced: true}); s.HI || s.Caps || !s.Ads {
		t.Fatalf("%+v", s)
	}
	bad := DefaultSettings()
	bad.Clean = map[string]subtext.Tidy{"en us": {}}
	if bad.Validate() == nil {
		t.Fatal("bad language key passed")
	}
}
