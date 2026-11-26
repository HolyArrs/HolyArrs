package library

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// saveRig is a movie in a Library folder and a mux serving the save route.
type saveRig struct {
	t     *testing.T
	s     *database.Store
	m     *http.ServeMux
	root  string
	title int64
	sl    mediafiles.Slot
	dir   string
}

func newSaveRig(t *testing.T) *saveRig {
	t.Helper()
	s := open(t)
	lf := addFolder(t, s)
	id, err := AddMovie(context.Background(), s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	m := http.NewServeMux()
	(&FileSaveHandlers{Store: s}).Routes(m)
	return &saveRig{t: t, s: s, m: m, root: lf.Path, title: id, sl: slotsOf(t, s, id)[0], dir: versionsOf(t, s, id)[0].FolderName}
}

// record adds a file row at rel (relative to the Library folder) with role
// and state; the file on disk is the caller's business.
func (g *saveRig) record(rel, role, state string) int64 {
	g.t.Helper()
	f := &mediafiles.File{SlotIDs: []int64{g.sl.ID}, VersionID: g.sl.VersionID, RelativePath: rel, Size: 1, AddedAt: now,
		Languages: []string{}, MediaInfo: map[string]any{}, Origin: "imported", PlacedBy: "copy", Role: role, State: state}
	if err := g.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "files", f, domain.System)
		return err
	}); err != nil {
		g.t.Fatal(err)
	}
	return f.ID
}

// add writes body under the movie's folder as name and records it.
func (g *saveRig) add(name, body, role, state string) int64 {
	g.t.Helper()
	rel := g.dir + "/" + name
	writeFile(g.t, filepath.Join(g.root, filepath.FromSlash(rel)), body)
	return g.record(rel, role, state)
}

func (g *saveRig) path(title, file int64) string {
	return "/api/v1/titles/" + strconv.FormatInt(title, 10) + "/files/" + strconv.FormatInt(file, 10) + "/save"
}

func (g *saveRig) get(method string, file int64, hdr map[string]string, caller *webserver.Caller) *httptest.ResponseRecorder {
	return g.getAt(method, g.path(g.title, file), hdr, caller)
}

func (g *saveRig) getAt(method, p string, hdr map[string]string, caller *webserver.Caller) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, p, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if caller != nil {
		req = req.WithContext(webserver.ContextWith(req.Context(), *caller))
	}
	w := httptest.NewRecorder()
	g.m.ServeHTTP(w, req)
	return w
}

func (g *saveRig) saved() []database.Event {
	g.t.Helper()
	evs, _, err := g.s.Events(context.Background(), database.EventFilter{Kinds: []string{domain.EventFileSaved}})
	if err != nil {
		g.t.Fatal(err)
	}
	return evs
}

// notFound fails unless w is a plain 404 that names no path and no file.
func (g *saveRig) notFound(what string, w *httptest.ResponseRecorder) {
	g.t.Helper()
	if w.Code != 404 || w.Header().Get("Content-Disposition") != "" || strings.Contains(w.Body.String(), g.root) || strings.Contains(w.Body.String(), "Dune") {
		g.t.Fatalf("%s: %d %v %s", what, w.Code, w.Header(), w.Body.String())
	}
}

