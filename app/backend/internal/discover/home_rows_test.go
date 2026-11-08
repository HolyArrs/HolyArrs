package discover

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// homeFixture adds Library titles for the Home row tests, one write each.
type homeFixture struct {
	t *testing.T
	s *database.Store
}

func (f homeFixture) write(fn func(ins func(table string, v any))) {
	f.t.Helper()
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		var failed error
		fn(func(table string, v any) {
			if failed == nil {
				_, failed = database.Insert(tx, table, v, domain.System)
			}
		})
		return failed
	})
	if err != nil {
		f.t.Fatal(err)
	}
}

// title adds a title and its main version.
func (f homeFixture) title(kind, name string) (mediafiles.Title, mediafiles.Version) {
	src := map[string]string{"movie": "tmdb", "show": "tvmaze", "artist": "musicbrainz"}[kind]
	t := mediafiles.Title{Kind: kind, Name: name, SortName: name, MetadataSource: src, WhyAdded: "by_hand", State: "active", NewPartsPolicy: "all", ReleaseGate: "released", GateDriver: "none", Status: "released"}
	var v mediafiles.Version
	f.write(func(ins func(string, any)) {
		ins("titles", &t)
		v = mediafiles.Version{TitleID: t.ID, Label: "Main", IsMain: true, LibraryFolderID: 1, PickRulesID: 1, FolderName: name}
		ins("versions", &v)
	})
	// Made long ago, so a show is not "New show".
	f.exec(`UPDATE titles SET created_at = ? WHERE id = ?`, database.FormatTime(time.Now().AddDate(-1, 0, 0)), t.ID)
	return t, v
}

func (f homeFixture) part(t mediafiles.Title, kind string, n *int, name string) int64 {
	p := mediafiles.Part{TitleID: t.ID, Kind: kind, Number: n, Name: name, KeepUpToDate: true, Included: true}
	f.write(func(ins func(string, any)) { ins("parts", &p) })
	return p.ID
}

// item adds one item with its main-copy slot; added set gives it a file that came in then.
func (f homeFixture) item(it mediafiles.Item, v mediafiles.Version, added *time.Time, state string) mediafiles.Slot {
	var slot mediafiles.Slot
	f.write(func(ins func(string, any)) {
		ins("items", &it)
		slot = mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotState(state)}
		if added != nil {
			file := mediafiles.File{VersionID: v.ID, RelativePath: fmt.Sprintf("f%d.mkv", it.ID), AddedAt: *added, Origin: "imported", PlacedBy: "move", Role: "main", State: "present"}
			ins("files", &file)
			slot.FileID = &file.ID
		}
		ins("slots", &slot)
	})
	return slot
}

// exec runs one change, outside any other write.
func (f homeFixture) exec(q string, args ...any) {
	f.t.Helper()
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
		f.t.Fatal(err)
	}
}

func homeNum(n int) *int { return &n }

func homeCardOf(cards []HomeCard, id int64) *HomeCard {
	for i := range cards {
		if cards[i].TitleID == id {
			return &cards[i]
		}
	}
	return nil
}

