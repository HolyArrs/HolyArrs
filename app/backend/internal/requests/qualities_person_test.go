package requests

import (
	"context"
	"errors"
	"testing"
)

// Without a choice of their own a person may pick every line that is shown,
// as before: 4K too, and their 4K asks still wait unless they hold the 4K
// auto-approve permission.
func TestPersonQualitiesDefaultIsEveryShownLine(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	got, err := ReadPersonQualities(ctx, r.st.Read(), r.ana.UserID)
	if err != nil || got.Movie != nil || got.Show != nil {
		t.Fatalf("default %+v %v", got, err)
	}
	set, _ := ReadSettings(ctx, r.st.Read())
	ls, _ := qualityLines(ctx, r.st.Read(), set, "movie")
	if ids(offered(ls, r.ana, "movie")) != "standard,4k" {
		t.Fatalf("offered %s", ids(offered(ls, r.ana, "movie")))
	}
	res, err := r.svc.AskQuality(ctx, r.ana, "693134", QualityFourK)
	if err != nil || res.Ask.State != "asked" {
		t.Fatalf("4K ask %+v %v", res.Ask, err)
	}
}

// The person editor narrows the lines one person may pick, per kind: a line
// left out is not offered and cannot be asked for; the other kind keeps all.
func TestPersonQualitiesNarrowWhatOnePersonMayPick(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if err := SetPersonQualities(ctx, r.st, r.ana.UserID, PersonQualities{Movie: []string{QualityStandard, "nope", QualityStandard}}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPersonQualities(ctx, r.st.Read(), r.ana.UserID)
	if err != nil || len(got.Movie) != 1 || got.Movie[0] != QualityStandard || got.Show != nil {
		t.Fatalf("stored %+v %v", got, err)
	}
	if _, err := r.svc.AskQuality(ctx, r.ana, "693134", QualityFourK); !errors.Is(err, ErrQualityNotOffered) {
		t.Fatalf("4K for ana: %v", err)
	}
	if _, err := r.svc.AskQuality(ctx, r.ana, "693134", QualityStandard); err != nil {
		t.Fatalf("standard for ana: %v", err)
	}
	if _, err := r.svc.AskQuality(ctx, r.bo, "693134", QualityFourK); err != nil {
		t.Fatalf("4K for bo: %v", err)
	}
	set, _ := ReadSettings(ctx, r.st.Read())
	shows, _ := qualityLines(ctx, r.st.Read(), set, "show")
	p := r.ana
	if err := loadPersonQualities(ctx, r.st.Read(), &p); err != nil {
		t.Fatal(err)
	}
	if want := ids(offered(shows, r.bo, "show")); ids(offered(shows, p, "show")) != want {
		t.Fatalf("shows %s want %s", ids(offered(shows, p, "show")), want)
	}
	// Back to every line.
	if err := SetPersonQualities(ctx, r.st, r.ana.UserID, PersonQualities{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := ReadPersonQualities(ctx, r.st.Read(), r.ana.UserID); got.Movie != nil {
		t.Fatalf("reset %+v", got)
	}
}

// A kind needs at least one line, and an unknown person is not found.
func TestPersonQualitiesChecks(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if err := SetPersonQualities(ctx, r.st, r.ana.UserID, PersonQualities{Show: []string{}}); !errors.Is(err, ErrQualityNoneForPerson) {
		t.Fatalf("empty: %v", err)
	}
	if err := SetPersonQualities(ctx, r.st, r.ana.UserID, PersonQualities{Show: []string{"bad id"}}); !errors.Is(err, ErrQualityNoneForPerson) {
		t.Fatalf("only unknown: %v", err)
	}
	if err := SetPersonQualities(ctx, r.st, 99999, PersonQualities{}); err == nil {
		t.Fatal("unknown person saved")
	}
}

// Weekly limits and the quality choice are saved apart: one never resets the
// other.
func TestPersonQualitiesKeepLimitsAndBack(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	three := 3
	if err := SetPersonLimit(ctx, r.st, r.ana.UserID, PersonLimit{Movies: &three}); err != nil {
		t.Fatal(err)
	}
	if err := SetPersonQualities(ctx, r.st, r.ana.UserID, PersonQualities{Movie: []string{QualityStandard}}); err != nil {
		t.Fatal(err)
	}
	if m, _, _, _ := PersonLimits(ctx, r.st.Read(), r.ana.UserID); m == nil || *m != 3 {
		t.Fatalf("limit lost: %v", m)
	}
	if err := SetPersonLimit(ctx, r.st, r.ana.UserID, PersonLimit{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := ReadPersonQualities(ctx, r.st.Read(), r.ana.UserID); len(got.Movie) != 1 {
		t.Fatalf("qualities lost: %+v", got)
	}
}
