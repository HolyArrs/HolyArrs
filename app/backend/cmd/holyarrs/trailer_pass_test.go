package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// passPool is a stored pool: Trending (the hero's source) and two more
// head rows, eight movies each (ids base..base+7).
func passPool(member int64, sw string, kid bool) engine.Pool {
	items := func(base int) []engine.Item {
		var out []engine.Item
		for i := range 8 {
			out = append(out, engine.Item{TMDBID: strconv.Itoa(base + i), Kind: "movie", Name: "T" + strconv.Itoa(base+i), Genres: []string{"Drama"}})
		}
		return out
	}
	return engine.Pool{Version: engine.PoolVersion, Member: member, Switch: sw, Date: engine.Day(time.Now()), Kid: kid, Built: time.Now(),
		Rows: []engine.Row{{ID: "trending", Type: engine.TypeTrending, Items: items(100)}, {ID: "best", Type: engine.TypeBest, Items: items(200)}, {ID: "classics", Type: engine.TypeClassics, Items: items(300)}}}
}

func storePool(t *testing.T, st *database.Store, p engine.Pool) {
	t.Helper()
	raw, _ := json.Marshal(p)
	err := st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO explore_pool (member_id, type_switch, day, built_at, stale, due, pool) VALUES (?, ?, ?, ?, 0, NULL, ?)`,
			p.Member, p.Switch, p.Date, database.FormatTime(time.Now()), string(raw))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// widePool is a stored pool with Trending (ids 100..107) and 14 more rows
// of eight movies (row i: ids 200+100i..), so some rows land in the tail.
func widePool(member int64) engine.Pool {
	items := func(base int) []engine.Item {
		var out []engine.Item
		for i := range 8 {
			out = append(out, engine.Item{TMDBID: strconv.Itoa(base + i), Kind: "movie", Name: "T" + strconv.Itoa(base+i), Genres: []string{"Drama"}})
		}
		return out
	}
	rows := []engine.Row{{ID: "trending", Type: engine.TypeTrending, Items: items(100)}}
	for i := range 14 {
		rows = append(rows, engine.Row{ID: "mood" + strconv.Itoa(i), Type: engine.TypeMood, Items: items(200 + 100*i)})
	}
	return engine.Pool{Version: engine.PoolVersion, Member: member, Switch: "all", Date: engine.Day(time.Now()), Built: time.Now(), Rows: rows}
}

// syncBuf is a log output safe for the loops logging next to a test.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

// passLines are the pass's log lines since the last call.
func (s *syncBuf) passLines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, l := range strings.Split(s.b.String(), "\n") {
		if i := strings.Index(l, "trailers: read-ahead"); i >= 0 {
			out = append(out, l[i:])
		}
	}
	s.b.Reset()
	return out
}

func catchLog(t *testing.T) *syncBuf {
	t.Helper()
	b := &syncBuf{}
	old, flags := log.Writer(), log.Flags()
	log.SetOutput(b)
	t.Cleanup(func() { log.SetOutput(old); log.SetFlags(flags) })
	return b
}

// passRig is an API whose trailer key is the metadata server's pick, which
// notes each title asked for (once per first lookup); "107" has none. The
// gap between lookups is 0.
func passRig(t *testing.T, st *database.Store, pc *parts.Control) (*handlers, func() []string) {
	t.Helper()
	gap := metadata.SetTrailerPassGap(0)
	t.Cleanup(func() { metadata.SetTrailerPassGap(gap) })
	var mu sync.Mutex
	var asked []string
	a := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test", Parts: pc, TrailerPick: func(ctx context.Context, kind, id, lang string) (string, error) {
		if !metadata.IsAhead(ctx) {
			t.Errorf("%s:%s asked without the fetch-ahead mark", kind, id)
		}
		mu.Lock()
		asked = append(asked, kind+":"+id)
		mu.Unlock()
		if id == "107" {
			return "", metadata.ErrNotFound
		}
		return "trailerKey1", nil
	}})
	return a, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), asked...)
	}
}

// The pass covers every card Explore shows: the hero candidates first,
// then every card of every head and tail row in page order (Trending on
// streaming from its kept lists), each once, none on the Never-add list. It
// does at most its budget per pass, logs what was due, and leftovers
// continue on the next pass; kept addresses and misses are not looked up
// again. The route then reads what it kept.
func TestTrailerPassCoversEveryCard(t *testing.T) {
	yt := fakeReader(t, "ok")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a, asked := passRig(t, st, nil)
	logs := catchLog(t)
	pool := widePool(1)
	storePool(t, st, pool)
	if err := st.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO explore_prefs (member_id, services, region) VALUES (1, '8', 'US')`); err != nil {
			return err
		}
		_, err := database.Insert(tx, "never_add", &mediafiles.NeverAdd{Kind: "movie", SourceIDs: map[string]string{"tmdb": "205"}, Name: "T205", Reason: "test", CreatedAt: time.Now()}, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	a.TrailerTest().PutServiceTop("US", metadata.ServiceTop{Region: "US", Services: []metadata.ServiceTopList{
		{Service: "netflix", Items: []metadata.ServiceTopItem{{TMDBID: 9001, Kind: "movie"}, {TMDBID: 100, Kind: "movie"}, {TMDBID: 9002, Kind: "tv"}}}}})

	// What the page shows, in its order.
	pg := a.TrailerTest().PoolPage(pool, 1, "all", "US", engine.Moment{Local: a.TrailerTest().Explore().Now().Local()})
	if len(pg.Tail) == 0 || pg.Head[0].Type != engine.TypeTopServices {
		t.Fatalf("pool lays out %d head, %d tail rows", len(pg.Head), len(pg.Tail))
	}
	var want []string
	seen := map[string]bool{}
	add := func(kind, id string) {
		if k := kind + ":" + id; !seen[k] && k != "movie:205" {
			seen[k] = true
			want = append(want, k)
		}
	}
	for _, it := range discover.HeroCandidates(pg, true) {
		add(it.Kind, it.TMDBID)
	}
	if len(want) != 8 {
		t.Fatalf("%d hero candidates", len(want))
	}
	for _, row := range append(append([]engine.Row{}, pg.Head...), pg.Tail...) {
		if row.Type == engine.TypeTopServices {
			add("movie", "9001")
			add("show", "9002")
		}
		for _, it := range row.Items {
			add(it.Kind, it.TMDBID)
		}
	}
	if len(want) != 8+14*8+2-1 {
		t.Fatalf("page shows %d titles", len(want))
	}

	ctx := metadata.Ahead(context.Background())
	if n := a.Trailers().Pass(ctx, 20); n != 20 || strings.Join(asked(), " ") != strings.Join(want[:20], " ") {
		t.Fatalf("first pass %d lookups: %v", n, asked())
	}
	if got := strings.Join(logs.passLines(), "|"); got != "trailers: read-ahead 121 due, 20 looked up, 101 left for the next pass" {
		t.Fatalf("log %q", got)
	}
	if n := a.Trailers().Pass(ctx, 20); n != 20 || strings.Join(asked()[20:], " ") != strings.Join(want[20:40], " ") {
		t.Fatalf("second pass %d lookups: %v", n, asked()[20:])
	}
	if got := strings.Join(logs.passLines(), "|"); got != "trailers: read-ahead 101 due, 20 looked up, 81 left for the next pass" {
		t.Fatalf("log %q", got)
	}
	if n := a.Trailers().Pass(ctx, 1000); n != 81 || strings.Join(asked(), " ") != strings.Join(want, " ") {
		t.Fatalf("last pass %d lookups", n)
	}
	for _, tr := range pg.Tail {
		for _, it := range tr.Items {
			// 205 is on Never add: never looked up, wherever the day's
			// layout puts it.
			if it.TMDBID == "205" {
				continue
			}
			if !slices.Contains(asked(), "movie:"+it.TMDBID) {
				t.Fatalf("tail card %s not looked up", it.TMDBID)
			}
		}
	}
	logs.passLines()
	runs := yt.runs.Load()
	if n := a.Trailers().Pass(ctx, 1000); n != 0 || yt.runs.Load() != runs {
		t.Fatalf("looked up again: %d", n)
	}
	if got := logs.passLines(); len(got) != 0 {
		t.Fatalf("a pass with nothing due logged %v", got)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	last := pg.Tail[len(pg.Tail)-1].Items
	trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/100", "", 200)
	trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/"+last[len(last)-1].TMDBID, "", 200)
	trDo(t, "GET", srv.URL+"/api/v1/trailer/show/9002", "", 200)
	trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/205", "", 404) // on the Never-add list
	if b := string(trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/107", "", 404)); !strings.Contains(b, `"no trailer"`) {
		t.Fatal(b)
	}
	if yt.runs.Load() != runs {
		t.Fatal("a route looked up")
	}
}

// Lookups are the pass gap apart (none before the first), and a pass
// waiting in a gap stops when its context ends.
func TestTrailerPassSpacing(t *testing.T) {
	fakeReader(t, "ok")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a, asked := passRig(t, st, nil)
	storePool(t, st, passPool(1, "all", false))
	ctx := metadata.Ahead(context.Background())
	gap := metadata.SetTrailerPassGap(150 * time.Millisecond)
	t.Cleanup(func() { metadata.SetTrailerPassGap(gap) })
	start := time.Now()
	if n := a.Trailers().Pass(ctx, 3); n != 3 {
		t.Fatalf("%d lookups", n)
	}
	if d := time.Since(start); d < 300*time.Millisecond {
		t.Fatalf("3 lookups in %v", d)
	}
	metadata.SetTrailerPassGap(time.Hour)
	cctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	start = time.Now()
	if n := a.Trailers().Pass(cctx, 3); n != 1 || len(asked()) != 4 {
		t.Fatalf("%d lookups, asked %v", n, asked())
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("the gap held the pass %v", d)
	}
}

// The work list is capped: what is past the cap (noted titles last) waits,
// and the log says how many.
func TestTrailerPassCap(t *testing.T) {
	fakeReader(t, "ok")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a, asked := passRig(t, st, nil)
	logs := catchLog(t)
	old := metadata.SetTrailerPassMax(5)
	t.Cleanup(func() { metadata.SetTrailerPassMax(old) })
	storePool(t, st, passPool(1, "all", false))
	a.Trailers().Note("en", [][2]string{{"movie", "555"}})
	ctx := metadata.Ahead(context.Background())
	// The noted title goes first and is never cut; the cap counts pool
	// titles only.
	if n := a.Trailers().Pass(ctx, 30); n != 6 || strings.Join(asked(), " ") != "movie:555 movie:100 movie:101 movie:102 movie:103 movie:104" {
		t.Fatalf("%d lookups: %v", n, asked())
	}
	if got := strings.Join(logs.passLines(), "|"); got != "trailers: read-ahead 6 due, 6 looked up, 0 left for the next pass, 19 past the cap" {
		t.Fatalf("log %q", got)
	}
	if got := a.Trailers().Asks(time.Now()); len(got) != 0 {
		t.Fatalf("noted %v", got)
	}
}

// A title outside every pool asked for on the route is noted, never looked
// up there: its key is learned in the background (for the embed), the next
// pass looks it up in the language asked, and the route then plays it. A
// pass that stops at YouTube's door keeps it noted.
func TestTrailerPassCoversAskedTitles(t *testing.T) {
	yt := fakeReader(t, "fail")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a, _ := passRig(t, st, nil)
	logs := catchLog(t)
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	if b := string(trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/555?language=de", "", 404)); !strings.Contains(b, `"key"`) || strings.Contains(b, `"reason"`) {
		t.Fatal(b)
	}
	if yt.runs.Load() != 0 {
		t.Fatal("the route looked up")
	}
	if got := a.Trailers().Asks(time.Now()); len(got) != 1 || got[0] != [3]string{"movie", "555", "de"} {
		t.Fatalf("noted %v", got)
	}
	for i := 0; ; i++ {
		if b := string(trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/555?language=de", "", 404)); strings.Contains(b, `"key":"trailerKey1"`) {
			break
		} else if i == 100 {
			t.Fatalf("key not learned: %s", b)
		}
		time.Sleep(20 * time.Millisecond)
	}
	ctx := metadata.Ahead(context.Background())
	if n := a.Trailers().Pass(ctx, 30); n != 1 || len(a.Trailers().Asks(time.Now())) != 1 {
		t.Fatalf("door failed: %d lookups, noted %v", n, a.Trailers().Asks(time.Now()))
	}
	if got := strings.Join(logs.passLines(), "|"); got != "trailers: read-ahead 1 due, 1 looked up, 0 left for the next pass" {
		t.Fatalf("log %q", got)
	}
	// Its short miss kept: still noted. Once it ends, the next pass asks.
	a.Trailers().ResetDoor()
	if n := a.Trailers().Pass(ctx, 30); n != 0 || len(a.Trailers().Asks(time.Now())) != 1 {
		t.Fatalf("short miss: %d lookups, noted %v", n, a.Trailers().Asks(time.Now()))
	}
	a.Trailers().Drop("movie|555|de")
	yt = fakeReader(t, "ok")
	if n := a.Trailers().Pass(ctx, 30); n != 1 || yt.runs.Load() != 1 || len(a.Trailers().Asks(time.Now())) != 0 {
		t.Fatalf("pass %d lookups, %d calls, noted %v", n, yt.runs.Load(), a.Trailers().Asks(time.Now()))
	}
	trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/555?language=de", "", 200)
	if n := a.Trailers().Pass(ctx, 30); n != 0 || yt.runs.Load() != 1 {
		t.Fatalf("looked up again: %d", n)
	}
}

// Noted titles are bounded: the oldest goes when full, and one no pass
// reached in a week is dropped.
func TestTrailerAskedBounded(t *testing.T) {
	a := buildHandlers(services{Version: "test"})
	a.Trailers().Note("en", [][2]string{{"movie", "1"}, {"music", "2"}, {"show", "x"}})
	a.Trailers().Note("english", [][2]string{{"movie", "3"}})
	if got := a.Trailers().Asks(time.Now()); len(got) != 1 || got[0] != [3]string{"movie", "1", "en"} {
		t.Fatalf("%v", got)
	}
	for i := 2; i <= notedMax+1; i++ {
		a.Trailers().Note("en", [][2]string{{"show", strconv.Itoa(i)}})
	}
	got := a.Trailers().Asks(time.Now())
	if len(got) != notedMax || got[0][1] == "1" || got[len(got)-1][1] != strconv.Itoa(notedMax+1) {
		t.Fatalf("%d noted, first %v last %v", len(got), got[0], got[len(got)-1])
	}
	if got := a.Trailers().Asks(time.Now().Add(7*24*time.Hour + time.Minute)); len(got) != 0 || a.Trailers().NotedTitles() != 0 {
		t.Fatalf("old ones kept: %v", got)
	}
}

// At the cap, the title asked for least lately goes: one that has waited
// longest but was asked for again just now stays, and keeps its place.
func TestTrailerAskedEvictsLeastLately(t *testing.T) {
	a := buildHandlers(services{Version: "test"})
	var full [][2]string
	for i := 1; i <= notedMax; i++ {
		full = append(full, [2]string{"movie", strconv.Itoa(i)})
	}
	a.Trailers().Note("en", full)
	time.Sleep(2 * time.Millisecond)
	a.Trailers().Note("en", [][2]string{{"movie", "1"}})
	time.Sleep(2 * time.Millisecond)
	a.Trailers().Note("en", [][2]string{{"movie", "9999"}})
	got := a.Trailers().Asks(time.Now())
	if len(got) != notedMax || got[0][1] != "1" || got[1][1] != "3" || got[len(got)-1][1] != "9999" {
		t.Fatalf("%d noted, first %v %v, last %v", len(got), got[0], got[1], got[len(got)-1])
	}
	// A new title before an old one in the same call: the old one is asked
	// for again, so it is not the one that makes room, and keeps its place.
	time.Sleep(2 * time.Millisecond)
	a.Trailers().Note("en", [][2]string{{"movie", "8888"}, {"movie", "3"}})
	got = a.Trailers().Asks(time.Now())
	if len(got) != notedMax || got[1][1] != "3" || got[2][1] == "4" || got[len(got)-1][1] != "8888" {
		t.Fatalf("same call: %d noted, %v %v %v, last %v", len(got), got[0], got[1], got[2], got[len(got)-1])
	}
}

// Kid pools, trailers turned off and a kind that is off are skipped:
// nothing is asked for.
func TestTrailerPassSkips(t *testing.T) {
	yt := fakeReader(t, "ok")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a, asked := passRig(t, st, nil)
	if err := st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash, max_rating) VALUES (3, 't', '{}', 't', '{}', 'kiddo', 'member', 'x', 'PG')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	storePool(t, st, passPool(3, "all", true))
	ctx := metadata.Ahead(context.Background())
	if n := a.Trailers().Pass(ctx, 30); n != 0 || len(asked()) != 0 || yt.runs.Load() != 0 {
		t.Fatalf("kid pool: %d %v %d", n, asked(), yt.runs.Load())
	}

	storePool(t, st, passPool(1, "all", false))
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	trDo(t, "PUT", srv.URL+"/api/v1/settings/ui", `{"autoplay_trailers":false}`, 200)
	if n := a.Trailers().Pass(ctx, 30); n != 0 || len(asked()) != 0 || yt.runs.Load() != 0 {
		t.Fatalf("autoplay off: %d %v %d", n, asked(), yt.runs.Load())
	}
	trDo(t, "PUT", srv.URL+"/api/v1/settings/ui", `{"autoplay_trailers":true}`, 200)

	pc := &parts.Control{Store: st, Reg: jobs.NewRegistry(), Drain: time.Second, Logf: func(string, ...any) {}}
	if err := pc.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := pc.Set(context.Background(), parts.Movies, false, domain.System); err != nil {
		t.Fatal(err)
	}
	off, offAsked := passRig(t, st, pc)
	if n := off.Trailers().Pass(ctx, 30); n != 0 || len(offAsked()) != 0 || yt.runs.Load() != 0 {
		t.Fatalf("movies off: %d %v %d", n, offAsked(), yt.runs.Load())
	}
}

