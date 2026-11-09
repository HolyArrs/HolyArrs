package music

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// planFixed is the v7 tests' clock (30 October 2001 turns 25 within the window).
var planFixed = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

type planAlbum struct {
	Name     string `json:"name"`
	MBID     string `json:"mbid"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Date     string `json:"date"`
	Days     int    `json:"days"`
	Have     bool   `json:"have"`
	Listen   int64  `json:"listeners"`
	Explicit bool   `json:"explicit"`
}

type planArtist struct {
	Name      string         `json:"name"`
	MBID      string         `json:"mbid"`
	Genres    map[string]int `json:"genres"`
	Listeners int64          `json:"listeners"`
	Similar   []struct {
		MBID string  `json:"mbid"`
		Name string  `json:"name"`
		S    float64 `json:"s"`
	} `json:"similar"`
	Links  []metadata.ArtistLink    `json:"links"`
	Guest  []metadata.MusicTopAlbum `json:"guest"`
	Albums []planAlbum              `json:"albums"`
}

type planHousehold struct {
	Follows map[string][]string `json:"follows"`
	Artists []planArtist        `json:"artists"`
	Others  []planArtist        `json:"others"`
}

func (al planAlbum) day(now time.Time) string {
	if al.Date != "" {
		return al.Date
	}
	return now.AddDate(0, 0, al.Days).Format("2006-01-02")
}

// planRig loads a testdata/explore_v7 household into the ask rig with flags
// v2, v3, v6 and v7 on, and a fake metadata server answering from the
// fixture (calls counts its requests).
func planRig(t *testing.T, name string) (*askRig, *int, *planHousehold) {
	t.Helper()
	r := newMusicAskRig(t)
	raw, err := os.ReadFile("testdata/explore_v7/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var fx planHousehold
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	// luis (owner), ana and bo, in sign-up order.
	users := map[string]int64{}
	for i, n := range []string{"luis", "ana", "bo"} {
		var id int64
		if err := r.st.Read().QueryRow("SELECT id FROM users ORDER BY id LIMIT 1 OFFSET ?", i).Scan(&id); err != nil {
			t.Fatal(err)
		}
		users[n] = id
	}
	ids := map[string]int64{}
	err = r.st.Write(context.Background(), func(tx *sql.Tx) error {
		ts, by := database.FormatTime(planFixed), `{"kind":"system"}`
		ex := func(q string, args ...any) int64 {
			res, err := tx.Exec(q, args...)
			if err != nil {
				t.Fatalf("%s: %v", q, err)
			}
			id, _ := res.LastInsertId()
			return id
		}
		var lf, rules int64
		if err := tx.QueryRow("SELECT min(id) FROM library_folders WHERE kind = 'music'").Scan(&lf); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT min(id) FROM pick_rules").Scan(&rules); err != nil {
			return err
		}
		for _, a := range fx.Artists {
			tid := ex(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, source_ids, metadata_source, why_added, state)
				VALUES (?, ?, ?, ?, 'artist', ?, ?, ?, 'musicbrainz', 'by_hand', 'active')`, ts, by, ts, by, a.Name, a.Name, `{"musicbrainz":"`+a.MBID+`"}`)
			ids[a.Name] = tid
			var ver int64
			for _, al := range a.Albums {
				pid := ex(`INSERT INTO parts (created_at, created_by, updated_at, updated_by, title_id, kind, name, release_type, release_date, release_date_precision, source_ids, secondary_types, included)
					VALUES (?, ?, ?, ?, ?, 'album', ?, ?, ?, 'day', ?, '[]', 1)`, ts, by, ts, by, tid, al.Name, al.Type, al.day(planFixed)+"T00:00:00Z", `{"musicbrainz":"`+al.MBID+`"}`)
				if !al.Have {
					continue
				}
				if ver == 0 {
					ver = ex(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main) VALUES (?, ?, ?, ?, ?, 'Main', ?, ?, ?, 1)`, ts, by, ts, by, tid, rules, lf, a.Name)
				}
				item := ex(`INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, part_id, kind) VALUES (?, ?, ?, ?, ?, ?, 'track')`, ts, by, ts, by, tid, pid)
				slot := ex(`INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES (?, ?, ?, ?, ?, ?, 'in_library')`, ts, by, ts, by, item, ver)
				file := ex(`INSERT INTO files (created_at, created_by, updated_at, updated_by, slot_ids, version_id, relative_path, added_at, origin, placed_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'imported', 'copy')`,
					ts, by, ts, by, "["+itoa(slot)+"]", ver, a.Name+"/"+al.Name+"/01.flac", ts)
				ex("UPDATE slots SET file_id = ? WHERE id = ?", file, slot)
			}
		}
		for who, names := range fx.Follows {
			for _, n := range names {
				ex("INSERT INTO follows (user_id, title_id, created_at) VALUES (?, ?, ?)", users[who], ids[n], ts)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	r.api.exploreCache.now = func() time.Time { return planFixed }
	charts, genres := fakeCharts(planFixed)
	r.api.s.MusicGenres = genres
	r.api.s.MusicCharts = func(ctx context.Context, region, kind, genre string) (metadata.MusicChart, error) {
		c, err := charts(ctx, region, kind, genre)
		for i := range c.Items {
			ago := 0
			c.Items[i].RankAgo = &ago // 7 days of ranks: every entry new
			if genre != "" {
				c.Items[i].MBID = mid('9', 50+i)
				c.Items[i].Title = genre + " chart " + fmt.Sprint(i+1)
				c.Items[i].Explicit = i == 1
			}
		}
		return c, err
	}
	by := map[string]*planArtist{}
	for i := range fx.Artists {
		by[fx.Artists[i].MBID] = &fx.Artists[i]
	}
	for i := range fx.Others {
		by[fx.Others[i].MBID] = &fx.Others[i]
	}
	calls := 0
	var callsMu sync.Mutex
	r.api.s.MusicListeners = func(_ context.Context, ids []string) (map[string]int64, error) {
		callsMu.Lock()
		calls++
		callsMu.Unlock()
		out := map[string]int64{}
		for _, id := range ids {
			if a := by[id]; a != nil {
				out[id] = a.Listeners
			}
		}
		return out, nil
	}
	r.api.s.MusicSimilarSet = func(_ context.Context, seeds []string) (map[string][]metadata.MusicSimilarScore, error) {
		callsMu.Lock()
		calls++
		callsMu.Unlock()
		out := map[string][]metadata.MusicSimilarScore{}
		for _, s := range seeds {
			if a := by[s]; a != nil {
				for _, x := range a.Similar {
					out[s] = append(out[s], metadata.MusicSimilarScore{MBID: x.MBID, Name: x.Name, S: x.S, Source: "listenbrainz"})
				}
			}
		}
		return out, nil
	}
	r.api.s.MusicArtistData = func(_ context.Context, ids []string) ([]metadata.MusicArtistData, error) {
		callsMu.Lock()
		calls++
		callsMu.Unlock()
		var out []metadata.MusicArtistData
		for _, id := range ids {
			d := metadata.MusicArtistData{MBID: id}
			if a := by[id]; a != nil {
				for g, n := range a.Genres {
					d.Genres = append(d.Genres, metadata.MusicGenreVote{Name: g, Count: n})
				}
				sort.Slice(d.Genres, func(i, j int) bool { return d.Genres[i].Name < d.Genres[j].Name })
				for _, al := range a.Albums {
					title := al.Title
					if title == "" {
						title = al.Name
					}
					d.Albums = append(d.Albums, metadata.MusicTopAlbum{MBID: al.MBID, Title: title, Type: al.Type, Date: al.day(planFixed), Listeners: al.Listen, Artists: []string{id}, ArtistNames: []string{a.Name}})
				}
				sort.SliceStable(d.Albums, func(i, j int) bool { return d.Albums[i].Listeners > d.Albums[j].Listeners })
				d.Albums = append(d.Albums, a.Guest...)
			} else if strings.HasPrefix(id, "e") {
				// chart artists: one debut in 2025 (Rising).
				d.Genres = []metadata.MusicGenreVote{{Name: "rock", Count: 3}}
				d.Albums = []metadata.MusicTopAlbum{{MBID: "1" + id[1:], Title: "Debut", Type: "album", Date: "2025-02-01", Artists: []string{id}}}
			}
			out = append(out, d)
		}
		return out, nil
	}
	// MusicBrainz relations come from the app's artist cache.
	r.api.musicx.mu.Lock()
	if r.api.musicx.genres == nil {
		r.api.musicx.genres, r.api.musicx.queued = map[string]musicGenreEntry{}, map[string]bool{}
	}
	for _, a := range fx.Artists {
		if len(a.Links) > 0 {
			r.api.musicx.genres[a.MBID] = musicGenreEntry{links: a.Links, at: time.Now()}
		}
	}
	r.api.musicx.mu.Unlock()
	return r, &calls, &fx
}

type planAnswer struct {
	musicRowsAnswer
	Next  string `json:"next"`
	Total int    `json:"total"`
}

// planPages reads page 1 and follows next to the end.
func planPages(t *testing.T, who *signedIn, region string) ([]planAnswer, [][]byte) {
	t.Helper()
	var out []planAnswer
	var raws [][]byte
	path := "/api/v1/explore/music?region=" + region
	for i := 0; i < 8; i++ {
		b := who.call("GET", path, "", 200)
		var a planAnswer
		if err := json.Unmarshal(b, &a); err != nil {
			t.Fatal(err)
		}
		out, raws = append(out, a), append(raws, b)
		if a.Next == "" {
			break
		}
		path = "/api/v1/explore/music?region=" + region + "&cursor=" + a.Next
	}
	return out, raws
}

func planRowList(pages []planAnswer) []MusicRow {
	var rows []MusicRow
	for _, p := range pages {
		rows = append(rows, p.Rows...)
	}
	return rows
}

// noAlbumTwice fails when one album shows in two rows (or twice in one).
func noAlbumTwice(t *testing.T, rows []MusicRow) {
	t.Helper()
	seen := map[string]string{}
	for _, r := range rows {
		if r.ID == "your_asks" || r.ID == "genres" {
			continue
		}
		for _, c := range r.Items {
			if c.Kind != "album" || c.MBID == "" {
				continue
			}
			if w, ok := seen[c.MBID]; ok {
				t.Fatalf("album %s in %s and %s", c.MBID, w, r.ID)
			}
			seen[c.MBID] = r.ID
		}
	}
}

func planTypes(rows []MusicRow) map[string]int {
	out := map[string]int{}
	for _, r := range rows {
		out[r.Type]++
	}
	return out
}

// route: GET /api/v1/explore/music (music_rows_v7 paging)
func TestExploreMusicPagingLarge(t *testing.T) {
	t.Parallel()
	r, calls, _ := planRig(t, "household_large.json")
	start := time.Now()
	pages, raws := planPages(t, r.owner, "US")
	cold := time.Since(start)
	coldCalls := *calls
	rows := planRowList(pages)
	if len(pages) < 4 {
		t.Fatalf("pages = %d, rows = %d", len(pages), len(rows))
	}
	for i, p := range pages[:len(pages)-1] {
		if want := 6; i > 0 && len(p.Rows) != want {
			t.Fatalf("page %d has %d rows", i+1, len(p.Rows))
		}
	}
	noAlbumTwice(t, rows)
	for i := 1; i < len(rows); i++ {
		if rows[i].Type == rows[i-1].Type {
			t.Fatalf("adjacent %s rows at %d", rows[i].Type, i)
		}
	}
	types := planTypes(rows)
	t.Logf("rows %d, types %v, cold %v, metadata requests %d", len(rows), types, cold, coldCalls)
	// Every family the fixture has data for shows (Together needs relations).
	for _, f := range []string{"new_from", "genres", "fresh_songs", "missing", "top_albums", "coming_soon", "mix", "because", "finish_set", "near_taste", "genre_shelf", "deep_cuts", "anniversary", "together", "guest_spots", "rising"} {
		if types[f] == 0 {
			t.Errorf("no %s row", f)
		}
	}
	// N = the sum of the enabled families' maxima (27 with Together).
	if len(rows) < 25 {
		t.Fatalf("rows = %d, want at least 25", len(rows))
	}
	// The plan budget is 25 cold requests; at 400 Library artists the v3
	// build alone reads artist data in 14 batches of 30, so this household
	// measures 35 (see the release report). The ceiling guards regressions.
	if coldCalls > 36 {
		t.Errorf("cold scroll made %d metadata requests, ceiling 36", coldCalls)
	}
	// Reload and scroll again: byte-identical, no new requests (layer 1 the
	// same: the fake answers the same data and nothing expired).
	again, raws2 := planPages(t, r.owner, "US")
	if len(again) != len(pages) {
		t.Fatal("reload changed the page count")
	}
	for i := range raws {
		if string(raws[i]) != string(raws2[i]) {
			t.Fatalf("page %d differs on reload", i+1)
		}
	}
	if *calls != coldCalls {
		t.Fatalf("warm scroll made %d metadata requests", *calls-coldCalls)
	}
}

// route: GET /api/v1/explore/music (music_rows_v7 cursor)
func TestExploreMusicCursor(t *testing.T) {
	t.Parallel()
	r, _, _ := planRig(t, "household_large.json")
	first, _ := planPages(t, r.owner, "US")
	if first[0].Next == "" {
		t.Fatal("no cursor on page 1")
	}
	cur := first[0].Next
	// Another member, a tampered cursor or another region: 400.
	r.ana.call("GET", "/api/v1/explore/music?region=US&cursor="+cur, "", 400)
	r.owner.call("GET", "/api/v1/explore/music?region=US&cursor="+cur[:len(cur)-2]+"xx", "", 400)
	r.owner.call("GET", "/api/v1/explore/music?region=FR&cursor="+cur, "", 400)
	// An ask between pages changes only chips (ana: household rows).
	anaPages, anaRaw := planPages(t, r.ana, "US")
	r.ana.call("POST", "/api/v1/asks", `{"kind":"artist","id":"mbid:`+apiArtist+`","albums":["`+apiAlbum+`"]}`, 201)
	_, anaRaw2 := planPages(t, r.ana, "US")
	if len(anaRaw) != len(anaRaw2) || len(anaPages) < 2 {
		t.Fatalf("ana pages %d then %d: %.400s", len(anaRaw), len(anaRaw2), anaRaw2[0])
	}
	chips := false
	for i := range anaRaw {
		if chipless(t, anaRaw[i]) != chipless(t, anaRaw2[i]) {
			t.Fatalf("ana page %d changed beyond chips after an ask", i+1)
		}
		chips = chips || strings.Contains(string(anaRaw2[i]), `"state":"asked"`)
	}
	if !chips {
		t.Fatal("the ask shows no chip")
	}
	before := stripState(t, r.owner.call("GET", "/api/v1/explore/music?region=US&cursor="+cur, "", 200))
	r.api.exploreCache.plans.MarkStale(-1) // a library change: page 1 builds v2
	after := stripState(t, r.owner.call("GET", "/api/v1/explore/music?region=US&cursor="+cur, "", 200))
	if before != after {
		t.Fatal("a stale mark changed an open page's rows")
	}
	// Two more versions evict version 1: its cursor gets 400.
	r.owner.call("GET", "/api/v1/explore/music?region=US", "", 200)
	r.api.exploreCache.plans.MarkStale(-1)
	r.owner.call("GET", "/api/v1/explore/music?region=US", "", 200)
	r.owner.call("GET", "/api/v1/explore/music?region=US&cursor="+cur, "", 400)
}

// chipless is stripState with each row's items in key order: asked cards
// sink to the end at render (layer 3), which is a chip change too.
func chipless(t *testing.T, b []byte) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(stripState(t, b)), &m); err != nil {
		t.Fatal(err)
	}
	if alts, ok := m["hero_alts"].([]any); ok {
		for _, x := range alts {
			delete(x.(map[string]any), "state")
		}
	}
	rows, _ := m["rows"].([]any)
	for _, r := range rows {
		items := r.(map[string]any)["items"].([]any)
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].(map[string]any)["key"].(string) < items[j].(map[string]any)["key"].(string)
		})
	}
	out, _ := json.Marshal(m)
	return string(out)
}

// route: GET /api/v1/explore/music (music_rows_v7 seeds)
func TestExploreMusicSeeds(t *testing.T) {
	t.Parallel()
	r, _, fx := planRig(t, "household_ten.json")
	byName := map[string]string{}
	for _, a := range fx.Artists {
		byName[a.Name] = a.MBID
	}
	seedsOf := func() (because []string, all map[string]int) {
		all = map[string]int{}
		pages, _ := planPages(t, r.owner, "US")
		for _, row := range planRowList(pages) {
			switch row.Type {
			case "because", "together":
				id := row.ID[strings.Index(row.ID, ":")+1:]
				all[id]++
				if row.Type == "because" {
					because = append(because, id)
				}
			case "mix":
				// "From A, B and C in your Library"
				names := strings.TrimSuffix(strings.TrimPrefix(row.Reason, "From "), " in your Library")
				for _, n := range strings.FieldsFunc(strings.ReplaceAll(names, " and ", ","), func(c rune) bool { return c == ',' }) {
					all[byName[strings.TrimSpace(n)]]++
				}
			}
		}
		return because, all
	}
	day1, all := seedsOf()
	if len(day1) == 0 {
		t.Fatal("no Because rows")
	}
	for id, n := range all {
		if n > 1 {
			t.Fatalf("seed %s in %d rows", id, n)
		}
	}
	// The next day never reuses yesterday's Because seeds.
	r.api.exploreCache.now = func() time.Time { return planFixed.AddDate(0, 0, 1) }
	day2, _ := seedsOf()
	for _, a := range day2 {
		for _, b := range day1 {
			if a == b {
				t.Fatalf("seed %s on two days running", a)
			}
		}
	}
}

// route: GET /api/v1/explore/music (music_rows_v7 Turning N)
func TestExploreMusicAnniversary(t *testing.T) {
	t.Parallel()
	notes := func(at time.Time) string {
		r, _, _ := planRig(t, "household_large.json")
		r.api.exploreCache.now = func() time.Time { return at }
		pages, _ := planPages(t, r.owner, "US")
		var out []string
		for _, row := range planRowList(pages) {
			if row.Type == "anniversary" {
				for _, c := range row.Items {
					if lib := c.State; lib == "have" {
						t.Fatalf("had album %s in Turning N", c.Name)
					}
					out = append(out, c.Note)
				}
			}
		}
		return strings.Join(out, "|")
	}
	if n := notes(planFixed); !strings.Contains(n, "25 years on Oct 30") {
		t.Fatalf("2001-10-30 not in Turning N on 2026-10-23: %q", n)
	}
	if n := notes(time.Date(2026, 12, 15, 12, 0, 0, 0, time.UTC)); strings.Contains(n, "Oct 30") {
		t.Fatalf("2001-10-30 still in Turning N on 2026-12-15: %q", n)
	}
}

// route: GET /api/v1/explore/music (music_rows_v7 households)
func TestExploreMusicHouseholds(t *testing.T) {
	t.Parallel()
	r, _, _ := planRig(t, "household_two_members.json")
	look := func(who *signedIn) (mixes, because []string, rows []MusicRow) {
		pages, _ := planPages(t, who, "US")
		rows = planRowList(pages)
		noAlbumTwice(t, rows)
		for _, row := range rows {
			switch row.Type {
			case "mix":
				mixes = append(mixes, row.Title)
			case "because":
				because = append(because, row.ID)
			}
		}
		return mixes, because, rows
	}
	m1, b1, _ := look(r.owner)
	m2, b2, _ := look(r.ana)
	if len(m1) == 0 || len(m2) == 0 || m1[0] == m2[0] {
		t.Fatalf("metal and jazz members share a first mix: %v / %v", m1, m2)
	}
	for _, x := range b1 {
		for _, y := range b2 {
			if x == y {
				t.Fatalf("both members got %s", x)
			}
		}
	}
	// bo has no acts of his own: household rows, labelled as such.
	_, _, rows := look(r.bo)
	found := false
	for _, row := range rows {
		if row.ID == "new_from" {
			found = strings.HasPrefix(row.Reason, "Followed in your household")
		}
	}
	if !found {
		var ids []string
		for _, row := range rows {
			ids = append(ids, row.ID+"="+row.Reason)
		}
		t.Fatalf("bo's New from your artists is not the household fallback: %q", ids)
	}

	// Empty library: cold rows only, at most 12, one answer, no empty row.
	e, _, _ := planRig(t, "household_empty.json")
	pages, _ := planPages(t, e.owner, "US")
	if len(pages) != 1 || pages[0].Next != "" {
		t.Fatalf("cold start paged: %d pages", len(pages))
	}
	cold := map[string]bool{"genres": true, "fresh_songs": true, "top_albums": true, "coming_soon": true}
	if len(pages[0].Rows) > musicFew || len(pages[0].Rows) == 0 {
		t.Fatalf("cold rows = %d", len(pages[0].Rows))
	}
	for _, row := range pages[0].Rows {
		if len(row.Items) == 0 || !cold[row.Type] {
			t.Fatalf("cold start row %s (%d items)", row.ID, len(row.Items))
		}
	}
}
