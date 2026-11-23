package sources

import (
	"context"
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// checkParams asks for Dune: Part Two with the Good everyday Pick rules.
func checkParams(t *testing.T, f fixture) map[string]any {
	t.Helper()
	var pr int64
	if err := f.s.Read().QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.GoodEverydayName).Scan(&pr); err != nil {
		t.Fatal(err)
	}
	return map[string]any{"tmdb": 693134, "imdb": "tt15239678", "name": "Dune: Part Two", "year": 2024, "pick_rules_id": pr, "by_hand": true}
}

// A check before the movie is in the Library lists the releases judged by
// the chosen Pick rules, stores nothing and moves no Slot.
func TestReleaseCheck_ListsWithoutStoring(t *testing.T) {
	f := seed(t, feedServer(t, duneFeed))
	sr := &Searcher{Store: f.s}
	j := runKind(t, f, sr, jobs.KindReleaseCheck, nil, checkParams(t, f))
	if j.State != "done" {
		t.Fatalf("check job %s: %+v", j.State, j.Error)
	}
	views, ok := sr.CheckResult(j.ID)
	if !ok || len(views) != 4 {
		t.Fatalf("want 4 releases, got %d (%v)", len(views), ok)
	}
	byName := map[string]ReleaseView{}
	for _, v := range views {
		byName[v.Name] = v
		if v.ID <= 0 {
			t.Errorf("%s has no id to pick it by", v.Name)
		}
	}
	if !byName["Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS"].Passed {
		t.Error("the 1080p WEB release should pass Good everyday")
	}
	if cam := byName["Dune.Part.Two.2024.HDCAM.x264-BAD"]; cam.Passed || !cam.NeedsForce {
		t.Errorf("the camera copy should fail and need force: %+v", cam)
	}
	if byName["Arcane.S01E01.1080p.WEB.H264-HOLYARRS"].Passed {
		t.Error("another title's release should not pass")
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM releases").Scan(&n)
	if n != 0 {
		t.Errorf("a check stored %d releases", n)
	}
	if st := slotState(t, f).State; st != mediafiles.SlotWanted {
		t.Errorf("a check moved the Slot to %s", st)
	}
}

// Picking a checked release once the movie is added stores it for the
// movie's Slot and chooses it; a failed one needs force.
func TestReleaseCheck_PickAfterAdd(t *testing.T) {
	f := seed(t, feedServer(t, duneFeed))
	sr := &Searcher{Store: f.s}
	j := runKind(t, f, sr, jobs.KindReleaseCheck, nil, checkParams(t, f))
	views, _ := sr.CheckResult(j.ID)
	var good, cam int64
	for _, v := range views {
		switch v.Name {
		case "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS":
			good = v.ID
		case "Dune.Part.Two.2024.HDCAM.x264-BAD":
			cam = v.ID
		}
	}
	var titleID int64
	_ = f.s.Read().QueryRow("SELECT title_id FROM items LIMIT 1").Scan(&titleID)
	ctx := context.Background()
	by := domain.Actor{Kind: "user", ID: 1}

	var nf *NeedsForceError
	if _, err := sr.CheckPick(ctx, j.ID, cam, titleID, false, by); !errors.As(err, &nf) {
		t.Fatalf("camera copy without force: %v", err)
	}
	if st := slotState(t, f).State; st != mediafiles.SlotWanted {
		t.Fatalf("a refused pick moved the Slot to %s", st)
	}
	ch, err := sr.CheckPick(ctx, j.ID, good, titleID, false, by)
	if err != nil {
		t.Fatal(err)
	}
	if ch.SlotID != f.slotID || slotState(t, f).State != mediafiles.SlotChosen {
		t.Fatalf("choice %+v, slot %s", ch, slotState(t, f).State)
	}
	if rel := relByName(t, f, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS"); rel.State != "chosen" || rel.FoundBy != "hand_search" {
		t.Fatalf("release %s found by %s", rel.State, rel.FoundBy)
	}
	if _, err := sr.CheckPick(ctx, j.ID+99, good, titleID, false, by); !errors.Is(err, ErrCheckGone) {
		t.Fatalf("unknown check: %v", err)
	}
}
