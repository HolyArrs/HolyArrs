package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// recordingTMDB is a fake TMDB that records every request it gets (the
// test proxy for the privacy check) and answers every list with 30 items.
type recordingTMDB struct {
	srv  *httptest.Server
	mu   sync.Mutex
	urls []string
}

func newRecordingTMDB(t *testing.T) *recordingTMDB {
	t.Helper()
	f := &recordingTMDB{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.urls = append(f.urls, r.URL.String())
		f.mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/genre/movie/list"), strings.HasSuffix(r.URL.Path, "/genre/tv/list"):
			_, _ = w.Write([]byte(`{"genres":[{"id":18,"name":"Drama"},{"id":35,"name":"Comedy"}]}`))
		case strings.HasSuffix(r.URL.Path, "/search/person"):
			_, _ = w.Write([]byte(`{"results":[{"id":5}]}`))
		case strings.HasSuffix(r.URL.Path, "/combined_credits"):
			_, _ = w.Write([]byte(`{"cast":[]}`))
		case strings.HasSuffix(r.URL.Path, "/collection/77"):
			var parts []string
			for i, d := range []string{"2001-05-01", "2004-05-01", "2008-05-01", "2030-05-01"} {
				parts = append(parts, fmt.Sprintf(`{"id":%d,"title":"Saga %d","release_date":%q,"genre_ids":[18],"popularity":50,"vote_average":7.5,"vote_count":400}`, 501+i, i+1, d))
			}
			_, _ = w.Write([]byte(`{"name":"Saga","parts":[` + strings.Join(parts, ",") + `]}`))
		default:
			var parts []string
			base := len(r.URL.String()) * 7
			for i := 0; i < 30; i++ {
				id := 1000 + (base+i*13)%900
				parts = append(parts, fmt.Sprintf(`{"id":%d,"title":"M%d","name":"S%d","release_date":"2026-09-01","first_air_date":"2026-09-01","genre_ids":[18],"popularity":%d,"vote_average":7.5,"vote_count":400}`, id, id, id, 100-i))
			}
			_, _ = w.Write([]byte(`{"results":[` + strings.Join(parts, ",") + `]}`))
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *recordingTMDB) provider() *metadata.TMDB {
	return &metadata.TMDB{BaseURL: f.srv.URL + "/3", Access: func(context.Context) (string, error) { return "fake-read-value", nil }, Gap: time.Millisecond}
}

func exploreRig(t *testing.T) (*handlers, *database.Store, *recordingTMDB) {
	t.Helper()
	f := newRecordingTMDB(t)
	_, s := discoverRig(t, f.provider())
	a := buildHandlers(services{Store: s, Metadata: f.provider(), Version: "test"})
	a.explore.Paced = &engine.Paced{Source: discover.ListerSource{L: f.provider()}, Gap: time.Millisecond}
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash, max_rating) VALUES
			(2, 't', '{}', 't', '{}', 'zelda-member', 'member', 'x', ''), (3, 't', '{}', 't', '{}', 'kiddo', 'member', 'x', 'PG')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return a, s, f
}

