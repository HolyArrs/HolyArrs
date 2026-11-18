package requests

import (
	"context"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestOwnerAutoApproveDefaultsOnAndSaves(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	s, err := ReadSettings(ctx, r.st.Read())
	if err != nil || !s.OwnerAutoApprove {
		t.Fatalf("default %+v %v", s, err)
	}
	s.OwnerAutoApprove = false
	if err := SaveSettings(ctx, r.st, s, domain.System); err != nil {
		t.Fatal(err)
	}
	if s, _ = ReadSettings(ctx, r.st.Read()); s.OwnerAutoApprove {
		t.Fatal("owner switch did not save")
	}
}

func TestOwnerAsksWaitWhenSwitchIsOff(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	owner := r.ana
	owner.Owner, owner.AutoApprove, owner.AutoApproveFourK = true, true, true
	s, _ := ReadSettings(ctx, r.st.Read())
	s.OwnerAutoApprove = false
	if err := SaveSettings(ctx, r.st, s, domain.System); err != nil {
		t.Fatal(err)
	}
	res, err := r.svc.Ask(ctx, owner, "693134")
	if err != nil || res.Ask.State != domain.AskAsked {
		t.Fatalf("owner ask with switch off %+v %v", res.Ask, err)
	}
	// Someone else with their own switch on is still approved straight away.
	other := r.bo
	other.AutoApprove = true
	if res, err = r.svc.Ask(ctx, other, "900002"); err != nil || res.Ask.State != domain.AskApproved {
		t.Fatalf("other ask %+v %v", res.Ask, err)
	}
	s.OwnerAutoApprove = true
	_ = SaveSettings(ctx, r.st, s, domain.System)
	if res, err = r.svc.Ask(ctx, owner, "900005"); err != nil || res.Ask.State != domain.AskApproved {
		t.Fatalf("owner ask with switch on %+v %v", res.Ask, err)
	}
}

func TestOwnerWatchlistWaitsWhenSwitchIsOff(t *testing.T) {
	r, f, w := watchRig(t)
	ctx := context.Background()
	if err := SaveWatchlistSwitches(ctx, r.st, r.luis.UserID, WatchlistSwitches{Movies: true}); err != nil {
		t.Fatal(err)
	}
	s, _ := ReadSettings(ctx, r.st.Read())
	s.OwnerAutoApprove = false
	if err := SaveSettings(ctx, r.st, s, domain.System); err != nil {
		t.Fatal(err)
	}
	f.set("tok-luis", plexMovie("k-paper", "Paper Harbour", 2023, "tmdb://900002"))
	if _, err := w.Pass(ctx, r.luis.UserID); err != nil {
		t.Fatal(err)
	}
	vs, _ := r.svc.Views(ctx, "a.tmdb_id = ?", "900002")
	if len(vs) != 1 || vs[0].State != domain.AskAsked {
		t.Fatalf("owner's watchlist ask %+v", vs)
	}
}
