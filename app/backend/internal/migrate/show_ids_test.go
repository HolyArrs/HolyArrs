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
	l, src := cunkSetup(t, ids, shows)
	mv := l.move(t, Choices{})
	return l, src, mv.Report
}

// cunkSeasons are Sonarr's season choices for Cunk on Earth.
var cunkSeasons = []Season{{Number: 1, Monitored: true}}

// cunkSetup reads a Sonarr holding Cunk on Earth; files are made in its
// folder (none: the folder is not there).
func cunkSetup(t *testing.T, ids map[string]string, shows []metadata.Show, files ...string) (*moveLab, *manyShows) {
	t.Helper()
	return cunkSetupOld(t, ids, shows, nil, files...)
}

// cunkSetupOld is cunkSetup with the files and episodes the old app lists.
func cunkSetupOld(t *testing.T, ids map[string]string, shows []metadata.Show, old []TitleFile, files ...string) (*moveLab, *manyShows) {
	t.Helper()
	l := newMoveLab(t)
	tv := filepath.Join(t.TempDir(), "tv")
	if err := os.MkdirAll(tv, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		p := filepath.Join(tv, "Cunk on Earth", filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("not really an episode"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src := &manyShows{list: shows}
	l.mv.Library.Shows = metadata.ShowSources{"tmdb": src}
	sonarr := &Snapshot{Kind: Sonarr, Via: ViaAPI, Folders: []Folder{{Path: "/tv"}},
		Titles: []Title{{SourceID: 1, Kind: "show", Name: "Cunk on Earth", Year: 2022, IDs: ids, Path: "/tv/Cunk on Earth", Monitored: true,
			Seasons: cunkSeasons, Files: old}}}
	sonarr.finish()
	saved, err := Save(context.Background(), l.st, 1, sonarr)
	if err != nil {
		t.Fatal(err)
	}
	l.snaps = append(l.snaps, saved.ID)
	l.rules = append(l.rules, PathRule{From: "/tv", To: filepath.ToSlash(tv)})
	return l, src
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
	l, _, rep := cunkLab(t, cunkSonarr, []metadata.Show{britain(), cunk(map[string]string{"tmdb": "211"})})
	if got := lineOf(t, rep, cunkKey); got.Result != ResCreated {
		t.Fatalf("%+v", got)
	}
	// The same name wins over the old app's TMDB id.
	if n := l.count(t, `SELECT count(*) FROM titles WHERE json_extract(source_ids, '$.tmdb') = '79063'`); n != 0 {
		t.Fatalf("%d shows by the old app's TMDB id", n)
	}
}

// Nothing finds it: the line says so plainly, in the dry run too.
func TestMoveShowNotFoundAtAll(t *testing.T) {
	ids := map[string]string{"tvdb": "414217", "imdb": "tt16867040"}
	l, _ := cunkSetup(t, ids, []metadata.Show{britain()}, "Season 01/Cunk.on.Earth.S01E01.1080p.WEB.mkv")
	dry := l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if got := lineOf(t, dry, cunkKey); got.Result != ResFailed || !strings.Contains(got.Detail, "could not be found by its ids or its name") {
		t.Fatalf("dry run %+v", got)
	}
	got := lineOf(t, l.move(t, Choices{}).Report, cunkKey)
	if got.Result != ResFailed || !strings.Contains(got.Detail, "could not be found by its ids or its name") {
		t.Fatalf("%+v", got)
	}
}

// cunkOn is TMDB's show 79063: Britain is its season 1, Earth its season 2.
func cunkOn() metadata.Show {
	aired := time.Date(2018, 9, 1, 0, 0, 0, 0, time.UTC)
	// TMDB lists the season names as other names of the show.
	sh := metadata.Show{Source: "tmdb", Name: "Cunk on...", Year: 2018, IDs: map[string]string{"tmdb": "79063", "imdb": "tt8697438"},
		OtherNames: []string{"Cunk on Earth", "Cunk on Britain"},
		Seasons:    []metadata.Season{{Number: 1, Name: "Britain"}, {Number: 2, Name: "Earth"}}}
	// TMDB's real names: Britain's first is "Beginnings", Earth's "In the Beginnings".
	names := map[[2]int]string{{1, 1}: "Beginnings", {1, 2}: "Episode 2", {2, 1}: "In the Beginnings", {2, 2}: "Faith/Off"}
	for s := 1; s <= 2; s++ {
		for e := 1; e <= 2; e++ {
			sh.Episodes = append(sh.Episodes, metadata.Episode{Season: s, Number: e, Name: names[[2]int{s, e}], AirDate: &aired, AirDay: "2018-09-01"})
		}
	}
	return sh
}

// The show itself, and every season of it, is not kept up to date.
const cunkKept = `SELECT t.keep_up_to_date + (SELECT count(*) FROM versions v WHERE v.title_id = t.id AND v.keep_up_to_date = 1) FROM titles t WHERE json_extract(t.source_ids, '$.tmdb') = '79063'`

// showFilled counts the show episodes that have a file.
const showFilled = `SELECT count(*) FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id WHERE t.kind = 'show' AND s.file_id IS NOT NULL`

var cunkFiles = []string{"Season 01/Cunk.on.Earth.S01E01.1080p.WEB.mkv", "Season 01/Cunk.on.Earth.S01E02.1080p.WEB.mkv"}

// Only the TMDB id the old app gave finds the show, under another name:
// the dry run and the import both take it, and the files wait for the
// person to place them, since the old app's S01E01 is not this show's.
func TestMoveShowFoundOnlyByOldAppTMDB(t *testing.T) {
	l, _ := cunkSetup(t, cunkSonarr, []metadata.Show{cunkOn()}, cunkFiles...)
	dry := l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if got := lineOf(t, dry, cunkKey); got.Result != ResAdopt || got.Attention != AttnSeasons || !strings.Contains(got.Detail, "Cunk on...") {
		t.Fatalf("dry run %+v", got)
	}
	got := lineOf(t, l.move(t, Choices{}).Report, cunkKey)
	if got.Result != ResAdopted || got.Attention != AttnSeasons || !strings.Contains(got.Detail, "check that it is the same show") ||
		!strings.Contains(got.Detail, "numbered differently") || !strings.Contains(got.Detail, "if it is not, remove it") || !strings.Contains(got.Detail, "wait in Leftover files") {
		t.Fatalf("move %+v", got)
	}
	// The line names the show as it is called here.
	if got.Here != "Cunk on..." || !strings.HasPrefix(got.Name, "Cunk on Earth") {
		t.Fatalf("names %q %q", got.Here, got.Name)
	}
	// The line names the show here, so the Done page can open it.
	if n := l.count(t, `SELECT count(*) FROM titles WHERE id = ? AND seasons_differ = 1`, got.TitleID); n != 1 {
		t.Fatalf("line points at title %d", got.TitleID)
	}
	if n := l.count(t, `SELECT count(*) FROM titles WHERE json_extract(source_ids, '$.tmdb') = '79063'`); n != 1 {
		t.Fatalf("%d shows by the old app's TMDB id", n)
	}
	placed := showFilled
	waiting := `SELECT count(*) FROM files WHERE origin = 'found' AND leftover_reason = 'seasons_differ' AND slot_ids = '[]'`
	if n := l.count(t, placed); n != 0 {
		t.Fatalf("%d episodes got a file by the old app's numbers", n)
	}
	if n := l.count(t, waiting); n != 2 {
		t.Fatalf("%d files wait to be placed", n)
	}
	// Nothing is searched for while its files wait: neither the show nor
	// its copy is kept up to date, and the show knows its seasons differ.
	if n := l.count(t, cunkKept); n != 0 {
		t.Fatalf("kept up to date: %d", n)
	}
	if n := l.count(t, `SELECT count(*) FROM titles WHERE seasons_differ = 1`); n != 1 {
		t.Fatalf("%d shows marked", n)
	}
	if !strings.Contains(got.Detail, "nothing is downloaded until you turn it on") {
		t.Fatalf("line %q", got.Detail)
	}
	// A rescan, later, still leaves them to the person.
	if _, err := l.mv.Library.Rescan(context.Background(), domain.System); err != nil {
		t.Fatal(err)
	}
	if n := l.count(t, placed); n != 0 {
		t.Fatalf("rescan placed %d", n)
	}
	if n := l.count(t, waiting); n != 2 {
		t.Fatalf("%d files wait after the rescan", n)
	}
	// Importing again finds the show it brought.
	if got := lineOf(t, l.move(t, Choices{}).Report, cunkKey); got.Result != ResAlready {
		t.Fatalf("again %+v", got)
	}
}

// The TVDB id finds the show and the old app's TMDB id is another show
// here already: the files fill episodes as usual and nothing is marked.
func TestMoveShowTVDBFoundFilesPlaced(t *testing.T) {
	right := cunk(map[string]string{"tmdb": "211", "tvdb": "414217", "imdb": "tt16867040"})
	aired := time.Date(2022, 9, 1, 0, 0, 0, 0, time.UTC)
	right.Episodes = []metadata.Episode{{Season: 1, Number: 1, AirDate: &aired, AirDay: "2022-09-01"}, {Season: 1, Number: 2, AirDate: &aired, AirDay: "2022-09-01"}}
	l, _ := cunkSetup(t, cunkSonarr, []metadata.Show{cunkOn(), right}, cunkFiles...)
	got := lineOf(t, l.move(t, Choices{}).Report, cunkKey)
	if got.Result != ResAdopted || got.Attention != "" {
		t.Fatalf("%+v", got)
	}
	if n := l.count(t, `SELECT count(*) FROM titles WHERE json_extract(source_ids, '$.tmdb') = '79063'`); n != 0 {
		t.Fatalf("%d shows by the wrong TMDB id", n)
	}
	if n := l.count(t, showFilled); n != 2 {
		t.Fatalf("%d episodes have a file", n)
	}
	if n := l.count(t, `SELECT count(*) FROM titles WHERE kind = 'show' AND keep_up_to_date = 1 AND seasons_differ = 0`); n != 1 {
		t.Fatalf("%d shows kept up to date", n)
	}
}

// Sonarr's season choices are its own numbers: a show whose seasons differ
// keeps this server's defaults for every season.
func TestMoveSeasonsDifferLeavesSeasonChoices(t *testing.T) {
	saved := cunkSeasons
	cunkSeasons = []Season{{Number: 1, Monitored: false}}
	t.Cleanup(func() { cunkSeasons = saved })
	l, _ := cunkSetup(t, cunkSonarr, []metadata.Show{cunkOn()}, cunkFiles...)
	got := lineOf(t, l.move(t, Choices{}).Report, cunkKey)
	if got.Result != ResAdopted || !strings.Contains(got.Detail, "season choices were not used") {
		t.Fatalf("%+v", got)
	}
	if n := l.count(t, `SELECT count(DISTINCT keep_up_to_date) FROM parts WHERE kind = 'season'`); n != 1 {
		t.Fatalf("seasons set apart: %d kinds", n)
	}
}

// With no folder of its own, the show still knows its seasons differ: files
// that turn up in its folder later are never put on episodes by number.
func TestMoveSeasonsDifferNoFolderYet(t *testing.T) {
	l, _, rep := cunkLab(t, cunkSonarr, []metadata.Show{cunkOn()})
	// Its old folder is not on this server: that attention comes first.
	// It says what is true: there are no files to place, but the show is
	// still to be checked, and the Done page can still open it.
	if got := lineOf(t, rep, cunkKey); got.Result != ResCreated || got.Attention != AttnUnreachable || !strings.Contains(got.Detail, "check that it is the same show") ||
		strings.Contains(got.Detail, "Leftover files") || got.Here != "Cunk on..." || got.TitleID == 0 {
		t.Fatalf("%+v", got)
	}
	if n := l.count(t, cunkKept); n != 0 {
		t.Fatalf("kept up to date: %d", n)
	}
	if n := l.count(t, `SELECT count(*) FROM titles WHERE seasons_differ = 1`); n != 1 {
		t.Fatal("the show was not marked")
	}
	tv := filepath.FromSlash(l.rules[len(l.rules)-1].To)
	p := filepath.Join(tv, "Cunk on Earth", "Season 01", "Cunk.on.Earth.S01E01.1080p.WEB.mkv")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("later"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := l.mv.Library.Rescan(context.Background(), domain.System); err != nil {
		t.Fatal(err)
	}
	if n := l.count(t, showFilled); n != 0 {
		t.Fatalf("rescan placed %d by number", n)
	}
	if n := l.count(t, `SELECT count(*) FROM files WHERE origin = 'found' AND leftover_reason = 'seasons_differ'`); n != 1 {
		t.Fatalf("%d files wait", n)
	}
}

// The Match sheet offers an episode only by its own name, never by the old
// app's numbers; a name that only counts ("Episode 2") is not a name.
func TestSeasonsDifferGuessByName(t *testing.T) {
	l, _ := cunkSetup(t, cunkSonarr, []metadata.Show{cunkOn()},
		"Season 01/Cunk on Earth - S01E01 - In the Beginnings WEBDL-1080p.mkv", "Season 01/Cunk on Earth - S01E02 - Episode 2 WEBDL-1080p.mkv")
	l.move(t, Choices{})
	var id int64
	if err := l.st.Read().QueryRow(`SELECT id FROM titles WHERE seasons_differ = 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	ls, err := l.mv.Library.Leftovers(context.Background(), id)
	if err != nil || len(ls) != 2 {
		t.Fatalf("%v %+v", err, ls)
	}
	for _, lo := range ls {
		named := strings.Contains(lo.RelativePath, "Beginnings")
		switch {
		case named && (lo.Guess == nil || lo.Guess.Season != 2 || len(lo.Guess.Episodes) != 1 || lo.Guess.Episodes[0] != 1):
			t.Fatalf("by name %+v", lo.Guess)
		case !named && lo.Guess != nil:
			t.Fatalf("by number %+v", lo.Guess)
		}
	}
}
