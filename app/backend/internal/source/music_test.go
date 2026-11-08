package source

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

const musicCaps = `<?xml version="1.0" encoding="UTF-8"?>
<caps><limits max="50" default="25"/><searching>
<search available="yes" supportedParams="q"/>
<movie-search available="yes" supportedParams="q,imdbid,tmdbid"/>
<music-search available="yes" supportedParams="q,artist,album"/>
</searching><categories><category id="2000" name="Movies"/><category id="3000" name="Audio"/></categories></caps>`

// musicRSS lists Releases; a name may end in "|<size in MB>" (default 400)
// and "#<files>".
func musicRSS(names ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0" xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>`)
	for i, n := range names {
		size, files := int64(400), 0
		if name, fc, ok := strings.Cut(n, "#"); ok {
			n = name
			fmt.Sscan(fc, &files)
		}
		if name, mb, ok := strings.Cut(n, "|"); ok {
			n = name
			fmt.Sscan(mb, &size)
		}
		fmt.Fprintf(&b, `<item><title>%s</title><pubDate>Sat, 10 Oct 2026 12:00:00 +0000</pubDate>
<enclosure url="http://src/%d.torrent" length="%d" type="application/x-bittorrent"/>
<torznab:attr name="seeders" value="50"/><torznab:attr name="files" value="%d"/></item>`, xmlEscape(n), i, size<<20, files)
	}
	b.WriteString(`</channel></rss>`)
	return b.String()
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

type musicServer struct {
	mu      sync.Mutex
	queries []url.Values
	answer  func(q url.Values) []string
}

func (ms *musicServer) start(t *testing.T) string {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("t") == "caps" {
			_, _ = w.Write([]byte(musicCaps))
			return
		}
		ms.mu.Lock()
		ms.queries = append(ms.queries, q)
		ms.mu.Unlock()
		_, _ = w.Write([]byte(musicRSS(ms.answer(q)...)))
	})
	return srv.URL + "/api"
}

type artistFix struct {
	fixture
	titleID, verID int64
	albums         map[string]int64 // album name -> part id
	slots          map[string]int64 // "Low Tide Atlas 1-2" -> slot id
}

const (
	lta  = "Low Tide Atlas"
	hs   = "Harbour Songs"
	flac = "Velvet Cartography - Low Tide Atlas (2019) [FLAC]"
	mp3  = "Velvet Cartography - Low Tide Atlas (2019) [MP3 320]|100"
)

