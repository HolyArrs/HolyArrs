package shows

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// fakeShows is a show details source over fixed answers, keyed by
// "<kind>:<id>" as the real sources take them.
type fakeShows struct {
	mu    sync.Mutex
	shows map[string]metadata.Show
	err   error
	calls []string
}

func (f *fakeShows) set(sh metadata.Show, keys ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.shows == nil {
		f.shows = map[string]metadata.Show{}
	}
	for _, k := range keys {
		f.shows[k] = sh
	}
}

func (f *fakeShows) SearchShows(_ context.Context, q string) ([]metadata.Show, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	var out []metadata.Show
	seen := map[string]bool{}
	for _, sh := range f.shows {
		if strings.Contains(strings.ToLower(sh.Name), strings.ToLower(q)) && !seen[sh.Name] {
			seen[sh.Name] = true
			cp := sh
			cp.Episodes = nil
			out = append(out, cp)
		}
	}
	return out, nil
}

func (f *fakeShows) Show(_ context.Context, id string) (metadata.Show, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, id)
	if f.err != nil {
		return metadata.Show{}, f.err
	}
	if sh, ok := f.shows[id]; ok {
		return sh, nil
	}
	return metadata.Show{}, metadata.ErrShowNotFound
}

func tm(s string) *time.Time { t, _ := time.Parse(time.RFC3339, s); return &t }

// labShow is the lab show as a source tells it: specials, a first season
// that has aired, and a second one that airs around now (see now in
// library_test.go: 13 October 2026 12:00 UTC).
func labShow(source, id string) metadata.Show {
	ids := map[string]string{"tvdb": "200", "imdb": "tt0100", source: id}
	return metadata.Show{Source: source, Name: "Lab Show", Year: 2025, Status: "running", Network: "Lab TV", IDs: ids,
		Seasons: []metadata.Season{{Number: 0, Name: "Specials"}, {Number: 1, Name: "Season 1"}, {Number: 2, Name: "Season 2"}},
		Episodes: []metadata.Episode{
			{Season: 0, Number: 1, Name: "Behind the Lab", AirDate: tm("2025-03-01T12:00:00Z"), Timed: true, AirDay: "2025-03-01", ID: source + "-s1"},
			{Season: 1, Number: 1, Name: "Pilot", AirDate: tm("2025-01-06T02:00:00Z"), Timed: true, AirDay: "2025-01-05", ID: source + "-101"},
			{Season: 1, Number: 2, Name: "Second", AirDate: tm("2025-01-13T02:00:00Z"), Timed: true, AirDay: "2025-01-12", ID: source + "-102"},
			{Season: 2, Number: 1, Name: "Return", AirDate: tm("2026-10-12T02:00:00Z"), Timed: true, AirDay: "2026-10-11", ID: source + "-201"},
			{Season: 2, Number: 2, Name: "Next Week", AirDate: tm("2026-10-19T02:00:00Z"), Timed: true, AirDay: "2026-10-18", ID: source + "-202"},
			{Season: 2, Number: 3, Name: "TBA", ID: source + "-203"},
		}}
}

