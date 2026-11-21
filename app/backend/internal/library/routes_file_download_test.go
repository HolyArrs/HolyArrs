package library

import (
	"context"
	"database/sql"
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
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// downloadRig is a movie in a Library folder and a mux serving the
// download route.
type downloadRig struct {
	t    *testing.T
	s    *database.Store
	m    *http.ServeMux
	root string
	sl   mediafiles.Slot
	dir  string
}

func newDownloadRig(t *testing.T) *downloadRig {
	t.Helper()
	s := open(t)
	lf := addFolder(t, s)
	id, err := AddMovie(context.Background(), s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	m := http.NewServeMux()
	(&FileDownloadHandlers{Store: s}).Routes(m)
	return &downloadRig{t: t, s: s, m: m, root: lf.Path, sl: slotsOf(t, s, id)[0], dir: versionsOf(t, s, id)[0].FolderName}
}

// add writes body under the movie's folder as name and records the file
// with role and state.
func (g *downloadRig) add(name, body, role, state string) int64 {
	g.t.Helper()
	rel := g.dir + "/" + name
	writeFile(g.t, filepath.Join(g.root, filepath.FromSlash(rel)), body)
	f := &mediafiles.File{SlotIDs: []int64{g.sl.ID}, VersionID: g.sl.VersionID, RelativePath: rel, Size: int64(len(body)), AddedAt: now,
		Languages: []string{}, MediaInfo: map[string]any{}, Origin: "imported", PlacedBy: "copy", Role: role, State: state}
	if err := g.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "files", f, domain.System)
		return err
	}); err != nil {
		g.t.Fatal(err)
	}
	return f.ID
}

func (g *downloadRig) get(method string, id int64, hdr map[string]string, caller *webserver.Caller) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1/files/"+strconv.FormatInt(id, 10)+"/download", nil)
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

func TestFileDownloadRanges(t *testing.T) {
	g := newDownloadRig(t)
	body := strings.Repeat("movie bytes ", 50)
	main := g.add("Dune.Part.Two.2024.mkv", body, "main", "present")

	w := g.get("GET", main, nil, nil)
	if w.Code != 200 || w.Body.String() != body || w.Header().Get("Content-Length") != strconv.Itoa(len(body)) ||
		w.Header().Get("Content-Type") != "video/x-matroska" ||
		w.Header().Get("Content-Disposition") != `attachment; filename=Dune.Part.Two.2024.mkv` {
		t.Fatalf("whole file: %d %v", w.Code, w.Header())
	}
	w = g.get("GET", main, map[string]string{"Range": "bytes=0-1"}, nil)
	if w.Code != 206 || w.Body.String() != "mo" || w.Header().Get("Content-Range") != "bytes 0-1/"+strconv.Itoa(len(body)) {
		t.Fatalf("range 0-1: %d %q %v", w.Code, w.Body.String(), w.Header())
	}
	w = g.get("GET", main, map[string]string{"Range": "bytes=100-199"}, nil)
	if w.Code != 206 || w.Body.String() != body[100:200] {
		t.Fatalf("range 100-199: %d %v", w.Code, w.Header())
	}
	w = g.get("HEAD", main, nil, nil)
	if w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("Content-Length") != strconv.Itoa(len(body)) {
		t.Fatalf("head: %d %v", w.Code, w.Header())
	}

	// Through the app's gzip layer, a whole video still goes as it is.
	gz := httptest.NewRequest("GET", "/api/v1/files/"+strconv.FormatInt(main, 10)+"/download", nil)
	gz.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	webserver.Compress(g.m).ServeHTTP(rec, gz)
	if rec.Code != 200 || rec.Header().Get("Content-Encoding") != "" || rec.Body.String() != body {
		t.Fatalf("gzip: %d %v", rec.Code, rec.Header())
	}

	// A name outside ASCII goes as filename*.
	odd := g.add("Diuna część druga.mp4", "x", "main", "present")
	w = g.get("GET", odd, nil, nil)
	if cd := w.Header().Get("Content-Disposition"); w.Code != 200 || cd != `attachment; filename*=utf-8''Diuna%20cz%C4%99%C5%9B%C4%87%20druga.mp4` ||
		w.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("non-ASCII: %d %q %v", w.Code, cd, w.Header())
	}
	if ct := g.get("GET", g.add("notes.nfo", "x", "main", "present"), nil, nil).Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("unknown type: %q", ct)
	}

	// Side files, missing files and files gone from disk are 404, with no
	// path or name in the answer.
	sub := g.add("Dune.Part.Two.2024.en.srt", "1\n", "subtitle", "present")
	missing := g.add("old.mkv", "x", "main", "missing")
	gone := g.add("vanished.mkv", "x", "main", "present")
	if err := os.Remove(filepath.Join(g.root, g.dir, "vanished.mkv")); err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]int64{"subtitle": sub, "missing": missing, "vanished": gone, "no such file": 999999} {
		w := g.get("GET", id, nil, nil)
		if w.Code != 404 || w.Header().Get("Content-Disposition") != "" || strings.Contains(w.Body.String(), g.root) || strings.Contains(w.Body.String(), "Dune") {
			t.Fatalf("%s: %d %v %s", name, w.Code, w.Header(), w.Body.String())
		}
	}
	if w := g.get("GET", 0, nil, nil); w.Code != 404 {
		t.Fatalf("id 0: %d", w.Code)
	}

	// A kid is 404 whatever she is ticked for; a grown-up with the tick gets it.
	kid := webserver.Caller{UserID: 3, Role: webserver.RoleMember, MaxRating: "PG", Perms: []webserver.Perm{webserver.PermSaveFiles}}
	if w := g.get("GET", main, nil, &kid); w.Code != 404 {
		t.Fatalf("kid: %d", w.Code)
	}
	adult := webserver.Caller{UserID: 2, Role: webserver.RoleMember, Perms: []webserver.Perm{webserver.PermSaveFiles}}
	if w := g.get("GET", main, nil, &adult); w.Code != 200 {
		t.Fatalf("member: %d", w.Code)
	}
}