// seedArtist adds Velvet Cartography on Lossless first: Low Tide Atlas
// (2019, disc 1 with three tracks and disc 2 with two, 9 minutes each, all
// wanted) and Harbour Songs (out next month, two tracks not out yet). A
// Dune movie is there too, and one Source for movies and music.
func seedArtist(t *testing.T, srcURL string) artistFix {
	t.Helper()
	f := artistFix{fixture: seed(t, srcURL), albums: map[string]int64{}, slots: map[string]int64{}}
	now := time.Now().UTC()
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr, lf int64
		if err := tx.QueryRow("SELECT id FROM pick_rules WHERE name = ?", domain.LosslessFirstName).Scan(&pr); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT id FROM library_folders LIMIT 1").Scan(&lf); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE sources SET kinds = '["movie","music"]', categories = '{"movie":[2000],"music":[3000]}', use_for = '{"auto_search":true,"hand_search":true,"feed":true}' WHERE id = ?`, f.srcID); err != nil {
			return err
		}
		ti := &domain.Title{Kind: "artist", GateDriver: "none", Name: "Velvet Cartography", SortName: "velvet cartography",
			Aliases: []string{}, SourceIDs: map[string]string{"musicbrainz": "mb-vc"}, MetadataSource: "musicbrainz",
			KeepUpToDate: true, NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{"album"}, WhyAdded: "by_hand",
			Artwork: []string{}, Status: "running", State: "active"}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		v := &domain.Version{TitleID: ti.ID, Label: "Main", PickRulesID: pr, LibraryFolderID: lf, FolderName: "Velvet Cartography", KeepUpToDate: true, IsMain: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		f.titleID, f.verID = ti.ID, v.ID
		for _, a := range []struct {
			name  string
			date  time.Time
			discs []int
		}{{lta, time.Date(2019, 5, 1, 0, 0, 0, 0, time.UTC), []int{3, 2}}, {hs, now.AddDate(0, 1, 0), []int{2}}} {
			date := a.date
			p := &domain.Part{TitleID: ti.ID, Kind: "album", Name: a.name, ReleaseType: ptr("album"), ReleaseDate: &date, KeepUpToDate: true,
				Included: true, SourceIDs: map[string]string{}, Pressings: []domain.Pressing{}, SecondaryTypes: []string{}}
			if _, err := database.Insert(tx, "parts", p, domain.System); err != nil {
				return err
			}
			f.albums[a.name] = p.ID
			for d, n := range a.discs {
				for k := 1; k <= n; k++ {
					it := &domain.Item{TitleID: ti.ID, PartID: &p.ID, Kind: "track", Numbers: domain.Numbers{Disc: ptr(d + 1), Track: ptr(k)},
						Name: ptr(fmt.Sprintf("%s %d-%d", a.name, d+1, k)), AirDate: &date, LengthMS: ptr(9 * 60_000), KeepUpToDate: true, SourceIDs: map[string]string{}}
					if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
						return err
					}
					sl := &domain.Slot{ItemID: it.ID, VersionID: v.ID, State: domain.SlotWanted}
					if date.After(now) {
						sl.State = domain.SlotNotOutYet
					}
					if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
						return err
					}
					f.slots[fmt.Sprintf("%s %d-%d", a.name, d+1, k)] = sl.ID
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f artistFix) state(t *testing.T, label string) domain.SlotState {
	t.Helper()
	var st string
	if err := f.s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", f.slots[label]).Scan(&st); err != nil {
		t.Fatal(err)
	}
	return domain.SlotState(st)
}

var ltaTracks = []string{"Low Tide Atlas 1-1", "Low Tide Atlas 1-2", "Low Tide Atlas 1-3", "Low Tide Atlas 2-1", "Low Tide Atlas 2-2"}

func (f artistFix) albumJob(t *testing.T, sr *Searcher, album string, params map[string]any) domain.Job {
	t.Helper()
	p := map[string]any{"version_id": f.verID}
	for k, v := range params {
		p[k] = v
	}
	return runKind(t, f.fixture, sr, KindSearchAlbum, &domain.Subject{Type: "part", ID: f.albums[album]}, p)
}

func TestAlbumSearchPicksOneReleaseForEveryTrack(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string {
		return []string{
			mp3,
			flac,
			"Velvet Cartography - Low Tide Atlas (2019) [FLAC] [CD]#3",
			"Velvet Cartography - Harbour Songs (2021) [FLAC]",
			"Velvet Cartography - Discography (2011-2024) [FLAC]|3000",
			"Velvet Cartography - Low Tide Atlas (2019) [128kbps]|40",
			"Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS",
		}
	}}
	f := seedArtist(t, ms.start(t))
	var grabs [][2]int64
	sr := &Searcher{Store: f.s, OnChosen: func(_ context.Context, slotID, releaseID int64) { grabs = append(grabs, [2]int64{slotID, releaseID}) }}
	j := f.albumJob(t, sr, lta, nil)
	if j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	q := ms.queries[0]
	if q.Get("t") != "music" || q.Get("artist") != "Velvet Cartography" || q.Get("album") != lta || q.Get("cat") != "3000" {
		t.Fatalf("query %v", q)
	}
	for _, l := range ltaTracks {
		if st := f.state(t, l); st != domain.SlotChosen {
			t.Fatalf("%s is %s", l, st)
		}
	}
	win := relByName(t, f.fixture, flac)
	if win.State != "chosen" || len(PickSlots(win)) != 5 || len(grabs) != 1 || grabs[0][1] != win.ID {
		t.Fatalf("winner %s %v, grabs %v", win.State, PickSlots(win), grabs)
	}
	for name, code := range map[string]string{
		"Velvet Cartography - Low Tide Atlas (2019) [MP3 320]":   "rank.lost",
		"Velvet Cartography - Low Tide Atlas (2019) [FLAC] [CD]": "match.track_count",
		"Velvet Cartography - Harbour Songs (2021) [FLAC]":       "match.not_asked",
		"Velvet Cartography - Discography (2011-2024) [FLAC]":    "match.discography",
		"Velvet Cartography - Low Tide Atlas (2019) [128kbps]":   "step.not_allowed",
		"Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS":             "match.none",
	} {
		if r := relByName(t, f.fixture, name); !hasCode(r, code) {
			t.Errorf("%s: no %s in %v", name, code, reasonCodes(r))
		}
	}
	if st := f.state(t, "Harbour Songs 1-1"); st != domain.SlotNotOutYet {
		t.Fatalf("other album touched: %s", st)
	}
}

