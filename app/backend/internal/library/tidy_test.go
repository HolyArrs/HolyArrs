package library

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func readBody(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

// The preview lists each loose movie with its new place and its subtitle
// and .nfo, the ones with no year apart, and moves nothing.
func TestPlanTidyPreview(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	lib := lf.Path
	writeFile(t, filepath.Join(lib, "Horror", "Alien (1979).mkv"), "a")
	writeFile(t, filepath.Join(lib, "Horror", "Alien (1979).en.srt"), "sub")
	writeFile(t, filepath.Join(lib, "Horror", "Alien (1979).nfo"), "nfo")
	writeFile(t, filepath.Join(lib, "Horror", "Halloween.1978.1080p.BluRay.x264.mkv"), "h")
	writeFile(t, filepath.Join(lib, "Horror", "some home video.mkv"), "x")
	writeFile(t, filepath.Join(lib, "Heat (1995)", "heat.mkv"), "kept")
	p, err := PlanTidy(context.Background(), s.Read(), lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]TidyMove{}
	for _, m := range p.Moves {
		got[m.From] = m
	}
	if len(p.Moves) != 2 || len(p.Blocked) != 0 {
		t.Fatalf("plan %+v", p)
	}
	a := got["Horror/Alien (1979).mkv"]
	if a.To != "Horror/Alien (1979)/Alien (1979).mkv" || len(a.Files) != 2 {
		t.Fatalf("alien %+v", a)
	}
	want := map[string]string{"Horror/Alien (1979).en.srt": "Horror/Alien (1979)/Alien (1979).en.srt", "Horror/Alien (1979).nfo": "Horror/Alien (1979)/Alien (1979).nfo"}
	for _, f := range a.Files {
		if want[f.From] != f.To {
			t.Fatalf("file %+v", f)
		}
	}
	if h := got["Horror/Halloween.1978.1080p.BluRay.x264.mkv"]; h.To != "Horror/Halloween (1978)/Halloween (1978).mkv" {
		t.Fatalf("halloween %+v", h)
	}
	if len(p.Unmatched) != 1 || p.Unmatched[0] != "Horror/some home video.mkv" {
		t.Fatalf("unmatched %q", p.Unmatched)
	}
	if p.Hash == "" || !exists(filepath.Join(lib, "Horror", "Alien (1979).mkv")) || exists(filepath.Join(lib, "Horror", "Alien (1979)")) {
		t.Fatal("the preview changed the disk")
	}
}

// A tidy moves the files and its extras; Undo puts every one back and
// removes the folders it made.
func TestApplyAndUndoTidy(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	lib := lf.Path
	writeFile(t, filepath.Join(lib, "Alien (1979).mkv"), "a")
	writeFile(t, filepath.Join(lib, "Alien (1979).en.srt"), "sub")
	writeFile(t, filepath.Join(lib, "Heat (1995).mkv"), "h")
	ctx := context.Background()
	p, _ := PlanTidy(ctx, s.Read(), lf.ID)
	res, err := ApplyTidy(ctx, s, lf.ID, p.Hash, domain.System)
	if err != nil || res.Moved != 3 || len(res.Failed) != 0 {
		t.Fatalf("apply %+v %v", res, err)
	}
	if readBody(t, filepath.Join(lib, "Alien (1979)", "Alien (1979).mkv")) != "a" ||
		readBody(t, filepath.Join(lib, "Alien (1979)", "Alien (1979).en.srt")) != "sub" ||
		readBody(t, filepath.Join(lib, "Heat (1995)", "Heat (1995).mkv")) != "h" || exists(filepath.Join(lib, "Alien (1979).mkv")) {
		t.Fatal("files not moved")
	}
	// Now each movie has its own folder: nothing loose is left.
	if p2, _ := PlanTidy(ctx, s.Read(), lf.ID); len(p2.Moves) != 0 || p2.UndoFiles != 3 {
		t.Fatalf("after %+v", p2)
	}
	if ap, _ := PlanAdopt(ctx, s.Read(), adoptTMDB, lf.ID); ap.LooseMovies != 0 || len(ap.Folders) != 2 {
		t.Fatalf("scan after tidy %+v", ap)
	}
	res, err = UndoTidy(ctx, s, lf.ID, domain.System)
	if err != nil || res.Moved != 3 {
		t.Fatalf("undo %+v %v", res, err)
	}
	if readBody(t, filepath.Join(lib, "Alien (1979).mkv")) != "a" || readBody(t, filepath.Join(lib, "Alien (1979).en.srt")) != "sub" ||
		exists(filepath.Join(lib, "Horror", "Alien (1979)")) || exists(filepath.Join(lib, "Heat (1995)")) {
		t.Fatal("undo did not put things back")
	}
	if _, err := UndoTidy(ctx, s, lf.ID, domain.System); err != ErrNothingToUndo {
		t.Fatalf("second undo %v", err)
	}
}

// Nothing moves without the preview's hash, or when the folder changed.
func TestApplyTidyNeedsThePreview(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	writeFile(t, filepath.Join(lf.Path, "Alien (1979).mkv"), "a")
	writeFile(t, filepath.Join(lf.Path, "Heat (1995).mkv"), "h")
	ctx := context.Background()
	p, _ := PlanTidy(ctx, s.Read(), lf.ID)
	if _, err := ApplyTidy(ctx, s, lf.ID, "", domain.System); err != ErrTidyChanged {
		t.Fatalf("no hash %v", err)
	}
	writeFile(t, filepath.Join(lf.Path, "Ronin (1998).mkv"), "r")
	if _, err := ApplyTidy(ctx, s, lf.ID, p.Hash, domain.System); err != ErrTidyChanged {
		t.Fatalf("changed %v", err)
	}
	if !exists(filepath.Join(lf.Path, "Alien (1979).mkv")) {
		t.Fatal("moved without a matching preview")
	}
}

// A file already at the new name, or a folder that already holds a
// movie, blocks that movie; a second cut keeps its own name.
func TestTidyConflicts(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	lib := lf.Path
	writeFile(t, filepath.Join(lib, "Drama", "Heat (1995).mkv"), "loose")
	writeFile(t, filepath.Join(lib, "Drama", "Ronin (1998).mkv"), "r")
	writeFile(t, filepath.Join(lib, "Drama", "Heat (1995)", "Heat (1995).mkv"), "there")
	writeFile(t, filepath.Join(lib, "Speed (1994).mkv"), "s1")
	writeFile(t, filepath.Join(lib, "Speed (1994) - Directors Cut.mkv"), "s2")
	writeFile(t, filepath.Join(lib, "Alien (1979).mkv"), "a")
	writeFile(t, filepath.Join(lib, "Alien (1979)", "old.mkv"), "other")
	ctx := context.Background()
	p, _ := PlanTidy(ctx, s.Read(), lf.ID)
	blocked := map[string]string{}
	for _, b := range p.Blocked {
		blocked[b.From] = b.Problem
	}
	if !strings.Contains(blocked["Drama/Heat (1995).mkv"], "already exists") || !strings.Contains(blocked["Alien (1979).mkv"], "already holds a movie") {
		t.Fatalf("blocked %v", blocked)
	}
	tos := map[string]string{}
	for _, m := range p.Moves {
		tos[m.From] = m.To
	}
	if tos["Speed (1994).mkv"] != "Speed (1994)/Speed (1994).mkv" || tos["Speed (1994) - Directors Cut.mkv"] != "Speed (1994)/Speed (1994) - Directors Cut.mkv" {
		t.Fatalf("moves %v", tos)
	}
	res, err := ApplyTidy(ctx, s, lf.ID, p.Hash, domain.System)
	if err != nil || len(res.Failed) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	if readBody(t, filepath.Join(lib, "Drama", "Heat (1995)", "Heat (1995).mkv")) != "there" || readBody(t, filepath.Join(lib, "Drama", "Heat (1995).mkv")) != "loose" {
		t.Fatal("a blocked movie was touched")
	}
	// Undo never replaces a file that came back in the old place.
	writeFile(t, filepath.Join(lib, "Drama", "Ronin (1998).mkv"), "new")
	res, err = UndoTidy(ctx, s, lf.ID, domain.System)
	if err != nil || len(res.Failed) != 1 || readBody(t, filepath.Join(lib, "Drama", "Ronin (1998).mkv")) != "new" {
		t.Fatalf("undo %+v %v", res, err)
	}
	if p2, _ := PlanTidy(ctx, s.Read(), lf.ID); p2.UndoFiles != 1 {
		t.Fatalf("left to undo %d", p2.UndoFiles)
	}
}

// Across disks the move copies and then removes, both ways.
func TestTidyAcrossDisks(t *testing.T) {
	crossDevice(t)
	s := open(t)
	lf := addFolder(t, s)
	writeFile(t, filepath.Join(lf.Path, "Alien (1979).mkv"), "alien")
	writeFile(t, filepath.Join(lf.Path, "Heat (1995).mkv"), "heat")
	ctx := context.Background()
	p, _ := PlanTidy(ctx, s.Read(), lf.ID)
	if res, err := ApplyTidy(ctx, s, lf.ID, p.Hash, domain.System); err != nil || res.Moved != 2 {
		t.Fatalf("%+v %v", res, err)
	}
	if readBody(t, filepath.Join(lf.Path, "Alien (1979)", "Alien (1979).mkv")) != "alien" || exists(filepath.Join(lf.Path, "Alien (1979).mkv")) {
		t.Fatal("copy across disks failed")
	}
	if res, err := UndoTidy(ctx, s, lf.ID, domain.System); err != nil || res.Moved != 2 {
		t.Fatalf("%+v %v", res, err)
	}
	if readBody(t, filepath.Join(lf.Path, "Heat (1995).mkv")) != "heat" {
		t.Fatal("undo across disks failed")
	}
}

// The three routes: preview, tidy and undo.
func TestTidyRoutes(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	writeFile(t, filepath.Join(lf.Path, "Alien (1979).mkv"), "a")
	writeFile(t, filepath.Join(lf.Path, "Heat (1995).mkv"), "h")
	h := &FolderHandlers{Store: s, DownloadPaths: func(*http.Request) []string { return nil },
		Actor: func(*http.Request) database.Actor { return domain.System },
		Fail:  func(w http.ResponseWriter, err error) { webserver.Fail(w, http.StatusInternalServerError, err.Error()) }}
	m := http.NewServeMux()
	h.Routes(m)
	base := "/api/v1/library-folders/" + strconv.FormatInt(lf.ID, 10) + "/tidy"
	do := func(method, url, body string, want int) []byte {
		t.Helper()
		rec := httptest.NewRecorder()
		m.ServeHTTP(rec, httptest.NewRequest(method, url, strings.NewReader(body)))
		if rec.Code != want {
			t.Fatalf("%s %s: %d %s", method, url, rec.Code, rec.Body)
		}
		return rec.Body.Bytes()
	}
	var p TidyPlan
	if err := json.Unmarshal(do("GET", base, "", 200), &p); err != nil || len(p.Moves) != 2 {
		t.Fatalf("%+v %v", p, err)
	}
	do("POST", base, `{"hash":"nope"}`, http.StatusConflict)
	var res TidyResult
	_ = json.Unmarshal(do("POST", base, `{"hash":"`+p.Hash+`"}`, 200), &res)
	if res.Moved != 2 {
		t.Fatalf("%+v", res)
	}
	_ = json.Unmarshal(do("POST", base+"/undo", "", 200), &res)
	if res.Moved != 2 {
		t.Fatalf("%+v", res)
	}
	do("POST", base+"/undo", "", http.StatusConflict)
	do("GET", "/api/v1/library-folders/999/tidy", "", http.StatusNotFound)
}
