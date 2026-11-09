package music

import (
	"context"
	"fmt"
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	musicscore "github.com/HolyArrs/holyarrs/app/backend/internal/music/score"
)

// fakeShelves answers every region with the same blocks: 30 items each,
// album MBIDs prefixed '7', artist MBIDs '6', 40 artists in turn, and
// blocks that overlap (top_now shares items with albums_week), so dedup
// across rows matters.
func fakeShelves(_ context.Context, region string) (metadata.MusicShelves, error) {
	sh := metadata.MusicShelves{Region: region, Chart: region, Neighbor: "ca", AsOf: "2026-10-24", Blocks: map[string][]metadata.MusicShelfItem{}}
	if region == "global" {
		sh.Neighbor = "us"
	}
	album := func(n int) metadata.MusicShelfItem {
		a := n % 40
		return metadata.MusicShelfItem{Kind: "album", MBID: mid('7', n), ArtistMBID: mid('6', a), Title: fmt.Sprintf("Album %d", n), Artist: fmt.Sprintf("Artist %d", a), Rank: n%30 + 1, Prev: n%30 + 4, P: 1 - float64(n%30)/30, M: 0.5, Days: 50, Date: "2026-10-20"}
	}
	artist := func(n int) metadata.MusicShelfItem {
		a := n % 40
		return metadata.MusicShelfItem{Kind: "artist", MBID: mid('6', a), Artist: fmt.Sprintf("Artist %d", a), Rank: n%30 + 1, P: 1 - float64(n%30)/30, Song: "Song " + fmt.Sprint(n)}
	}
	for b, start := range map[string]int{"top_now": 0, "top_world": 20, "albums_week": 10, "albums_world": 40, "climbing": 60, "new_entries": 80, "new_friday": 100,
		"big_new_albums": 120, "long_runners": 140, "number_ones": 160, "comebacks": 180, "neighbor": 200} {
		for i := 0; i < 30; i++ {
			sh.Blocks[b] = append(sh.Blocks[b], album(start+i))
		}
	}
	for _, b := range []string{"artists_week", "singles_artists", "video_artists"} {
		for i := 0; i < 30; i++ {
			sh.Blocks[b] = append(sh.Blocks[b], artist(i+map[string]int{"artists_week": 0, "singles_artists": 15, "video_artists": 5}[b]))
		}
	}
	for i, k := range []string{"pop", "rock", "latin", "chill", "party", "decades", "fresh_faces", "dance", "country"} {
		for j := 0; j < 24; j++ {
			sh.Blocks["tile:"+k] = append(sh.Blocks["tile:"+k], album(220+30*i+j))
		}
	}
	return sh, nil
}

func listRig(t *testing.T, name string) *askRig {
	r, _, _ := planRig(t, name)
	r.api.s.MusicShelves = fakeShelves
	return r
}

// route: GET /api/v1/explore/music (music_lists_v1)
func TestExploreMusicListRows(t *testing.T) {
	for _, c := range []struct {
		name string
		min  int
	}{{"household_empty.json", 16}, {"household_indie.json", 16}, {"household_large.json", 28}} {
		r := listRig(t, c.name)
		pages, _ := planPages(t, r.owner, "US")
		rows := planRowList(pages)
		noAlbumTwice(t, rows)
		lists := 0
		for _, row := range rows {
			if strings.TrimSpace(row.Reason) == "" {
				t.Fatalf("%s: row %s has no reason", c.name, row.ID)
			}
			if strings.HasPrefix(row.ID, "lists_") {
				lists++
				per := map[string]int{}
				for _, it := range row.Items {
					per[it.ArtistMBID]++
					if per[it.ArtistMBID] > 2 {
						t.Fatalf("%s: %s has 3 cards of %s", c.name, row.ID, it.Artist)
					}
				}
				if len(per) < 4 {
					t.Fatalf("%s: %s has %d artists", c.name, row.ID, len(per))
				}
			}
		}
		if len(rows) < c.min || lists < 6 {
			t.Fatalf("%s: %d rows (%d list rows), want ≥ %d", c.name, len(rows), lists, c.min)
		}
		if h := pages[0].Hero; h == nil || h.Kicker != "Biggest right now" {
			t.Fatalf("%s: Spotlight not led by the chart: %+v", c.name, h)
		}
		// Browse grid: shape grid, every cover different.
		grid := false
		covers := map[string]bool{}
		for _, row := range rows {
			if row.ID != "genres" {
				continue
			}
			grid = row.Shape == "grid" && row.Title == "Browse"
			for _, it := range row.Items {
				if it.Cover != "" {
					if covers[it.Cover] {
						t.Fatalf("cover %s on two tiles", it.Cover)
					}
					covers[it.Cover] = true
				}
			}
		}
		if !grid || len(covers) == 0 {
			t.Fatalf("%s: no Browse grid with covers", c.name)
		}
	}
	// Same plan on reload the same day.
	r := listRig(t, "household_indie.json")
	a, _ := planPages(t, r.owner, "US")
	b, _ := planPages(t, r.owner, "US")
	if fmt.Sprint(rowIDs(planRowList(a))) != fmt.Sprint(rowIDs(planRowList(b))) {
		t.Fatal("plan changed on reload")
	}
}