// Just arrived counts only files from the last two weeks: an old season
// with one new episode elsewhere names the season that is new, a season
// with one new file among old ones says one new episode, and titles with
// nothing new stay on the row with a plain line.
func TestHomeArrivalsCountsOnlyNewFiles(t *testing.T) {
	t.Parallel()
	s := homeStore(t)
	f := homeFixture{t, s}
	now := time.Now()
	old, fresh := now.AddDate(0, 0, -90), now.Add(-time.Hour)
	aired := now.AddDate(0, 0, -100)

	// Season 5 all came in three months ago; today one Season 1 episode arrived.
	a, av := f.title("show", "Old Five")
	a1, a5 := f.part(a, "season", homeNum(1), "Season 1"), f.part(a, "season", homeNum(5), "Season 5")
	for e := 1; e <= 10; e++ {
		f.item(mediafiles.Item{TitleID: a.ID, PartID: &a5, Kind: "episode", AirDate: &aired, Numbers: mediafiles.Numbers{Season: homeNum(5), Episode: homeNum(e)}}, av, &old, "in_library")
	}
	f.item(mediafiles.Item{TitleID: a.ID, PartID: &a1, Kind: "episode", AirDate: &aired, Numbers: mediafiles.Numbers{Season: homeNum(1), Episode: homeNum(3)}}, av, &fresh, "in_library")

	// Nine episodes from a month ago and one from today, same season.
	b, bv := f.title("show", "One New")
	b5 := f.part(b, "season", homeNum(5), "Season 5")
	month := now.AddDate(0, 0, -30)
	for e := 1; e <= 10; e++ {
		at := &month
		if e == 10 {
			at = &fresh
		}
		f.item(mediafiles.Item{TitleID: b.ID, PartID: &b5, Kind: "episode", AirDate: &aired, Numbers: mediafiles.Numbers{Season: homeNum(5), Episode: homeNum(e)}}, bv, at, "in_library")
	}

	// Nothing new: a show, an artist with old albums, a movie.
	c, cv := f.title("show", "Quiet Show")
	c1 := f.part(c, "season", homeNum(1), "Season 1")
	f.item(mediafiles.Item{TitleID: c.ID, PartID: &c1, Kind: "episode", AirDate: &aired}, cv, &old, "in_library")
	ar, arv := f.title("artist", "Old Band")
	for _, name := range []string{"First", "Second"} {
		al := f.part(ar, "album", nil, name)
		f.item(mediafiles.Item{TitleID: ar.ID, PartID: &al, Kind: "track"}, arv, &old, "in_library")
	}
	m, mv := f.title("movie", "Old Movie")
	f.item(mediafiles.Item{TitleID: m.ID, Kind: "movie"}, mv, &old, "in_library")

	// One new album, one old one.
	nb, nbv := f.title("artist", "New Band")
	oldAlbum, newAlbum := f.part(nb, "album", nil, "Back Catalog"), f.part(nb, "album", nil, "Fresh Record")
	f.item(mediafiles.Item{TitleID: nb.ID, PartID: &oldAlbum, Kind: "track"}, nbv, &old, "in_library")
	f.item(mediafiles.Item{TitleID: nb.ID, PartID: &newAlbum, Kind: "track"}, nbv, &fresh, "in_library")

	cards, err := homeArrivals(context.Background(), s.Read(), "t.state != 'removed'", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 6 {
		t.Fatalf("every title stays on the row: %d cards", len(cards))
	}
	for _, w := range []struct {
		id         int64
		line, href string
	}{
		{a.ID, "S1 · 1 new episode", fmt.Sprintf("/library/%d?season=1", a.ID)},
		{b.ID, "S5 · 1 new episode", fmt.Sprintf("/library/%d?season=5", b.ID)},
		{c.ID, "Show", fmt.Sprintf("/library/%d", c.ID)},
		{ar.ID, "Music", fmt.Sprintf("/library/%d", ar.ID)},
		{m.ID, "Movie", fmt.Sprintf("/library/%d", m.ID)},
		{nb.ID, "Fresh Record", fmt.Sprintf("/library/%d", nb.ID)},
	} {
		got := homeCardOf(cards, w.id)
		if got == nil || got.Line != w.line || got.Href != w.href {
			t.Errorf("title %d: %+v, want line %q href %q", w.id, got, w.line, w.href)
		}
	}
	if got := homeCardOf(cards, nb.ID); got == nil || len(got.Albums) != 1 {
		t.Errorf("only the new album is listed: %+v", got)
	}
	if got := homeCardOf(cards, ar.ID); got == nil || len(got.Albums) != 0 {
		t.Errorf("no album is new: %+v", got)
	}
}

// Coming soon and New season show the day an episode really airs. A
// day-only episode is stored as the next midnight UTC and must not read one
// day late; a timed one is shown on the server's local day. A movie already
// in the Library is not coming soon because of a later disc date.
func TestHomeSoonDays(t *testing.T) {
	t.Parallel()
	s := homeStore(t)
	f := homeFixture{t, s}
	zone := time.FixedZone("east-us", -5*3600)
	now := time.Now().In(zone)
	midnight := func(days int) time.Time {
		d := now.UTC().AddDate(0, 0, days)
		return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	}

	// A day-only episode in a week: stored as the end of its day.
	day := midnight(7)
	dayEnd := day.AddDate(0, 0, 1)
	dayText := day.Format("2006-01-02")
	u, uv := f.title("show", "Day Only")
	u2 := f.part(u, "season", homeNum(2), "Season 2")
	f.item(mediafiles.Item{TitleID: u.ID, PartID: &u2, Kind: "episode", AirDate: &dayEnd, AirDay: &dayText, Numbers: mediafiles.Numbers{Season: homeNum(2), Episode: homeNum(1)}}, uv, nil, "not_out_yet")

	// A timed episode at 02:00 UTC is 21:00 the evening before here.
	timedAt := midnight(9).Add(2 * time.Hour)
	tm, tv := f.title("show", "Prime Time")
	t1 := f.part(tm, "season", homeNum(1), "Season 1")
	f.item(mediafiles.Item{TitleID: tm.ID, PartID: &t1, Kind: "episode", AirDate: &timedAt, AirTimed: true, Numbers: mediafiles.Numbers{Season: homeNum(1), Episode: homeNum(4)}}, tv, nil, "not_out_yet")

	// Two movies with a disc date in ten days: one already on disk.
	disc := midnight(10)
	have, hv := f.title("movie", "Have It")
	f.item(mediafiles.Item{TitleID: have.ID, Kind: "movie"}, hv, &now, "in_library")
	want, wv := f.title("movie", "Want It")
	f.item(mediafiles.Item{TitleID: want.ID, Kind: "movie"}, wv, nil, "wanted")
	for _, id := range []int64{have.ID, want.ID} {
		f.exec(`UPDATE titles SET physical_date = ? WHERE id = ?`, database.FormatTime(disc), id)
	}

	ctx := context.Background()
	soon, err := homeComingSoon(ctx, s.Read(), "t.state != 'removed'", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if c := homeCardOf(soon, u.ID); c == nil || c.Line != "S2E1 · "+day.Format("Jan 2") || c.At != dayText {
		t.Errorf("day-only episode: %+v, want %s", c, day.Format("Jan 2"))
	}
	local := timedAt.In(zone)
	if c := homeCardOf(soon, tm.ID); c == nil || c.Line != "S1E4 · "+local.Format("Jan 2") || c.At[:10] != local.Format("2006-01-02") {
		t.Errorf("timed episode: %+v, want %s", c, local.Format("Jan 2"))
	}
	if c := homeCardOf(soon, have.ID); c != nil {
		t.Errorf("a movie in the Library is not coming soon: %+v", c)
	}
	if c := homeCardOf(soon, want.ID); c == nil || c.Line != "Out "+disc.Format("Jan 2") {
		t.Errorf("a movie without a copy is coming soon: %+v", c)
	}

	seasons, err := homeNewSeasons(ctx, s.Read(), "t.state != 'removed'", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if c := homeCardOf(seasons, u.ID); c == nil || c.Line != "Season 2 · "+day.Format("Jan 2") || c.At != dayText {
		t.Errorf("new season: %+v, want %s", c, day.Format("Jan 2"))
	}
}

func TestHomeAirDay(t *testing.T) {
	t.Parallel()
	zone := time.FixedZone("east-us", -5*3600)
	for _, c := range []struct {
		at       string
		timed    bool
		airDay   string
		day, at2 string
	}{
		{"2026-11-11T00:00:00.000000000Z", false, "2026-11-10", "2026-11-10", "2026-11-10"},
		{"2026-11-11T00:00:00.000000000Z", false, "", "2026-11-10", "2026-11-10"},
		{"2026-11-11T02:00:00.000000000Z", true, "2026-11-10", "2026-11-10", "2026-11-10T21:00:00-05:00"},
		{"2026-11-11T15:00:00.000000000Z", true, "", "2026-11-11", "2026-11-11T10:00:00-05:00"},
		{"odd", false, "", "odd", "odd"},
	} {
		if day, at := homeAirDay(c.at, c.timed, c.airDay, zone); day != c.day || at != c.at2 {
			t.Errorf("%+v: %q %q", c, day, at)
		}
	}
	if got := homeDayLabel("2026-11-10"); got != "Nov 10" {
		t.Errorf("label %q", got)
	}
}

// Your asks tells a stuck or paused download apart from one waiting to start.
func TestHomeAskProgressStates(t *testing.T) {
	t.Parallel()
	s := homeStore(t)
	f := homeFixture{t, s}
	f.exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (1, 't', '{}', 't', '{}', 'member', 'member', 'x')`)
	states := map[string]float64{"downloading": 0.5, "stalled": 0.7, "paused": 0.3, "queued": 0}
	ids := map[string]int64{}
	for state, progress := range states {
		m, v := f.title("movie", "Asked "+state)
		slot := f.item(mediafiles.Item{TitleID: m.ID, Kind: "movie"}, v, nil, "downloading")
		ids[state] = m.ID
		var did int64
		f.write(func(ins func(string, any)) {
			dr := domain.Downloader{Adapter: "sabnzbd", Name: "Lab " + state, Protocol: "usenet", URL: "http://lab.invalid", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
			ins("downloaders", &dr)
			d := domain.Download{DownloaderID: dr.ID, ExternalID: "lab-" + state, SlotIDs: []int64{slot.ID}, Progress: progress, State: state, FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
			ins("downloads", &d)
			ask := domain.Ask{Kind: "movie", TMDBID: fmt.Sprint(m.ID), Name: m.Name, State: "approved", TitleID: &m.ID, AskedBy: 1, AskedAt: time.Now(), SourceIDs: map[string]string{}}
			ins("asks", &ask)
			did = d.ID
		})
		f.exec(`UPDATE slots SET active_download_id = ? WHERE id = ?`, did, slot.ID)
	}
	got, err := homeAskProgress(context.Background(), s.Read(), 1)
	if err != nil {
		t.Fatal(err)
	}
	for state, progress := range states {
		p, ok := got[fmt.Sprint(ids[state])]
		if !ok || p.State != state || p.Progress != progress {
			t.Errorf("%s: %+v (listed %v)", state, p, ok)
		}
	}
}
