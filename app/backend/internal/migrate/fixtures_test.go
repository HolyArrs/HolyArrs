package migrate

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Fixtures are written from the public API docs only:
// the API paths and fields, and the backup files, tables and columns they
// describe. Every secret in them starts with "S3CR3T" so one search proves
// none reaches a Snapshot.

const secretMark = "S3CR3T"

// fakeAPI serves routes (path to answer) and refuses any request whose
// header (or apikey query, for SABnzbd) does not carry key. An answer may
// be a string (sent as it is), a func (called with the request) or any
// value (sent as JSON).
func fakeAPI(t *testing.T, header, key string, routes map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ans, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if header != "" && r.Header.Get(header) != key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if f, ok := ans.(func(*http.Request) any); ok {
			ans = f(r)
		}
		if s, ok := ans.(string); ok {
			_, _ = w.Write([]byte(s))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ans)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// makeDB writes a SQLite database at path from statements.
func makeDB(t *testing.T, path string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// makeZip writes a zip of files (name to content) at path.
func makeZip(t *testing.T, path string, files map[string][]byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// dbBytes builds a database and returns its bytes (for zips).
func dbBytes(t *testing.T, stmts ...string) []byte {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.db")
	makeDB(t, p, stmts...)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// read reads src and fails the test on an error.
func read(t *testing.T, src Source) *Snapshot {
	t.Helper()
	s, err := Read(context.Background(), src)
	if err != nil {
		t.Fatalf("read %s via %s: %v", src.Kind, src.Via(), err)
	}
	noSecrets(t, s)
	return s
}

// noSecrets fails when the Snapshot's JSON holds a fixture secret.
func noSecrets(t *testing.T, s *Snapshot) {
	t.Helper()
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), secretMark) {
		i := strings.Index(string(b), secretMark)
		t.Fatalf("%s snapshot holds a secret near %q", s.Kind, string(b[max(0, i-80):min(len(b), i+20)]))
	}
}

// api is a Source for a fake API on the local machine.
func api(kind Kind, url, key string) Source {
	return Source{Kind: kind, URL: url, Key: key, AllowLocal: true}
}

func file(kind Kind, path string) Source { return Source{Kind: kind, Path: path} }

// findTitle returns the title with that name.
func findTitle(t *testing.T, s *Snapshot, name string) Title {
	t.Helper()
	for _, x := range s.Titles {
		if x.Name == name {
			return x
		}
	}
	t.Fatalf("no title %q in %+v", name, s.Titles)
	return Title{}
}

func hasNote(s *Snapshot, part string) bool {
	for _, n := range s.Notes {
		if strings.Contains(n, part) {
			return true
		}
	}
	return false
}

func hasReentry(s *Snapshot, part string) bool {
	for _, n := range s.NeedsReentry {
		if strings.Contains(n, part) {
			return true
		}
	}
	return false
}

func mkdir(t *testing.T, p string) string {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}
