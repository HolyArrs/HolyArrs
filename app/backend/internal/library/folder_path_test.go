package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// pathWorld is a movie folder with two present files (each recorded at 10
// bytes) and a new, empty folder to point it at.
func pathWorld(t *testing.T) (*database.Store, mediafiles.LibraryFolder, []mediafiles.File, string) {
	t.Helper()
	_, s, lf, id := renameWorld(t)
	fs := []mediafiles.File{
		putFile(t, s, lf, id, "Dune - Part Two (2024)/Dune.mkv", "Dune.Part.Two.2024.1080p", "imported"),
		putFile(t, s, lf, id, "Dune - Part Two (2024)/extra.mkv", "Dune.Part.Two.2024.2160p", "found"),
	}
	return s, lf, fs, t.TempDir()
}

// putAt writes rel under root with n bytes.
func putAt(t *testing.T, root, rel string, n int) {
	t.Helper()
	writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), strings.Repeat("x", n))
}

func folderPath(t *testing.T, s *database.Store, id int64) string {
	t.Helper()
	var lf mediafiles.LibraryFolder
	if err := database.Get(context.Background(), s.Read(), "library_folders", id, &lf); err != nil {
		t.Fatal(err)
	}
	return lf.Path
}

// Every present file is at the new path with its size: the path is saved,
// the folder is checked there, and the File rows stay as they were.
func TestChangeFolderPathSaves(t *testing.T) {
	s, lf, fs, to := pathWorld(t)
	for _, f := range fs {
		putAt(t, to, f.RelativePath, 10)
	}
	got, err := ChangeFolderPath(context.Background(), s, lf.ID, to+string(filepath.Separator), nil, domain.System)
	if err != nil {
		t.Fatalf("change path: %v", err)
	}
	if got.Path != filepath.Clean(to) || folderPath(t, s, lf.ID) != filepath.Clean(to) {
		t.Fatalf("path %q, saved %q, want %q", got.Path, folderPath(t, s, lf.ID), to)
	}
	if got.Health != "ok" || got.HealthCheckedAt == nil || !got.HealthCheckedAt.After(*lf.HealthCheckedAt) && !got.HealthCheckedAt.Equal(*lf.HealthCheckedAt) {
		t.Fatalf("folder not checked at the new path: %+v", got)
	}
	for _, f := range fs {
		if r := fileRow(t, s, f.ID); r.RelativePath != f.RelativePath || r.State != "present" {
			t.Fatalf("file row changed: %+v", r)
		}
	}
}

// A file missing at the new path stops the change and is named; nothing is saved.
func TestChangeFolderPathMissingFile(t *testing.T) {
	s, lf, fs, to := pathWorld(t)
	putAt(t, to, fs[0].RelativePath, 10)
	_, err := ChangeFolderPath(context.Background(), s, lf.ID, to, nil, domain.System)
	var pe *PathFileError
	if !errors.As(err, &pe) || pe.RelativePath != fs[1].RelativePath || !pe.Missing {
		t.Fatalf("got %v, want the missing %s named", err, fs[1].RelativePath)
	}
	if !strings.Contains(err.Error(), fs[1].RelativePath) {
		t.Fatalf("message does not name the file: %v", err)
	}
	if p := folderPath(t, s, lf.ID); p != lf.Path {
		t.Fatalf("path saved anyway: %s", p)
	}
}

// A file of another size stops at that file, before any later one is read.
func TestChangeFolderPathSizeDiffers(t *testing.T) {
	s, lf, fs, to := pathWorld(t)
	putAt(t, to, fs[0].RelativePath, 11)
	_, err := ChangeFolderPath(context.Background(), s, lf.ID, to, nil, domain.System)
	var pe *PathFileError
	if !errors.As(err, &pe) || pe.RelativePath != fs[0].RelativePath || pe.Missing || pe.Want != 10 || pe.Got != 11 {
		t.Fatalf("got %#v (%v), want the first file named with 10 and 11 bytes", pe, err)
	}
	if p := folderPath(t, s, lf.ID); p != lf.Path {
		t.Fatalf("path saved anyway: %s", p)
	}
}