// An address that would be dropped within trailerRenewAhead (two passes
// and the longest a pass takes) is looked up again (one that lasts past it
// is not); a renewal that fails keeps the
// old address and the pass stops there, the rest left for the next pass.
func TestTrailerPassRenewsNearlyExpired(t *testing.T) {
	yt := fakeReader(t, "fail")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a, _ := passRig(t, st, nil)
	logs := catchLog(t)
	storePool(t, st, passPool(1, "all", false))
	ctx := metadata.Ahead(context.Background())
	now := time.Now()
	keep := func(id string, exp time.Time) {
		s := metadata.TrailerStream{Type: "mp4", URL: "https://rr1---sn-abc.googlevideo.com/videoplayback?expire=" + strconv.FormatInt(exp.Unix(), 10), ExpiresAt: exp, Key: "trailerKey1"}
		b, _ := json.Marshal(s)
		if err := (metadata.LookupCache{Store: st}).Put(ctx, "trailer|movie|"+id+"|en", b, now, exp.Add(-metadata.TrailerEarly)); err != nil {
			t.Fatal(err)
		}
	}
	soon := now.Add(metadata.TrailerEarly + metadata.TrailerRenewAhead() - 2*time.Minute).Truncate(time.Second)
	keep("100", soon)
	keep("101", now.Add(5*time.Hour))
	keep("103", now.Add(metadata.TrailerEarly+metadata.TrailerRenewAhead()+2*time.Minute))
	if due, renew := a.Trailers().PassDue(ctx, "movie|100|en", now); !due || !renew {
		t.Fatal("nearly expired not due")
	}
	if due, _ := a.Trailers().PassDue(ctx, "movie|101|en", now); due {
		t.Fatal("fresh address due")
	}
	if due, _ := a.Trailers().PassDue(ctx, "movie|103|en", now); due {
		t.Fatal("an address lasting past the renewal window due")
	}
	if metadata.TrailerRenewAhead() < 2*metadata.TrailerPassEvery()+time.Duration(metadata.TrailerPassBudget)*40*time.Second {
		t.Fatalf("renewal window %v is shorter than two passes and a full one", metadata.TrailerRenewAhead())
	}
	// YouTube refuses: the old address stays, and nothing more is asked.
	if n := a.Trailers().Pass(ctx, 30); n != 1 || yt.runs.Load() != 1 {
		t.Fatalf("failed renewal: %d lookups, %d calls", n, yt.runs.Load())
	}
	if got := strings.Join(logs.passLines(), "|"); got != "trailers: read-ahead 22 due, 1 looked up, 21 left for the next pass" {
		t.Fatalf("log %q", got)
	}
	if e, ok := a.Trailers().Kept(ctx, "movie|100|en", time.Now()); !ok || !e.Miss.IsZero() || !e.Stream.ExpiresAt.Equal(soon) {
		t.Fatalf("old address lost: %+v %v", e, ok)
	}
	yt = fakeReader(t, "ok")
	if n := a.Trailers().Pass(ctx, 2); n != 2 || yt.count("trailerKey1") != 2 {
		t.Fatalf("pass %d lookups, %d calls", n, yt.count("trailerKey1"))
	}
	if e, ok := a.Trailers().Kept(ctx, "movie|100|en", time.Now()); !ok || e.Stream.Type != "hls" || time.Until(e.Stream.ExpiresAt) < 5*time.Hour {
		t.Fatalf("not renewed: %+v", e)
	}
	if e, _ := a.Trailers().Kept(ctx, "movie|101|en", time.Now()); e.Stream.Type != "mp4" {
		t.Fatalf("a fresh address was looked up again: %+v", e)
	}
	if _, ok := a.Trailers().Kept(ctx, "movie|102|en", time.Now()); !ok {
		t.Fatal("the next title was not looked up")
	}
}

