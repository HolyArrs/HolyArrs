package shows

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A dubbed Version that is not kept up to date wants only the episodes asked
// for in it; the main Version is left as it was, and each may leave an
// episode out or follow the episode again.
func TestEpisodeInOneVersion(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{Seasons: "none", NewSeasons: "none"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	off := false
	es, err := library.AddVersion(ctx, s, id, library.VersionInput{Label: "es", LibraryFolderID: lf.ID, KeepUpToDate: &off, AudioLanguages: []string{"es"}}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var main mediafiles.Version
	if err := s.Read().QueryRow("SELECT id FROM versions WHERE title_id = ? AND is_main = 1", id).Scan(&main.ID); err != nil {
		t.Fatal(err)
	}
	ep := episodes(t, s, id)["S01E02"].ItemID
	slotKeep := func(versionID int64) *bool {
		t.Helper()
		got, err := database.Select[mediafiles.Slot](ctx, s.Read(), "slots", "item_id = ? AND version_id = ?", ep, versionID)
		if err != nil || len(got) != 1 {
			t.Fatalf("slots %+v %v", got, err)
		}
		return got[0].KeepUpToDate
	}
	searched := func() map[int64]bool {
		t.Helper()
		got, err := ShowSeasonsToSearch(ctx, s.Read(), id)
		if err != nil {
			t.Fatal(err)
		}
		out := map[int64]bool{}
		for _, sv := range got {
			out[sv.VersionID] = true
		}
		return out
	}
	if got := searched(); len(got) != 0 {
		t.Fatalf("nothing kept, searched %v", got)
	}

	// Asked in the dubbed Version: only that Version looks for it.
	on := true
	it, sl, v, err := svc.SetEpisodeInVersion(ctx, ep, es.ID, &on, domain.System)
	if err != nil || it.KeepUpToDate || sl.KeepUpToDate == nil || !*sl.KeepUpToDate || v.ID != es.ID {
		t.Fatalf("set in es: %+v %+v %v", it, sl, err)
	}
	if slotKeep(main.ID) != nil {
		t.Fatal("the main Version changed")
	}
	if got := searched(); len(got) != 1 || !got[es.ID] {
		t.Fatalf("only es searched, got %v", got)
	}

	// Season 1 switched on for the show: the main Version looks for it too,
	// until each of its episodes is left out of the main Version.
	if err := svc.SetSeason(ctx, id, 1, true, domain.System); err != nil {
		t.Fatal(err)
	}
	if got := searched(); len(got) != 2 {
		t.Fatalf("both searched, got %v", got)
	}
	for code, e := range episodes(t, s, id) {
		if strings.HasPrefix(code, "S01") {
			if _, _, _, err := svc.SetEpisodeInVersion(ctx, e.ItemID, main.ID, &off, domain.System); err != nil {
				t.Fatal(err)
			}
		}
	}
	if got := searched(); len(got) != 1 || !got[es.ID] {
		t.Fatalf("main left out, got %v", got)
	}

	// Following the episode again: the dubbed Version is not kept up to date, so it stops.
	if _, _, _, err := svc.SetEpisodeInVersion(ctx, ep, es.ID, nil, domain.System); err != nil || slotKeep(es.ID) != nil {
		t.Fatalf("follow again: %v", err)
	}
	if got := searched(); len(got) != 0 {
		t.Fatalf("nothing wanted, got %v", got)
	}

	// Refused: a Version of another title, an Item that is not an episode.
	if _, _, _, err := svc.SetEpisodeInVersion(ctx, ep, 99999, &on, domain.System); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("unknown Version: %v", err)
	}
	mlf, err := library.AddFolder(ctx, s, library.FolderInput{Path: t.TempDir(), Kind: "movie", Label: "Movies"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	mid, err := library.AddMovie(ctx, s, dune, library.AddOptions{LibraryFolderID: mlf.ID}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var movieItem, movieVersion int64
	if err := s.Read().QueryRow("SELECT i.id, v.id FROM items i JOIN versions v ON v.title_id = i.title_id WHERE i.title_id = ?", mid).Scan(&movieItem, &movieVersion); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := svc.SetEpisodeInVersion(ctx, movieItem, movieVersion, &on, domain.System); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("a movie: %v", err)
	}
}
