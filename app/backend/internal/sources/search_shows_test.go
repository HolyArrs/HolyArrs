package sources

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

const tvCaps = `<?xml version="1.0" encoding="UTF-8"?>
<caps><limits max="50" default="25"/><searching>
<search available="yes" supportedParams="q"/>
<tv-search available="yes" supportedParams="q,season,ep,tvdbid"/>
<movie-search available="yes" supportedParams="q,imdbid,tmdbid"/>
</searching><categories><category id="2000" name="Movies"/><category id="5000" name="TV"/></categories></caps>`

// tvRSS lists releases. A name may end in |<size in MB>, default 1200 for an
// episode and 3600 for a season.
func tvRSS(names ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0" xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>`)
	for i, n := range names {
		size := int64(1200)
		if strings.Contains(n, ".S01.") || strings.Contains(n, ".S02.") {
			size = 3600
		}
		if name, mb, ok := strings.Cut(n, "|"); ok {
			n = name
			fmt.Sscan(mb, &size)
		}
		fmt.Fprintf(&b, `<item><title>%s</title><pubDate>%s</pubDate>
<enclosure url="http://src/%d.torrent" length="%d" type="application/x-bittorrent"/>
<torznab:attr name="seeders" value="50"/></item>`, n, time.Now().UTC().Add(-time.Hour).Format(time.RFC1123Z), i, size<<20)
	}
	b.WriteString(`</channel></rss>`)
	return b.String()
}

// tvServer answers caps and searches. answer picks the releases for each
// search's query, and every query is kept.
type tvServer struct {
	mu      sync.Mutex
	queries []url.Values
	answer  func(q url.Values) []string
}

func (ts *tvServer) start(t *testing.T) string {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("t") == "caps" {
			_, _ = w.Write([]byte(tvCaps))
			return
		}
		ts.mu.Lock()
		ts.queries = append(ts.queries, q)
		ts.mu.Unlock()
		_, _ = w.Write([]byte(tvRSS(ts.answer(q)...)))
	})
	return srv.URL + "/api"
}

// showFix is the show fixture.
type showFix struct {
	fixture
	titleID, verID int64
	parts          map[int]int64
	slots          map[string]int64
	items          map[string]int64
	states         map[string]mediafiles.SlotState
}

// seedShow adds a show with season 1 of three aired episodes and season 2 of
// one aired and one airing next month. Every Slot is wanted or not out yet,
// kept up to date, with one Source for shows. A movie is there too.
func seedShow(t *testing.T, srcURL string, set func(label string, sl *mediafiles.Slot)) showFix {
	t.Helper()
	f := showFix{fixture: seed(t, srcURL), parts: map[int]int64{}, slots: map[string]int64{}, items: map[string]int64{}}
	now := time.Now().UTC()
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr, lf int64
		if err := tx.QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.GoodEverydayName).Scan(&pr); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT id FROM library_folders LIMIT 1").Scan(&lf); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE sources SET kinds = '["movie","tv"]', categories = '{"movie":[2000],"tv":[5000]}', use_for = '{"auto_search":true,"hand_search":true,"feed":true}' WHERE id = ?`, f.srcID); err != nil {
			return err
		}
		ti := &mediafiles.Title{Kind: "show", GateDriver: "none", Name: "Copper Valley", SortName: "copper valley", Year: ptr(2015),
			Aliases: []string{}, SourceIDs: map[string]string{"tvdb": "4242"}, MetadataSource: "tvmaze", EpisodeOrder: ptr("aired"),
			KeepUpToDate: true, NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand",
			Artwork: []string{}, Status: "running", State: "active", OriginalLanguage: ptr("en")}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		v := &mediafiles.Version{TitleID: ti.ID, Label: "HD", PickRulesID: pr, LibraryFolderID: lf, FolderName: "Copper Valley", KeepUpToDate: true, IsMain: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		f.titleID, f.verID = ti.ID, v.ID
		eps := map[int][]time.Time{
			1: {now.AddDate(0, -3, 0), now.AddDate(0, -3, 7), now.AddDate(0, -3, 14)},
			2: {now.AddDate(0, 0, -7), now.AddDate(0, 1, 0)},
		}
		for season := 1; season <= 2; season++ {
			p := &mediafiles.Part{TitleID: ti.ID, Kind: "season", Number: ptr(season), Name: fmt.Sprintf("Season %d", season), KeepUpToDate: true,
				SourceIDs: map[string]string{}, Pressings: []mediafiles.Pressing{}}
			if _, err := database.Insert(tx, "parts", p, domain.System); err != nil {
				return err
			}
			f.parts[season] = p.ID
			for i, air := range eps[season] {
				n := i + 1
				air := air
				day := air.Format("2006-01-02")
				it := &mediafiles.Item{TitleID: ti.ID, PartID: &p.ID, Kind: "episode", Numbers: mediafiles.Numbers{Season: ptr(season), Episode: ptr(n)},
					Name: ptr(fmt.Sprintf("Episode %d", n)), AirDate: &air, AirDay: &day, Runtime: ptr(45), KeepUpToDate: true, SourceIDs: map[string]string{}}
				if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
					return err
				}
				label := fmt.Sprintf("S%02dE%02d", season, n)
				sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotWanted}
				if air.After(now) {
					sl.State = mediafiles.SlotNotOutYet
				}
				if set != nil {
					set(label, sl)
				}
				if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
					return err
				}
				f.slots[label], f.items[label] = sl.ID, it.ID
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f showFix) state(t *testing.T, label string) mediafiles.SlotState {
	t.Helper()
	var st string
	if err := f.s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", f.slots[label]).Scan(&st); err != nil {
		t.Fatal(err)
	}
	return mediafiles.SlotState(st)
}