func poolState(t *testing.T, s *database.Store, member int64, sw string) (bool, bool) {
	t.Helper()
	var stale bool
	err := s.Read().QueryRow("SELECT stale FROM explore_pool WHERE member_id = ? AND type_switch = ?", member, sw).Scan(&stale)
	if err == sql.ErrNoRows {
		return false, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return true, stale
}

// Criterion 11: an ask marks that member's pools stale; an add marks every
// pool of that switch stale (debounced 10 minutes); a background rebuild
// never uses the per-person Explore limit.
func TestExplorePoolInvalidation(t *testing.T) {
	t.Parallel()
	a, s, _ := exploreRig(t)
	ctx := context.Background()
	for _, m := range []int64{1, 2} {
		for _, sw := range discover.ExploreSwitches {
			if err := a.explore.BuildPool(ctx, m, sw); err != nil {
				t.Fatal(err)
			}
		}
	}
	if a.discover.HitCount() != 0 {
		t.Fatalf("a pool build used the per-person limit: %v", a.discover.HitCount())
	}
	if ok, stale := poolState(t, s, 1, "all"); !ok || stale {
		t.Fatal("pool not stored fresh")
	}
	a.explore.OnEvent(domain.Event{Kind: domain.EventAskMade, Actor: domain.Actor{Kind: "user", ID: 2}})
	if _, stale := poolState(t, s, 2, "all"); !stale {
		t.Fatal("an ask did not mark the member's pool stale")
	}
	if _, stale := poolState(t, s, 1, "all"); stale {
		t.Fatal("an ask marked another member's pool stale")
	}
	var showID int64
	_ = s.Read().QueryRow("SELECT id FROM titles WHERE kind = 'show'").Scan(&showID)
	now := time.Now()
	a.explore.Clock = func() time.Time { return now }
	a.explore.OnEvent(domain.Event{Kind: domain.EventTitleAdded, TitleID: &showID})
	for _, m := range []int64{1, 2} {
		if _, stale := poolState(t, s, m, "shows"); !stale {
			t.Fatalf("member %d shows pool not stale after a show was added", m)
		}
		if _, stale := poolState(t, s, m, "all"); !stale {
			t.Fatalf("member %d all pool not stale after a show was added", m)
		}
	}
	if _, stale := poolState(t, s, 1, "movies"); stale {
		t.Fatal("a show add marked the movies pool stale")
	}
	// Debounce: a second add moves the rebuild on; nothing is due yet.
	a.explore.Clock = func() time.Time { return now.Add(5 * time.Minute) }
	a.explore.OnEvent(domain.Event{Kind: domain.EventTitleAdded, TitleID: &showID})
	a.explore.Clock = func() time.Time { return now.Add(11 * time.Minute) }
	for {
		k, ok := a.explore.NextDue()
		if !ok {
			break
		}
		if k.Switch == "shows" {
			t.Fatal("a show pool came due 11 minutes after the first add, 6 after the last")
		}
	}
	a.explore.Clock = func() time.Time { return now.Add(16 * time.Minute) }
	n := 0
	for {
		k, ok := a.explore.NextDue()
		if !ok {
			break
		}
		if k.Switch == "shows" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("%d shows rebuilds due after the wait, want one per member (2)", n)
	}
}

// Criterion 6: the outside requests of a pool build carry no member id,
// name, play or ask.
func TestExplorePoolBuildPrivacy(t *testing.T) {
	t.Parallel()
	a, s, f := exploreRig(t)
	ctx := context.Background()
	err := s.Write(ctx, func(tx *sql.Tx) error {
		ask := domain.Ask{Kind: "movie", TMDBID: "777", Name: "Secret Ask", State: domain.AskAsked, AskedBy: 2, AskedAt: time.Now(), SourceIDs: map[string]string{"tmdb": "777"}}
		if _, err := database.Insert(tx, "asks", &ask, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, 2, ?, 1)", ask.ID, database.FormatTime(time.Now()))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.explore.BuildPool(ctx, 2, "all"); err != nil {
		t.Fatal(err)
	}
	if len(f.urls) == 0 {
		t.Fatal("no outside requests recorded")
	}
	for _, u := range f.urls {
		for _, bad := range []string{"zelda", "member", "user", "Secret", "Ask", "asked"} {
			if strings.Contains(u, bad) {
				t.Fatalf("outside request %q carries %q", u, bad)
			}
		}
	}
}

// Criterion 5 (server side): a Kid's pool keeps only titles their limit
// allows; unknown ratings stay hidden.
func TestExploreKidPoolFiltered(t *testing.T) {
	t.Parallel()
	a, _, _ := exploreRig(t)
	ctx := context.Background()
	for i := 1000; i < 1900; i++ {
		r := "R"
		if i%2 == 0 {
			r = "PG"
		}
		a.mf.KeepRating("movie", fmt.Sprint(i), r)
	}
	if err := a.explore.BuildPool(ctx, 3, "movies"); err != nil {
		t.Fatal(err)
	}
	p, _, err := a.explore.ReadPool(ctx, 3, "movies", engine.Day(time.Now()), false)
	if err != nil || p == nil {
		t.Fatalf("pool %v %v", p, err)
	}
	n := 0
	for _, r := range p.Rows {
		for _, it := range r.Items {
			n++
			var id int
			fmt.Sscan(it.TMDBID, &id)
			if id%2 != 0 && it.Kind == "movie" {
				t.Fatalf("Kid pool row %s has %s rated R", r.ID, it.Key())
			}
		}
	}
	if n == 0 {
		t.Fatal("Kid pool empty")
	}
	if !p.Kid {
		t.Fatal("pool not marked Kid")
	}
}

// The head answers in one request from the stored pool, and a missing
// pool answers cold-start rows at once and queues a build.
func TestExploreHeadAndTail(t *testing.T) {
	t.Parallel()
	a, _, _ := exploreRig(t)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	get := func(path string) map[string]any {
		resp, err := http.Get(srv.URL + path)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("%s: %v %v", path, err, resp.StatusCode)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out
	}
	cold := get("/api/v1/explore/head?type=all")
	if cold["building"] != true {
		t.Fatalf("no pool yet, building = %v", cold["building"])
	}
	if k, ok := a.explore.NextDue(); !ok || k.Member != 1 || k.Switch != "all" {
		t.Fatal("head did not queue a pool build")
	}
	if err := a.explore.BuildPool(context.Background(), 1, "all"); err != nil {
		t.Fatal(err)
	}
	h := get("/api/v1/explore/head?type=all")
	if h["building"] != false || len(h["head"].([]any)) == 0 {
		t.Fatalf("head %v", h)
	}
	// A title at most once in the first engine.HeadDedupe rows.
	seen := map[string]bool{}
	for i, r := range h["head"].([]any) {
		if i == engine.HeadDedupe {
			break
		}
		for _, it := range r.(map[string]any)["items"].([]any) {
			k := it.(map[string]any)["kind"].(string) + it.(map[string]any)["tmdb_id"].(string)
			if seen[k] {
				t.Fatalf("%s twice in the head (row %d %v)", k, i, r.(map[string]any)["id"])
			}
			seen[k] = true
		}
	}
	tail := get("/api/v1/explore/tail?type=all&page=0")
	if _, ok := tail["rows"].([]any); !ok {
		t.Fatalf("tail %v", tail)
	}
	// Search Browse: genre and mood tiles from the same page, each its own backdrop.
	br := get("/api/v1/explore/browse?type=all")
	tiles, ok := br["tiles"].([]any)
	if !ok {
		t.Fatalf("browse %v", br)
	}
	tb := map[string]bool{}
	for _, x := range tiles {
		b := x.(map[string]any)["backdrop"].(string)
		if b == "" || tb[b] {
			t.Fatalf("browse tile backdrop %q empty or repeated", b)
		}
		tb[b] = true
	}
	// Browse by genre: cards from the same page, each its own backdrop.
	gr := get("/api/v1/explore/genre-row?type=all")
	cards, ok := gr["genres"].([]any)
	if !ok {
		t.Fatalf("genre row %v", gr)
	}
	backs := map[string]bool{}
	for _, c := range cards {
		b := c.(map[string]any)["backdrop"].(string)
		if b == "" || backs[b] {
			t.Fatalf("genre card backdrop %q empty or repeated", b)
		}
		backs[b] = true
	}
}

// Finish the collection (Movies): the released parts of a TMDB collection
// the Library has some of, never the owned or unreleased ones.
func TestExploreFinishCollection(t *testing.T) {
	t.Parallel()
	a, s, f := exploreRig(t)
	ctx := context.Background()
	err := s.Write(ctx, func(tx *sql.Tx) error {
		own := mediafiles.Title{Kind: "movie", Name: "Saga 1", SortName: "saga 1", SourceIDs: map[string]string{"tmdb": "501"}, MetadataSource: "tmdb", NewPartsPolicy: "all", WhyAdded: "by_hand", State: "active", ReleaseGate: "released", GateDriver: "none", Status: "released"}
		if _, err := database.Insert(tx, "titles", &own, domain.System); err != nil {
			return err
		}
		res, err := tx.Exec("INSERT INTO movie_collections(source_id, name) VALUES('tmdb:77', 'Saga')")
		if err != nil {
			return err
		}
		cid, _ := res.LastInsertId()
		_, err = tx.Exec("INSERT INTO movie_collection_titles(collection_id, title_id) VALUES(?, ?)", cid, own.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.explore.BuildPool(ctx, 1, "movies"); err != nil {
		t.Fatal(err)
	}
	p, _, _ := a.explore.ReadPool(ctx, 1, "movies", engine.Day(time.Now()), false)
	if p.Version != engine.PoolVersion {
		t.Fatalf("pool version %d", p.Version)
	}
	for _, r := range p.Rows {
		if r.Type != engine.TypeCollection {
			continue
		}
		var got []string
		for _, it := range r.Items {
			got = append(got, it.Key())
		}
		if fmt.Sprint(got) != "[movie:502 movie:503]" {
			t.Fatalf("Finish the collection %v, want the two released missing parts", got)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, u := range f.urls {
			if strings.Contains(u, "/collection/77") {
				return
			}
		}
		t.Fatal("collection never looked up")
	}
	t.Fatal("no Finish the collection row")
}

// route: POST /api/v1/explore/counts
// Counts accept an open or ask from a known row type and refuse anything else.
func TestExploreCountsRoute(t *testing.T) {
	t.Parallel()
	a, _, _ := exploreRig(t)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	post := func(body string) int {
		resp, err := http.Post(srv.URL+"/api/v1/explore/counts", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if got := post(`{"row_type":"hero","event":"open"}`); got != http.StatusNoContent {
		t.Fatalf("hero open = %d, want 204", got)
	}
	if got := post(`{"row_type":"hero","event":"play"}`); got != http.StatusBadRequest {
		t.Fatalf("unknown event = %d, want 400", got)
	}
	if got := post(`{"row_type":"nope","event":"ask"}`); got != http.StatusBadRequest {
		t.Fatalf("unknown row = %d, want 400", got)
	}
	if got := post(`not json`); got != http.StatusBadRequest {
		t.Fatalf("bad body = %d, want 400", got)
	}
}
