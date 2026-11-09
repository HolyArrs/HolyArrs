package library

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// putFile writes a file under the Library folder and records it as a File
// of the Title's main Version with the given origin.
func putFile(t *testing.T, s *database.Store, lf mediafiles.LibraryFolder, titleID int64, rel, release, origin string) mediafiles.File {
	t.Helper()
	ctx := context.Background()
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(rel)), "video:"+rel)
	vs, err := database.Select[mediafiles.Version](ctx, s.Read(), "versions", "title_id = ? AND is_main = 1", titleID)
	if err != nil || len(vs) != 1 {
		t.Fatalf("versions %v %v", vs, err)
	}
	sl := slotsOf(t, s, titleID)
	f := mediafiles.File{SlotIDs: []int64{sl[0].ID}, VersionID: vs[0].ID, RelativePath: rel, Size: 10, AddedAt: now, Languages: []string{},
		MediaInfo: map[string]any{}, ReleaseName: &release, Origin: origin, PlacedBy: "hardlink", Role: "main", State: "present"}
	if origin != "imported" {
		f.PlacedBy = "none"
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error { _, err := database.Insert(tx, "files", &f, domain.System); return err }); err != nil {
		t.Fatal(err)
	}
	return f
}

func renameWorld(t *testing.T) (*Service, *database.Store, mediafiles.LibraryFolder, int64) {
	t.Helper()
	s := open(t)
	lf := addFolder(t, s)
	id, err := AddMovie(context.Background(), s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return &Service{Store: s, Now: func() time.Time { return now }}, s, lf, id
}

func fileRow(t *testing.T, s *database.Store, id int64) mediafiles.File {
	t.Helper()
	var f mediafiles.File
	if err := database.Get(context.Background(), s.Read(), "files", id, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

// 5.1.1: what the preview shows is exactly what the rename does, and a
// file HolyArrs only found is never renamed.
func TestRenamePreviewMatchesResult(t *testing.T) {
	svc, s, lf, id := renameWorld(t)
	ctx := context.Background()
	imp := putFile(t, s, lf, id, "Dune - Part Two (2024)/Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS.mkv", "Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS", "imported")
	found := putFile(t, s, lf, id, "Dune - Part Two (2024)/extra cut.mkv", "Dune.Part.Two.2024.2160p.WEB-DL-X", "found")
	if err := mediafiles.PutNamingSettings(ctx, s, mediafiles.NamingSettings{FilePattern: "{title} ({year}) [{quality}] {group}", FolderPattern: mediafiles.DefaultFolderPattern,
		Colon: mediafiles.ColonSpaceDashSpace, Illegal: mediafiles.IllegalReplace, RenameOnImport: true}, domain.System); err != nil {
		t.Fatal(err)
	}
	preview, err := PlanRename(ctx, s.Read(), []int64{id})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]RenameStep{}
	for _, st := range preview {
		byID[st.FileID] = st
	}
	want := "Dune - Part Two (2024)/Dune - Part Two (2024) [1080p BluRay] HOLYARRS.mkv"
	if st := byID[imp.ID]; st.To != want || st.Skip != "" {
		t.Fatalf("imported step %+v", st)
	}
	if st := byID[found.ID]; st.Skip == "" || !strings.Contains(st.Skip, "found") || st.To != "" {
		t.Fatalf("found step %+v", st)
	}
	n, err := svc.ApplyRename(ctx, preview, 0, domain.System)
	if err != nil || n != 1 {
		t.Fatalf("renamed %d %v", n, err)
	}
	for _, st := range preview {
		f := fileRow(t, s, st.FileID)
		if st.Skip != "" {
			if f.RelativePath != st.From {
				t.Fatalf("skipped file moved: %+v", f)
			}
			continue
		}
		if f.RelativePath != st.To {
			t.Fatalf("db %q, preview said %q", f.RelativePath, st.To)
		}
		if _, err := os.Stat(filepath.Join(lf.Path, filepath.FromSlash(st.To))); err != nil {
			t.Fatalf("not on disk at the previewed name: %v", err)
		}
		if _, err := os.Stat(filepath.Join(lf.Path, filepath.FromSlash(st.From))); !os.IsNotExist(err) {
			t.Fatal("old name still there")
		}
	}
	if b, _ := os.ReadFile(filepath.Join(lf.Path, "Dune - Part Two (2024)", "extra cut.mkv")); string(b) == "" {
		t.Fatal("found file touched")
	}
	// A second preview has nothing left to change.
	again, _ := PlanRename(ctx, s.Read(), []int64{id})
	for _, st := range again {
		if st.Skip == "" {
			t.Fatalf("still to rename: %+v", st)
		}
	}
}

func TestRenamePlanSkipsClashesAndStrangers(t *testing.T) {
	_, s, lf, id := renameWorld(t)
	ctx := context.Background()
	a := putFile(t, s, lf, id, "Dune - Part Two (2024)/a.mkv", "Dune.Part.Two.2024.1080p.BluRay-HOLYARRS", "imported")
	b := putFile(t, s, lf, id, "Dune - Part Two (2024)/b.mkv", "Dune.Part.Two.2024.1080p.BluRay-HOLYARRS", "adopted")
	// Someone else's file already has the name a would get.
	plan, _ := PlanRename(ctx, s.Read(), []int64{id})
	steps := map[int64]RenameStep{}
	for _, st := range plan {
		steps[st.FileID] = st
	}
	if steps[a.ID].Skip != "" || !strings.Contains(steps[b.ID].Skip, "another file") {
		t.Fatalf("clash %+v", plan)
	}
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(steps[a.ID].To)), "stranger")
	plan, _ = PlanRename(ctx, s.Read(), []int64{id})
	for _, st := range plan {
		if st.FileID == a.ID && !strings.Contains(st.Skip, "already there") {
			t.Fatalf("stranger not seen %+v", st)
		}
	}
}