// A folder (not a file) where a file should be counts as missing.
func TestChangeFolderPathFolderInPlaceOfFile(t *testing.T) {
	s, lf, fs, to := pathWorld(t)
	if err := os.MkdirAll(filepath.Join(to, filepath.FromSlash(fs[0].RelativePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	var pe *PathFileError
	if _, err := ChangeFolderPath(context.Background(), s, lf.ID, to, nil, domain.System); !errors.As(err, &pe) || !pe.Missing {
		t.Fatalf("got %v, want missing", err)
	}
}

// The new path is checked like a new folder: full path, exists, not the
// config folder, not inside or around another Library folder.
func TestChangeFolderPathValidates(t *testing.T) {
	s, lf, _, _ := pathWorld(t)
	ctx := context.Background()
	other, err := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "show"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(other.Path, "inner")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path string
		want error
	}{
		{"", ErrFolderPath},
		{"relative/movies", ErrFolderPath},
		{s.Dir(), ErrFolderPath},
		{filepath.Join(t.TempDir(), "not-there"), ErrFolderAbsent},
		{other.Path, ErrFolderTaken},
		{inner, ErrFolderTaken},
	}
	for _, c := range cases {
		if _, err := ChangeFolderPath(ctx, s, lf.ID, c.path, nil, domain.System); !errors.Is(err, c.want) {
			t.Errorf("%q: got %v, want %v", c.path, err, c.want)
		}
	}
	if _, err := ChangeFolderPath(ctx, s, 9999, t.TempDir(), nil, domain.System); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("unknown folder: got %v", err)
	}
	if p := folderPath(t, s, lf.ID); p != lf.Path {
		t.Fatalf("path changed by a refused call: %s", p)
	}
}

// A folder with no present files moves without any file check; a file
// already gone or missing is not looked for.
func TestChangeFolderPathSkipsAbsentFiles(t *testing.T) {
	s, lf, fs, to := pathWorld(t)
	ctx := context.Background()
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE files SET state = 'missing' WHERE id = ?", fs[0].ID); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE files SET state = 'gone' WHERE id = ?", fs[1].ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ChangeFolderPath(ctx, s, lf.ID, to, nil, domain.System); err != nil {
		t.Fatalf("no present files: %v", err)
	}
}

// The API: PUT .../path answers 409 naming the file, then 200 with the folder.
func TestChangeFolderPathAPI(t *testing.T) {
	s, lf, fs, to := pathWorld(t)
	h := &FolderHandlers{Store: s, DownloadPaths: func(*http.Request) []string { return nil },
		Actor: func(*http.Request) domain.Actor { return domain.System }, Fail: WriteError}
	m := http.NewServeMux()
	h.Routes(m)
	put := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		m.ServeHTTP(rec, httptest.NewRequest("PUT", "/api/v1/library-folders/"+strconv.FormatInt(lf.ID, 10)+"/path", strings.NewReader(body)))
		return rec
	}
	body, _ := json.Marshal(map[string]string{"path": to})
	rec := put(string(body))
	var fail struct {
		Error string `json:"error"`
		File  string `json:"file"`
	}
	if rec.Code != http.StatusConflict || json.Unmarshal(rec.Body.Bytes(), &fail) != nil || fail.File != fs[0].RelativePath {
		t.Fatalf("mismatch: %d %s", rec.Code, rec.Body)
	}
	if rec := put(`{"path": "relative"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad path: %d %s", rec.Code, rec.Body)
	}
	for _, f := range fs {
		putAt(t, to, f.RelativePath, 10)
	}
	rec = put(string(body))
	var v FolderView
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &v) != nil || v.Path != filepath.Clean(to) {
		t.Fatalf("change: %d %s", rec.Code, rec.Body)
	}
}
