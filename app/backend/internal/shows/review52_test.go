package shows

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// 5.2 review: episode renames that change only case (.MKV to .mkv, a
// season folder's case) finish with the file on disk under the name
// recorded, a second plan is empty, and a season folder that is a link
// out of the Library folder is never renamed through.
func TestEpisodeCaseOnlyRenames(t *testing.T) {
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
	put := func(rel string, slotIDs ...int64) mediafiles.File {
		writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(rel)), "video:"+rel)
		release := "Lab.Show.1080p.WEB.H264-HOLYARRS"
		f := mediafiles.File{SlotIDs: slotIDs, VersionID: vID, RelativePath: rel, Size: 10, AddedAt: now, Languages: []string{},
			MediaInfo: map[string]any{"step": "1080p_web"}, ReleaseName: &release, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		if err := s.Write(ctx, func(tx *sql.Tx) error { _, err := database.Insert(tx, "files", &f, domain.System); return err }); err != nil {
			t.Fatal(err)
		}
		return f
	}
	// The names the patterns give, found from a first plan.
	targetOf := func(slot int64) string {
		probe := put("Lab Show (2025)/x/probe.mkv", slot)
		plan, _ := library.PlanRename(ctx, s.Read(), []int64{id})
		_ = s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec("DELETE FROM files WHERE id = ?", probe.ID); return err })
		if len(plan) != 1 || plan[0].To == "" {
			t.Fatalf("probe plan %+v", plan)
		}
		return plan[0].To
	}
	target := targetOf(slots["S01E01"])  // Lab Show (2025)/Season 1/... .mkv
	target2 := targetOf(slots["S01E02"]) // its folder written "season 1" below
	upper := put(strings.TrimSuffix(target, ".mkv")+".MKV", slots["S01E01"])
	folder := put(strings.Replace(target2, "Season 1/", "season 1/", 1), slots["S01E02"])
	plan, err := library.PlanRename(ctx, s.Read(), []int64{id})
	if err != nil || len(plan) != 2 {
		t.Fatalf("plan %+v %v", plan, err)
	}
	for _, st := range plan {
		if st.Skip != "" || (st.FileID == upper.ID && st.To != target) || (st.FileID == folder.ID && st.To != target2) {
			t.Fatalf("step %+v", st)
		}
	}
	if n, err := svc.ApplyRename(ctx, plan, 0, domain.System); err != nil || n != 2 {
		t.Fatalf("renamed %d %v", n, err)
	}
	if f := fileRow(t, s, upper.ID); f.RelativePath != target {
		t.Fatalf("db %q", f.RelativePath)
	}
	if f := fileRow(t, s, folder.ID); f.RelativePath != target2 {
		t.Fatalf("db %q", f.RelativePath)
	}
	if b, err := os.ReadFile(filepath.Join(lf.Path, filepath.FromSlash(target2))); err != nil || !strings.Contains(string(b), "season 1/") {
		t.Fatalf("folder case-only: %q %v", b, err)
	}
	if !library.ExactName(filepath.Join(lf.Path, filepath.FromSlash(target))) {
		t.Fatal("the file is not on disk under its new name")
	}
	if b, err := os.ReadFile(filepath.Join(lf.Path, filepath.FromSlash(target))); err != nil || !strings.Contains(string(b), ".MKV") {
		t.Fatalf("content %q %v", b, err)
	}
	if again, _ := library.PlanRename(ctx, s.Read(), []int64{id}); len(again) != 0 {
		t.Fatalf("still to rename: %+v", again)
	}

	// A season folder that is a link out of the Library folder.
	out := t.TempDir()
	writeFile(t, filepath.Join(out, "S02.mkv"), "outside")
	if err := os.Symlink(out, filepath.Join(lf.Path, "Lab Show (2025)", "season 2")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	linked := put("Lab Show (2025)/season 2/S02.mkv", slots["S02E01"])
	plan, _ = library.PlanRename(ctx, s.Read(), []int64{id})
	for _, st := range plan {
		if st.FileID == linked.ID && st.Skip == "" {
			t.Fatalf("a file behind a link out of the Library folder would be renamed: %+v", st)
		}
	}
	if n, _ := svc.ApplyRename(ctx, plan, 0, domain.System); n != 0 {
		t.Fatalf("renamed %d through a link", n)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "S02.mkv")); string(b) != "video:Lab Show (2025)/season 2/S02.mkv" {
		t.Fatalf("outside file %q", b)
	}
}