func TestTrackSearchSearchesItsAlbum(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{flac} }}
	f := seedArtist(t, ms.start(t))
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["Low Tide Atlas 1-2"]}, map[string]any{"by_hand": true})
	if j.Result["album_search_job_id"] == nil {
		t.Fatalf("track search did not queue its album: %+v", j.Result)
	}
	var kind, params string
	_ = f.s.Read().QueryRow("SELECT kind, params FROM jobs WHERE id = ?", int64(j.Result["album_search_job_id"].(float64))).Scan(&kind, &params)
	if kind != KindSearchAlbum || !strings.Contains(params, `"by_hand":true`) {
		t.Fatalf("queued %s %s", kind, params)
	}
}

func TestAlbumNotOutIsSearchedOnlyByHand(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{"Velvet Cartography - Harbour Songs (2026) [FLAC]|160"} }}
	f := seedArtist(t, ms.start(t))
	sr := &Searcher{Store: f.s}
	j := f.albumJob(t, sr, hs, map[string]any{"bulk": true})
	if j.Result["skipped"] == nil || len(ms.queries) != 0 {
		t.Fatalf("an album not out yet was searched: %+v", j.Result)
	}
	j = f.albumJob(t, sr, hs, map[string]any{"by_hand": true})
	if j.State != "done" || f.state(t, "Harbour Songs 1-1") != domain.SlotChosen {
		t.Fatalf("hand search %s %+v, %s", j.State, j.Result, f.state(t, "Harbour Songs 1-1"))
	}
	if r := relByName(t, f.fixture, "Velvet Cartography - Harbour Songs (2026) [FLAC]"); !hasCode(r, "hard.gate_skipped") {
		t.Fatalf("skipped gate not said: %v", reasonCodes(r))
	}
}

func TestOneDiscFillsOnlyItsTracks(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string {
		return []string{"Velvet Cartography - Low Tide Atlas (2019) CD2 [FLAC]|160"}
	}}
	f := seedArtist(t, ms.start(t))
	sr := &Searcher{Store: f.s}
	if j := f.albumJob(t, sr, lta, nil); j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	for _, l := range ltaTracks {
		want := domain.SlotWanted
		if strings.Contains(l, " 2-") {
			want = domain.SlotChosen
		}
		if st := f.state(t, l); st != want {
			t.Fatalf("%s is %s, want %s", l, st, want)
		}
	}
}

