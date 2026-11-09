package sources

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

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// musicCaps is a Torznab caps answer that offers music search on category 3000.
const musicCaps = `<?xml version="1.0" encoding="UTF-8"?>
<caps><limits max="50" default="25"/><searching>
<search available="yes" supportedParams="q"/>
<movie-search available="yes" supportedParams="q,imdbid,tmdbid"/>
<music-search available="yes" supportedParams="q,artist,album"/>
</searching><categories><category id="2000" name="Movies"/><category id="3000" name="Audio"/></categories></caps>`

// musicRSS builds a Torznab feed of one album release per name. A name may end in "|MB" for its size and "#N" for its file count.
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

// xmlEscape escapes the characters that would break the feed XML.
func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// musicServer is a fake music indexer that records each query and answers it with the names that answer returns.
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

// artistFix is a fixture with one artist, one Version and two albums, indexed by album name and track label.
type artistFix struct {
	fixture
	titleID, verID int64
	albums         map[string]int64
	slots          map[string]int64
}

const (
	// Album names and the two release names the tests search for.
	lta  = "Low Tide Atlas"
	hs   = "Harbour Songs"
	flac = "Velvet Cartography - Low Tide Atlas (2019) [FLAC]"
	mp3  = "Velvet Cartography - Low Tide Atlas (2019) [MP3 320]|100"
)

// seedArtist stores the artist: Low Tide Atlas (released, two discs of 3 and 2 tracks) and Harbour Songs (due next month, so its Slots are not out yet).
func seedArtist(t *testing.T, srcURL string) artistFix {
	t.Helper()
	f := artistFix{fixture: seed(t, srcURL), albums: map[string]int64{}, slots: map[string]int64{}}
	now := time.Now().UTC()
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr, lf int64
		if err := tx.QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.LosslessFirstName).Scan(&pr); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT id FROM library_folders LIMIT 1").Scan(&lf); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE sources SET kinds = '["movie","music"]', categories = '{"movie":[2000],"music":[3000]}', use_for = '{"auto_search":true,"hand_search":true,"feed":true}' WHERE id = ?`, f.srcID); err != nil {
			return err
		}
		ti := &mediafiles.Title{Kind: "artist", GateDriver: "none", Name: "Velvet Cartography", SortName: "velvet cartography",
			Aliases: []string{}, SourceIDs: map[string]string{"musicbrainz": "mb-vc"}, MetadataSource: "musicbrainz",
			KeepUpToDate: true, NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{"album"}, WhyAdded: "by_hand",
			Artwork: []string{}, Status: "running", State: "active"}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		v := &mediafiles.Version{TitleID: ti.ID, Label: "Main", PickRulesID: pr, LibraryFolderID: lf, FolderName: "Velvet Cartography", KeepUpToDate: true, IsMain: true}
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
			p := &mediafiles.Part{TitleID: ti.ID, Kind: "album", Name: a.name, ReleaseType: ptr("album"), ReleaseDate: &date, KeepUpToDate: true,
				Included: true, SourceIDs: map[string]string{}, Pressings: []mediafiles.Pressing{}, SecondaryTypes: []string{}}
			if _, err := database.Insert(tx, "parts", p, domain.System); err != nil {
				return err
			}
			f.albums[a.name] = p.ID
			for d, n := range a.discs {
				for k := 1; k <= n; k++ {
					it := &mediafiles.Item{TitleID: ti.ID, PartID: &p.ID, Kind: "track", Numbers: mediafiles.Numbers{Disc: ptr(d + 1), Track: ptr(k)},
						Name: ptr(fmt.Sprintf("%s %d-%d", a.name, d+1, k)), AirDate: &date, LengthMS: ptr(9 * 60_000), KeepUpToDate: true, SourceIDs: map[string]string{}}
					if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
						return err
					}
					sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotWanted}
					if date.After(now) {
						sl.State = mediafiles.SlotNotOutYet
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

// state returns the current state of the Slot for a track label such as "Low Tide Atlas 1-2".
func (f artistFix) state(t *testing.T, label string) mediafiles.SlotState {
	t.Helper()
	var st string
	if err := f.s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", f.slots[label]).Scan(&st); err != nil {
		t.Fatal(err)
	}
	return mediafiles.SlotState(st)
}

// ltaTracks are the labels of every Low Tide Atlas track.
var ltaTracks = []string{"Low Tide Atlas 1-1", "Low Tide Atlas 1-2", "Low Tide Atlas 1-3", "Low Tide Atlas 2-1", "Low Tide Atlas 2-2"}