func reasonCodes(rel domain.Release) []string {
	var out []string
	rs, _ := rel.Verdict["reasons"].([]any)
	for _, x := range rs {
		m, _ := x.(map[string]any)
		out = append(out, fmt.Sprint(m["step"], ".", m["rule"], ":", m["effect"]))
	}
	return out
}

func hasCode(rel domain.Release, code string) bool {
	for _, c := range reasonCodes(rel) {
		if strings.HasPrefix(c, code+":") {
			return true
		}
	}
	return false
}

func TestSearch_Episode(t *testing.T) {
	ts := &tvServer{answer: func(q url.Values) []string {
		return []string{
			"Copper.Valley.S01E02.1080p.WEB-DL.H.264-QUILL",
			"Copper.Valley.S01E02.720p.HDTV.x264-OXBOW",
			"Copper.Valley.S01E03.1080p.WEB-DL.H.264-QUILL",
			"Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL",
			"Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS",
		}
	}}
	f := seedShow(t, ts.start(t), nil)
	var chosen []int64
	sr := &Searcher{Store: f.s, OnChosen: func(_ context.Context, slotID, releaseID int64) { chosen = append(chosen, slotID) }}
	j := runKind(t, f.fixture, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S01E02"]}, nil)
	if j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	q := ts.queries[0]
	if q.Get("t") != "tvsearch" || q.Get("tvdbid") != "4242" || q.Get("season") != "1" || q.Get("ep") != "2" || q.Get("cat") != "5000" {
		t.Fatalf("query %v", q)
	}
	if st := f.state(t, "S01E02"); st != mediafiles.SlotChosen {
		t.Fatalf("S01E02 is %s", st)
	}
	if st := f.state(t, "S01E03"); st != mediafiles.SlotWanted {
		t.Fatalf("S01E03 is %s: an episode search picks only its episode", st)
	}
	if r := relByName(t, f.fixture, "Copper.Valley.S01E02.1080p.WEB-DL.H.264-QUILL"); r.State != "chosen" {
		t.Fatalf("winner %s %v", r.State, reasonCodes(r))
	}
	if r := relByName(t, f.fixture, "Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL"); !hasCode(r, "match.season_pack") || r.State != "refused" {
		t.Fatalf("pack in an episode search: %s %v", r.State, reasonCodes(r))
	}
	if r := relByName(t, f.fixture, "Copper.Valley.S01E03.1080p.WEB-DL.H.264-QUILL"); !hasCode(r, "match.not_asked") {
		t.Fatalf("other episode: %v", reasonCodes(r))
	}
	if r := relByName(t, f.fixture, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS"); !hasCode(r, "match.none") {
		t.Fatalf("a movie in a show search: %v", reasonCodes(r))
	}
	if len(chosen) != 1 || chosen[0] != f.slots["S01E02"] {
		t.Fatalf("OnChosen %v", chosen)
	}
}

func TestHandleSeason_EpisodeNotAiredSearchedOnlyByHand(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return []string{"Copper.Valley.S02E02.1080p.WEB-DL.H.264-QUILL"} }}
	f := seedShow(t, ts.start(t), nil)
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S02E02"]}, nil)
	if j.Result["skipped"] == nil || len(ts.queries) != 0 {
		t.Fatalf("an episode not aired yet was searched: %+v", j.Result)
	}
	j = runKind(t, f.fixture, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S02E02"]}, map[string]any{"by_hand": true})
	if j.State != "done" || f.state(t, "S02E02") != mediafiles.SlotChosen {
		t.Fatalf("hand search %s %+v, slot %s", j.State, j.Result, f.state(t, "S02E02"))
	}
	r := relByName(t, f.fixture, "Copper.Valley.S02E02.1080p.WEB-DL.H.264-QUILL")
	if !hasCode(r, "hard.gate_skipped") {
		t.Fatalf("the skipped gate is not said: %v", reasonCodes(r))
	}
}

