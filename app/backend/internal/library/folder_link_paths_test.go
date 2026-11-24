package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func addMappedDownloader(t *testing.T, s *database.Store, name string, ours []string) {
	t.Helper()
	var pm []domain.PathMap
	for _, o := range ours {
		pm = append(pm, domain.PathMap{Theirs: "/downloads/" + filepath.Base(o), Ours: o})
	}
	raw, _ := json.Marshal(pm)
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url, category, category_movies, category_shows, priority, path_map, after_import, state, last_error, tags, removed)
			VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', ?, 'qbittorrent', 'torrent', 'http://qbit:8080', ?, 'category_movies', 'category_shows', 1, ?, 'leave', 'ok', '', '[]', 0)`, name, name, string(raw))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A Library folder is checked against the download folders its titles can
// really come from. When one Downloader keeps several download folders
// (one per kind of title, often one per disk), the folder only needs
// hardlinks from one of them. A disk "gone" below is a folder that cannot
// be written, which fails the hardlink probe like a second disk does.
func TestLinkPathsPerDisk(t *testing.T) {
	type dl struct {
		name  string
		paths []string // relative to the test root; a "gone/" prefix is another disk
	}
	cases := []struct {
		name  string
		dls   []dl
		lib   string
		extra []string
		want  string
	}{
		{"same disk, one download folder", []dl{{"qbit", []string{"data/downloads"}}}, "data/media/movies", nil, "ok"},
		{"same disk, one folder per kind", []dl{{"qbit", []string{"data/downloads/movies", "data/downloads/tv"}}}, "data/media/tv", nil, "ok"},
		{"separate disks per kind, movie library", []dl{{"qbit", []string{"data/movies/Downloads", "gone/data/series/Downloads"}}}, "data/movies/Movies", nil, "ok"},
		{"separate disks per kind, show library", []dl{{"qbit", []string{"gone/data/movies/Downloads", "data/series/Downloads"}}}, "data/series/Tv Shows", nil, "ok"},
		{"docker bind mounts, one per kind", []dl{{"qbit", []string{"movies/downloads", "gone/tv/downloads"}}}, "movies/library", nil, "ok"},
		{"unraid /mnt/user share", []dl{{"qbit", []string{"mnt/user/data/torrents/movies", "mnt/user/data/torrents/tv"}}}, "mnt/user/data/media/movies", nil, "ok"},
		{"unraid cache disk vs array share", []dl{{"qbit", []string{"gone/mnt/cache/torrents"}}}, "mnt/user/data/media/movies", nil, "no_hardlink"},
		{"every folder of the Downloader on another disk", []dl{{"qbit", []string{"gone/a", "gone/b"}}}, "data/media/movies", nil, "no_hardlink"},
		{"two Downloaders, one on another disk", []dl{{"qbit", []string{"data/downloads"}}, {"sab", []string{"gone/usenet"}}}, "data/media/movies", nil, "no_hardlink"},
		{"save path on another disk", []dl{{"qbit", []string{"data/downloads"}}}, "data/media/movies", []string{"gone/save"}, "no_hardlink"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := open(t)
			ctx := context.Background()
			root := t.TempDir()
			at := func(rel string) string {
				p := filepath.Join(root, filepath.FromSlash(rel))
				if !startsGone(rel) {
					if err := os.MkdirAll(p, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				return p
			}
			for _, d := range c.dls {
				var abs []string
				for _, p := range d.paths {
					abs = append(abs, at(p))
				}
				addMappedDownloader(t, s, d.name, abs)
			}
			var extra []string
			for _, p := range c.extra {
				extra = append(extra, at(p))
			}
			all, err := DownloadPaths(ctx, s.Read(), extra...)
			if err != nil {
				t.Fatal(err)
			}
			lib := at(c.lib)
			got := CheckFolder(lib, LinkPaths(ctx, s.Read(), lib, all, nil))
			if got.State != c.want {
				t.Fatalf("state %q (%s), want %q", got.State, got.Reason, c.want)
			}
		})
	}
}

func startsGone(rel string) bool { return len(rel) >= 5 && rel[:5] == "gone/" }
