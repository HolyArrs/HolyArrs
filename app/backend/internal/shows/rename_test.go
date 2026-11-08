package shows

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// 4.2.1: episode files are renamed by the show patterns (show folder,
// season folder, episode name), a multi-episode file by its run, and the
// preview is exactly what the rename does.
func TestRenameEpisodesPreviewMatchesResult(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{Seasons: library.SeasonsAll}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	slots := map[string]int64{}
	var vID int64
	rows, err := s.Read().Query(`SELECT s.id, s.version_id, printf('S%02dE%02d', json_extract(i.numbers, '$.season'), json_extract(i.numbers, '$.episode'))
		FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ?`, id)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var sid int64
		var label string
		if err := rows.Scan(&sid, &vID, &label); err != nil {
			t.Fatal(err)
		}
		slots[label] = sid
	}
	rows.Close()
	put := func(rel, release string, slotIDs ...int64) mediafiles.File {
		writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(rel)), "video:"+rel)
		f := mediafiles.File{SlotIDs: slotIDs, VersionID: vID, RelativePath: rel, Size: 10, AddedAt: now, Languages: []string{},
			MediaInfo: map[string]any{"step": "1080p_web"}, ReleaseName: &release, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		if err := s.Write(ctx, func(tx *sql.Tx) error { _, err := database.Insert(tx, "files", &f, domain.System); return err }); err != nil {
			t.Fatal(err)
		}
		return f
	}
	multi := put("Lab Show (2025)/Lab.Show.S01E01E02.1080p.WEB.H264-HOLYARRS.mkv", "Lab.Show.S01E01E02.1080p.WEB.H264-HOLYARRS", slots["S01E01"], slots["S01E02"])
	special := put("Lab Show (2025)/Season 0/Lab.Show.S00E01.1080p.WEB.H264-HOLYARRS.mkv", "Lab.Show.S00E01.1080p.WEB.H264-HOLYARRS", slots["S00E01"])
	plan, err := library.PlanRename(ctx, s.Read(), []int64{id})
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64]string{
		multi.ID:   "Lab Show (2025)/Season 1/Lab Show - S01E01-E02 - Pilot + Second 1080p WEB.mkv",
		special.ID: "Lab Show (2025)/Specials/Lab Show - S00E01 - Behind the Lab 1080p WEB.mkv",
	}
	if len(plan) != 2 {
		t.Fatalf("plan %+v", plan)
	}
	for _, st := range plan {
		if st.To != want[st.FileID] || st.Skip != "" {
			t.Fatalf("step %+v, want %s", st, want[st.FileID])
		}
	}
	if n, err := svc.ApplyRename(ctx, plan, 0, domain.System); err != nil || n != 2 {
		t.Fatalf("renamed %d %v", n, err)
	}
	for fid, to := range want {
		if f := fileRow(t, s, fid); f.RelativePath != to {
			t.Fatalf("db %q, want %q", f.RelativePath, to)
		}
		if _, err := os.Stat(filepath.Join(lf.Path, filepath.FromSlash(to))); err != nil {
			t.Fatal(err)
		}
	}
	if again, _ := library.PlanRename(ctx, s.Read(), []int64{id}); len(again) != 0 {
		t.Fatalf("still to rename: %+v", again)
	}
}