func TestHandleSeason_TakesPack(t *testing.T) {
	ts := &tvServer{answer: func(q url.Values) []string {
		return []string{
			"Copper.Valley.S01E01.1080p.WEB-DL.H.264-QUILL",
			"Copper.Valley.S01E02.1080p.WEB-DL.H.264-QUILL",
			"Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL",
			"Copper.Valley.S01-S02.1080p.WEB-DL.H.264-QUILL|6000",
		}
	}}
	f := seedShow(t, ts.start(t), nil)
	var grabs [][2]int64
	sr := &Searcher{Store: f.s, OnChosen: func(_ context.Context, slotID, releaseID int64) { grabs = append(grabs, [2]int64{slotID, releaseID}) }}
	j := runKind(t, f.fixture, sr, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID})
	if j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	if q := ts.queries[0]; q.Get("t") != "tvsearch" || q.Get("season") != "1" || q.Has("ep") {
		t.Fatalf("season query %v", q)
	}
	for _, l := range []string{"S01E01", "S01E02", "S01E03"} {
		if st := f.state(t, l); st != mediafiles.SlotChosen {
			t.Fatalf("%s is %s; result %+v", l, st, j.Result)
		}
	}
	pack := relByName(t, f.fixture, "Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL")
	if pack.State != "chosen" || len(PickSlots(pack)) != 3 {
		t.Fatalf("pack %s %v %v", pack.State, PickSlots(pack), reasonCodes(pack))
	}
	if len(grabs) != 1 || grabs[0][1] != pack.ID {
		t.Fatalf("one Download for the season: %v", grabs)
	}
	if r := relByName(t, f.fixture, "Copper.Valley.S01-S02.1080p.WEB-DL.H.264-QUILL"); !hasCode(r, "match.several_seasons") {
		t.Fatalf("several seasons: %v", reasonCodes(r))
	}
	if r := relByName(t, f.fixture, "Copper.Valley.S01E01.1080p.WEB-DL.H.264-QUILL"); r.State != "passed_over" {
		t.Fatalf("single episode beside the pack: %s", r.State)
	}
	if j.Result["chosen_release_id"] == nil {
		t.Fatalf("result %+v", j.Result)
	}
}

func TestHandleSeason_FallsBackToEpisodes(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), func(label string, sl *mediafiles.Slot) {
		if label == "S01E01" {
			sl.State = mediafiles.SlotInLibrary
		}
	})
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID})
	if n, _ := j.Result["episodes_queued"].(float64); n != 2 {
		t.Fatalf("result %+v", j.Result)
	}
	var queued int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ? AND json_extract(subject, '$.id') IN (?, ?)", jobs.KindSearchSlot,
		f.slots["S01E02"], f.slots["S01E03"]).Scan(&queued)
	if queued != 2 {
		t.Fatalf("queued %d episode searches", queued)
	}
}

