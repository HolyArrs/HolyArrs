package requests

import (
	"context"
	"errors"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// An owner adds lines of their own: a name and Pick rules, kept in the order
// given and stored with the list. Each needs both, the Pick rules must exist
// and a list has a top size.
func TestOwnLinesAreStoredAndChecked(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	everyday := rulesID(t, r, mediafiles.GoodEverydayName)
	saveQualities(t, r, func(q *QualityLists) {
		q.Movie = append([]QualityLine{{ID: "own-small", Name: " Small ", PickRulesID: everyday}}, q.Movie...)
	})
	back, _ := ReadSettings(ctx, r.st.Read())
	if ids(back.Qualities.Movie) != "own-small,standard,4k" || back.Qualities.Movie[0].Name != "Small" || back.Qualities.Movie[0].PickRulesID != everyday || ids(back.Qualities.Show) != "standard,4k" {
		t.Fatalf("saved %+v", back.Qualities)
	}
	ls, err := qualityLines(ctx, r.st.Read(), back, "movie")
	if err != nil {
		t.Fatal(err)
	}
	if ls[0].Extra || !ls[0].Ready || ls[0].PickRulesID != everyday || ls[1].PickRulesID != 0 {
		t.Fatalf("lines %+v", ls)
	}
	try := func(l QualityLine) error {
		bad := back
		bad.Qualities = defaultQualities()
		bad.Qualities.Show = append(bad.Qualities.Show, l)
		return SaveSettings(ctx, r.st, bad, domain.System)
	}
	if err := try(QualityLine{ID: "own-a", PickRulesID: everyday}); !errors.Is(err, ErrQualityOwnLine) {
		t.Fatalf("no name: %v", err)
	}
	if err := try(QualityLine{ID: "own-a", Name: "A"}); !errors.Is(err, ErrQualityOwnLine) {
		t.Fatalf("no Pick rules: %v", err)
	}
	if err := try(QualityLine{ID: "own-a", Name: "A", PickRulesID: 999999}); !errors.Is(err, library.ErrNoPickRules) {
		t.Fatalf("Pick rules that are not there: %v", err)
	}
	many := back
	many.Qualities = defaultQualities()
	for i := 0; i < qualityLinesMax; i++ {
		many.Qualities.Movie = append(many.Qualities.Movie, QualityLine{ID: "own-" + strings.Repeat("a", i+1), Name: "A", PickRulesID: everyday})
	}
	if err := SaveSettings(ctx, r.st, many, domain.System); !errors.Is(err, ErrQualityTooMany) {
		t.Fatalf("too many: %v", err)
	}
	// A line's id is a built-in one or one of the owner's; anything else is dropped.
	if got := parseQualities(`{"movie":[{"id":"own-","name":"x","pick_rules_id":3},{"id":"own-OK","name":"x","pick_rules_id":3},{"id":"own-b2","name":"B","pick_rules_id":3}]}`); ids(got.Movie) != "own-b2,standard,4k" {
		t.Fatalf("parsed %+v", got.Movie)
	}
	// Its Pick rules removed later: the line is no longer offered.
	pinExec(t, r, "DELETE FROM pick_rules WHERE id = ?", everyday)
	ls, _ = qualityLines(ctx, r.st.Read(), back, "movie")
	if ls[0].Ready || ids(offered(ls, r.luis, "movie")) != "standard,4k" {
		t.Fatalf("rules gone %+v", ls)
	}
}

// A line of the owner's own asks for the title's main copy on its Pick
// rules: the ask remembers them, approval puts them on the copy, and a
// Standard ask for the same movie joins it (one main copy, one open ask).
func TestOwnLineAsksForTheMainCopyOnItsRules(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	everyday := rulesID(t, r, mediafiles.GoodEverydayName)
	saveQualities(t, r, func(q *QualityLists) {
		q.Movie = append(q.Movie, QualityLine{ID: "own-small", Name: "Small", PickRulesID: everyday})
	})
	res, err := r.svc.AskQuality(ctx, r.ana, "900002", "own-small")
	if err != nil {
		t.Fatal(err)
	}
	a := res.Ask
	if a.Quality != "own-small" || a.QualityName != "Small" || a.QualityRulesID != everyday || a.QualityExtra || a.FourK {
		t.Fatalf("ask %+v", a)
	}
	joined, err := r.svc.AskQuality(ctx, r.bo, "900002", QualityStandard)
	if err != nil || !joined.Joined || joined.Ask.ID != a.ID {
		t.Fatalf("standard ask beside it %+v %v", joined, err)
	}
	uhd, err := r.svc.AskQuality(ctx, r.bo, "900002", QualityFourK)
	if err != nil || uhd.Joined || uhd.Ask.ID == a.ID {
		t.Fatalf("4K ask beside it %+v %v", uhd, err)
	}
	got, err := r.svc.Approve(ctx, a.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cs := copies(t, r, *got.TitleID); len(cs) != 1 || !cs[0].main || cs[0].fourK || cs[0].rules != everyday {
		t.Fatalf("copies %+v", cs)
	}
}

// Shows work the same way: an own line asks for the show's main copy, and a
// Standard ask for the same seasons joins it.
func TestOwnLineOnAShowJoinsTheMainCopyAsk(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	everyday := rulesID(t, r, mediafiles.GoodEverydayName)
	saveQualities(t, r, func(q *QualityLists) {
		q.Show = append(q.Show, QualityLine{ID: "own-small", Name: "Small", PickRulesID: everyday})
	})
	res, err := r.svc.AskShowQuality(ctx, r.ana, "tmdb:100", []int{1}, nil, false, "own-small")
	if err != nil {
		t.Fatal(err)
	}
	if res.Ask.Quality != "own-small" || res.Ask.QualityRulesID != everyday || res.Ask.QualityExtra {
		t.Fatalf("ask %+v", res.Ask)
	}
	joined, err := r.svc.AskShowQuality(ctx, r.bo, "tmdb:100", []int{1}, nil, false, QualityStandard)
	if err != nil || !joined.Joined || joined.Ask.ID != res.Ask.ID {
		t.Fatalf("standard ask beside it %+v %v", joined, err)
	}
}
