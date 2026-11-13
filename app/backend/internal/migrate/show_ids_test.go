package migrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// manyShows knows each show by any of its ids and counts the calls.
type manyShows struct {
	list  []metadata.Show
	calls int
}

func (m *manyShows) SearchShows(context.Context, string) ([]metadata.Show, error) {
	m.calls++
	return m.list, nil
}

func (m *manyShows) Show(_ context.Context, id string) (metadata.Show, error) {
	m.calls++
	for _, sh := range m.list {
		for k, v := range sh.IDs {
			if id == k+":"+v {
				return sh, nil
			}
		}
	}
	return metadata.Show{}, metadata.ErrShowNotFound
}

func cunkLab(t *testing.T, ids map[string]string, shows []metadata.Show) (*moveLab, *manyShows, *Report) {
	t.Helper()
	l := newMoveLab(t)
	tv := filepath.Join(t.TempDir(), "tv")
	if err := os.MkdirAll(tv, 0o755); err != nil {
		t.Fatal(err)
	}
	src := &manyShows{list: shows}
	l.mv.Library.Shows = metadata.ShowSources{"tmdb": src}
	sonarr := &Snapshot{Kind: Sonarr, Via: ViaAPI, Folders: []Folder{{Path: "/tv"}},
		Titles: []Title{{SourceID: 1, Kind: "show", Name: "Cunk on Earth", Year: 2022, IDs: ids, Path: "/tv/Cunk on Earth", Monitored: true,
			Seasons: []Season{{Number: 1, Monitored: true}}}}}
	sonarr.finish()
	saved, err := Save(context.Background(), l.st, 1, sonarr)
	if err != nil {
		t.Fatal(err)
	}
	l.snaps = append(l.snaps, saved.ID)
	l.rules = append(l.rules, PathRule{From: "/tv", To: filepath.ToSlash(tv)})
	mv := l.move(t, Choices{})
	return l, src, mv.Report
}

func cunk(ids map[string]string) metadata.Show {
	return metadata.Show{Source: "tmdb", Name: "Cunk on Earth", Year: 2022, IDs: ids, Seasons: []metadata.Season{{Number: 1}}}
}

func britain() metadata.Show {
	return metadata.Show{Source: "tmdb", Name: "Cunk on Britain", Year: 2018, IDs: map[string]string{"tmdb": "79063", "tvdb": "999"}}
}

var cunkSonarr = map[string]string{"tvdb": "414217", "tmdb": "79063", "imdb": "tt16867040"}

const cunkKey = "title:show:tvdb:414217"

// The TVDB id finds the show: Sonarr's different TMDB id is ignored.
func TestMoveShowWrongTMDBFromSonarr(t *testing.T) {
	l, src, rep := cunkLab(t, cunkSonarr, []metadata.Show{britain(), cunk(map[string]string{"tmdb": "211", "tvdb": "414217", "imdb": "tt16867040"})})
	if got := lineOf(t, rep, cunkKey); got.Result != ResCreated {
		t.Fatalf("%+v", got)
	}
	if n := l.count(t, `SELECT count(*) FROM titles WHERE json_extract(source_ids, '$.tmdb') = '211' AND json_extract(source_ids, '$.tvdb') = '414217'`); n != 1 {
		t.Fatalf("%d shows by the right ids", n)
	}
	if n := l.count(t, `SELECT count(*) FROM titles WHERE json_extract(source_ids, '$.tmdb') = '79063'`); n != 0 {
		t.Fatalf("%d shows by the wrong TMDB id", n)
	}
	if src.calls > 3 {
		t.Fatalf("%d metadata calls", src.calls)
	}
}

// The TVDB id is unknown: the IMDb id finds it.
func TestMoveShowTVDBMissingIMDbRight(t *testing.T) {
	_, _, rep := cunkLab(t, cunkSonarr, []metadata.Show{britain(), cunk(map[string]string{"tmdb": "211", "imdb": "tt16867040"})})
	if got := lineOf(t, rep, cunkKey); got.Result != ResCreated {
		t.Fatalf("%+v", got)
	}
}

// Neither id finds it: the name and year do.
func TestMoveShowFoundByNameAndYear(t *testing.T) {
	_, _, rep := cunkLab(t, cunkSonarr, []metadata.Show{britain(), cunk(map[string]string{"tmdb": "211"})})
	if got := lineOf(t, rep, cunkKey); got.Result != ResCreated {
		t.Fatalf("%+v", got)
	}
}

// Nothing finds it: the line says so plainly.
func TestMoveShowNotFoundAtAll(t *testing.T) {
	_, _, rep := cunkLab(t, cunkSonarr, []metadata.Show{britain()})
	got := lineOf(t, rep, cunkKey)
	if got.Result != ResFailed || !strings.Contains(got.Detail, "could not be found by its ids or its name") {
		t.Fatalf("%+v", got)
	}
}