// albumJob runs an album search for one album with extra Job params.
func (f artistFix) albumJob(t *testing.T, sr *Searcher, album string, params map[string]any) domain.Job {
	t.Helper()
	p := map[string]any{"version_id": f.verID}
	for k, v := range params {
		p[k] = v
	}
	return runKind(t, f.fixture, sr, jobs.KindSearchAlbum, &domain.Subject{Type: "part", ID: f.albums[album]}, p)
}

func TestAlbumSearch_OneReleaseForEveryTrack(t *testing.T) {
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
		if st := f.state(t, l); st != mediafiles.SlotChosen {
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
	if st := f.state(t, "Harbour Songs 1-1"); st != mediafiles.SlotNotOutYet {
		t.Fatalf("other album touched: %s", st)
	}
}

func TestTrackSearch_SearchesItsAlbum(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{flac} }}
	f := seedArtist(t, ms.start(t))
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["Low Tide Atlas 1-2"]}, map[string]any{"by_hand": true})
	if j.Result["album_search_job_id"] == nil {
		t.Fatalf("track search did not queue its album: %+v", j.Result)
	}
	var kind, params string
	_ = f.s.Read().QueryRow("SELECT kind, params FROM jobs WHERE id = ?", int64(j.Result["album_search_job_id"].(float64))).Scan(&kind, &params)
	if kind != jobs.KindSearchAlbum || !strings.Contains(params, `"by_hand":true`) {
		t.Fatalf("queued %s %s", kind, params)
	}
}

func TestAlbumSearch_NotOutOnlyByHand(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{"Velvet Cartography - Harbour Songs (2026) [FLAC]|160"} }}
	f := seedArtist(t, ms.start(t))
	sr := &Searcher{Store: f.s}
	j := f.albumJob(t, sr, hs, map[string]any{"bulk": true})
	if j.Result["skipped"] == nil || len(ms.queries) != 0 {
		t.Fatalf("an album not out yet was searched: %+v", j.Result)
	}
	j = f.albumJob(t, sr, hs, map[string]any{"by_hand": true})
	if j.State != "done" || f.state(t, "Harbour Songs 1-1") != mediafiles.SlotChosen {
		t.Fatalf("hand search %s %+v, %s", j.State, j.Result, f.state(t, "Harbour Songs 1-1"))
	}
	if r := relByName(t, f.fixture, "Velvet Cartography - Harbour Songs (2026) [FLAC]"); !hasCode(r, "hard.gate_skipped") {
		t.Fatalf("skipped gate not said: %v", reasonCodes(r))
	}
}

func TestAlbumSearch_OneDiscFillsOnlyItsTracks(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string {
		return []string{"Velvet Cartography - Low Tide Atlas (2019) CD2 [FLAC]|160"}
	}}
	f := seedArtist(t, ms.start(t))
	sr := &Searcher{Store: f.s}
	if j := f.albumJob(t, sr, lta, nil); j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	for _, l := range ltaTracks {
		want := mediafiles.SlotWanted
		if strings.Contains(l, " 2-") {
			want = mediafiles.SlotChosen
		}
		if st := f.state(t, l); st != want {
			t.Fatalf("%s is %s, want %s", l, st, want)
		}
	}
}

func TestAlbumSearch_BetterCopy(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string {
		return []string{"Velvet Cartography - Low Tide Atlas (2019) [MP3 V0]|100", flac}
	}}
	f := seedArtist(t, ms.start(t))
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		for _, l := range ltaTracks {
			name := "Velvet Cartography - Low Tide Atlas (2019) [MP3 320]"
			fl := &mediafiles.File{SlotIDs: []int64{f.slots[l]}, VersionID: f.verID, RelativePath: l + ".mp3", Size: 1, AddedAt: time.Now(),
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
	if st := f.state(t, ltaTracks[0]); st != mediafiles.SlotChosen {
		t.Fatalf("not replaced: %s", st)
	}
	if r := relByName(t, f.fixture, "Velvet Cartography - Low Tide Atlas (2019) [MP3 V0]"); !hasCode(r, "match.album_gain") {
		t.Fatalf("V0 over 320: %v", reasonCodes(r))
	}
}

func TestAlbumSearch_TurnedDownNeverPickedAgain(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{flac} }}
	f := seedArtist(t, ms.start(t))
	sr := &Searcher{Store: f.s}
	f.albumJob(t, sr, lta, map[string]any{"choose": false})
	r := relByName(t, f.fixture, flac)
	if err := sr.TurnDown(context.Background(), r.ID, "bad rip", domain.System); err != nil {
		t.Fatal(err)
	}
	f.albumJob(t, sr, lta, nil)
	if st := f.state(t, ltaTracks[0]); st != mediafiles.SlotWanted {
		t.Fatalf("turned-down Release picked: %s", st)
	}
	if r := relByName(t, f.fixture, flac); r.State != "turned_down" && !hasCode(r, "hard.turned_down") {
		t.Fatalf("%s %v", r.State, reasonCodes(r))
	}
}

