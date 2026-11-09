package movies

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

var now = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func renameWorld(t *testing.T) (*Service, *database.Store, mediafiles.LibraryFolder, int64) {
	t.Helper()
	s := open(t)
	lf := addFolder(t, s)
	id, err := library.AddMovie(context.Background(), s, dune, library.AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return New(&library.Service{Store: s, Now: func() time.Time { return now }}), s, lf, id
}

func deleteWorld(t *testing.T) (*Service, *database.Store, mediafiles.LibraryFolder, int64, [3]mediafiles.File) {
	t.Helper()
	svc, s, lf, id := renameWorld(t)
	imp := putFile(t, s, lf, id, "Dune - Part Two (2024)/Dune - Part Two (2024) 1080p BluRay.mkv", "Dune.Part.Two.2024.1080p.BluRay-HOLYARRS", "imported")
	ado := putFile(t, s, lf, id, "Dune - Part Two (2024)/old copy.mkv", "Dune.Part.Two.2024.720p.WEB-DL-X", "adopted")
	fnd := putFile(t, s, lf, id, "Dune - Part Two (2024)/someone else.mkv", "x", "found")
	return svc, s, lf, id, [3]mediafiles.File{imp, ado, fnd}
}

func addFolder(t *testing.T, s *database.Store) mediafiles.LibraryFolder {
	t.Helper()
	lf, err := library.AddFolder(context.Background(), s, library.FolderInput{Path: t.TempDir(), Kind: "movie", Label: "Movies"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return lf
}

func open(t *testing.T) *database.Store {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func slotsOf(t *testing.T, s *database.Store, titleID int64) []mediafiles.Slot {
	t.Helper()
	sl, err := database.Select[mediafiles.Slot](context.Background(), s.Read(), "slots", "item_id IN (SELECT id FROM items WHERE title_id = ?) ORDER BY id", titleID)
	if err != nil {
		t.Fatal(err)
	}
	return sl
}

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

func writeFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

var dune = metadata.Movie{Name: "Dune: Part Two", Year: 2024, IDs: map[string]string{"tmdb": "693134"}, Runtime: 166,
	CinemaDate: d("2024-03-01"), DigitalDate: d("2024-04-16"), PhysicalDate: d("2024-05-14")}

func d(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return &t
}
