package asks

import (
	"context"
	"errors"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

func TestApproveSomeSeasonsOfAShowAsk(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	res, err := r.svc.AskShow(ctx, r.ana, "tmdb:100", []int{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	// A season the ask is not for is refused, and the ask stays open.
	if _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{Seasons: []int{0}}); !errors.Is(err, ErrNotAskedPart) {
		t.Fatalf("season 0: %v", err)
	}
	got, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{Seasons: []int{2, 2}})
	if err != nil {
		t.Fatal(err)
	}
	if got.State != domain.AskApproved || len(got.Seasons) != 1 || got.Seasons[0] != 2 {
		t.Fatalf("approved %+v", got)
	}
	keeps := seasonKeeps(t, r.st, *got.TitleID)
	if keeps[1] || !keeps[2] {
		t.Fatalf("season switches %v", keeps)
	}
}

func TestApproveSomeSeasonsOfAWholeShowAsk(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	res, err := r.svc.AskShow(ctx, r.ana, "tmdb:100", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{Seasons: []int{9}}); !errors.Is(err, ErrNoSuchSeason) {
		t.Fatalf("season 9: %v", err)
	}
	got, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{Seasons: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Seasons) != 1 || got.Seasons[0] != 1 {
		t.Fatalf("approved %+v", got)
	}
	keeps := seasonKeeps(t, r.st, *got.TitleID)
	if !keeps[1] || keeps[2] {
		t.Fatalf("season switches %v", keeps)
	}
}

func TestApproveSomeAlbumsOfAnArtistAsk(t *testing.T) {
	r := musicRig(t)
	ctx := context.Background()
	res, err := r.svc.AskArtist(ctx, r.ana, "mbid:"+mArtist, []string{mFirst, mLive})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{Albums: []string{mSecond}}); !errors.Is(err, ErrNotAskedPart) {
		t.Fatalf("album not asked: %v", err)
	}
	if _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{Seasons: []int{1}}); !errors.Is(err, ErrNotAskedPart) {
		t.Fatalf("seasons on music: %v", err)
	}
	got, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{Albums: []string{mLive}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Albums) != 1 || got.Albums[0].Name != "Live Night" {
		t.Fatalf("approved %+v", got)
	}
	keeps := albumKeeps(t, r.st, *got.TitleID)
	if !keeps["Live Night"] || keeps["First Light"] {
		t.Fatalf("album keeps %v", keeps)
	}
}

func TestApproveSomeAlbumsOfAWholeArtistAsk(t *testing.T) {
	r := musicRig(t)
	ctx := context.Background()
	res, err := r.svc.AskArtist(ctx, r.ana, "mbid:"+mArtist, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{Albums: []string{mSecond}})
	if err != nil {
		t.Fatal(err)
	}
	var a domain.Ask
	if err := store.Get(ctx, r.st.Read(), "asks", got.ID, &a); err != nil {
		t.Fatal(err)
	}
	if len(a.Albums) != 1 || a.Albums[0].MBID != mSecond {
		t.Fatalf("stored ask %+v", a)
	}
	keeps := albumKeeps(t, r.st, *got.TitleID)
	if !keeps["Second Wind"] || keeps["First Light"] {
		t.Fatalf("album keeps %v", keeps)
	}
}