// Renewals go before first lookups, whatever their place on the page: a
// pass whose budget the new titles before it would fill still renews the
// kept addresses further down first, in page order.
func TestTrailerPassRenewsFirst(t *testing.T) {
	yt := fakeReader(t, "ok")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a, asked := passRig(t, st, nil)
	storePool(t, st, passPool(1, "all", false))
	ctx := metadata.Ahead(context.Background())
	now := time.Now()
	soon := now.Add(metadata.TrailerEarly + time.Minute)
	for _, id := range []string{"305", "203"} {
		s := metadata.TrailerStream{Type: "mp4", URL: "https://rr1---sn-abc.googlevideo.com/videoplayback?expire=" + strconv.FormatInt(soon.Unix(), 10), ExpiresAt: soon, Key: "trailerKey1"}
		b, _ := json.Marshal(s)
		if err := (metadata.LookupCache{Store: st}).Put(ctx, "trailer|movie|"+id+"|en", b, now, soon.Add(-metadata.TrailerEarly)); err != nil {
			t.Fatal(err)
		}
	}
	if n := a.Trailers().Pass(ctx, 3); n != 3 || yt.runs.Load() != 3 {
		t.Fatalf("%d lookups, %d calls", n, yt.runs.Load())
	}
	for _, id := range []string{"203", "305"} {
		if e, ok := a.Trailers().Kept(ctx, "movie|"+id+"|en", time.Now()); !ok || e.Stream.Type != "hls" {
			t.Fatalf("%s not renewed first: %+v", id, e)
		}
	}
	if _, ok := a.Trailers().Kept(ctx, "movie|101|en", time.Now()); ok {
		t.Fatal("a first lookup went before a renewal")
	}
	if got := asked(); len(got) != 3 || got[2] != "movie:100" || !slices.Contains(got[:2], "movie:203") || !slices.Contains(got[:2], "movie:305") {
		t.Fatalf("asked %v", got)
	}
}