// Find a better copy per album: a 320 copy in place is replaced by lossless;
// a Release no better for every track is refused.
func TestAlbumBetterCopy(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string {
		return []string{"Velvet Cartography - Low Tide Atlas (2019) [MP3 V0]|100", flac}
	}}
	f := seedArtist(t, ms.start(t))
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		for _, l := range ltaTracks {
			name := "Velvet Cartography - Low Tide Atlas (2019) [MP3 320]"
			fl := &domain.File{SlotIDs: []int64{f.slots[l]}, VersionID: f.verID, RelativePath: l + ".mp3", Size: 1, AddedAt: time.Now(),
				Languages: []string{}, MediaInfo: map[string]any{}, ReleaseName: &name, Origin: "imported", PlacedBy: "copy", Role: "main", State: "present"}
			if _, err := database.Insert(tx, "files", fl, domain.System); err != nil {
				return err
			}
			if _, err := tx.Exec("UPDATE slots SET state = 'better_wanted', file_id = ? WHERE id = ?", fl.ID, f.slots[l]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sr := &Searcher{Store: f.s}
	if j := f.albumJob(t, sr, lta, nil); j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	if st := f.state(t, ltaTracks[0]); st != domain.SlotChosen {
		t.Fatalf("not replaced: %s", st)
	}
	if r := relByName(t, f.fixture, "Velvet Cartography - Low Tide Atlas (2019) [MP3 V0]"); !hasCode(r, "match.album_gain") {
		t.Fatalf("V0 over 320: %v", reasonCodes(r))
	}
}

func TestAlbumTurnedDownNeverPickedAgain(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{flac} }}
	f := seedArtist(t, ms.start(t))
	sr := &Searcher{Store: f.s}
	f.albumJob(t, sr, lta, map[string]any{"choose": false})
	r := relByName(t, f.fixture, flac)
	if err := sr.TurnDown(context.Background(), r.ID, "bad rip", domain.System); err != nil {
		t.Fatal(err)
	}
	f.albumJob(t, sr, lta, nil)
	if st := f.state(t, ltaTracks[0]); st != domain.SlotWanted {
		t.Fatalf("turned-down Release picked: %s", st)
	}
	if r := relByName(t, f.fixture, flac); r.State != "turned_down" && !hasCode(r, "hard.turned_down") {
		t.Fatalf("%s %v", r.State, reasonCodes(r))
	}
}

func TestDiscographyWithSetting(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{"Velvet Cartography - Discography (2011-2024) [FLAC]|2000"} }}
	f := seedArtist(t, ms.start(t))
	exec(t, f.fixture, `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
		VALUES ('what_to_pick', ?, 'true', '2026-01-01', '{}', '2026-01-01', '{}')`, SettingDiscography)
	sr := &Searcher{Store: f.s}
	f.albumJob(t, sr, lta, nil)
	if st := f.state(t, ltaTracks[0]); st != domain.SlotChosen {
		t.Fatalf("discography not taken with the setting: %s", st)
	}
}

func TestFeedCheckPicksAlbum(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{flac, "Dune.Part.Two.2024.1080p.WEB.H264-OTHER"} }}
	f := seedArtist(t, ms.start(t))
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, KindFeedCheck, nil, nil)
	if j.State != "done" {
		t.Fatalf("feed %s %+v", j.State, j.Error)
	}
	if q := ms.queries[0]; !strings.Contains(q.Get("cat"), "3000") {
		t.Fatalf("feed categories %v", q)
	}
	for _, l := range ltaTracks {
		if st := f.state(t, l); st != domain.SlotChosen {
			t.Fatalf("%s is %s", l, st)
		}
	}
}

func TestMissingSearchQueuesAlbums(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return nil }}
	f := seedArtist(t, ms.start(t))
	exec(t, f.fixture, "UPDATE slots SET state = 'in_library' WHERE id = ?", f.slotID) // the movie
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, KindSearchMissing, nil, map[string]any{"which": "missing"})
	if j.State != "done" || j.Result["albums"] != float64(1) || j.Result["queued"] != float64(1) {
		t.Fatalf("missing %s %+v", j.State, j.Result)
	}
	exec(t, f.fixture, "DELETE FROM jobs WHERE kind = ?", KindSearchAlbum)
	exec(t, f.fixture, `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
		VALUES ('what_to_pick', ?, 'false', '2026-01-01', '{}', '2026-01-01', '{}')`, SettingMissingMusic)
	j = runKind(t, f.fixture, sr, KindSearchMissing, nil, map[string]any{"which": "missing"})
	if j.Result["albums"] != float64(0) {
		t.Fatalf("albums searched with the setting off: %+v", j.Result)
	}
}

