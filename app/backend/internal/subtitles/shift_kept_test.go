package subtitles

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites/fakesite"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
)

// A timing shift made by hand is a person's work: a later timed search for a
// better subtitle must not throw it away.
func TestService_HandShiftSurvivesBetterSearch(t *testing.T) {
	f := newFix(t)
	site := &fakesite.Site{Subs: []fakesite.Sub{
		{ID: "ok", Kind: "movie", Language: "en", Title: "The Lantern", Year: 2010, Release: "The.Lantern.2010.1080p.BluRay.x264-OTHER", Format: "srt", Text: fakesite.Cue("first", 1000)},
	}}
	s, _ := svc(t, f, site)
	ctx := context.Background()
	wishAll(t, f, Want{Language: "en"})
	res, _ := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System)
	if res.Outcomes[0].Result != "saved" {
		t.Fatalf("%+v", res.Outcomes)
	}
	rows, _ := database.Select[Subtitle](ctx, f.st.Read(), "subtitles", "place = 'beside' AND state = 'present'")
	if _, err := s.Sync(ctx, rows[0].ID, subtext.Fix{OffsetMS: 2000}, domain.System); err != nil {
		t.Fatal(err)
	}
	after, _ := database.Select[Subtitle](ctx, f.st.Read(), "subtitles", "place = 'beside' AND state = 'present'")
	if len(after) != 1 || after[0].Origin != "fetched" {
		t.Fatalf("a shifted subtitle must keep its source: %+v", after)
	}
	site.Subs = append(site.Subs, fakesite.Sub{ID: "best", Kind: "movie", Language: "en", Title: "The Lantern", Year: 2010,
		Release: "The.Lantern.2010.1080p.BluRay.x264-GRP", Format: "srt", Text: fakesite.Cue("second", 1000)})
	res, _ = s.Search(ctx, f.movieFile, SearchOptions{Better: true, BetterDays: 7}, domain.System)
	for _, o := range res.Outcomes {
		if o.Result == "upgraded" {
			t.Fatalf("the hand-shifted subtitle was replaced: %+v", res.Outcomes)
		}
	}
	if err := s.handleBetter(ctx, &jobs.Run{}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f.abs("The Lantern (2010)/The Lantern (2010) 1080p BluRay.en.srt"))
	if !strings.Contains(string(b), "first") {
		t.Fatalf("file changed: %q", b)
	}
}
