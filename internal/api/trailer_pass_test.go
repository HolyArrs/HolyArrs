package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/explore"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/parts"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/supervisor"
)

// passPool is a stored pool: Trending (the hero's source) and two more
// head rows, eight movies each (ids base..base+7).
func passPool(member int64, sw string, kid bool) explore.Pool {
	items := func(base int) []explore.Item {
		var out []explore.Item
		for i := range 8 {
			out = append(out, explore.Item{TMDBID: strconv.Itoa(base + i), Kind: "movie", Name: "T" + strconv.Itoa(base+i), Genres: []string{"Drama"}})
		}
		return out
	}
	return explore.Pool{Version: explore.PoolVersion, Member: member, Switch: sw, Date: explore.Day(time.Now()), Kid: kid, Built: time.Now(),
		Rows: []explore.Row{{ID: "trending", Type: explore.TypeTrending, Items: items(100)}, {ID: "best", Type: explore.TypeBest, Items: items(200)}, {ID: "classics", Type: explore.TypeClassics, Items: items(300)}}}
}

func storePool(t *testing.T, st *store.Store, p explore.Pool) {
	t.Helper()
	raw, _ := json.Marshal(p)
	err := st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO explore_pool (member_id, type_switch, day, built_at, stale, due, pool) VALUES (?, ?, ?, ?, 0, NULL, ?)`,
			p.Member, p.Switch, p.Date, store.FormatTime(time.Now()), string(raw))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// passRig is an API whose trailer key is the metadata server's pick, which
// notes each title asked for (once per first lookup); "107" has none.
func passRig(t *testing.T, st *store.Store, pc *parts.Control) (*API, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var asked []string
	a := New(Server{Store: st, Metadata: trailerTMDB(t), Version: "test", Parts: pc, TrailerPick: func(ctx context.Context, kind, id, lang string) (string, error) {
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

// The pass covers a stored pool's hero candidates first, then the first 4
// cards of its first rows, within its budget; leftovers continue on the
// next pass, and kept addresses and misses are not looked up again. The
// route then reads what it kept.
func TestTrailerPassHeroFirstWithinBudget(t *testing.T) {
	yt := fakeReader(t, "ok")
	_, st := discoverRig(t, trailerTMDB(t))
	a, asked := passRig(t, st, nil)
	storePool(t, st, passPool(1, "all", false))
	ctx := metadata.Ahead(context.Background())
	if n := a.trailerPass(ctx, 5); n != 5 {
		t.Fatalf("first pass %d lookups", n)
	}
	if got := strings.Join(asked(), " "); got != "movie:100 movie:101 movie:102 movie:103 movie:104" {
		t.Fatalf("first pass asked %s", got)
	}
	if n := a.trailerPass(ctx, 30); n != 11 {
		t.Fatalf("second pass %d lookups (%v)", n, asked())
	}
	got := asked()
	if strings.Join(got[5:8], " ") != "movie:105 movie:106 movie:107" {
		t.Fatalf("hero candidates not first: %v", got)
	}
	cards := map[string]bool{}
	for _, k := range got[8:] {
		cards[k] = true
	}
	for _, base := range []int{200, 300} {
		for i := range 4 {
			if !cards["movie:"+strconv.Itoa(base+i)] {
				t.Fatalf("row card %d not looked up: %v", base+i, got)
			}
		}
	}
	runs := yt.runs.Load()
	if n := a.trailerPass(ctx, 30); n != 0 || yt.runs.Load() != runs {
		t.Fatalf("third pass %d lookups (%v)", n, asked()[16:])
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	do(t, "GET", srv.URL+"/api/v1/trailer/movie/100", "", 200)
	do(t, "GET", srv.URL+"/api/v1/trailer/movie/203", "", 200)
	do(t, "GET", srv.URL+"/api/v1/trailer/movie/204", "", 404) // past the first 4 cards
	if b := string(do(t, "GET", srv.URL+"/api/v1/trailer/movie/107", "", 404)); !strings.Contains(b, `"no trailer"`) {
		t.Fatal(b)
	}
	if yt.runs.Load() != runs {
		t.Fatal("a route looked up")
	}
}

// A title outside every pool asked for on the route is noted, never looked
// up there: its key is learned in the background (for the embed), the next
// pass looks it up in the language asked, and the route then plays it. A
// pass that stops at YouTube's door keeps it noted.
func TestTrailerPassCoversAskedTitles(t *testing.T) {
	yt := fakeReader(t, "fail")
	_, st := discoverRig(t, trailerTMDB(t))
	a, _ := passRig(t, st, nil)
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	if b := string(do(t, "GET", srv.URL+"/api/v1/trailer/movie/555?language=de", "", 404)); !strings.Contains(b, `"key"`) || strings.Contains(b, `"reason"`) {
		t.Fatal(b)
	}
	if yt.runs.Load() != 0 {
		t.Fatal("the route looked up")
	}
	if got := a.trailerAsks(time.Now()); len(got) != 1 || got[0] != [3]string{"movie", "555", "de"} {
		t.Fatalf("noted %v", got)
	}
	for i := 0; ; i++ {
		if b := string(do(t, "GET", srv.URL+"/api/v1/trailer/movie/555?language=de", "", 404)); strings.Contains(b, `"key":"trailerKey1"`) {
			break
		} else if i == 100 {
			t.Fatalf("key not learned: %s", b)
		}
		time.Sleep(20 * time.Millisecond)
	}
	ctx := metadata.Ahead(context.Background())
	if n := a.trailerPass(ctx, 30); n != 1 || len(a.trailerAsks(time.Now())) != 1 {
		t.Fatalf("door failed: %d lookups, noted %v", n, a.trailerAsks(time.Now()))
	}
	// Its short miss kept: still noted. Once it ends, the next pass asks.
	a.trailers.failed.Store(0)
	if n := a.trailerPass(ctx, 30); n != 0 || len(a.trailerAsks(time.Now())) != 1 {
		t.Fatalf("short miss: %d lookups, noted %v", n, a.trailerAsks(time.Now()))
	}
	a.trailers.mu.Lock()
	delete(a.trailers.cache, "movie|555|de")
	a.trailers.mu.Unlock()
	yt = fakeReader(t, "ok")
	if n := a.trailerPass(ctx, 30); n != 1 || yt.runs.Load() != 1 || len(a.trailerAsks(time.Now())) != 0 {
		t.Fatalf("pass %d lookups, %d calls, noted %v", n, yt.runs.Load(), a.trailerAsks(time.Now()))
	}
	do(t, "GET", srv.URL+"/api/v1/trailer/movie/555?language=de", "", 200)
	if n := a.trailerPass(ctx, 30); n != 0 || yt.runs.Load() != 1 {
		t.Fatalf("looked up again: %d", n)
	}
}

// Noted titles are bounded: the oldest goes when full, and one no pass
// reached in a week is dropped.
func TestTrailerAskedBounded(t *testing.T) {
	a := New(Server{Version: "test"})
	a.noteTrailerTitles("en", [][2]string{{"movie", "1"}, {"music", "2"}, {"show", "x"}})
	a.noteTrailerTitles("english", [][2]string{{"movie", "3"}})
	if got := a.trailerAsks(time.Now()); len(got) != 1 || got[0] != [3]string{"movie", "1", "en"} {
		t.Fatalf("%v", got)
	}
	for i := 2; i <= trailerAskedMax+1; i++ {
		a.noteTrailerTitles("en", [][2]string{{"show", strconv.Itoa(i)}})
	}
	got := a.trailerAsks(time.Now())
	if len(got) != trailerAskedMax || got[0][1] == "1" || got[len(got)-1][1] != strconv.Itoa(trailerAskedMax+1) {
		t.Fatalf("%d noted, first %v last %v", len(got), got[0], got[len(got)-1])
	}
	if got := a.trailerAsks(time.Now().Add(trailerAskedKeep + time.Minute)); len(got) != 0 || len(a.trailerAsked.m) != 0 {
		t.Fatalf("old ones kept: %v", got)
	}
}

// Kid pools, trailers turned off and a kind that is off are skipped:
// nothing is asked for.
func TestTrailerPassSkips(t *testing.T) {
	yt := fakeReader(t, "ok")
	_, st := discoverRig(t, trailerTMDB(t))
	a, asked := passRig(t, st, nil)
	if err := st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash, max_rating) VALUES (3, 't', '{}', 't', '{}', 'kiddo', 'member', 'x', 'PG')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	storePool(t, st, passPool(3, "all", true))
	ctx := metadata.Ahead(context.Background())
	if n := a.trailerPass(ctx, 30); n != 0 || len(asked()) != 0 || yt.runs.Load() != 0 {
		t.Fatalf("kid pool: %d %v %d", n, asked(), yt.runs.Load())
	}

	storePool(t, st, passPool(1, "all", false))
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	do(t, "PUT", srv.URL+"/api/v1/settings/ui", `{"autoplay_trailers":false}`, 200)
	if n := a.trailerPass(ctx, 30); n != 0 || len(asked()) != 0 || yt.runs.Load() != 0 {
		t.Fatalf("autoplay off: %d %v %d", n, asked(), yt.runs.Load())
	}
	do(t, "PUT", srv.URL+"/api/v1/settings/ui", `{"autoplay_trailers":true}`, 200)

	pc := &parts.Control{Store: st, Reg: supervisor.NewRegistry(), Drain: time.Second, Logf: func(string, ...any) {}}
	if err := pc.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := pc.Set(context.Background(), parts.Movies, false, domain.System); err != nil {
		t.Fatal(err)
	}
	off, offAsked := passRig(t, st, pc)
	if n := off.trailerPass(ctx, 30); n != 0 || len(offAsked()) != 0 || yt.runs.Load() != 0 {
		t.Fatalf("movies off: %d %v %d", n, offAsked(), yt.runs.Load())
	}
}

// An address that would expire before the next two passes is looked up
// again (a fresh one is not); a renewal that fails keeps the old address
// and the pass stops there.
func TestTrailerPassRenewsNearlyExpired(t *testing.T) {
	yt := fakeReader(t, "fail")
	_, st := discoverRig(t, trailerTMDB(t))
	a, _ := passRig(t, st, nil)
	storePool(t, st, passPool(1, "all", false))
	ctx := metadata.Ahead(context.Background())
	now := time.Now()
	keep := func(id string, exp time.Time) {
		s := TrailerStream{Type: "mp4", URL: "https://rr1---sn-abc.googlevideo.com/videoplayback?expire=" + strconv.FormatInt(exp.Unix(), 10), ExpiresAt: exp, Key: "trailerKey1"}
		b, _ := json.Marshal(s)
		if err := (store.LookupCache{Store: st}).Put(ctx, "trailer|movie|"+id+"|en", b, now, exp.Add(-trailerEarly)); err != nil {
			t.Fatal(err)
		}
	}
	soon := now.Add(trailerEarly + 20*time.Minute).Truncate(time.Second)
	keep("100", soon)
	keep("101", now.Add(5*time.Hour))
	if due, renew := a.trailerPassDue(ctx, "movie|100|en", now); !due || !renew {
		t.Fatal("nearly expired not due")
	}
	if due, _ := a.trailerPassDue(ctx, "movie|101|en", now); due {
		t.Fatal("fresh address due")
	}
	// YouTube refuses: the old address stays, and nothing more is asked.
	if n := a.trailerPass(ctx, 30); n != 1 || yt.runs.Load() != 1 {
		t.Fatalf("failed renewal: %d lookups, %d calls", n, yt.runs.Load())
	}
	if e, ok := a.keptTrailer(ctx, "movie|100|en", time.Now()); !ok || !e.miss.IsZero() || !e.s.ExpiresAt.Equal(soon) {
		t.Fatalf("old address lost: %+v %v", e, ok)
	}
	yt = fakeReader(t, "ok")
	if n := a.trailerPass(ctx, 2); n != 2 || yt.count("trailerKey1") != 2 {
		t.Fatalf("pass %d lookups, %d calls", n, yt.count("trailerKey1"))
	}
	if e, ok := a.keptTrailer(ctx, "movie|100|en", time.Now()); !ok || e.s.Type != "hls" || time.Until(e.s.ExpiresAt) < 5*time.Hour {
		t.Fatalf("not renewed: %+v", e)
	}
	if e, _ := a.keptTrailer(ctx, "movie|101|en", time.Now()); e.s.Type != "mp4" {
		t.Fatalf("a fresh address was looked up again: %+v", e)
	}
	if _, ok := a.keptTrailer(ctx, "movie|102|en", time.Now()); !ok {
		t.Fatal("the next title was not looked up")
	}
}

// The pass covers the languages already noted, most used first, at most 3.
func TestTrailerPassLanguages(t *testing.T) {
	_, st := discoverRig(t, trailerTMDB(t))
	a := New(Server{Store: st, Version: "test"})
	ctx := context.Background()
	if got := strings.Join(a.trailerPassLanguages(ctx), " "); got != "en" {
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
	if got := strings.Join(a.trailerPassLanguages(ctx), " "); got != "de fr en" {
		t.Fatalf("noted: %s", got)
	}
	// Once pages asked for languages, only those count: English pictures
	// filled on their own do not take a slot.
	a.noteCardLang("es")
	a.noteCardLang("de")
	if got := strings.Join(a.trailerPassLanguages(ctx), " "); got != "de es" {
		t.Fatalf("asked: %s", got)
	}
}

// The loop is a long one with its own schedule, owned by a part.
func TestTrailerLoops(t *testing.T) {
	ls := New(Server{Version: "test"}).TrailerLoops()
	if len(ls) != 1 || ls[0].Name != "trailer_pass" || ls[0].Run == nil || ls[0].Every != 0 {
		t.Fatalf("%+v", ls)
	}
	if parts.Loops["trailer_pass"] != parts.Media {
		t.Fatal("trailer_pass has no part")
	}
}