// Held per album: a wait holds the Release for every track; once the wait
// is over it is chosen for all of them at once. A Source past its picks
// limit gives way to the next.
func TestAlbumHeldThenChosenAndPicksLimit(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{flac} }}
	f := seedArtist(t, ms.start(t))
	exec(t, f.fixture, `UPDATE pick_rules SET body = json_set(body, '$.protocol.wait_minutes_torrent', 100000) WHERE name = ?`, domain.LosslessFirstName)
	sr := &Searcher{Store: f.s}
	f.albumJob(t, sr, lta, nil)
	for _, l := range ltaTracks {
		if st := f.state(t, l); st != domain.SlotHeld {
			t.Fatalf("%s is %s", l, st)
		}
	}
	exec(t, f.fixture, "UPDATE slots SET next_try_at = '2000-01-01T00:00:00Z' WHERE state = 'held'")
	if n, err := sr.grabHeld(context.Background(), domain.System, nil, time.Now()); err != nil || n != 1 {
		t.Fatalf("grab held %d %v", n, err)
	}
	for _, l := range ltaTracks {
		if st := f.state(t, l); st != domain.SlotChosen {
			t.Fatalf("%s is %s after the wait", l, st)
		}
	}
	// Picks limit: the Source already gave one pick in its window.
	g := seedArtist(t, ms.start(t))
	exec(t, g.fixture, `UPDATE sources SET limits = json_set(COALESCE(limits, '{}'), '$.grabs_per_window', 1, '$.window_seconds', 86400)`)
	exec(t, g.fixture, `INSERT INTO releases (created_at, created_by, updated_at, updated_by, source_id, name, link_ref, protocol, size, flags, parsed, matched_slot_ids, verdict, found_by, state)
		VALUES (strftime('%Y-%m-%dT%H:%M:%fZ','now'), '{}', strftime('%Y-%m-%dT%H:%M:%fZ','now'), '{}', ?, 'x', 'http://x', 'torrent', 1, '[]', '{}', '[]', '{}', 'auto_search', 'chosen')`, g.srcID)
	g.albumJob(t, &Searcher{Store: g.s}, lta, nil)
	if st := g.state(t, ltaTracks[0]); st != domain.SlotWanted {
		t.Fatalf("picked past the Source limit: %s", st)
	}
	if r := relByName(t, g.fixture, flac); !hasCode(r, "rank.source_limit") {
		t.Fatalf("limit not said: %v", reasonCodes(r))
	}
}