func TestHandleSeason_LeftoversSearchedAlone(t *testing.T) {
	ts := &tvServer{answer: func(q url.Values) []string {
		if q.Get("ep") == "" {
			return []string{"Copper.Valley.S01E01.1080p.WEB-DL.H.264-QUILL"}
		}
		return nil
	}}
	f := seedShow(t, ts.start(t), nil)
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID})
	if f.state(t, "S01E01") != mediafiles.SlotChosen {
		t.Fatalf("S01E01 %s %+v", f.state(t, "S01E01"), j.Result)
	}
	if n, _ := j.Result["episodes_queued"].(float64); n != 2 {
		t.Fatalf("result %+v", j.Result)
	}
}

func TestFeedShow_PackRules(t *testing.T) {
	feed := []string{
		"Copper.Valley.S02.1080p.WEB-DL.H.264-QUILL",
		"Copper.Valley.S02E01.1080p.WEB-DL.H.264-QUILL",
		"Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL",
	}
	ts := &tvServer{answer: func(url.Values) []string { return feed }}
	var fileSlot int64
	f := seedShow(t, ts.start(t), func(label string, sl *mediafiles.Slot) {
		if label == "S01E01" {
			sl.State = mediafiles.SlotInLibrary
		}
	})
	fileSlot = f.slots["S01E01"]
	addFile(t, f, fileSlot, "Copper.Valley.S01E01.1080p.WEB-DL.H.264-OTHER")
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, jobs.KindFeedCheck, nil, nil)
	if j.State != "done" {
		t.Fatalf("feed %s %+v", j.State, j.Error)
	}
	if q := ts.queries[0]; q.Get("cat") != "2000,5000" {
		t.Fatalf("feed categories %v", q)
	}
	if r := relByName(t, f.fixture, "Copper.Valley.S02.1080p.WEB-DL.H.264-QUILL"); !hasCode(r, "hard.gate") || r.State != "refused" {
		t.Fatalf("airing season pack: %s %v", r.State, reasonCodes(r))
	}
	if st := f.state(t, "S02E01"); st != mediafiles.SlotChosen {
		t.Fatalf("S02E01 %s", st)
	}
	pack := relByName(t, f.fixture, "Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL")
	if !hasCode(pack, "match.pack_share") || pack.State != "refused" || f.state(t, "S01E02") != mediafiles.SlotWanted {
		t.Fatalf("pack over a copy as good: %s %v", pack.State, reasonCodes(pack))
	}

	setSetting(t, f.fixture, SettingPackUpgrade, "any")
	feed = []string{"Copper.Valley.S01.1080p.WEB-DL.H.265-LARKSPUR"}
	j = runKind(t, f.fixture, sr, jobs.KindFeedCheck, nil, nil)
	pack = relByName(t, f.fixture, "Copper.Valley.S01.1080p.WEB-DL.H.265-LARKSPUR")
	if pack.State != "chosen" || f.state(t, "S01E02") != mediafiles.SlotChosen || f.state(t, "S01E03") != mediafiles.SlotChosen {
		t.Fatalf("partial pack: %s %v, %s %s", pack.State, reasonCodes(pack), f.state(t, "S01E02"), f.state(t, "S01E03"))
	}
	if f.state(t, "S01E01") != mediafiles.SlotInLibrary || len(PickSlots(pack)) != 2 {
		t.Fatalf("the episode with a copy is left alone: %s %v", f.state(t, "S01E01"), PickSlots(pack))
	}
}