// The pass covers the languages already noted, most used first, at most 3.
func TestTrailerPassLanguages(t *testing.T) {
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a := buildHandlers(services{Store: st, Version: "test"})
	ctx := context.Background()
	if got := strings.Join(a.TrailerTest().PassLanguages(ctx), " "); got != "en" {
		t.Fatalf("none noted: %s", got)
	}
	err := st.Write(ctx, func(tx *sql.Tx) error {
		for i, l := range []string{"de", "de", "de", "fr", "fr", "en", "xx-1", "it"} {
			if _, err := tx.Exec("INSERT INTO card_art (kind, tmdb_id, lang, fetched_at) VALUES ('movie', ?, ?, 't')", strconv.Itoa(i), l); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(a.TrailerTest().PassLanguages(ctx), " "); got != "de fr en" {
		t.Fatalf("noted: %s", got)
	}
	// Once pages asked for languages, only those count: English pictures
	// filled on their own do not take a slot.
	a.TrailerTest().NoteCardLang("es")
	a.TrailerTest().NoteCardLang("de")
	if got := strings.Join(a.TrailerTest().PassLanguages(ctx), " "); got != "de es" {
		t.Fatalf("asked: %s", got)
	}
}

// The loop is a long one with its own schedule, owned by a part.
func TestTrailerLoops(t *testing.T) {
	ls := buildHandlers(services{Version: "test"}).Trailers().Loops()
	if len(ls) != 1 || ls[0].Name != "trailer_pass" || ls[0].Run == nil || ls[0].Every != 0 {
		t.Fatalf("%+v", ls)
	}
	if parts.Loops()["trailer_pass"] != parts.Media {
		t.Fatal("trailer_pass has no part")
	}
}

// The pass's order: a renewal that would lapse before the next pass, then
// the noted titles (people are looking at them), then renewals that get
// one more pass, then the pool's first lookups. A title noted again keeps
// its place in the queue.
func TestTrailerPassOrder(t *testing.T) {
	fakeReader(t, "ok")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a, asked := passRig(t, st, nil)
	storePool(t, st, passPool(1, "all", false))
	ctx := metadata.Ahead(context.Background())
	now := time.Now()
	for id, left := range map[string]time.Duration{"305": time.Minute, "203": metadata.TrailerRenewAhead() - time.Minute} {
		exp := now.Add(metadata.TrailerEarly + left)
		s := metadata.TrailerStream{Type: "mp4", URL: "https://rr1---sn-abc.googlevideo.com/videoplayback?expire=" + strconv.FormatInt(exp.Unix(), 10), ExpiresAt: exp, Key: "trailerKey1"}
		b, _ := json.Marshal(s)
		if err := (metadata.LookupCache{Store: st}).Put(ctx, "trailer|movie|"+id+"|en", b, now, exp.Add(-metadata.TrailerEarly)); err != nil {
			t.Fatal(err)
		}
	}
	a.Trailers().Note("en", [][2]string{{"movie", "555"}, {"movie", "556"}})
	a.Trailers().Note("en", [][2]string{{"movie", "555"}})
	if got := a.Trailers().Asks(time.Now()); len(got) != 2 || got[0][1] != "555" || got[1][1] != "556" {
		t.Fatalf("noted again moved it: %v", got)
	}
	if n := a.Trailers().Pass(ctx, 4); n != 4 {
		t.Fatalf("%d lookups", n)
	}
	if got := strings.Join(asked(), " "); got != "movie:305 movie:555 movie:556 movie:203" {
		t.Fatalf("order %q", got)
	}
}

// A renewal that fails because the video will not play any more counts
// toward the health check (it played here before): the pass marks it.
func TestTrailerPassRenewalFailureCounts(t *testing.T) {
	fakeReader(t, "unavailable")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a, _ := passRig(t, st, nil)
	recentGood(a, time.Hour)
	storePool(t, st, passPool(1, "all", false))
	ctx := metadata.Ahead(context.Background())
	now := time.Now()
	exp := now.Add(metadata.TrailerEarly + time.Minute)
	s := metadata.TrailerStream{Type: "mp4", URL: "https://rr1---sn-abc.googlevideo.com/videoplayback?expire=" + strconv.FormatInt(exp.Unix(), 10), ExpiresAt: exp, Key: "trailerKey1"}
	b, _ := json.Marshal(s)
	if err := (metadata.LookupCache{Store: st}).Put(ctx, "trailer|movie|305|en", b, now, exp.Add(-metadata.TrailerEarly)); err != nil {
		t.Fatal(err)
	}
	if n := a.Trailers().Pass(ctx, 1); n != 1 {
		t.Fatalf("%d lookups", n)
	}
	if r := a.Trailers().ReaderState(); r.Videos != 1 {
		t.Fatalf("renewal failure not counted: %+v", r)
	}
	// That first lookups do not count is TestTrailerUpkeepIgnoresFirstLookupMisses
	// and TestTrailerUncountedMissNotMarked: here every title lists the same
	// key, which this renewal marked bad, so they would not reach YouTube.
}

// notedMax is how many titles the pass keeps noted at most.
const notedMax = 500
