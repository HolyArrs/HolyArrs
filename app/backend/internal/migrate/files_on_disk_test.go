package migrate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// onDiskLab is an import whose old root folder is not matched here, while
// the movie's own folder (same name) sits in the Library folder here.
func onDiskLab(t *testing.T, files map[string]string) *moveLab {
	t.Helper()
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Folders = append(radarr.Folders, Folder{Path: "/data/movies"})
		radarr.Titles = append(radarr.Titles, Title{SourceID: 2, Kind: "movie", Name: "The Matrix Reloaded", Year: 2003,
			IDs: map[string]string{"tmdb": "604"}, Path: "/data/movies/The Matrix Reloaded (2003)", Monitored: true, ProfileID: 2})
	})
	reloadedMeta(l)
	for rel, body := range files {
		p := filepath.Join(l.movies, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return l
}

const linkedReloaded = `SELECT count(*) FROM files f JOIN versions v ON v.id = f.version_id JOIN titles t ON t.id = v.title_id
	WHERE json_extract(t.source_ids, '$.tmdb') = '604' AND f.state = 'present' AND json_array_length(f.slot_ids) = 1`

// The movie's file already in its folder here is linked by the import.
func TestMoveLinksFileOnDiskWhenRootNotMatched(t *testing.T) {
	l := onDiskLab(t, map[string]string{"The Matrix Reloaded (2003)/The.Matrix.Reloaded.2003.1080p.mkv": "a movie"})
	l.move(t, Choices{})
	if n := l.count(t, linkedReloaded); n != 1 {
		t.Fatalf("file not linked after import: %d", n)
	}
}

// A title imported before (wanted, no files) gets its file on the next rescan.
func TestRescanLinksFileOfImportedWantedMovie(t *testing.T) {
	l := onDiskLab(t, nil)
	l.move(t, Choices{})
	if n := l.count(t, linkedReloaded); n != 0 {
		t.Fatalf("linked with no file: %d", n)
	}
	onDiskWrite(t, l, "The Matrix Reloaded (2003)/The.Matrix.Reloaded.2003.1080p.mkv")
	if _, err := l.mv.Library.Rescan(context.Background(), domain.System); err != nil {
		t.Fatal(err)
	}
	if n := l.count(t, linkedReloaded); n != 1 {
		t.Fatalf("file not linked after rescan: %d", n)
	}
}

// A sample is never linked, and neither is a file in another movie's folder.
func TestMoveDoesNotLinkSampleOrOtherFolder(t *testing.T) {
	l := onDiskLab(t, map[string]string{
		"The Matrix Reloaded (2003)/sample/The.Matrix.Reloaded.2003.sample.mkv": "s",
		"The Matrix Revolutions (2003)/The.Matrix.Revolutions.2003.1080p.mkv":   "other",
	})
	l.move(t, Choices{})
	if n := l.count(t, linkedReloaded); n != 0 {
		t.Fatalf("sample or other folder linked: %d", n)
	}
	if _, err := l.mv.Library.Rescan(context.Background(), domain.System); err != nil {
		t.Fatal(err)
	}
	if n := l.count(t, linkedReloaded); n != 0 {
		t.Fatalf("sample or other folder linked after rescan: %d", n)
	}
}

func onDiskWrite(t *testing.T, l *moveLab, rel string) {
	t.Helper()
	p := filepath.Join(l.movies, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("a movie"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// reloadedAt is an import of The Matrix Reloaded from an old root not on
// this server, with its folder named folder.
func reloadedAt(t *testing.T, folder string) *moveLab {
	t.Helper()
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Folders = append(radarr.Folders, Folder{Path: "/data/movies"})
		radarr.Titles = append(radarr.Titles, Title{SourceID: 2, Kind: "movie", Name: "The Matrix Reloaded", Year: 2003,
			IDs: map[string]string{"tmdb": "604"}, Path: "/data/movies/" + folder, Monitored: true, ProfileID: 2})
	})
	reloadedMeta(l)
	return l
}

// wantedWithAttention checks the dry run and the import both bring the
// movie over as wanted, needing attention.
func wantedWithAttention(t *testing.T, l *moveLab) {
	t.Helper()
	rep := l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if got := lineOf(t, rep, reloaded); got.Result != ResCreate || got.Attention != AttnUnreachable {
		t.Fatalf("dry %+v", got)
	}
	mv := l.move(t, Choices{})
	if got := lineOf(t, mv.Report, reloaded); got.Result != ResCreated || got.Attention != AttnUnreachable {
		t.Fatalf("move %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM titles WHERE json_extract(source_ids, '$.tmdb') = '604' AND state = 'active'"); n != 1 {
		t.Fatalf("titles %d", n)
	}
	if n := l.count(t, linkedReloaded); n != 0 {
		t.Fatalf("linked %d", n)
	}
}

// A folder another title owns is never used for this one.
func TestMoveSameNameFolderOwnedByOtherTitleStaysWanted(t *testing.T) {
	wantedWithAttention(t, reloadedAt(t, "The Matrix (1999)"))
}

// A folder that is a link is not used.
func TestMoveSameNameFolderLinkStaysWanted(t *testing.T) {
	l := reloadedAt(t, "The Matrix Reloaded (2003)")
	tgt := filepath.Join(t.TempDir(), "real")
	onDiskFile(t, filepath.Join(tgt, "The.Matrix.Reloaded.2003.1080p.mkv"))
	if err := os.Symlink(tgt, filepath.Join(l.movies, "The Matrix Reloaded (2003)")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	wantedWithAttention(t, l)
}

// A folder holding only a trailer is not the movie.
func TestMoveSameNameFolderWithOnlyTrailerStaysWanted(t *testing.T) {
	l := reloadedAt(t, "The Matrix Reloaded (2003)")
	onDiskFile(t, filepath.Join(l.movies, "The Matrix Reloaded (2003)", "extras", "x-trailer.mkv"))
	onDiskFile(t, filepath.Join(l.movies, "The Matrix Reloaded (2003)", "The.Matrix.Reloaded-trailer.mkv"))
	wantedWithAttention(t, l)
}

// failOnce fails the first lookup of one movie, as a lookup timing out.
type failOnce struct {
	metadata.Provider
	id     string
	failed bool
}

func (f *failOnce) Movie(ctx context.Context, id string) (metadata.Movie, error) {
	if id == f.id && !f.failed {
		f.failed = true
		return metadata.Movie{}, errors.New("lookup timed out")
	}
	return f.Provider.Movie(ctx, id)
}

// When adopting the folder fails, the movie still comes over as wanted.
func TestMoveSameNameFolderAdoptErrorStaysWanted(t *testing.T) {
	l := onDiskLab(t, map[string]string{"The Matrix Reloaded (2003)/The.Matrix.Reloaded.2003.1080p.mkv": "a movie"})
	l.mv.Library.Metadata = &failOnce{Provider: l.mv.Library.Metadata, id: "tmdb:604"}
	mv := l.move(t, Choices{})
	if got := lineOf(t, mv.Report, reloaded); got.Result != ResCreated || got.Attention != AttnUnreachable {
		t.Fatalf("move %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM titles WHERE json_extract(source_ids, '$.tmdb') = '604' AND state = 'active'"); n != 1 {
		t.Fatalf("titles %d", n)
	}
}

// The dry run says the folder is used, as the import does.
func TestMoveSameNameFolderDryRunSaysAdopt(t *testing.T) {
	l := onDiskLab(t, map[string]string{"The Matrix Reloaded (2003)/The.Matrix.Reloaded.2003.1080p.mkv": "a movie"})
	rep := l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if got := lineOf(t, rep, reloaded); got.Result != ResAdopt || got.Attention != "" {
		t.Fatalf("dry %+v", got)
	}
	if got := lineOf(t, l.move(t, Choices{}).Report, reloaded); got.Result != ResAdopted || got.Attention != "" {
		t.Fatalf("move %+v", got)
	}
}

func onDiskFile(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("a movie"), 0o644); err != nil {
		t.Fatal(err)
	}
}