func addShowFolder(t *testing.T, s *database.Store) mediafiles.LibraryFolder {
	t.Helper()
	lf, err := library.AddFolder(context.Background(), s, library.FolderInput{Path: t.TempDir(), Kind: "show", Label: "Shows"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return lf
}

// seedShow writes a show Title with a main Version, then syncs it from sh
// with the given season choice, the way an add does.
func seedShow(t *testing.T, s *database.Store, sh metadata.Show, choice string) mediafiles.Title {
	t.Helper()
	ctx := context.Background()
	lf := addShowFolder(t, s)
	defs, _ := library.ReadShowDefaults(ctx, s.Read())
	order := library.OrderAired
	ti := mediafiles.Title{Kind: "show", Aliases: []string{}, MetadataSource: sh.Source, EpisodeOrder: &order, KeepUpToDate: true, NewPartsPolicy: "all",
		ReleaseGate: library.GateReleased, MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, State: "active", GateDriver: "none", SeasonChoice: &choice}
	applyShow(&ti, sh)
	ti.RefreshedAt = &now
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := database.Insert(tx, "titles", &ti, domain.System); err != nil {
			return err
		}
		var pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.GoodEverydayName).Scan(&pr); err != nil {
			return err
		}
		if _, err := database.Insert(tx, "versions", &mediafiles.Version{TitleID: ti.ID, Label: "Main", PickRulesID: pr, LibraryFolderID: lf.ID,
			FolderName: "Lab Show", KeepUpToDate: true, IsMain: true}, domain.System); err != nil {
			return err
		}
		_, err := syncShowTx(ctx, tx, ti, sh, defs, choice, now, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return ti
}

type epRow struct {
	ItemID  int64
	Name    string
	Keep    bool
	Abs     *int
	SlotID  int64
	State   mediafiles.SlotState
	Reason  string
	Dropped bool
	IDs     map[string]string
}

// episodes reads a show's episodes by "SxxEyy".
func episodes(t *testing.T, s *database.Store, titleID int64) map[string]epRow {
	t.Helper()
	ctx := context.Background()
	its, err := database.Select[mediafiles.Item](ctx, s.Read(), "items", "title_id = ? AND kind = 'episode'", titleID)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]epRow{}
	for _, it := range its {
		r := epRow{ItemID: it.ID, Keep: it.KeepUpToDate, Abs: it.Numbers.Absolute, Dropped: it.DroppedAt != nil, IDs: it.SourceIDs}
		if it.Name != nil {
			r.Name = *it.Name
		}
		sl, _ := database.Select[mediafiles.Slot](ctx, s.Read(), "slots", "item_id = ?", it.ID)
		if len(sl) == 1 {
			r.SlotID, r.State = sl[0].ID, sl[0].State
			if sl[0].LastReason != nil {
				r.Reason = *sl[0].LastReason
			}
		}
		out[key(*it.Numbers.Season, *it.Numbers.Episode)] = r
	}
	return out
}

func key(s, e int) string { return "S" + two(s) + "E" + two(e) }
func two(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func showService(s *database.Store, sources metadata.ShowSources) *Service {
	return New(&library.Service{Store: s, Shows: sources, Now: func() time.Time { return now }})
}

func TestShowRefreshFromTMDB(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	tmdb := &fakeShows{}
	sh := labShow("tmdb", "100")
	tmdb.set(sh, "tmdb:100")
	ti := seedShow(t, s, sh, library.SeasonsAll)
	svc := showService(s, metadata.ShowSources{"tmdb": tmdb})

	eps := episodes(t, s, ti.ID)
	if len(eps) != 6 {
		t.Fatalf("episodes %v", eps)
	}
	// The air-date gate: aired episodes are wanted, the rest not out yet.
	for k, want := range map[string]mediafiles.SlotState{"S00E01": mediafiles.SlotWanted, "S01E01": mediafiles.SlotWanted, "S02E01": mediafiles.SlotWanted,
		"S02E02": mediafiles.SlotNotOutYet, "S02E03": mediafiles.SlotNotOutYet} {
		if eps[k].State != want {
			t.Errorf("%s state %s, want %s (%s)", k, eps[k].State, want, eps[k].Reason)
		}
	}
	if eps["S02E03"].Reason != "no air date yet" || eps["S02E02"].Reason != "airs 2026-10-19 02:00 UTC" || eps["S01E01"].Reason != "aired 2025-01-06 02:00 UTC" {
		t.Fatalf("reasons %+v", eps)
	}

	// The source changes: S01E02 renamed, S02E01 moved a week later, a new
	// S02E04, S02E03 gone (nothing holds it), S01E01 gone but it has a File.
	err := s.Write(ctx, func(tx *sql.Tx) error {
		f := &mediafiles.File{SlotIDs: []int64{eps["S01E01"].SlotID}, VersionID: 1, RelativePath: "Lab Show/Season 01/a.mkv", Languages: []string{},
			Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		if _, err := database.Insert(tx, "files", f, domain.System); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET file_id = ? WHERE id = ?", f.ID, eps["S01E01"].SlotID); err != nil {
			return err
		}
		return mediafiles.MoveSlotTx(tx, eps["S01E01"].SlotID, mediafiles.SlotSearching, "", domain.System)
	})
	if err != nil {
		t.Fatal(err)
	}
	sh2 := labShow("tmdb", "100")
	sh2.Episodes = []metadata.Episode{sh.Episodes[0],
		{Season: 1, Number: 2, Name: "Second, renamed", AirDate: tm("2025-01-13T02:00:00Z"), AirDay: "2025-01-12", ID: "tmdb-102"},
		{Season: 2, Number: 1, Name: "Return", AirDate: tm("2026-10-19T02:00:00Z"), AirDay: "2026-10-18", ID: "tmdb-201"},
		sh.Episodes[4],
		{Season: 2, Number: 4, Name: "Later", AirDate: tm("2026-11-02T02:00:00Z"), AirDay: "2026-11-01", ID: "tmdb-204"}}
	tmdb.set(sh2, "tmdb:100")
	out, err := svc.RefreshShow(ctx, ti.ID, domain.System, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.EpisodesAdded != 1 || out.EpisodesRemoved != 1 || out.EpisodesDropped != 1 || len(out.Closed) != 1 || out.Source != "tmdb" {
		t.Fatalf("refresh %+v", out)
	}
	after := episodes(t, s, ti.ID)
	if _, ok := after["S02E03"]; ok {
		t.Fatal("an episode nothing holds should be removed")
	}
	if !after["S01E01"].Dropped || after["S01E01"].SlotID != eps["S01E01"].SlotID {
		t.Fatalf("held episode %+v", after["S01E01"])
	}
	if after["S01E02"].Name != "Second, renamed" || after["S01E02"].ItemID != eps["S01E02"].ItemID {
		t.Fatalf("renamed %+v", after["S01E02"])
	}
	if after["S02E01"].State != mediafiles.SlotNotOutYet || after["S02E04"].State != mediafiles.SlotNotOutYet || !after["S02E04"].Keep {
		t.Fatalf("moved and new %+v %+v", after["S02E01"], after["S02E04"])
	}
	evs, _, _ := s.Events(ctx, database.EventFilter{TitleID: ti.ID, Limit: 500})
	kinds := map[string]int{}
	for _, e := range evs {
		kinds[e.Kind]++
	}
	if kinds[domain.EventTitleRefreshed] != 1 || kinds[domain.EventEpisodesChanged] != 1 || kinds[domain.EventGateClosed] != 1 {
		t.Fatalf("events %v", kinds)
	}

	// The source forgets the show: flagged, never deleted.
	tmdb.set(metadata.Show{}, "nothing")
	delete(tmdb.shows, "tmdb:100")
	if out, err := svc.RefreshShow(ctx, ti.ID, domain.System, nil); err != nil || !out.Dropped {
		t.Fatalf("forgotten %+v %v", out, err)
	}
	var back mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", ti.ID, &back)
	if back.DroppedAt == nil || len(episodes(t, s, ti.ID)) != 6 {
		t.Fatalf("forgotten show %+v", back)
	}
}

func TestSwitchShowSource(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	tmdb, maze := &fakeShows{}, &fakeShows{}
	sh := labShow("tmdb", "100")
	tmdb.set(sh, "tmdb:100")
	ms := labShow("tvmaze", "300")
	// TVMaze calls the second episode by another name and has one more.
	ms.Episodes[2].Name = "Second (TVMaze)"
	ms.Episodes = append(ms.Episodes, metadata.Episode{Season: 2, Number: 4, Name: "Extra", AirDate: tm("2026-11-02T02:00:00Z"), AirDay: "2026-11-01", ID: "tvmaze-204"})
	maze.set(ms, "tvdb:200", "tvmaze:300") // TVMaze finds it by its TVDB id
	ti := seedShow(t, s, sh, library.SeasonsAll)
	before := episodes(t, s, ti.ID)
	svc := showService(s, metadata.ShowSources{"tmdb": tmdb, "tvmaze": maze, "tvdb": &fakeShows{err: metadata.ErrNoTVDBKey}})

	out, err := svc.SwitchSource(ctx, ti.ID, "tvmaze", domain.System)
	if err != nil || out.Source != "tvmaze" || out.EpisodesAdded != 1 {
		t.Fatalf("switch %+v %v", out, err)
	}
	if maze.calls[0] != "tvdb:200" {
		t.Fatalf("TVMaze asked %v; the TVDB id comes first when it has no id of its own", maze.calls)
	}
	var back mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", ti.ID, &back)
	if back.MetadataSource != "tvmaze" || back.SourceIDs["tvmaze"] != "300" || back.SourceIDs["tmdb"] != "100" {
		t.Fatalf("title after switch %+v", back)
	}
	after := episodes(t, s, ti.ID)
	for k, b := range before {
		if after[k].ItemID != b.ItemID || after[k].SlotID != b.SlotID {
			t.Errorf("%s changed rows: %+v -> %+v", k, b, after[k])
		}
	}
	if after["S01E02"].Name != "Second (TVMaze)" || after["S01E02"].IDs["tmdb"] != "tmdb-102" || after["S01E02"].IDs["tvmaze"] != "tvmaze-102" {
		t.Fatalf("episode after switch %+v", after["S01E02"])
	}
	// The next refresh asks TVMaze by its own id.
	maze.calls = nil
	if _, err := svc.RefreshShow(ctx, ti.ID, domain.System, nil); err != nil || maze.calls[0] != "tvmaze:300" || len(tmdb.calls) != 0 {
		t.Fatalf("refresh after switch: %v maze %v tmdb %v", err, maze.calls, tmdb.calls)
	}

	// TVDB is off: refused, nothing changes.
	if _, err := svc.SwitchSource(ctx, ti.ID, "tvdb", domain.System); !errors.Is(err, metadata.ErrUnknownSource) {
		t.Fatalf("tvdb is off: %v", err)
	}
	// A source that does not know the show: refused, nothing changes.
	if _, err := svc.SwitchSource(ctx, ti.ID, "tmdb", domain.System); err != nil {
		t.Fatalf("back to tmdb: %v", err)
	}
	delete(maze.shows, "tvdb:200")
	delete(maze.shows, "tvmaze:300")
	if _, err := svc.SwitchSource(ctx, ti.ID, "tvmaze", domain.System); !errors.Is(err, metadata.ErrShowNotFound) {
		t.Fatalf("unknown on tvmaze: %v", err)
	}
	_ = database.Get(ctx, s.Read(), "titles", ti.ID, &back)
	if back.MetadataSource != "tmdb" || back.DroppedAt != nil {
		t.Fatalf("after a refused switch %+v", back)
	}
	if _, err := svc.SwitchSource(ctx, ti.ID, "musicbrainz", domain.System); !errors.Is(err, metadata.ErrUnknownSource) {
		t.Fatalf("bad source: %v", err)
	}
}

// TestAbsoluteNumbers covers the episode order data: the source's absolute
// numbers win; without them regular episodes are counted in aired order
// across seasons; specials never get one.
func TestAbsoluteNumbers(t *testing.T) {
	e := func(s, n, a int) metadata.Episode { return metadata.Episode{Season: s, Number: n, Absolute: a} }
	cases := []struct {
		name string
		eps  []metadata.Episode
		want map[[2]int]int
	}{
		{"counted across seasons", []metadata.Episode{e(2, 1, 0), e(1, 2, 0), e(0, 1, 0), e(1, 1, 0)},
			map[[2]int]int{{1, 1}: 1, {1, 2}: 2, {2, 1}: 3}},
		{"source numbers kept", []metadata.Episode{e(1, 1, 1), e(1, 2, 2), e(2, 1, 13), e(0, 1, 0)},
			map[[2]int]int{{1, 1}: 1, {1, 2}: 2, {2, 1}: 13}},
		{"partly given: only given ones", []metadata.Episode{e(1, 1, 1), e(1, 2, 0)},
			map[[2]int]int{{1, 1}: 1}},
		{"specials only", []metadata.Episode{e(0, 1, 0), e(0, 2, 0)}, map[[2]int]int{}},
	}
	for _, c := range cases {
		got := absolutes(c.eps)
		if len(got) != len(c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
			continue
		}
		for k, v := range c.want {
			if got[k] != v {
				t.Errorf("%s: %v = %d, want %d", c.name, k, got[k], v)
			}
		}
	}
}

// A special added before the others shifts their numbers: each row, Slot
// and File stays on its own special, matched by the source's episode id.
func TestRefreshKeepsSpecialsByEpisodeID(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	src := &fakeShows{}
	sh := labShow("tvmaze", "300")
	sh.Episodes = append(sh.Episodes, metadata.Episode{Season: 0, Number: 2, Name: "Lab Tour", AirDate: tm("2025-04-01T12:00:00Z"), AirDay: "2025-04-01", ID: "tvmaze-s2"})
	src.set(sh, "tvmaze:300")
	ti := seedShow(t, s, sh, library.SeasonsAll)
	svc := showService(s, metadata.ShowSources{"tvmaze": src})
	eps := episodes(t, s, ti.ID)
	err := s.Write(ctx, func(tx *sql.Tx) error {
		f := &mediafiles.File{SlotIDs: []int64{eps["S00E01"].SlotID}, VersionID: 1, RelativePath: "Lab Show/Specials/behind.mkv", Languages: []string{},
			Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		if _, err := database.Insert(tx, "files", f, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE slots SET file_id = ?, state = 'in_library' WHERE id = ?", f.ID, eps["S00E01"].SlotID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	sh2 := labShow("tvmaze", "300")
	sh2.Episodes = append([]metadata.Episode{{Season: 0, Number: 1, Name: "Early Look", AirDate: tm("2025-02-01T12:00:00Z"), AirDay: "2025-02-01", ID: "tvmaze-s0"}},
		sh2.Episodes...)
	sh2.Episodes[1].Number = 2
	sh2.Episodes = append(sh2.Episodes, metadata.Episode{Season: 0, Number: 3, Name: "Lab Tour", AirDate: tm("2025-04-01T12:00:00Z"), AirDay: "2025-04-01", ID: "tvmaze-s2"})
	src.set(sh2, "tvmaze:300")
	if _, err := svc.RefreshShow(ctx, ti.ID, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	after := episodes(t, s, ti.ID)
	if after["S00E02"].ItemID != eps["S00E01"].ItemID || after["S00E02"].SlotID != eps["S00E01"].SlotID || after["S00E02"].Name != "Behind the Lab" {
		t.Fatalf("special with a File moved: %+v (was %+v)", after["S00E02"], eps["S00E01"])
	}
	if after["S00E03"].ItemID != eps["S00E02"].ItemID || after["S00E03"].Name != "Lab Tour" {
		t.Fatalf("second special %+v", after["S00E03"])
	}
	if n := after["S00E01"]; n.ItemID == eps["S00E01"].ItemID || n.ItemID == eps["S00E02"].ItemID || n.Name != "Early Look" || n.State == mediafiles.SlotInLibrary {
		t.Fatalf("new special %+v", n)
	}
	var fileSlot int64
	_ = s.Read().QueryRow("SELECT id FROM slots WHERE file_id IS NOT NULL").Scan(&fileSlot)
	if fileSlot != after["S00E02"].SlotID {
		t.Fatalf("File on slot %d, want %d", fileSlot, after["S00E02"].SlotID)
	}
}

// A searching or held episode the source no longer lists is kept and
// flagged, never deleted.
func TestRefreshKeepsSearchingAndHeldEpisodes(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	src := &fakeShows{}
	sh := labShow("tmdb", "100")
	src.set(sh, "tmdb:100")
	ti := seedShow(t, s, sh, library.SeasonsAll)
	svc := showService(s, metadata.ShowSources{"tmdb": src})
	eps := episodes(t, s, ti.ID)
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE slots SET state = 'searching' WHERE id = ?", eps["S02E01"].SlotID); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE slots SET state = 'held' WHERE id = ?", eps["S01E02"].SlotID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	sh2 := labShow("tmdb", "100")
	sh2.Episodes = []metadata.Episode{sh.Episodes[0], sh.Episodes[1], sh.Episodes[4], sh.Episodes[5]}
	src.set(sh2, "tmdb:100")
	if _, err := svc.RefreshShow(ctx, ti.ID, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	after := episodes(t, s, ti.ID)
	for _, k := range []string{"S02E01", "S01E02"} {
		if r, ok := after[k]; !ok || !r.Dropped {
			t.Fatalf("%s should be kept and flagged: %+v %v", k, r, ok)
		}
	}
}

// TestFutureSeasonsOnlyRefresh: a show added with Future seasons only
// keeps wanting only what airs after it was added, and a new season the
// source lists later is wanted.
func TestFutureSeasonsOnlyRefresh(t *testing.T) {
	svc, s, tmdb, _ := addShowService(t)
	ctx := context.Background()
	addShowFolder(t, s)
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{Seasons: library.SeasonsFuture}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var ti mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", id, &ti)
	if ti.FutureFrom == nil || !ti.FutureFrom.Equal(now) {
		t.Fatalf("future_from %v", ti.FutureFrom)
	}
	sh := labShow("tmdb", "100")
	sh.Seasons = append(sh.Seasons, metadata.Season{Number: 3, Name: "Season 3"})
	sh.Episodes = append(sh.Episodes,
		metadata.Episode{Season: 1, Number: 3, Name: "Found late", AirDate: tm("2025-01-20T02:00:00Z"), ID: "tmdb-103"},
		metadata.Episode{Season: 3, Number: 1, Name: "New season", AirDate: tm("2027-01-10T02:00:00Z"), ID: "tmdb-301"})
	tmdb.set(sh, "tmdb:100", "tvdb:200", "imdb:tt0100")
	if _, err := svc.RefreshShow(ctx, id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	eps := episodes(t, s, id)
	if eps["S01E03"].Keep || !eps["S03E01"].Keep || eps["S02E01"].Keep || !eps["S02E02"].Keep {
		t.Fatalf("keep S01E03 %v S03E01 %v S02E01 %v S02E02 %v", eps["S01E03"].Keep, eps["S03E01"].Keep, eps["S02E01"].Keep, eps["S02E02"].Keep)
	}
	ss, _ := Seasons(ctx, s.Read(), id, now)
	for _, se := range ss {
		if want := se.Number >= 2; se.KeepUpToDate != want {
			t.Errorf("season %d keep %v", se.Number, se.KeepUpToDate)
		}
	}
}

// TestKeepFutureOnExistingShow: a show already in the Library switched to
// Future seasons only keeps what it kept, and now also wants episodes
// still to air and new seasons.
func TestKeepFutureOnExistingShow(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	addShowFolder(t, s)
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{Seasons: library.SeasonsNone, NewSeasons: "none"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.KeepFuture(ctx, id, domain.System); err != nil {
		t.Fatal(err)
	}
	eps := episodes(t, s, id)
	if eps["S01E01"].Keep || eps["S02E01"].Keep || !eps["S02E02"].Keep || !eps["S02E03"].Keep {
		t.Fatalf("eps %+v", eps)
	}
	var ti mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", id, &ti)
	if !ti.KeepUpToDate || ti.NewPartsPolicy != "all" || ti.FutureFrom == nil || *ti.SeasonChoice != library.SeasonsFuture {
		t.Fatalf("title %+v", ti)
	}
	ss, _ := Seasons(ctx, s.Read(), id, now)
	for _, se := range ss {
		if want := se.Number == 2; se.KeepUpToDate != want {
			t.Errorf("season %d keep %v", se.Number, se.KeepUpToDate)
		}
	}
}

// The release-name numbers from the anime map are stored on the episode; a
// refresh without them clears them, unless the map could not be read.
func TestRefreshStoresReleaseNumbers(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	tmdb := &fakeShows{}
	withAlt := labShow("tmdb", "100")
	withAlt.Episodes[2].AltSeason, withAlt.Episodes[2].AltEpisode = 2, 5 // S01E02
	tmdb.set(withAlt, "tmdb:100")
	ti := seedShow(t, s, withAlt, library.SeasonsAll)
	svc := showService(s, metadata.ShowSources{"tmdb": tmdb})
	alt := func() [2]int {
		t.Helper()
		its, err := database.Select[mediafiles.Item](ctx, s.Read(), "items", "title_id = ? AND kind = 'episode'", ti.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range its {
			if *it.Numbers.Season == 1 && *it.Numbers.Episode == 2 {
				var a [2]int
				if it.Numbers.AltSeason != nil {
					a[0] = *it.Numbers.AltSeason
				}
				if it.Numbers.AltEpisode != nil {
					a[1] = *it.Numbers.AltEpisode
				}
				return a
			}
		}
		t.Fatal("no S01E02")
		return [2]int{}
	}
	refresh := func(sh metadata.Show) {
		t.Helper()
		tmdb.set(sh, "tmdb:100")
		if _, err := svc.RefreshShow(ctx, ti.ID, domain.System, nil); err != nil {
			t.Fatal(err)
		}
	}
	if a := alt(); a != [2]int{2, 5} {
		t.Fatalf("after add: %v", a)
	}
	refresh(labShow("tmdb", "100"))
	if a := alt(); a != [2]int{} {
		t.Fatalf("after a refresh without them: %v", a)
	}
	refresh(withAlt)
	stale := labShow("tmdb", "100")
	stale.AltStale = true
	refresh(stale)
	if a := alt(); a != [2]int{2, 5} {
		t.Fatalf("after a refresh that could not read the map: %v", a)
	}
}
