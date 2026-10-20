package api

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

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/explore"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
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

func exploreRig(t *testing.T) (*API, *store.Store, *recordingTMDB) {
	t.Helper()
	f := newRecordingTMDB(t)
	_, s := discoverRig(t, f.provider())
	a := New(Server{Store: s, Metadata: f.provider(), Version: "test"})
	a.explore.paced = &explore.Paced{Source: listerSource{f.provider()}, Gap: time.Millisecond}
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

func poolState(t *testing.T, s *store.Store, member int64, sw string) (bool, bool) {
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
	a, s, _ := exploreRig(t)
	ctx := context.Background()
	for _, m := range []int64{1, 2} {
		for _, sw := range exploreSwitches {
			if err := a.buildPool(ctx, m, sw); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(a.discover.hits) != 0 {
		t.Fatalf("a pool build used the per-person limit: %v", a.discover.hits)
	}
	if ok, stale := poolState(t, s, 1, "all"); !ok || stale {
		t.Fatal("pool not stored fresh")
	}
	a.exploreOnEvent(domain.Event{Kind: domain.EventAskMade, Actor: domain.Actor{Kind: "user", ID: 2}})
	if _, stale := poolState(t, s, 2, "all"); !stale {
		t.Fatal("an ask did not mark the member's pool stale")
	}
	if _, stale := poolState(t, s, 1, "all"); stale {
		t.Fatal("an ask marked another member's pool stale")
	}
	var showID int64
	_ = s.Read().QueryRow("SELECT id FROM titles WHERE kind = 'show'").Scan(&showID)
	now := time.Now()
	a.explore.now = func() time.Time { return now }
	a.exploreOnEvent(domain.Event{Kind: domain.EventTitleAdded, TitleID: &showID})
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
	a.explore.now = func() time.Time { return now.Add(5 * time.Minute) }
	a.exploreOnEvent(domain.Event{Kind: domain.EventTitleAdded, TitleID: &showID})
	a.explore.now = func() time.Time { return now.Add(11 * time.Minute) }
	for {
		k, ok := a.nextDue()
		if !ok {
			break
		}
		if k.sw == "shows" {
			t.Fatal("a show pool came due 11 minutes after the first add, 6 after the last")
		}
	}
	a.explore.now = func() time.Time { return now.Add(16 * time.Minute) }
	n := 0
	for {
		k, ok := a.nextDue()
		if !ok {
			break
		}
		if k.sw == "shows" {
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
	a, s, f := exploreRig(t)
	ctx := context.Background()
	err := s.Write(ctx, func(tx *sql.Tx) error {
		ask := domain.Ask{Kind: "movie", TMDBID: "777", Name: "Secret Ask", State: domain.AskAsked, AskedBy: 2, AskedAt: time.Now(), SourceIDs: map[string]string{"tmdb": "777"}}
		if _, err := store.Insert(tx, "asks", &ask, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, 2, ?, 1)", ask.ID, store.FormatTime(time.Now()))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.buildPool(ctx, 2, "all"); err != nil {
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
	a, _, _ := exploreRig(t)
	ctx := context.Background()
	for i := 1000; i < 1900; i++ {
		r := "R"
		if i%2 == 0 {
			r = "PG"
		}
		a.rated.put(fmt.Sprintf("movie|%d", i), r, time.Now())
	}
	if err := a.buildPool(ctx, 3, "movies"); err != nil {
		t.Fatal(err)
	}
	p, _, err := a.readPool(ctx, 3, "movies", explore.Day(time.Now()), false)
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
	if k, ok := a.nextDue(); !ok || k.member != 1 || k.sw != "all" {
		t.Fatal("head did not queue a pool build")
	}
	if err := a.buildPool(context.Background(), 1, "all"); err != nil {
		t.Fatal(err)
	}
	h := get("/api/v1/explore/head?type=all")
	if h["building"] != false || len(h["head"].([]any)) == 0 {
		t.Fatalf("head %v", h)
	}
	seen := map[string]bool{}
	for _, r := range h["head"].([]any) {
		for _, it := range r.(map[string]any)["items"].([]any) {
			k := it.(map[string]any)["kind"].(string) + it.(map[string]any)["tmdb_id"].(string)
			if seen[k] {
				t.Fatalf("%s twice in the head", k)
			}
			seen[k] = true
		}
	}
	tail := get("/api/v1/explore/tail?type=all&page=0")
	if _, ok := tail["rows"].([]any); !ok {
		t.Fatalf("tail %v", tail)
	}
}

// Revision 4 ship gate: Just arrived shows only Library adds from the last
// 14 days, newest first.
func TestExploreJustArrived(t *testing.T) {
	a, s, _ := exploreRig(t)
	ctx := context.Background()
	err := s.Write(ctx, func(tx *sql.Tx) error {
		old := domain.Title{Kind: "movie", Name: "Old", SortName: "old", SourceIDs: map[string]string{"tmdb": "55"}, MetadataSource: "tmdb", NewPartsPolicy: "all", WhyAdded: "by_hand", State: "active", ReleaseGate: "released", GateDriver: "none", Status: "released"}
		if _, err := store.Insert(tx, "titles", &old, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE titles SET created_at = ? WHERE id = ?", store.FormatTime(time.Now().AddDate(0, 0, -30)), old.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.buildPool(ctx, 1, "all"); err != nil {
		t.Fatal(err)
	}
	p, _, _ := a.readPool(ctx, 1, "all", explore.Day(time.Now()), false)
	for _, r := range p.Rows {
		if r.Type != explore.TypeJustArrived {
			continue
		}
		var got []string
		for _, it := range r.Items {
			got = append(got, it.Key())
		}
		if fmt.Sprint(got) != "[show:22 movie:11]" {
			t.Fatalf("Just arrived %v, want the two recent adds, newest first", got)
		}
		return
	}
	t.Fatal("no Just arrived row")
}

// route: POST /api/v1/explore/counts
// Counts accept an open or ask from a known row type and refuse anything else.
func TestExploreCountsRoute(t *testing.T) {
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
