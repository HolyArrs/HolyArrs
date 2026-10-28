package asks

import (
	"context"
	"errors"
	"fmt"
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

// episodeKeeps reads Keep up to date per episode of a season ("S1E2": on).
func episodeKeeps(t *testing.T, st *store.Store, titleID int64) map[string]bool {
	t.Helper()
	rows, err := st.Read().Query(`SELECT p.number, json_extract(i.numbers, '$.episode'), p.keep_up_to_date AND i.keep_up_to_date FROM items i JOIN parts p ON p.id = i.part_id
		WHERE i.title_id = ? AND i.kind = 'episode'`, titleID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var s, e int
		var keep bool
		if err := rows.Scan(&s, &e, &keep); err != nil {
			t.Fatal(err)
		}
		out[fmt.Sprintf("S%dE%d", s, e)] = keep
	}
	return out
}

func TestAskPartOfASeason(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	res, err := r.svc.AskShowParts(ctx, r.ana, "tmdb:100", []int{1}, []domain.AskEpisodes{{Season: 2, Episodes: []int{2, 2}}, {Season: 1, Episodes: []int{1}}})
	if err != nil {
		t.Fatal(err)
	}
	a := res.Ask
	// Season 1 is whole, so its single episode folds into it.
	if len(a.Seasons) != 1 || a.Seasons[0] != 1 || len(a.Episodes) != 1 || a.Episodes[0].Season != 2 || len(a.Episodes[0].Episodes) != 1 || a.Episodes[0].Episodes[0] != 2 {
		t.Fatalf("asked %+v %+v", a.Seasons, a.Episodes)
	}
	if w := askSeasonWords(a); w != "season 1 and episode 2 of season 2" {
		t.Fatalf("words %q", w)
	}
	// Asking again for what it covers joins; bad parts are refused.
	if again, err := r.svc.AskShowParts(ctx, r.bo, "tmdb:100", nil, []domain.AskEpisodes{{Season: 2, Episodes: []int{2}}}); err != nil || again.Ask.ID != a.ID {
		t.Fatalf("join %v %+v", err, again.Ask)
	}
	if _, err := r.svc.AskShowParts(ctx, r.bo, "tmdb:100", nil, []domain.AskEpisodes{{Season: 9, Episodes: []int{1}}}); !errors.Is(err, ErrNoSuchSeason) {
		t.Fatalf("season 9: %v", err)
	}
	if _, err := r.svc.AskShowParts(ctx, r.bo, "tmdb:100", nil, []domain.AskEpisodes{{Season: 2, Episodes: []int{0}}}); !errors.Is(err, ErrBadEpisode) {
		t.Fatalf("episode 0: %v", err)
	}
	got, err := r.svc.Approve(ctx, a.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	k := episodeKeeps(t, r.st, *got.TitleID)
	if !k["S1E1"] || !k["S1E2"] || k["S2E1"] || !k["S2E2"] {
		t.Fatalf("episode switches %v", k)
	}
}

func TestApproveAddsAnEpisodeNotAskedFor(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	res, err := r.svc.AskShowParts(ctx, r.ana, "tmdb:100", nil, []domain.AskEpisodes{{Season: 2, Episodes: []int{2}}})
	if err != nil {
		t.Fatal(err)
	}
	// Only seasons the ask touches can be picked from.
	if _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{Episodes: []domain.AskEpisodes{{Season: 1, Episodes: []int{1}}}}); !errors.Is(err, ErrNotAskedPart) {
		t.Fatalf("season 1: %v", err)
	}
	got, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{Episodes: []domain.AskEpisodes{{Season: 2, Episodes: []int{1, 2}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Episodes) != 1 || len(got.Episodes[0].Episodes) != 2 {
		t.Fatalf("approved %+v", got.Episodes)
	}
	k := episodeKeeps(t, r.st, *got.TitleID)
	if k["S1E1"] || k["S1E2"] || !k["S2E1"] || !k["S2E2"] {
		t.Fatalf("episode switches %v", k)
	}
}

func TestApproveDropsAnAskedEpisode(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	res, err := r.svc.AskShow(ctx, r.ana, "tmdb:100", []int{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{Seasons: []int{1}, Episodes: []domain.AskEpisodes{{Season: 2, Episodes: []int{1}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Seasons) != 1 || len(got.Episodes) != 1 || got.Episodes[0].Season != 2 {
		t.Fatalf("approved %+v %+v", got.Seasons, got.Episodes)
	}
	k := episodeKeeps(t, r.st, *got.TitleID)
	if !k["S1E1"] || !k["S1E2"] || !k["S2E1"] || k["S2E2"] {
		t.Fatalf("episode switches %v", k)
	}
}

func TestApproveKeepsSomeSongs(t *testing.T) {
	r := musicRig(t)
	ctx := context.Background()
	res, err := r.svc.AskArtist(ctx, r.ana, "mbid:"+mArtist, []string{mFirst})
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tid := *got.TitleID
	var ids []int64
	rows, err := r.st.Read().Query(`SELECT i.id FROM items i JOIN parts p ON p.id = i.part_id WHERE i.title_id = ? AND i.kind = 'track' AND json_extract(p.source_ids, '$.musicbrainz') = ? ORDER BY i.id`, tid, mFirst)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) < 2 {
		t.Skipf("the lab album has %d tracks", len(ids))
	}
	if err := r.svc.keepTracks(ctx, tid, []AlbumTracks{{Album: mFirst, Tracks: []int64{ids[0] + 100000}}}, r.luis); !errors.Is(err, ErrNotAlbumTrack) {
		t.Fatalf("foreign track: %v", err)
	}
	if err := r.svc.keepTracks(ctx, tid, []AlbumTracks{{Album: mFirst, Tracks: ids[:1]}}, r.luis); err != nil {
		t.Fatal(err)
	}
	var on int
	if err := r.st.Read().QueryRow("SELECT count(*) FROM items WHERE title_id = ? AND kind = 'track' AND keep_up_to_date = 1 AND id IN (?, ?)", tid, ids[0], ids[1]).Scan(&on); err != nil || on != 1 {
		t.Fatalf("kept %d %v", on, err)
	}
}