func TestAlbumSearch_DiscographySetting(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{"Velvet Cartography - Discography (2011-2024) [FLAC]|2000"} }}
	f := seedArtist(t, ms.start(t))
	exec(t, f.fixture, `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
		VALUES ('what_to_pick', ?, 'true', '2026-01-01', '{}', '2026-01-01', '{}')`, SettingDiscography)
	sr := &Searcher{Store: f.s}
	f.albumJob(t, sr, lta, nil)
	if st := f.state(t, ltaTracks[0]); st != mediafiles.SlotChosen {
		t.Fatalf("discography not taken with the setting: %s", st)
	}
}

func TestHandleFeed_PicksAlbum(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{flac, "Dune.Part.Two.2024.1080p.WEB.H264-OTHER"} }}
	f := seedArtist(t, ms.start(t))
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, jobs.KindFeedCheck, nil, nil)
	if j.State != "done" {
		t.Fatalf("feed %s %+v", j.State, j.Error)
	}
	if q := ms.queries[0]; !strings.Contains(q.Get("cat"), "3000") {
		t.Fatalf("feed categories %v", q)
	}
	for _, l := range ltaTracks {
		if st := f.state(t, l); st != mediafiles.SlotChosen {
			t.Fatalf("%s is %s", l, st)
		}
	}
}

func TestHandleSearchMissing_QueuesAlbums(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return nil }}
	f := seedArtist(t, ms.start(t))
	exec(t, f.fixture, "UPDATE slots SET state = 'in_library' WHERE id = ?", f.slotID)
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, map[string]any{"which": "missing"})
	if j.State != "done" || j.Result["albums"] != float64(1) || j.Result["queued"] != float64(1) {
		t.Fatalf("missing %s %+v", j.State, j.Result)
	}
	exec(t, f.fixture, "DELETE FROM jobs WHERE kind = ?", jobs.KindSearchAlbum)
	exec(t, f.fixture, `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
		VALUES ('what_to_pick', ?, 'false', '2026-01-01', '{}', '2026-01-01', '{}')`, SettingMissingMusic)
	j = runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, map[string]any{"which": "missing"})
	if j.Result["albums"] != float64(0) {
		t.Fatalf("albums searched with the setting off: %+v", j.Result)
	}
}

func TestAlbumSearch_HeldThenChosenAndPicksLimit(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{flac} }}
	f := seedArtist(t, ms.start(t))
	exec(t, f.fixture, `UPDATE pick_rules SET body = json_set(body, '$.protocol.wait_minutes_torrent', 100000) WHERE name = ?`, mediafiles.LosslessFirstName)
	sr := &Searcher{Store: f.s}
	f.albumJob(t, sr, lta, nil)
	for _, l := range ltaTracks {
		if st := f.state(t, l); st != mediafiles.SlotHeld {
			t.Fatalf("%s is %s", l, st)
		}
	}
	exec(t, f.fixture, "UPDATE slots SET next_try_at = '2000-01-01T00:00:00Z' WHERE state = 'held'")
	if n, err := sr.grabHeld(context.Background(), domain.System, nil, time.Now()); err != nil || n != 1 {
		t.Fatalf("grab held %d %v", n, err)
	}
	for _, l := range ltaTracks {
		if st := f.state(t, l); st != mediafiles.SlotChosen {
			t.Fatalf("%s is %s after the wait", l, st)
		}
	}

	g := seedArtist(t, ms.start(t))
	exec(t, g.fixture, `UPDATE sources SET limits = json_set(COALESCE(limits, '{}'), '$.grabs_per_window', 1, '$.window_seconds', 86400)`)
	exec(t, g.fixture, `INSERT INTO releases (created_at, created_by, updated_at, updated_by, source_id, name, link_ref, protocol, size, flags, parsed, matched_slot_ids, verdict, found_by, state)
		VALUES (strftime('%Y-%m-%dT%H:%M:%fZ','now'), '{}', strftime('%Y-%m-%dT%H:%M:%fZ','now'), '{}', ?, 'x', 'http://x', 'torrent', 1, '[]', '{}', '[]', '{}', 'auto_search', 'chosen')`, g.srcID)
	g.albumJob(t, &Searcher{Store: g.s}, lta, nil)
	if st := g.state(t, ltaTracks[0]); st != mediafiles.SlotWanted {
		t.Fatalf("picked past the Source limit: %s", st)
	}
	if r := relByName(t, g.fixture, flac); !hasCode(r, "rank.source_limit") {
		t.Fatalf("limit not said: %v", reasonCodes(r))
	}
}