func addFile(t *testing.T, f showFix, slotID int64, name string) {
	t.Helper()
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		fl := &mediafiles.File{SlotIDs: []int64{slotID}, VersionID: f.verID, RelativePath: "Season 01/" + name + ".mkv", Size: 1200 << 20,
			AddedAt: time.Now().Add(-48 * time.Hour), Languages: []string{}, MediaInfo: map[string]any{}, ReleaseName: &name, Origin: "imported",
			PlacedBy: "move", State: "present", Role: "main"}
		if _, err := database.Insert(tx, "files", fl, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE slots SET file_id = ? WHERE id = ?", fl.ID, slotID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func setSetting(t *testing.T, f fixture, key, value string) {
	t.Helper()
	now := database.FormatTime(time.Now())
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('what_to_pick', ?, ?, ?, '{}', ?, '{}')
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value`, key, value, now, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestHandleSeason_TurnedDownForOneEpisode(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return []string{"Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL"} }}
	f := seedShow(t, ts.start(t), nil)
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID, "choose": false})
	if j.Result["shown"] != true {
		t.Fatalf("show only: %+v", j.Result)
	}
	pack := relByName(t, f.fixture, "Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL")
	if pack.State != "judged" || len(pack.MatchedSlotIDs) != 3 {
		t.Fatalf("pack %s %v", pack.State, pack.MatchedSlotIDs)
	}
	ctx := context.Background()
	if err := sr.TurnDownFor(ctx, pack.ID, f.slots["S01E02"], "bad audio on E02", domain.System); err != nil {
		t.Fatal(err)
	}
	td2, _ := sr.turnedDown(ctx, f.slots["S01E02"])
	td3, _ := sr.turnedDown(ctx, f.slots["S01E03"])
	if !slices.Contains(td2, pack.Name) || slices.Contains(td3, pack.Name) {
		t.Fatalf("turned down per episode: E02 %v, E03 %v", td2, td3)
	}
	set, err := sr.loadShow(ctx, mustTitle(t, f), mustVersion(t, f))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range []string{"S01E01", "S01E02", "S01E03"} {
		set.target[f.slots[l]] = true
	}
	_, vs, _ := sr.judgeShow(ctx, set, []found{{src: domain.Source{ID: f.srcID}, item: Item{Title: pack.Name, Size: 3600 << 20, Protocol: "torrent", Published: time.Now().Add(-time.Hour)}}}, false, false, sr.showRules(ctx))
	if !vs[0].Passed || slices.Contains(vs[0].SlotIDs, f.slots["S01E02"]) || len(vs[0].SlotIDs) != 2 {
		t.Fatalf("judged per Slot: %+v", vs[0])
	}
	ch, err := sr.Choose(ctx, pack.ID, f.slots["S01E03"], false, domain.System)
	if err != nil || ch.Forced {
		t.Fatalf("choose for E03: %+v %v", ch, err)
	}
	var nf *NeedsForceError
	if _, err := sr.Choose(ctx, pack.ID, f.slots["S01E02"], false, domain.System); err == nil || !asNeedsForce(err, &nf) {
		t.Fatalf("choose for E02 without force: %v", err)
	}
}

func asNeedsForce(err error, nf **NeedsForceError) bool {
	e, ok := err.(*NeedsForceError)
	*nf = e
	return ok
}

func mustTitle(t *testing.T, f showFix) mediafiles.Title {
	var ti mediafiles.Title
	if err := database.Get(context.Background(), f.s.Read(), "titles", f.titleID, &ti); err != nil {
		t.Fatal(err)
	}
	return ti
}

func mustVersion(t *testing.T, f showFix) mediafiles.Version {
	var v mediafiles.Version
	if err := database.Get(context.Background(), f.s.Read(), "versions", f.verID, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestChoose_PackByHandFillsSeason(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return []string{"Copper.Valley.S01.720p.HDTV.x264-OXBOW"} }}
	f := seedShow(t, ts.start(t), nil)
	var grabs []int64
	sr := &Searcher{Store: f.s, OnChosen: func(_ context.Context, slotID, _ int64) { grabs = append(grabs, slotID) }}
	runKind(t, f.fixture, sr, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID, "choose": false, "by_hand": true})
	pack := relByName(t, f.fixture, "Copper.Valley.S01.720p.HDTV.x264-OXBOW")
	ch, err := sr.Choose(context.Background(), pack.ID, 0, false, domain.System)
	if err != nil || len(ch.SlotIDs) != 3 || len(grabs) != 1 {
		t.Fatalf("choose the pack: %+v %v %v", ch, err, grabs)
	}
	for _, l := range []string{"S01E01", "S01E02", "S01E03"} {
		if f.state(t, l) != mediafiles.SlotChosen {
			t.Fatalf("%s %s", l, f.state(t, l))
		}
	}
}

func TestMissingSearch_GroupsSeasons(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, nil)
	if j.State != "done" {
		t.Fatalf("%s %+v", j.State, j.Error)
	}
	if n, _ := j.Result["seasons"].(float64); n != 1 {
		t.Fatalf("result %+v", j.Result)
	}
	var seasons, eps int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ? AND json_extract(params, '$.version_id') = ?", jobs.KindSearchSeason, f.verID).Scan(&seasons)
	_ = f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ? AND json_extract(subject, '$.id') IN (?, ?)", jobs.KindSearchSlot, f.slots["S02E01"], f.slotID).Scan(&eps)
	var unaired int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE json_extract(subject, '$.id') = ? AND kind = ?", f.slots["S02E02"], jobs.KindSearchSlot).Scan(&unaired)
	if seasons != 1 || eps != 2 || unaired != 0 {
		t.Fatalf("seasons %d, episode and movie searches %d, unaired %d", seasons, eps, unaired)
	}

	setSetting(t, f.fixture, SettingMissingShows, "false")
	if _, err := f.s.Read().Exec("SELECT 1"); err != nil {
		t.Fatal(err)
	}
	_ = f.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM jobs WHERE kind != ?", jobs.KindSearchMissing)
		return err
	})
	j = runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, nil)
	if n, _ := j.Result["slots"].(float64); n != 1 {
		t.Fatalf("with shows off: %+v", j.Result)
	}
}

func TestMissingSearch_OneKind(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, map[string]any{"which": "missing", "kind": "movie"})
	if n, _ := j.Result["slots"].(float64); j.State != "done" || n != 1 {
		t.Fatalf("movies: %s %+v", j.State, j.Result)
	}
	var movie, show int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ? AND json_extract(subject, '$.id') = ?", jobs.KindSearchSlot, f.slotID).Scan(&movie)
	_ = f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind IN (?, ?)", jobs.KindSearchSeason, jobs.KindSearchSlot).Scan(&show)
	if movie != 1 || show != 1 {
		t.Fatalf("movie searches %d, all searches %d", movie, show)
	}
	j = runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, map[string]any{"which": "missing", "kind": "show"})
	if n, _ := j.Result["slots"].(float64); n != 4 {
		t.Fatalf("shows: %+v", j.Result)
	}
	if s, _ := j.Result["seasons"].(float64); s != 1 {
		t.Fatalf("shows: %+v", j.Result)
	}
}

func TestShowQuery_DailyEpisode(t *testing.T) {
	day := time.Now().UTC().AddDate(0, 0, -3)
	ts := &tvServer{answer: func(url.Values) []string {
		return []string{"Copper.Valley." + day.Format("2006.01.02") + ".1080p.WEB-DL.H.264-NIGHTDESK"}
	}}
	f := seedShow(t, ts.start(t), nil)
	_ = f.s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE titles SET episode_order = 'daily' WHERE id = ?", f.titleID); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE items SET air_day = ? WHERE id = ?", day.Format("2006-01-02"), f.items["S02E01"])
		return err
	})
	sr := &Searcher{Store: f.s}
	runKind(t, f.fixture, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S02E01"]}, nil)
	q := ts.queries[0]
	if q.Get("season") != day.Format("2006") || q.Get("ep") != day.Format("01/02") {
		t.Fatalf("daily query %v", q)
	}
	if f.state(t, "S02E01") != mediafiles.SlotChosen {
		t.Fatalf("daily episode %s", f.state(t, "S02E01"))
	}
}

// A title_id keeps the missing search to one title: its episodes are searched
// and the other title's movie is not.
func TestMissingSearch_OneTitle(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, map[string]any{"title_id": float64(f.titleID)})
	if n, _ := j.Result["slots"].(float64); j.State != "done" || n != 4 {
		t.Fatalf("one title: %s %+v", j.State, j.Result)
	}
	var movie int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ? AND json_extract(subject, '$.id') = ?", jobs.KindSearchSlot, f.slotID).Scan(&movie)
	if movie != 0 {
		t.Fatalf("the movie was searched too (%d)", movie)
	}
}

// The nightly show switch does not stop the search an approved show request
// starts for its own title.
func TestMissingSearch_OneTitleIgnoresNightlyShowSwitch(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	setSetting(t, f.fixture, SettingMissingShows, "false")
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, map[string]any{"title_id": float64(f.titleID)})
	if n, _ := j.Result["slots"].(float64); n != 4 {
		t.Fatalf("one title with the nightly show switch off: %+v", j.Result)
	}
	j = runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, nil)
	if n, _ := j.Result["slots"].(float64); n != 1 {
		t.Fatalf("nightly with shows off searches the movie only: %+v", j.Result)
	}
}