func rowIDs(rows []MusicRow) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

func TestNewMusicListRowsLeadWithCharts(t *testing.T) {
	r := listRig(t, "household_indie.json")
	req := httptest.NewRequest("GET", "/api/v1/explore/new/music?region=US", nil)
	existing := []MusicRow{{ID: "old", Type: "old", Title: "Old", Reason: "r", Shape: "square", Items: []MusicCard{listCard(metadata.MusicShelfItem{Kind: "album", MBID: mid('7', 0), Title: "Album 0", Artist: "Artist 0"})}}}
	hero, rows := r.api.musicNewListRows(context.Background(), req, musicNewInput{}, nil, existing, planFixed)
	// The hero is the recent charting albums, momentum first (every
	// fixture album is 4 days old).
	if len(hero) < 3 || hero[0].Kind != "album" || hero[0].Kicker != "New album" {
		t.Fatalf("hero is not recent charting albums: %+v", hero)
	}
	if len(rows) < 12 || rows[0].ID != "lists_new_charting" {
		t.Fatalf("%d rows, first %v", len(rows), rowIDs(rows))
	}
	noAlbumTwice(t, rows)
	for _, row := range rows {
		for _, c := range row.Items {
			for _, h := range hero {
				if c.Key == h.Key {
					t.Fatalf("hero album %s again in %s", c.Name, row.ID)
				}
			}
		}
	}
}

// Every reason line is one of our templates, rendered; never a list name.
func TestListReasonsAreTemplates(t *testing.T) {
	for _, region := range []string{"us", "global", "br", "zz"} {
		sh, _ := fakeShelves(context.Background(), region)
		for _, d := range listRows(&sh) {
			ok := false
			for _, tpl := range listTemplates {
				re := "^" + regexp.QuoteMeta(tpl.Reason) + "$"
				for _, slot := range []string{"{place2}", "{place}", "{neighbor}"} {
					re = strings.ReplaceAll(re, regexp.QuoteMeta(slot), "(worldwide|in [A-Z][A-Za-z ]+|in the [A-Z][A-Za-z ]+)")
				}
				re = strings.ReplaceAll(re, regexp.QuoteMeta("{tile}"), `([A-Z][A-Za-z&\- ]+)`)
				if regexp.MustCompile(re).MatchString(d.Reason) {
					ok = true
				}
			}
			if !ok || strings.Contains(d.Title, "{") {
				t.Fatalf("%s: %q / %q not rendered from a template", region, d.Title, d.Reason)
			}
		}
	}
}