// 5.2 review: a mount that is gone but leaves something in the Library
// folder (a leftover folder), or a pool that lost one of its disks, must
// not make a library look missing; a few files gone are still missing.
func TestRescanPartlyGoneMount(t *testing.T) {
	svc, s, lf, _ := showDeleteWorld(t)
	ctx := context.Background()
	show := filepath.Join(lf.Path, "Lab Show (2025)")
	for i := 3; i <= 22; i++ {
		writeFile(t, filepath.Join(show, "Season 1", fmt.Sprintf("Lab.Show.S01E%02d.720p.mkv", i)), "x")
	}
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	var known int
	_ = s.Read().QueryRow("SELECT count(*) FROM files WHERE state = 'present'").Scan(&known)
	if known < 20 {
		t.Fatalf("only %d files known", known)
	}
	// Two files gone: they are missing.
	for _, n := range []int{3, 4} {
		_ = os.Rename(filepath.Join(show, "Season 1", fmt.Sprintf("Lab.Show.S01E%02d.720p.mkv", n)), filepath.Join(t.TempDir(), "away.mkv"))
	}
	if r, _ := svc.Rescan(ctx, domain.System); r.Missing != 2 || r.Unreachable != 0 {
		t.Fatalf("two gone %+v", r)
	}
	// More than half gone at once (a pool lost a disk): none missing.
	for i := 5; i <= 22; i++ {
		_ = os.Remove(filepath.Join(show, "Season 1", fmt.Sprintf("Lab.Show.S01E%02d.720p.mkv", i)))
	}
	if r, _ := svc.Rescan(ctx, domain.System); r.Missing != 0 || r.Unreachable != 1 {
		t.Fatalf("pool lost a disk %+v", r)
	}
	// The mount is gone but a leftover folder stays: none missing.
	_ = os.RemoveAll(show)
	if r, _ := svc.Rescan(ctx, domain.System); r.Missing != 0 || r.Unreachable != 1 {
		t.Fatalf("leftover folder %+v", r)
	}
	var missing int
	_ = s.Read().QueryRow("SELECT count(*) FROM files WHERE state = 'missing'").Scan(&missing)
	if missing != 2 {
		t.Fatalf("%d files missing, want the 2 really gone", missing)
	}
}

// 5.2 review: the Events 5.2 adds (adopted, file held, episodes removed,
// found and missing files) carry no secret-looking field and no absolute
// path. The live stream sends the very Event stored (Service.event).
func TestShowEventsCarryNothingSecret(t *testing.T) {
	svc, s, lf, id := showDeleteWorld(t)
	ctx := context.Background()
	two := 2
	if _, err := svc.RemoveEpisodes(ctx, id, library.RemoveScope{Season: &two}, library.RemoveChoice{DeleteFiles: true}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	es, _, err := s.Events(ctx, database.EventFilter{})
	if err != nil || len(es) < 3 {
		t.Fatalf("events %d %v", len(es), err)
	}
	secret := regexp.MustCompile(`(?i)"[^"]*(secret|token|password|api_?key|smtp|webhook)[^"]*"\s*:`)
	root, _ := json.Marshal(lf.Path)
	rootSlash, _ := json.Marshal(filepath.ToSlash(lf.Path))
	kinds := map[string]bool{}
	for _, e := range es {
		kinds[e.Kind] = true
		b, _ := json.Marshal(e)
		if secret.Match(b) {
			t.Errorf("%s carries a secret-looking field: %s", e.Kind, b)
		}
		for _, r := range [][]byte{root, rootSlash} {
			if len(r) > 2 && regexp.MustCompile(regexp.QuoteMeta(string(r[1:len(r)-1]))).Match(b) {
				t.Errorf("%s carries the Library folder's absolute path: %s", e.Kind, b)
			}
		}
	}
	for _, k := range []string{domain.EventAdopted, domain.EventFileHeld, domain.EventEpisodesRemoved} {
		if !kinds[k] {
			t.Errorf("no %s Event to check (%v)", k, kinds)
		}
	}
}