// A crash between moving the file and recording its new name, or before
// the move, is finished by the next run; nothing is lost or doubled.
func TestRenameCrashMidwayIsFinishedNextRun(t *testing.T) {
	for _, stage := range []string{"before_move", "after_move"} {
		t.Run(stage, func(t *testing.T) {
			svc, s, lf, id := renameWorld(t)
			ctx := context.Background()
			f := putFile(t, s, lf, id, "Dune - Part Two (2024)/x.mkv", "Dune.Part.Two.2024.1080p.BluRay-HOLYARRS", "imported")
			plan, _ := PlanRename(ctx, s.Read(), []int64{id})
			renameCrash = func(at string) error {
				if at == stage {
					return errors.New("power cut")
				}
				return nil
			}
			_, err := svc.ApplyRename(ctx, plan, 0, domain.System)
			renameCrash = nil
			if err == nil {
				t.Fatal("no crash")
			}
			if fileRow(t, s, f.ID).RelativePath != f.RelativePath {
				t.Fatal("the new name was recorded before the move finished")
			}
			n, err := svc.RecoverRenames(ctx, domain.System)
			if err != nil || n != 1 {
				t.Fatalf("recovered %d %v", n, err)
			}
			got := fileRow(t, s, f.ID)
			if got.RelativePath != plan[0].To {
				t.Fatalf("db %q want %q", got.RelativePath, plan[0].To)
			}
			if b, _ := os.ReadFile(filepath.Join(lf.Path, filepath.FromSlash(plan[0].To))); string(b) != "video:"+f.RelativePath {
				t.Fatalf("content %q", b)
			}
			if _, err := os.Stat(filepath.Join(lf.Path, filepath.FromSlash(f.RelativePath))); !os.IsNotExist(err) {
				t.Fatal("old file left behind")
			}
			if n, _ := svc.RecoverRenames(ctx, domain.System); n != 0 {
				t.Fatal("recovered twice")
			}
		})
	}
}

func TestRenameJobRuns(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	svc, _, _ := runQueue(t, s, nil)
	ctx := context.Background()
	id, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	f := putFile(t, s, lf, id, "Dune - Part Two (2024)/x.mkv", "Dune.Part.Two.2024.2160p.WEB-DL-HOLYARRS", "adopted")
	_, hash, _ := PlanRenameHashed(ctx, s.Read(), []int64{id})
	jobID, err := svc.Rename(ctx, []int64{id}, nil, hash, domain.System)
	if err != nil || jobID == 0 {
		t.Fatalf("job %d %v", jobID, err)
	}
	js := waitJobs(t, s, 1)
	if js[0].State != "done" || js[0].Result["renamed"] != float64(1) {
		t.Fatalf("job %+v %+v", js[0], js[0].Error)
	}
	if got := fileRow(t, s, f.ID).RelativePath; got != "Dune - Part Two (2024)/Dune - Part Two (2024) 2160p WEB.mkv" {
		t.Fatalf("path %q", got)
	}
}