// The list source's brand never shows: not in the web app, the API's
// strings, log lines or settings (plan 24 October 2026 §8).
func TestNoListBrandWord(t *testing.T) {
	brand := regexp.MustCompile(`(?i)` + "spot" + "ify")
	roots := []string{"../../../frontend/src", ".", "../../cmd", "../music", "../metadata"}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if n := d.Name(); n == "node_modules" || n == ".svelte-kit" || n == "testdata" || n == ".omc" {
					return filepath.SkipDir
				}
				return nil
			}
			ext := filepath.Ext(p)
			if strings.HasSuffix(p, "_test.go") || (ext != ".go" && ext != ".svelte" && ext != ".ts" && ext != ".json" && ext != ".html") {
				return nil
			}
			if strings.HasSuffix(p, ".test.ts") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if loc := brand.FindIndex(b); loc != nil {
				t.Errorf("brand word in %s", p)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// Every album and artist a list row shows is in the fetch-ahead set made
// when the shelves arrive (container start, list refresh).
func TestListAheadCoversEveryListCard(t *testing.T) {
	r := listRig(t, "household_large.json")
	sh, _ := fakeShelves(context.Background(), "us")
	ahead := map[aheadItem]bool{}
	for _, it := range listAhead(&sh) {
		ahead[it] = true
	}
	pages, _ := planPages(t, r.owner, "US")
	n := 0
	for _, row := range planRowList(pages) {
		if !strings.HasPrefix(row.ID, "lists_") {
			continue
		}
		for _, c := range row.Items {
			n++
			if !ahead[aheadItem{c.Kind, c.MBID}] || c.Kind == "album" && !ahead[aheadItem{"artist", c.ArtistMBID}] {
				t.Fatalf("%s card %s not fetched ahead", row.ID, c.Key)
			}
		}
	}
	if n == 0 {
		t.Fatal("no list cards")
	}
	if pages[0].Hero != nil && !ahead[aheadItem{"album", pages[0].Hero.MBID}] {
		t.Fatal("hero not fetched ahead")
	}
}

// Tile rows (editorial lists) on Explore: our names only, a reason each,
// moods and genres; the video row only with artists whose videos are known.
func TestExploreMusicTileAndVideoRows(t *testing.T) {
	r := listRig(t, "household_empty.json")
	pages, _ := planPages(t, r.owner, "US")
	rows := planRowList(pages)
	noAlbumTwice(t, rows)
	tiles := 0
	for _, row := range rows {
		if strings.HasPrefix(row.ID, "lists_tile_") {
			tiles++
			if strings.Contains(row.Title, "{") || row.Reason == "" {
				t.Fatalf("tile row %+v", row)
			}
		}
		if row.ID == "lists_videos" {
			t.Fatal("video row without known videos")
		}
	}
	if tiles < 6 || len(rows) < 25 {
		t.Fatalf("%d tile rows, %d rows", tiles, len(rows))
	}
	// Known videos for 8 artists: the row shows, wide, only those artists.
	r2 := listRig(t, "household_empty.json")
	known := map[string]bool{}
	r2.api.artistVideos.mu.Lock()
	if r2.api.artistVideos.m == nil {
		r2.api.artistVideos.m = map[string]artistVideoList{}
	}
	for i := 5; i < 13; i++ {
		known[mid('6', i)] = true
		r2.api.artistVideos.m[mid('6', i)] = artistVideoList{at: time.Now(), list: []metadata.ArtistVideo{{ID: "v" + fmt.Sprint(i)}}}
	}
	r2.api.artistVideos.mu.Unlock()
	sh, _ := fakeShelves(context.Background(), "us")
	var d listRowDef
	for _, x := range listRows(&sh) {
		if x.ID == "lists_videos" {
			d = x
		}
	}
	items := r2.api.listBlock(context.Background(), &sh, d)
	if len(items) != 8 || listRowMeta(d).Shape != "wide" {
		t.Fatalf("video items %d shape %s", len(items), listRowMeta(d).Shape)
	}
	for _, it := range items {
		if !known[it.MBID] {
			t.Fatalf("artist %s without videos in the row", it.Artist)
		}
	}
}

// New & Popular adds up to six "Big this week in {genre}" rows after the
// chart rows, genres only.
func TestNewMusicWeekRows(t *testing.T) {
	r := listRig(t, "household_indie.json")
	req := httptest.NewRequest("GET", "/api/v1/explore/new/music?region=US", nil)
	_, rows := r.api.musicNewListRows(context.Background(), req, musicNewInput{}, nil, nil, planFixed)
	week := 0
	for _, row := range rows {
		if strings.HasPrefix(row.ID, "lists_big_") {
			week++
			if strings.Contains(row.ID, "chill") || strings.Contains(row.ID, "party") {
				t.Fatalf("mood in genre rows: %s", row.ID)
			}
		}
	}
	if week == 0 || week > listNewWeekRows || len(rows) < 18 {
		t.Fatalf("%d week rows of %d", week, len(rows))
	}
	noAlbumTwice(t, rows)
}

func TestListTileOfSlug(t *testing.T) {
	for slug, key := range map[string]string{"pop": "pop", "r-and-b": "rnb", "k-pop": "kpop", "electronic": "dance", "chill": "chill"} {
		if lt, ok := listTileOf(slug); !ok || lt.Key != key {
			t.Fatalf("%s -> %+v", slug, lt)
		}
	}
	if _, ok := listTileOf("polka"); ok {
		t.Fatal("polka has no tile")
	}
}

// After a restart, kept plans built from older shelves are marked stale so
// the day's plan picks up the new list rows (the stale plan is still served
// while it is rebuilt); unchanged shelves keep them current.
func TestListShelvesChangeWhileDownDropsKeptPlans(t *testing.T) {
	r := listRig(t, "household_indie.json")
	a := r.api
	ctx := context.Background()
	now := time.Now()
	snap := musicSnapKey(1, "US")
	put := func() {
		_ = metadata.LookupCache{Store: a.s.Store}.Put(ctx, snap, []byte("{}"), now, now.Add(time.Hour))
	}
	kept := func() bool {
		_, ok, _ := metadata.LookupCache{Store: a.s.Store}.Get(ctx, snap, now)
		clock := now
		if a.exploreCache.now != nil {
			clock = a.exploreCache.now()
		}
		_, stale, _ := metadata.LookupCache{Store: a.s.Store}.Get(ctx, musicStalePrefix+"all", clock)
		return ok && !stale
	}
	wait := func(want bool) {
		for i := 0; i < 50 && kept() != want; i++ {
			time.Sleep(20 * time.Millisecond)
		}
		if kept() != want {
			t.Fatalf("kept plan present=%v, want %v", kept(), want)
		}
	}
	// A restart with the same shelves as stored: the plan stays.
	sh, _ := fakeShelves(ctx, "us")
	_ = metadata.LookupCache{Store: a.s.Store}.Put(ctx, listSigPrefix+"shelves:us", []byte(sh.AsOf+"|"+fmt.Sprint(listCount(&sh))), now, now.Add(time.Hour))
	put()
	a.listSeen = nil
	a.exploreCache.cand = musicscore.Candidates{}
	if a.musicShelvesOf(ctx, "us", now) == nil {
		t.Fatal("no shelves")
	}
	time.Sleep(100 * time.Millisecond)
	wait(true)
	// Shelves changed while down: the kept plan is no longer current.
	_ = metadata.LookupCache{Store: a.s.Store}.Put(ctx, listSigPrefix+"shelves:us", []byte("2026-10-23|5"), now, now.Add(time.Hour))
	a.listSeen = nil
	a.exploreCache.cand = musicscore.Candidates{}
	a.musicShelvesOf(ctx, "us", now)
	wait(false)
}

// oldShelves is fakeShelves with every even item a 5,000-day-old album
// that charts today and every odd one out 20 days ago.
func oldShelves(ctx context.Context, region string) (metadata.MusicShelves, error) {
	sh, _ := fakeShelves(ctx, region)
	old := planFixed.AddDate(0, 0, -5000).Format("2006-01-02")
	fresh := planFixed.AddDate(0, 0, -20).Format("2006-01-02")
	for b, l := range sh.Blocks {
		for i := range l {
			if i%2 == 0 {
				l[i].Date = old
			} else {
				l[i].Date = fresh
			}
		}
		sh.Blocks[b] = l
	}
	return sh, nil
}

func TestListKickerSaysTheRealReason(t *testing.T) {
	now := planFixed
	old := metadata.MusicShelfItem{Kind: "album", Rank: 12, Date: now.AddDate(0, 0, -5000).Format("2006-01-02")}
	if k := listKicker("top_world", "worldwide", old, now); k != "#12 worldwide" {
		t.Fatalf("old chart album: %q", k)
	}
	if k := listKicker("top_now", "in the US", old, now); k != "#12 in the US" {
		t.Fatalf("old chart album: %q", k)
	}
	if k := listKicker("climbing", "in the US", old, now); k != "Climbing" {
		t.Fatalf("old climber: %q", k)
	}
	fresh := old
	fresh.Date = now.AddDate(0, 0, -20).Format("2006-01-02")
	if k := listKicker("top_world", "worldwide", fresh, now); k != "New album" {
		t.Fatalf("new album: %q", k)
	}
	edge := old
	edge.Date = now.AddDate(0, 0, -61).Format("2006-01-02")
	if k := listKicker("albums_week", "", edge, now); strings.HasPrefix(k, "New") {
		t.Fatalf("61 days old says %q", k)
	}
	cards := []MusicCard{
		{Kicker: "New album", Badge: "New album", Date: now.AddDate(0, 0, -5000).Format("2006-01-02"), Rank: 3},
		{Badge: "New EP", Date: "2012"},
		{Kicker: "New album", Date: now.AddDate(0, 0, -10).Format("2006-01-02")},
	}
	honestLabels(cards, now)
	if cards[0].Kicker != "#3 album this week" || cards[0].Badge != "" || cards[1].Badge != "" || cards[1].Kicker != "Popular now" || cards[2].Kicker != "New album" {
		t.Fatalf("labels %+v", cards)
	}
}

// New & Popular: the hero and the first rows are music out in the last 90
// days; no card says New when it is older than 60 days; old chart hits only
// in the popularity rows below; one album chart when worldwide.
func TestNewMusicRecentFirstAndHonestLabels(t *testing.T) {
	for _, region := range []string{"US", ""} {
		r := listRig(t, "household_indie.json")
		r.api.s.MusicShelves = oldShelves
		req := httptest.NewRequest("GET", "/api/v1/explore/new/music?region="+region, nil)
		own := []MusicRow{{ID: "np_music_top", Type: "np_music_top", Title: "Top albums", Reason: "r", Shape: "square",
			Items: []MusicCard{{Key: "album:" + mid('8', 1), Kind: "album", MBID: mid('8', 1), Name: "Old", Artist: "Z", Badge: "New album", Date: "2012-05-01"}}}}
		var songs []MusicCard
		for i := 0; i < 8; i++ {
			d := planFixed.AddDate(0, 0, -10).Format("2006-01-02")
			if i%2 == 0 {
				d = "2025-03-01"
			}
			songs = append(songs, MusicCard{Key: fmt.Sprintf("track:%d", i), Kind: "album", MBID: mid('9', i), Name: fmt.Sprint("Song ", i), Artist: fmt.Sprint("S", i), Date: d})
		}
		own = append(own, MusicRow{ID: "np_music_tracks", Type: "np_music_tracks", Title: "Best new songs", Reason: "r", Shape: "song", Items: songs})
		hero, rows := r.api.musicNewListRows(context.Background(), req, musicNewInput{}, nil, own, planFixed)
		if len(hero) < 3 {
			t.Fatalf("%q: hero %d", region, len(hero))
		}
		for _, c := range hero {
			if d, ok := listAge(c.Date, planFixed); !ok || d > listRecentDays {
				t.Fatalf("%q: hero %s from %s", region, c.Name, c.Date)
			}
		}
		check := func(where string, c MusicCard) {
			d, ok := listAge(c.Date, planFixed)
			if (strings.HasPrefix(c.Kicker, "New") || strings.HasPrefix(c.Badge, "New")) && (!ok || d > listRecentDays) {
				t.Fatalf("%q: %s labelled New from %s (%s)", region, c.Name, c.Date, where)
			}
		}
		for _, c := range hero {
			check("hero", c)
		}
		recentPart := true
		world, week := false, false
		for _, row := range rows {
			if !strings.HasPrefix(row.ID, "lists_new") && row.ID != "lists_big_new" && !listNewOwnFirst(row.ID) {
				recentPart = false
			}
			world = world || row.ID == "lists_albums_world"
			week = week || row.ID == "lists_albums_week"
			for _, c := range row.Items {
				check(row.ID, c)
				if recentPart && c.Kind == "album" {
					if d, ok := listAge(c.Date, planFixed); !ok || d > listRecentDays {
						t.Fatalf("%q: %s from %s in recent row %s", region, c.Name, c.Date, row.ID)
					}
				}
			}
		}
		for _, row := range rows {
			if row.ID == "np_music_tracks" {
				for _, c := range row.Items {
					if !listRecent(c.Date, listRecentDays, planFixed) {
						t.Fatalf("%q: old song %s (%s) in Best new songs", region, c.Name, c.Date)
					}
				}
			}
		}
		if rows[0].ID != "lists_new_charting" {
			t.Fatalf("%q: first row %s", region, rows[0].ID)
		}
		if region == "" && world && week {
			t.Fatal("worldwide page shows the week's album chart twice")
		}
		noAlbumTwice(t, rows)
	}
}
