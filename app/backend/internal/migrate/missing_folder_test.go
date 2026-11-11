package migrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// A movie the old app wanted but never got has no folder of its own. Its
// Library folder is here, so it comes over as a title with no files, the
// way Radarr keeps it as missing.
func TestMoveAddsTitleWithoutItsFolder(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Titles = append(radarr.Titles, Title{SourceID: 2, Kind: "movie", Name: "The Matrix Reloaded", Year: 2003,
			IDs: map[string]string{"tmdb": "604"}, Path: "/movies/The Matrix Reloaded (2003)", Monitored: true, ProfileID: 2,
			Extra: map[string]any{"minimum_availability": "inCinemas"}})
	})
	l.mv.Library.Metadata = &metadata.Static{Movies: []metadata.Movie{
		{Name: "The Matrix", Year: 1999, IDs: map[string]string{"tmdb": "603", "imdb": "tt0133093"}},
		{Name: "The Matrix Reloaded", Year: 2003, IDs: map[string]string{"tmdb": "604", "imdb": "tt0234215"}},
	}}
	const key = "title:movie:tmdb:604"

	// The dry run says it will be added with no files yet, so Before you
	// import counts it with the titles that come over.
	rep := l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if got := lineOf(t, rep, key); got.Result != ResCreate || !strings.Contains(got.Detail, "no files") {
		t.Fatalf("dry run %+v", got)
	}

	mv := l.move(t, Choices{})
	if got := lineOf(t, mv.Report, key); got.Result != ResCreated || !strings.Contains(got.Detail, "no files") {
		t.Fatalf("move %+v", got)
	}
	var keep bool
	var folder, rules, gate, state string
	if err := l.st.Read().QueryRow(`SELECT t.keep_up_to_date, v.folder_name, p.name, t.release_gate, s.state FROM titles t
		JOIN versions v ON v.title_id = t.id JOIN pick_rules p ON p.id = v.pick_rules_id JOIN slots s ON s.version_id = v.id
		WHERE json_extract(t.source_ids, '$.tmdb') = '604'`).Scan(&keep, &folder, &rules, &gate, &state); err != nil {
		t.Fatal(err)
	}
	if !keep || folder != "The Matrix Reloaded (2003)" || rules != "4K" || gate != "in_cinemas" || state == "in_library" {
		t.Fatalf("keep %v folder %q rules %q gate %q slot %q", keep, folder, rules, gate, state)
	}
	if n := l.count(t, "SELECT count(*) FROM files f JOIN versions v ON v.id = f.version_id JOIN titles t ON t.id = v.title_id WHERE json_extract(t.source_ids, '$.tmdb') = '604'"); n != 0 {
		t.Fatalf("%d files for a title with no folder", n)
	}

	// Undo takes it out again with the rest.
	if got := l.undo(t, mv.ID); got.State != "undone" {
		t.Fatalf("undo %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM titles WHERE json_extract(source_ids, '$.tmdb') = '604' AND state = 'active'"); n != 0 {
		t.Fatalf("undo left the title: %d", n)
	}
}

// With no Library folder here, the title still says no folder here.
func TestDryRunTitleWithoutLibraryFolderNotFound(t *testing.T) {
	l := newMoveLab(t)
	l.rules = nil
	rep := l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if got := lineOf(t, rep, "title:movie:tmdb:603"); got.Result != ResNotFound {
		t.Fatalf("title %+v", got)
	}
}

// A title folder that is there but is a link or not a folder is not
// treated as missing: it stays not found, and nothing is added.
func TestDryRunTitleFolderThereButNotPlainFolder(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string){
		"file": func(t *testing.T, dir string) {
			if err := os.RemoveAll(dir); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dir, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"link": func(t *testing.T, dir string) {
			real := filepath.Join(t.TempDir(), "real")
			if err := os.Rename(dir, real); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(real, dir); err != nil {
				t.Skip("no symlinks here:", err)
			}
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			l := newMoveLab(t)
			setup(t, filepath.Join(l.movies, "The Matrix (1999)"))
			rep := l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
			if got := lineOf(t, rep, "title:movie:tmdb:603"); got.Result != ResNotFound || strings.Contains(got.Detail, "no files") {
				t.Fatalf("dry run %+v", got)
			}
		})
	}
}

type oneShow struct{ sh metadata.Show }

func (o oneShow) SearchShows(context.Context, string) ([]metadata.Show, error) {
	return []metadata.Show{o.sh}, nil
}

func (o oneShow) Show(_ context.Context, id string) (metadata.Show, error) {
	for k, v := range o.sh.IDs {
		if id == k+":"+v {
			return o.sh, nil
		}
	}
	return metadata.Show{}, metadata.ErrShowNotFound
}

// A show with no folder of its own comes over with its episodes wanted, as
// Sonarr keeps it.
func TestMoveAddsShowWithoutItsFolder(t *testing.T) {
	l := newMoveLab(t)
	tv := filepath.Join(t.TempDir(), "tv")
	if err := os.MkdirAll(tv, 0o755); err != nil {
		t.Fatal(err)
	}
	aired := time.Date(2025, 1, 6, 2, 0, 0, 0, time.UTC)
	l.mv.Library.Shows = metadata.ShowSources{"tmdb": oneShow{metadata.Show{Source: "tmdb", Name: "Lab Show", Year: 2025, IDs: map[string]string{"tmdb": "100", "tvdb": "200"},
		Seasons:  []metadata.Season{{Number: 1, Name: "Season 1"}},
		Episodes: []metadata.Episode{{Season: 1, Number: 1, Name: "Pilot", AirDate: &aired, Timed: true, AirDay: "2025-01-05", ID: "tmdb-101"}}}}}
	sonarr := &Snapshot{Kind: Sonarr, Via: ViaAPI, Folders: []Folder{{Path: "/tv"}},
		Titles: []Title{{SourceID: 1, Kind: "show", Name: "Lab Show", IDs: map[string]string{"tvdb": "200"}, Path: "/tv/Lab Show", Monitored: true,
			Seasons: []Season{{Number: 1, Monitored: true}}}}}
	sonarr.finish()
	saved, err := Save(context.Background(), l.st, 1, sonarr)
	if err != nil {
		t.Fatal(err)
	}
	l.snaps = append(l.snaps, saved.ID)
	l.rules = append(l.rules, PathRule{From: "/tv", To: filepath.ToSlash(tv)})
	const key = "title:show:tvdb:200"

	rep := l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if got := lineOf(t, rep, key); got.Result != ResCreate {
		t.Fatalf("dry run %+v", got)
	}
	mv := l.move(t, Choices{})
	if got := lineOf(t, mv.Report, key); got.Result != ResCreated {
		t.Fatalf("move %+v", got)
	}
	var folder string
	if err := l.st.Read().QueryRow(`SELECT v.folder_name FROM versions v JOIN titles t ON t.id = v.title_id WHERE json_extract(t.source_ids, '$.tvdb') = '200'`).Scan(&folder); err != nil {
		t.Fatal(err)
	}
	if folder != "Lab Show" {
		t.Fatalf("folder %q", folder)
	}
	if n := l.count(t, `SELECT count(*) FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
		WHERE json_extract(t.source_ids, '$.tvdb') = '200' AND s.state = 'wanted'`); n != 1 {
		t.Fatalf("%d wanted episodes", n)
	}
}