func TestFileSaveSendsTheFileAsItIs(t *testing.T) {
	g := newSaveRig(t)
	body := strings.Repeat("movie bytes ", 50)
	main := g.add("Dune.Part.Two.2024.mkv", body, "main", "present")
	n := strconv.Itoa(len(body))

	w := g.get("GET", main, nil, nil)
	if w.Code != 200 || w.Body.String() != body || w.Header().Get("Content-Length") != n ||
		w.Header().Get("Content-Type") != "video/x-matroska" || w.Header().Get("Accept-Ranges") != "bytes" ||
		w.Header().Get("Content-Disposition") != `attachment; filename=Dune.Part.Two.2024.mkv` ||
		w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("ETag") == "" {
		t.Fatalf("whole file: %d %v", w.Code, w.Header())
	}
	etag, mod := w.Header().Get("ETag"), w.Header().Get("Last-Modified")

	// A broken save resumes with a range, guarded by If-Range.
	w = g.get("GET", main, map[string]string{"Range": "bytes=100-199", "If-Range": etag}, nil)
	if w.Code != 206 || w.Body.String() != body[100:200] || w.Header().Get("Content-Range") != "bytes 100-199/"+n {
		t.Fatalf("range: %d %v", w.Code, w.Header())
	}
	w = g.get("GET", main, map[string]string{"Range": "bytes=100-", "If-Range": mod}, nil)
	if w.Code != 206 || w.Body.String() != body[100:] {
		t.Fatalf("range by date: %d %v", w.Code, w.Header())
	}
	// A changed file (another tag) sends the whole file again.
	w = g.get("GET", main, map[string]string{"Range": "bytes=100-199", "If-Range": `"something-else"`}, nil)
	if w.Code != 200 || w.Body.String() != body {
		t.Fatalf("stale If-Range: %d %v", w.Code, w.Header())
	}
	w = g.get("HEAD", main, nil, nil)
	if w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("Content-Length") != n || w.Header().Get("Content-Disposition") == "" {
		t.Fatalf("head: %d %v", w.Code, w.Header())
	}

	// Through the app's gzip layer the file still goes as it is.
	gz := httptest.NewRequest("GET", g.path(g.title, main), nil)
	gz.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	webserver.Compress(g.m).ServeHTTP(rec, gz)
	if rec.Code != 200 || rec.Header().Get("Content-Encoding") != "" || rec.Body.String() != body {
		t.Fatalf("gzip: %d %v", rec.Code, rec.Header())
	}
}

func TestFileSaveNamesTheFileSafely(t *testing.T) {
	g := newSaveRig(t)
	odd := g.add("Diuna część druga.mp4", "x", "main", "present")
	w := g.get("GET", odd, nil, nil)
	if cd := w.Header().Get("Content-Disposition"); w.Code != 200 || cd != `attachment; filename*=utf-8''Diuna%20cz%C4%99%C5%9B%C4%87%20druga.mp4` ||
		w.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("non-ASCII: %d %q %v", w.Code, cd, w.Header())
	}
	quoted := g.add(`Say "hi"; now.mkv`, "x", "main", "present")
	if cd := g.get("GET", quoted, nil, nil).Header().Get("Content-Disposition"); cd != `attachment; filename="Say \"hi\"; now.mkv"` {
		t.Fatalf("quotes: %q", cd)
	}
	if runtime.GOOS != "windows" {
		// A line break in a name on disk can't reach the header.
		broken := g.add("Bad\r\nSet-Cookie: x=1.mkv", "x", "main", "present")
		w := g.get("GET", broken, nil, nil)
		if cd := w.Header().Get("Content-Disposition"); w.Code != 200 || strings.ContainsAny(cd, "\r\n") || w.Header().Get("Set-Cookie") != "" ||
			cd != `attachment; filename="BadSet-Cookie: x=1.mkv"` {
			t.Fatalf("line break: %d %q", w.Code, cd)
		}
	}
	if ct := g.get("GET", g.add("notes.nfo", "x", "main", "present"), nil, nil).Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("unknown type: %q", ct)
	}
}