// #43: choosing an MP3 album by hand over tracks that are all FLAC already
// needs force, whatever state the Release is in: no track would gain.
func TestChooseLossyAlbumOverLosslessNeedsForce(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{mp3} }}
	f := seedArtist(t, ms.start(t))
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		for _, l := range ltaTracks {
			name := flac
			fl := &domain.File{SlotIDs: []int64{f.slots[l]}, VersionID: f.verID, RelativePath: l + ".flac", Size: 1, AddedAt: time.Now(),
				Languages: []string{}, MediaInfo: map[string]any{}, ReleaseName: &name, Origin: "imported", PlacedBy: "copy", Role: "main", State: "present"}
			if _, err := database.Insert(tx, "files", fl, domain.System); err != nil {
				return err
			}
			if _, err := tx.Exec("UPDATE slots SET state = 'better_wanted', file_id = ? WHERE id = ?", fl.ID, f.slots[l]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sr := &Searcher{Store: f.s}
	f.albumJob(t, sr, lta, map[string]any{"choose": false})
	r := relByName(t, f.fixture, "Velvet Cartography - Low Tide Atlas (2019) [MP3 320]")
	// Whatever the verdict made of it, a person picks it from the list.
	ids := make([]string, len(ltaTracks))
	for i, l := range ltaTracks {
		ids[i] = fmt.Sprint(f.slots[l])
	}
	exec(t, f.fixture, "UPDATE releases SET state = 'passed_over', matched_slot_ids = ? WHERE id = ?", "["+strings.Join(ids, ",")+"]", r.ID)
	var nf *NeedsForceError
	if _, err := sr.Choose(context.Background(), r.ID, 0, false, domain.System); !errors.As(err, &nf) || !strings.Contains(strings.Join(nf.Reasons, " "), "already has a copy as good") {
		t.Fatalf("picked over FLAC without force: %v", err)
	}
	// The same for one track named by the page.
	if _, err := sr.Choose(context.Background(), r.ID, f.slots[ltaTracks[0]], false, domain.System); !errors.As(err, &nf) {
		t.Fatalf("picked over FLAC for one track without force: %v", err)
	}
	if st := f.state(t, ltaTracks[0]); st != domain.SlotBetterWanted {
		t.Fatalf("slot moved: %s", st)
	}
	if ch, err := sr.Choose(context.Background(), r.ID, 0, true, domain.System); err != nil || !ch.Forced || len(ch.SlotIDs) != len(ltaTracks) {
		t.Fatalf("with force: %+v %v", ch, err)
	}
}

// #43: a FLAC copy whose stored name says no format (an adopted or older
// row) is still known as lossless by its extension: an MP3 album is no gain.
func TestCurrentCopyKnownByExtension(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{mp3} }}
	f := seedArtist(t, ms.start(t))
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		for _, l := range ltaTracks {
			name := "Velvet Cartography - Low Tide Atlas (2019)"
			fl := &domain.File{SlotIDs: []int64{f.slots[l]}, VersionID: f.verID, RelativePath: l + ".flac", Size: 1, AddedAt: time.Now(),
				Languages: []string{}, MediaInfo: map[string]any{}, ReleaseName: &name, Origin: "adopted", PlacedBy: "none", Role: "main", State: "present"}
			if _, err := database.Insert(tx, "files", fl, domain.System); err != nil {
				return err
			}
			if _, err := tx.Exec("UPDATE slots SET state = 'better_wanted', file_id = ? WHERE id = ?", fl.ID, f.slots[l]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sr := &Searcher{Store: f.s}
	f.albumJob(t, sr, lta, nil)
	if st := f.state(t, ltaTracks[0]); st == domain.SlotChosen {
		t.Fatalf("MP3 320 chosen over FLAC with a plain name: %s", st)
	}
	// Picked by hand for one track, it needs force too.
	r := relByName(t, f.fixture, "Velvet Cartography - Low Tide Atlas (2019) [MP3 320]")
	exec(t, f.fixture, "UPDATE releases SET state = 'passed_over', matched_slot_ids = json_array(?) WHERE id = ?", f.slots[ltaTracks[0]], r.ID)
	var nf *NeedsForceError
	if _, err := sr.Choose(context.Background(), r.ID, f.slots[ltaTracks[0]], false, domain.System); !errors.As(err, &nf) {
		t.Fatalf("picked by hand over a plain-named FLAC copy: %v", err)
	}
}

// #43: a TAK copy with a plain stored name is lossless at search: an MP3
// album is no gain over it.
func TestTAKCopyKnownAtSearch(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{mp3} }}
	f := seedArtist(t, ms.start(t))
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		for _, l := range ltaTracks {
			name := "Velvet Cartography - Low Tide Atlas (2019)"
			fl := &domain.File{SlotIDs: []int64{f.slots[l]}, VersionID: f.verID, RelativePath: l + ".tak", Size: 1, AddedAt: time.Now(),
				Languages: []string{}, MediaInfo: map[string]any{}, ReleaseName: &name, Origin: "adopted", PlacedBy: "none", Role: "main", State: "present"}
			if _, err := database.Insert(tx, "files", fl, domain.System); err != nil {
				return err
			}
			if _, err := tx.Exec("UPDATE slots SET state = 'better_wanted', file_id = ? WHERE id = ?", fl.ID, f.slots[l]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.albumJob(t, &Searcher{Store: f.s}, lta, nil)
	if st := f.state(t, ltaTracks[0]); st == domain.SlotChosen {
		t.Fatalf("MP3 320 chosen over TAK: %s", st)
	}
}