func TestChoose_LossyAlbumOverLosslessNeedsForce(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{mp3} }}
	f := seedArtist(t, ms.start(t))
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		for _, l := range ltaTracks {
			name := flac
			fl := &mediafiles.File{SlotIDs: []int64{f.slots[l]}, VersionID: f.verID, RelativePath: l + ".flac", Size: 1, AddedAt: time.Now(),
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

	ids := make([]string, len(ltaTracks))
	for i, l := range ltaTracks {
		ids[i] = fmt.Sprint(f.slots[l])
	}
	exec(t, f.fixture, "UPDATE releases SET state = 'passed_over', matched_slot_ids = ? WHERE id = ?", "["+strings.Join(ids, ",")+"]", r.ID)
	var nf *NeedsForceError
	if _, err := sr.Choose(context.Background(), r.ID, 0, false, domain.System); !errors.As(err, &nf) || !strings.Contains(strings.Join(nf.Reasons, " "), "already has a copy as good") {
		t.Fatalf("picked over FLAC without force: %v", err)
	}

	if _, err := sr.Choose(context.Background(), r.ID, f.slots[ltaTracks[0]], false, domain.System); !errors.As(err, &nf) {
		t.Fatalf("picked over FLAC for one track without force: %v", err)
	}
	if st := f.state(t, ltaTracks[0]); st != mediafiles.SlotBetterWanted {
		t.Fatalf("slot moved: %s", st)
	}
	if ch, err := sr.Choose(context.Background(), r.ID, 0, true, domain.System); err != nil || !ch.Forced || len(ch.SlotIDs) != len(ltaTracks) {
		t.Fatalf("with force: %+v %v", ch, err)
	}
}

func TestCurrentCopy_KnownByExtension(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{mp3} }}
	f := seedArtist(t, ms.start(t))
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		for _, l := range ltaTracks {
			name := "Velvet Cartography - Low Tide Atlas (2019)"
			fl := &mediafiles.File{SlotIDs: []int64{f.slots[l]}, VersionID: f.verID, RelativePath: l + ".flac", Size: 1, AddedAt: time.Now(),
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
	if st := f.state(t, ltaTracks[0]); st == mediafiles.SlotChosen {
		t.Fatalf("MP3 320 chosen over FLAC with a plain name: %s", st)
	}

	r := relByName(t, f.fixture, "Velvet Cartography - Low Tide Atlas (2019) [MP3 320]")
	exec(t, f.fixture, "UPDATE releases SET state = 'passed_over', matched_slot_ids = json_array(?) WHERE id = ?", f.slots[ltaTracks[0]], r.ID)
	var nf *NeedsForceError
	if _, err := sr.Choose(context.Background(), r.ID, f.slots[ltaTracks[0]], false, domain.System); !errors.As(err, &nf) {
		t.Fatalf("picked by hand over a plain-named FLAC copy: %v", err)
	}
}

func TestCurrentCopy_TAKKnownAtSearch(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{mp3} }}
	f := seedArtist(t, ms.start(t))
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		for _, l := range ltaTracks {
			name := "Velvet Cartography - Low Tide Atlas (2019)"
			fl := &mediafiles.File{SlotIDs: []int64{f.slots[l]}, VersionID: f.verID, RelativePath: l + ".tak", Size: 1, AddedAt: time.Now(),
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
	if st := f.state(t, ltaTracks[0]); st == mediafiles.SlotChosen {
		t.Fatalf("MP3 320 chosen over TAK: %s", st)
	}
}