func TestFileSaveRefusesWhatIsNotAPresentMainFile(t *testing.T) {
	g := newSaveRig(t)
	main := g.add("Dune.Part.Two.2024.mkv", "x", "main", "present")
	sub := g.add("Dune.Part.Two.2024.en.srt", "1\n", "subtitle", "present")
	missing := g.add("old.mkv", "x", "main", "missing")
	gone := g.add("vanished.mkv", "x", "main", "present")
	if err := os.Remove(filepath.Join(g.root, g.dir, "vanished.mkv")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(g.root, g.dir, "Extras"), 0o755); err != nil {
		t.Fatal(err)
	}
	folder := g.record(g.dir+"/Extras", "main", "present")
	climb := g.record("../outside.mkv", "main", "present")
	writeFile(t, filepath.Join(filepath.Dir(g.root), "outside.mkv"), "secret")
	for name, id := range map[string]int64{"subtitle": sub, "missing": missing, "vanished": gone, "folder": folder, "climb": climb, "no such file": 999999} {
		g.notFound(name, g.get("GET", id, nil, nil))
	}
	for _, p := range []string{g.path(g.title, 0), g.path(0, main), "/api/v1/titles/x/files/1/save", "/api/v1/titles/1/files/-1/save"} {
		g.notFound(p, g.getAt("GET", p, nil, nil))
	}

	// A file of another title, asked for under this one, is not there.
	other, err := AddMovie(context.Background(), g.s, future, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	g.notFound("other title", g.getAt("GET", g.path(other, main), nil, nil))

	// A removed title's files are gone with it.
	if err := g.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET state = 'removed' WHERE id = ?", g.title)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	g.notFound("removed title", g.get("GET", main, nil, nil))
	if n := len(g.saved()); n != 0 {
		t.Fatalf("refusals wrote %d events", n)
	}
}

func TestFileSaveRefusesLinksOutOfTheLibraryFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need extra rights on Windows")
	}
	g := newSaveRig(t)
	outside := filepath.Join(t.TempDir(), "secret.mkv")
	writeFile(t, outside, "secret")
	dir := filepath.Join(g.root, g.dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link.mkv")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(dir, "away")); err != nil {
		t.Fatal(err)
	}
	g.notFound("file link out", g.get("GET", g.record(g.dir+"/link.mkv", "main", "present"), nil, nil))
	g.notFound("folder link out", g.get("GET", g.record(g.dir+"/away/secret.mkv", "main", "present"), nil, nil))

	// A link that stays inside the Library folder is followed.
	writeFile(t, filepath.Join(dir, "real.mkv"), "inside")
	if err := os.Symlink("real.mkv", filepath.Join(dir, "near.mkv")); err != nil {
		t.Fatal(err)
	}
	if w := g.get("GET", g.record(g.dir+"/near.mkv", "main", "present"), nil, nil); w.Code != 200 || w.Body.String() != "inside" {
		t.Fatalf("link inside: %d %s", w.Code, w.Body.String())
	}
}

func TestFileSaveNeverForAKid(t *testing.T) {
	g := newSaveRig(t)
	main := g.add("Dune.Part.Two.2024.mkv", "movie", "main", "present")
	kid := webserver.Caller{UserID: 3, Via: "session", Role: webserver.RoleMember, MaxRating: "R", Perms: []webserver.Perm{webserver.PermAsk, webserver.PermSaveFiles}}
	g.notFound("kid", g.get("GET", main, nil, &kid))
	adult := webserver.Caller{UserID: 2, Via: "session", Role: webserver.RoleMember, Perms: []webserver.Perm{webserver.PermSaveFiles}}
	if w := g.get("GET", main, nil, &adult); w.Code != 200 || w.Body.String() != "movie" {
		t.Fatalf("member: %d", w.Code)
	}
}

func TestFileSaveWritesOneEventPerSave(t *testing.T) {
	g := newSaveRig(t)
	main := g.add("Dune.Part.Two.2024.mkv", strings.Repeat("x", 100), "main", "present")
	who := webserver.Caller{UserID: 2, Name: "ana", Via: "session", Role: webserver.RoleMember, Perms: []webserver.Perm{webserver.PermSaveFiles}}
	g.get("GET", main, nil, &who)
	g.get("HEAD", main, nil, &who)
	g.get("GET", main, map[string]string{"Range": "bytes=50-"}, &who)
	g.get("GET", main, map[string]string{"Range": "bytes=0-"}, &who)
	evs := g.saved()
	if len(evs) != 2 {
		t.Fatalf("want 2 events (a whole save and a save from the start), got %d: %+v", len(evs), evs)
	}
	e := evs[0]
	if e.Actor != (database.Actor{Kind: "user", ID: 2}) || e.TitleID == nil || *e.TitleID != g.title ||
		strings.Contains(e.Summary, "Dune") || strings.Contains(e.Summary, g.root) || time.Since(e.At) > time.Hour {
		t.Fatalf("event: %+v", e)
	}
	for k, v := range e.Data {
		if s, ok := v.(string); ok && (strings.Contains(s, "Dune") || strings.Contains(s, "/")) {
			t.Fatalf("event data %s holds a name or path: %q", k, s)
		}
	}
	if !strings.Contains(strings.Join(database.AuditEventKinds, ","), domain.EventFileSaved) {
		t.Fatal("a saved file is on the audit trail, read only by those who may read it")
	}
}