// 5.1.7 critical 1 through the rename itself: .MKV to .mkv on this disk,
// with a crash between the two renames finished by recovery.
func TestRenameCaseOnlyKeepsTheFile(t *testing.T) {
	for _, crash := range []bool{false, true} {
		svc, s, lf, id := renameWorld(t)
		ctx := context.Background()
		rel := "Dune - Part Two (2024)/Dune - Part Two (2024) 2160p WEB.MKV"
		f := putFile(t, s, lf, id, rel, "Dune.Part.Two.2024.2160p.WEB-DL-HOLYARRS", "adopted")
		plan, err := PlanRename(ctx, s.Read(), []int64{id})
		if err != nil || len(plan) != 1 || plan[0].Skip != "" || plan[0].To != strings.TrimSuffix(rel, ".MKV")+".mkv" {
			t.Fatalf("plan %+v %v", plan, err)
		}
		if crash && !caseInsensitive(lf.Path) {
			continue // a case-sensitive disk: a plain move, no temp name
		}
		if crash {
			caseCrash = func() error { return errors.New("power cut") }
			_, err := svc.ApplyRename(ctx, plan, 0, domain.System)
			caseCrash = nil
			if err == nil {
				t.Fatal("no crash")
			}
			if n, err := svc.RecoverRenames(ctx, domain.System); err != nil || n != 1 {
				t.Fatalf("recovered %d %v", n, err)
			}
		} else if n, err := svc.ApplyRename(ctx, plan, 0, domain.System); err != nil || n != 1 {
			t.Fatalf("renamed %d %v", n, err)
		}
		if b, err := os.ReadFile(filepath.Join(lf.Path, filepath.FromSlash(plan[0].To))); string(b) != "video:"+rel {
			t.Fatalf("file lost: %q %v", b, err)
		}
		if got := fileRow(t, s, f.ID).RelativePath; got != plan[0].To {
			t.Fatalf("db %q", got)
		}
	}
}

// 5.1.7 important 2: a file or a new name that a symlink leads outside the
// Library folder is skipped in the plan, and checked again just before the
// move.
func TestRenameSkipsPathsThatLeaveThroughSymlinks(t *testing.T) {
	svc, s, lf, id := renameWorld(t)
	ctx := context.Background()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(lf.Path, "Dune - Part Two (2024)")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	f := putFile(t, s, lf, id, "Dune - Part Two (2024)/x.mkv", "Dune.Part.Two.2024.1080p.BluRay-HOLYARRS", "imported")
	plan, err := PlanRename(ctx, s.Read(), []int64{id})
	if err != nil || len(plan) != 1 || !strings.Contains(plan[0].Skip, "outside the Library folder") {
		t.Fatalf("plan %+v %v", plan, err)
	}
	// Planned while it was a real folder, swapped for a link before the move.
	_ = os.Remove(filepath.Join(lf.Path, "Dune - Part Two (2024)"))
	writeFile(t, filepath.Join(lf.Path, "Dune - Part Two (2024)", "x.mkv"), "video")
	plan, _ = PlanRename(ctx, s.Read(), []int64{id})
	if len(plan) != 1 || plan[0].Skip != "" {
		t.Fatalf("plan %+v", plan)
	}
	_ = os.RemoveAll(filepath.Join(lf.Path, "Dune - Part Two (2024)"))
	if err := os.Symlink(outside, filepath.Join(lf.Path, "Dune - Part Two (2024)")); err != nil {
		t.Fatal(err)
	}
	n, err := svc.ApplyRename(ctx, plan, 0, domain.System)
	if err == nil || n != 0 || !strings.Contains(plan[0].Result, "outside") {
		t.Fatalf("renamed %d %v %+v", n, err, plan[0])
	}
	if b, _ := os.ReadFile(filepath.Join(outside, "x.mkv")); string(b) != "video:"+f.RelativePath {
		t.Fatalf("outside file touched: %q", b)
	}
	var state string
	_ = s.Read().QueryRow("SELECT state FROM rename_steps ORDER BY id DESC LIMIT 1").Scan(&state)
	if state != "failed" {
		t.Fatalf("step %q", state)
	}
}

// 5.1.7 important 4: a target path longer than the platform allows is
// skipped with a reason.
func TestRenameSkipsTooLongPaths(t *testing.T) {
	_, s, lf, id := renameWorld(t)
	putFile(t, s, lf, id, "Dune - Part Two (2024)/x.mkv", "Dune.Part.Two.2024.1080p.BluRay-HOLYARRS", "imported")
	old := importer.MaxPathLen
	importer.MaxPathLen = len(lf.Path) + 30
	t.Cleanup(func() { importer.MaxPathLen = old })
	plan, _ := PlanRename(context.Background(), s.Read(), []int64{id})
	if len(plan) != 1 || !strings.Contains(plan[0].Skip, "too long") {
		t.Fatalf("plan %+v", plan)
	}
}

