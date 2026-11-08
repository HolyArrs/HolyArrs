package shows

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestShowVersions adds a show with a 4K Version next to the main one: each
// episode gets one Slot per Version with the same air-date gate, the 4K
// Version shares the show Library folder under its own folder name, a later
// Version follows the same rules, movie folders are refused, and a Version
// holding a file or a download under way is not removed.
func TestShowVersions(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	var fourK int64
	if err := s.Read().QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.FourKName).Scan(&fourK); err != nil {
		t.Fatal(err)
	}
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{Seasons: library.SeasonsAll,
		MoreVersions: []library.VersionInput{{Label: "4K", PickRulesID: fourK}}}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	vs, _ := database.Select[mediafiles.Version](ctx, s.Read(), "versions", "title_id = ? ORDER BY is_main DESC, id", id)
	if len(vs) != 2 || !vs[0].IsMain || vs[1].Label != "4K" || vs[1].PickRulesID != fourK || vs[1].LibraryFolderID != lf.ID ||
		vs[1].FolderName != vs[0].FolderName+" [4K]" {
		t.Fatalf("versions %+v", vs)
	}
	its, _ := database.Select[mediafiles.Item](ctx, s.Read(), "items", "title_id = ? AND kind = 'episode'", id)
	for _, it := range its {
		sl, _ := database.Select[mediafiles.Slot](ctx, s.Read(), "slots", "item_id = ? ORDER BY version_id", it.ID)
		if len(sl) != 2 || sl[0].VersionID != vs[0].ID || sl[1].VersionID != vs[1].ID || sl[0].State != sl[1].State {
			t.Fatalf("episode %d slots %+v", it.ID, sl)
		}
	}

	// A third Version added later opens on the same air dates.
	v3, err := library.AddVersion(ctx, s, id, library.VersionInput{Label: "Remux"}, nil, now, domain.System)
	if err != nil || !strings.HasSuffix(v3.FolderName, "[Remux]") {
		t.Fatalf("third %+v %v", v3, err)
	}
	var mismatched int
	_ = s.Read().QueryRow(`SELECT count(*) FROM slots a JOIN slots b ON a.item_id = b.item_id
		WHERE a.version_id = ? AND b.version_id = ? AND a.state != b.state`, vs[0].ID, v3.ID).Scan(&mismatched)
	var n int
	_ = s.Read().QueryRow("SELECT count(*) FROM slots WHERE version_id = ?", v3.ID).Scan(&n)
	if mismatched != 0 || n != len(its) {
		t.Fatalf("third slots %d of %d, %d differ", n, len(its), mismatched)
	}

	// Movie folders are refused for a show's Version.
	mf, err := library.AddFolder(ctx, s, library.FolderInput{Path: t.TempDir(), Kind: "movie", Label: "Movies"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := library.AddVersion(ctx, s, id, library.VersionInput{Label: "X", LibraryFolderID: mf.ID}, nil, now, domain.System); !errors.Is(err, library.ErrNotShowFolder) {
		t.Fatalf("movie folder: %v", err)
	}
	if _, _, err := svc.AddShow(ctx, "tvmaze:300", ShowAddOptions{MoreVersions: []library.VersionInput{{Label: "4K", LibraryFolderID: mf.ID}}}, domain.System); !errors.Is(err, library.ErrNotShowFolder) {
		t.Fatalf("add with movie folder: %v", err)
	}

	// Busy and holding files: refused; empty: removed with its Slots.
	var slotID int64
	_ = s.Read().QueryRow("SELECT id FROM slots WHERE version_id = ? LIMIT 1", vs[1].ID).Scan(&slotID)
	exec := func(q string, args ...any) error {
		return s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err })
	}
	if err := exec("UPDATE slots SET state = 'downloading' WHERE id = ?", slotID); err != nil {
		t.Fatal(err)
	}
	if err := library.RemoveVersion(ctx, s, id, vs[1].ID, domain.System); !errors.Is(err, library.ErrVersionBusy) {
		t.Fatalf("busy: %v", err)
	}
	// A finished download waiting on its import (here in Needs a decision)
	// still holds the Version; once imported it no longer does.
	if err := exec("UPDATE slots SET state = 'needs_a_decision' WHERE id = ?", slotID); err != nil {
		t.Fatal(err)
	}
	var dlID int64
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		dr := &domain.Downloader{Name: "Lab", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://127.0.0.1:1", Category: "holyarrs", Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", dr, domain.System); err != nil {
			return err
		}
		d := &domain.Download{DownloaderID: dr.ID, SlotIDs: []int64{slotID}, ExternalID: "x", State: "finished", Progress: 1, FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		_, err := database.Insert(tx, "downloads", d, domain.System)
		dlID = d.ID
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := library.RemoveVersion(ctx, s, id, vs[1].ID, domain.System); !errors.Is(err, library.ErrVersionBusy) {
		t.Fatalf("finished download waiting on import: %v", err)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "imports", &mediafiles.Import{DownloadID: &dlID, FromPath: "/x", Kind: "episodes", Plan: []mediafiles.PlanStep{}, Result: []mediafiles.PlaceResult{}, Mode: "auto", State: "done"}, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := exec("UPDATE slots SET state = 'in_library' WHERE id = ?", slotID); err != nil {
		t.Fatal(err)
	}
	f := &mediafiles.File{SlotIDs: []int64{slotID}, VersionID: vs[1].ID, RelativePath: "Season 01/a.mkv", Size: 1, AddedAt: now, State: "present", Role: "main", Origin: "imported", PlacedBy: "move"}
	if err := s.Write(ctx, func(tx *sql.Tx) error { _, err := database.Insert(tx, "files", f, domain.System); return err }); err != nil {
		t.Fatal(err)
	}
	if err := library.RemoveVersion(ctx, s, id, vs[1].ID, domain.System); !errors.Is(err, library.ErrVersionHasFiles) {
		t.Fatalf("files: %v", err)
	}
	if err := library.RemoveVersion(ctx, s, id, v3.ID, domain.System); err != nil {
		t.Fatal(err)
	}
	_ = s.Read().QueryRow("SELECT count(*) FROM slots WHERE version_id = ?", v3.ID).Scan(&n)
	if n != 0 {
		t.Fatalf("slots left %d", n)
	}

	// The Calendar lists each episode once, with both Versions' states.
	es, err := library.Calendar(ctx, s.Read(), now.AddDate(-2, 0, 0), now.AddDate(0, 1, 0), library.CalendarShows)
	if err != nil || len(es) == 0 {
		t.Fatalf("calendar %v %v", es, err)
	}
	seen := map[int64]bool{}
	for _, e := range es {
		if seen[e.ItemID] || len(e.Versions) != 2 || e.Versions[0].Label != "Main" || e.Versions[1].Label != "4K" || e.Versions[0].State != e.State {
			t.Fatalf("calendar entry %+v", e)
		}
		seen[e.ItemID] = true
	}
}

// 4.8.11: a rename keeps each Version's episode files in its own folder,
// so the HD and 4K copies of one episode never collide.
func TestShowVersionsRename(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{Seasons: library.SeasonsAll, MoreVersions: []library.VersionInput{{Label: "4K"}}}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	vs, _ := database.Select[mediafiles.Version](ctx, s.Read(), "versions", "title_id = ? ORDER BY is_main DESC, id", id)
	var put []mediafiles.File
	for i, v := range vs {
		var slotID int64
		if err := s.Read().QueryRow(`SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.version_id = ?
			AND json_extract(i.numbers, '$.season') = 1 AND json_extract(i.numbers, '$.episode') = 1`, v.ID).Scan(&slotID); err != nil {
			t.Fatal(err)
		}
		step, rel := "1080p_web", v.FolderName+"/Lab.Show.S01E01.1080p.WEB.H264-HOLYARRS.mkv"
		if i == 1 {
			step, rel = "2160p_web", v.FolderName+"/Lab.Show.S01E01.2160p.WEB.H265-HOLYARRS.mkv"
		}
		writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(rel)), "video:"+rel)
		release := strings.TrimSuffix(filepath.Base(rel), ".mkv")
		f := mediafiles.File{SlotIDs: []int64{slotID}, VersionID: v.ID, RelativePath: rel, Size: 10, AddedAt: now, Languages: []string{},
			MediaInfo: map[string]any{"step": step}, ReleaseName: &release, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		if err := s.Write(ctx, func(tx *sql.Tx) error { _, err := database.Insert(tx, "files", &f, domain.System); return err }); err != nil {
			t.Fatal(err)
		}
		put = append(put, f)
	}
	plan, err := library.PlanRename(ctx, s.Read(), []int64{id})
	if err != nil || len(plan) != 2 {
		t.Fatalf("plan %+v %v", plan, err)
	}
	if n, err := svc.ApplyRename(ctx, plan, 0, domain.System); err != nil || n != 2 {
		t.Fatalf("renamed %d %v", n, err)
	}
	for i, f := range put {
		got := fileRow(t, s, f.ID)
		if !strings.HasPrefix(got.RelativePath, vs[i].FolderName+"/Season 1/") {
			t.Fatalf("Version %s file at %s", vs[i].Label, got.RelativePath)
		}
		if _, err := os.Stat(filepath.Join(lf.Path, filepath.FromSlash(got.RelativePath))); err != nil {
			t.Fatal(err)
		}
	}
	// Deleting the episode with its files takes every Version's copy.
	var itemID int64
	_ = s.Read().QueryRow(`SELECT id FROM items WHERE title_id = ? AND json_extract(numbers, '$.season') = 1 AND json_extract(numbers, '$.episode') = 1`, id).Scan(&itemID)
	p, err := svc.RemoveEpisodes(ctx, id, library.RemoveScope{EpisodeIDs: []int64{itemID}}, library.RemoveChoice{DeleteFiles: true}, true, domain.System)
	if err != nil || len(p.Held) != 2 {
		t.Fatalf("delete %+v %v", p, err)
	}
	for _, f := range put {
		if got := fileRow(t, s, f.ID); got.State == "present" {
			t.Fatalf("file %d still present after the delete", f.ID)
		}
	}
}