// 5.1.7 important 4: one failed step does not stop the rest; each step
// records its result and the failed row is marked failed.
func TestRenameOneFailureDoesNotStopTheBatch(t *testing.T) {
	svc, s, lf, id := renameWorld(t)
	ctx := context.Background()
	a := putFile(t, s, lf, id, "Dune - Part Two (2024)/a.mkv", "Dune.Part.Two.2024.1080p.BluRay-HOLYARRS", "imported")
	b := putFile(t, s, lf, id, "Dune - Part Two (2024)/b.mkv", "Dune.Part.Two.2024.2160p.WEB-DL-HOLYARRS", "imported")
	plan, _ := PlanRename(ctx, s.Read(), []int64{id})
	if len(plan) != 2 || plan[0].Skip != "" || plan[1].Skip != "" {
		t.Fatalf("plan %+v", plan)
	}
	_ = os.Remove(filepath.Join(lf.Path, filepath.FromSlash(a.RelativePath)))
	n, err := svc.ApplyRename(ctx, plan, 0, domain.System)
	if err == nil || n != 1 {
		t.Fatalf("renamed %d %v", n, err)
	}
	res := map[int64]string{}
	for _, st := range plan {
		res[st.FileID] = st.Result
	}
	if !strings.HasPrefix(res[a.ID], "failed") || res[b.ID] != "renamed" {
		t.Fatalf("results %v", res)
	}
	if fileRow(t, s, b.ID).RelativePath == b.RelativePath {
		t.Fatal("b not renamed")
	}
	var pending int
	_ = s.Read().QueryRow("SELECT COUNT(*) FROM rename_steps WHERE state = 'pending'").Scan(&pending)
	if pending != 0 {
		t.Fatalf("%d steps left pending", pending)
	}
}

// 5.1.7 important 4: a recovery that fails marks its row failed and does
// not block the next rename.
func TestRenameFailedRecoveryDoesNotBlock(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	svc, _, _ := runQueue(t, s, nil)
	ctx := context.Background()
	id, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	f := putFile(t, s, lf, id, "Dune - Part Two (2024)/x.mkv", "Dune.Part.Two.2024.2160p.WEB-DL-HOLYARRS", "adopted")
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO rename_steps (created_at, file_id, from_path, to_path, to_rel, state) VALUES (?, ?, ?, ?, ?, 'pending')`,
			database.FormatTime(now), f.ID, filepath.Join(lf.Path, "gone.mkv"), filepath.Join(lf.Path, "also gone.mkv"), "also gone.mkv")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	plan, hash, err := PlanRenameHashed(ctx, s.Read(), []int64{id})
	if err != nil || len(plan) != 1 {
		t.Fatalf("plan %+v %v", plan, err)
	}
	if _, err := svc.Rename(ctx, []int64{id}, nil, hash, domain.System); err != nil {
		t.Fatal(err)
	}
	js := waitJobs(t, s, 1)
	if js[0].State != "done" || js[0].Result["renamed"] != float64(1) {
		t.Fatalf("job %+v %+v", js[0], js[0].Error)
	}
	var state string
	_ = s.Read().QueryRow("SELECT state FROM rename_steps ORDER BY id LIMIT 1").Scan(&state)
	if state != "failed" {
		t.Fatalf("old step %q", state)
	}
}

// 5.1.7 important 3: the rename is bound to the preview it was asked from;
// a changed plan (a new file, other settings) fails and nothing moves.
func TestRenameRefusesAChangedPlan(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	svc, _, _ := runQueue(t, s, nil)
	ctx := context.Background()
	id, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	a := putFile(t, s, lf, id, "Dune - Part Two (2024)/a.mkv", "Dune.Part.Two.2024.2160p.WEB-DL-HOLYARRS", "adopted")
	_, hash, err := PlanRenameHashed(ctx, s.Read(), nil)
	if err != nil || hash == "" {
		t.Fatal(err)
	}
	// A file added after the preview.
	b := putFile(t, s, lf, id, "Dune - Part Two (2024)/b.mkv", "Dune.Part.Two.2024.1080p.BluRay-HOLYARRS", "adopted")
	if _, err := svc.Rename(ctx, nil, nil, hash, domain.System); err != nil {
		t.Fatal(err)
	}
	js := waitJobs(t, s, 1)
	if js[0].State != "failed" || js[0].Error == nil || !strings.Contains(js[0].Error.Message, "preview again") {
		t.Fatalf("job %+v %+v", js[0], js[0].Error)
	}
	if fileRow(t, s, a.ID).RelativePath != a.RelativePath || fileRow(t, s, b.ID).RelativePath != b.RelativePath {
		t.Fatal("renamed against a changed plan")
	}
}
