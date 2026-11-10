package requests

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// fakeWatchlist is a stand-in for Plex's discover service: one watchlist
// per token, read with the X-Plex-Token header.
type fakeWatchlist struct {
	mu    sync.Mutex
	lists map[string][]map[string]any
	calls int
	bad   map[string]bool // tokens answered 401
	// lookupFails is how many more id lookups of a key answer 500.
	lookupFails map[string]int
	srv         *httptest.Server
}

func newFakeWatchlist(t *testing.T) *fakeWatchlist {
	f := &fakeWatchlist{lists: map[string][]map[string]any{}, bad: map[string]bool{}, lookupFails: map[string]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls++
		tok := r.Header.Get("X-Plex-Token")
		if r.Header.Get("X-Plex-Product") != "HolyArrs" || r.Header.Get("Accept") != "application/json" {
			http.Error(w, "headers", http.StatusBadRequest)
			return
		}
		list, ok := f.lists[tok]
		if !ok || f.bad[tok] {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		var out []map[string]any
		switch {
		case r.URL.Path == "/library/sections/watchlist/all":
			q := r.URL.Query()
			if q.Get("X-Plex-Container-Size") != "50" || q.Get("includeGuids") != "1" || q.Get("sort") != "watchlistedAt:desc" {
				http.Error(w, "query", http.StatusBadRequest)
				return
			}
			for _, m := range list {
				c := map[string]any{}
				for k, v := range m {
					if k != "hidden_guid" {
						c[k] = v
					}
				}
				out = append(out, c)
			}
		case strings.HasPrefix(r.URL.Path, "/library/metadata/"):
			key := strings.TrimPrefix(r.URL.Path, "/library/metadata/")
			if f.lookupFails[key] > 0 {
				f.lookupFails[key]--
				http.Error(w, "busy", http.StatusInternalServerError)
				return
			}
			for _, m := range list {
				if m["ratingKey"] == key {
					c := map[string]any{"ratingKey": key, "type": m["type"], "title": m["title"]}
					if g, ok := m["hidden_guid"]; ok {
						c["Guid"] = []map[string]string{{"id": g.(string)}}
					}
					out = append(out, c)
				}
			}
		default:
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"size": len(out), "Metadata": out}})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeWatchlist) set(tok string, items ...map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists[tok] = items
}

func (f *fakeWatchlist) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func plexMovie(key, title string, year int, guids ...string) map[string]any {
	m := map[string]any{"ratingKey": key, "type": "movie", "title": title, "year": year}
	var gs []map[string]string
	for _, g := range guids {
		gs = append(gs, map[string]string{"id": g})
	}
	if gs != nil {
		m["Guid"] = gs
	}
	return m
}

// watchRig is the show rig with a fake watchlist, the Requests setting on
// and ana's token known.
func watchRig(t *testing.T) (*rig, *fakeWatchlist, *Watchlists) {
	t.Helper()
	r := showRig(t)
	ctx := context.Background()
	f := newFakeWatchlist(t)
	tokens := map[int64]string{r.ana.UserID: "tok-ana", r.bo.UserID: "tok-bo", r.luis.UserID: "tok-luis"}
	w := &Watchlists{Asks: r.svc, Base: f.srv.URL, Token: func(_ context.Context, id int64) (string, error) { return tokens[id], nil },
		ClientID: func(context.Context) string { return "holyarrs-test" }}
	set, err := ReadSettings(ctx, r.st.Read())
	if err != nil {
		t.Fatal(err)
	}
	set.PlexWatchlists = true
	if err := SaveSettings(ctx, r.st, set, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE users SET perms = '["ask"]' WHERE role = 'member'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return r, f, w
}

func itemStates(t *testing.T, r *rig, userID int64) map[string]string {
	t.Helper()
	rows, err := r.st.Read().Query("SELECT plex_key, state FROM plex_watchlist_items WHERE user_id = ?", userID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, s string
		if err := rows.Scan(&k, &s); err != nil {
			t.Fatal(err)
		}
		out[k] = s
	}
	return out
}

func TestWatchlistAsksOnceAndSkipsQuietly(t *testing.T) {
	r, f, w := watchRig(t)
	ctx := context.Background()
	// Switches off: nothing is read at all.
	if need, _, _ := w.Probe(ctx, r.clock); need {
		t.Fatal("probe wants a pass with every switch off")
	}
	if _, err := w.Pass(ctx, r.ana.UserID); err != nil || f.count() != 0 {
		t.Fatalf("pass with switches off: %v, %d calls", err, f.count())
	}
	if err := SaveWatchlistSwitches(ctx, r.st, r.ana.UserID, WatchlistSwitches{Movies: true, Shows: true}); err != nil {
		t.Fatal(err)
	}
	if need, _, _ := w.Probe(ctx, r.clock); !need {
		t.Fatal("probe skips a person with a switch on")
	}
	// Paper Harbour is in the Library; Salt Choir is on Never add; one
	// entry has no ids; one is music (left out); the show is asked whole.
	if _, _, err := r.svc.Library.Add(ctx, movie("Paper Harbour", 2023, "900002"), library.AddOptions{}, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO never_add (created_at, created_by, updated_at, updated_by, kind, name, source_ids)
			VALUES ('2026-10-01T00:00:00.000000000Z', '{"kind":"system"}', '2026-10-01T00:00:00.000000000Z', '{"kind":"system"}', 'movie', 'Salt Choir', '{"tmdb":"900005"}')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	f.set("tok-ana",
		plexMovie("k-dune", "Dune: Part Two", 2024, "imdb://tt15239678", "tmdb://693134"),
		plexMovie("k-paper", "Paper Harbour", 2023, "tmdb://900002"),
		plexMovie("k-salt", "Salt Choir", 2021, "tmdb://900005"),
		map[string]any{"ratingKey": "k-orchard", "type": "movie", "title": "Quiet Orchard", "hidden_guid": "tmdb://900003"},
		map[string]any{"ratingKey": 4401, "type": "movie", "title": "Nowhere"},
		map[string]any{"ratingKey": "k-song", "type": "track", "title": "A Song"},
		map[string]any{"ratingKey": "k-show", "type": "show", "title": "Lab Show", "year": "2025", "Guid": []map[string]string{{"id": "tvdb://200"}}},
	)
	res, err := w.Pass(ctx, r.ana.UserID)
	if err != nil {
		t.Fatal(err)
	}
	got := itemStates(t, r, r.ana.UserID)
	want := map[string]string{"k-dune": WatchAsked, "k-paper": WatchInLibrary, "k-salt": WatchSkipped, "k-orchard": WatchAsked, "4401": WatchNoMatch, "k-show": WatchAsked}
	if len(got) != len(want) {
		t.Fatalf("entries %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q (all %v)", k, got[k], v, got)
		}
	}
	if res.Asked != 3 {
		t.Fatalf("asked %d", res.Asked)
	}
	// The asks are ana's own, waiting, marked as from her watchlist; the show is whole.
	vs, err := r.svc.Views(ctx, "a.id IN (SELECT ask_id FROM ask_people WHERE user_id = ?)", r.ana.UserID)
	if err != nil || len(vs) != 3 {
		t.Fatalf("views %v %+v", err, vs)
	}
	for _, v := range vs {
		if v.State != domain.AskAsked || len(v.People) != 1 || !v.People[0].FromWatchlist {
			t.Fatalf("ask %+v", v)
		}
		if v.Kind == "show" && (len(v.Seasons) != 0 || len(v.Episodes) != 0) {
			t.Fatalf("show not asked whole: %+v", v)
		}
	}
	// A second pass asks for nothing again, and a taken back ask stays taken back.
	var dune int64
	for _, v := range vs {
		if v.TMDBID == "693134" {
			dune = v.ID
		}
	}
	if err := r.svc.Withdraw(ctx, dune, r.ana, false); err != nil {
		t.Fatal(err)
	}
	calls := f.count()
	if res, err := w.Pass(ctx, r.ana.UserID); err != nil || res.Asked != 0 {
		t.Fatalf("second pass %+v %v", res, err)
	}
	if f.count() != calls+1 {
		t.Fatalf("second pass made %d calls, want 1", f.count()-calls)
	}
	var n int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM asks WHERE tmdb_id = '693134' AND state = 'asked'").Scan(&n)
	if n != 0 {
		t.Fatal("a taken back ask was asked for again")
	}
	st, err := ReadWatchlist(ctx, r.st.Read(), r.ana.UserID)
	if err != nil || st.CheckedAt == nil || st.Error != "" || len(st.Items) != 6 || !st.Movies || !st.Shows {
		t.Fatalf("state %+v %v", st, err)
	}
}

func TestWatchlistJoinsAutoApprovesAndKeepsLimits(t *testing.T) {
	r, f, w := watchRig(t)
	ctx := context.Background()
	for _, id := range []int64{r.ana.UserID, r.bo.UserID, r.luis.UserID} {
		if err := SaveWatchlistSwitches(ctx, r.st, id, WatchlistSwitches{Movies: true}); err != nil {
			t.Fatal(err)
		}
	}
	// bo asked for Dune by hand; ana's watchlist joins it.
	if _, err := r.svc.Ask(ctx, r.bo, "693134"); err != nil {
		t.Fatal(err)
	}
	f.set("tok-ana", plexMovie("k-dune", "Dune", 2024, "tmdb://693134"))
	f.set("tok-bo", plexMovie("k-dune2", "Dune", 2024, "tmdb://693134"))
	if _, err := w.Pass(ctx, r.ana.UserID); err != nil {
		t.Fatal(err)
	}
	if got := itemStates(t, r, r.ana.UserID)["k-dune"]; got != WatchJoined {
		t.Fatalf("ana's entry %q", got)
	}
	// bo had asked already: nothing new.
	if _, err := w.Pass(ctx, r.bo.UserID); err != nil {
		t.Fatal(err)
	}
	if got := itemStates(t, r, r.bo.UserID)["k-dune2"]; got != WatchAlready {
		t.Fatalf("bo's entry %q", got)
	}
	// The owner's watchlist ask is approved straight away.
	f.set("tok-luis", plexMovie("k-paper", "Paper Harbour", 2023, "tmdb://900002"))
	if _, err := w.Pass(ctx, r.luis.UserID); err != nil {
		t.Fatal(err)
	}
	vs, _ := r.svc.Views(ctx, "a.tmdb_id = ?", "900002")
	if len(vs) != 1 || vs[0].State != domain.AskApproved || !vs[0].AutoApproved {
		t.Fatalf("owner's ask %+v", vs)
	}
	// ana's weekly limit is used up: her entry waits and is tried again.
	one := 1
	if err := SetPersonLimit(ctx, r.st, r.ana.UserID, PersonLimit{Movies: &one}); err != nil {
		t.Fatal(err)
	}
	useAsk(t, r, r.ana.UserID, "movie")
	f.set("tok-ana", plexMovie("k-dune", "Dune", 2024, "tmdb://693134"), plexMovie("k-salt", "Salt Choir", 2021, "tmdb://900005"))
	if _, err := w.Pass(ctx, r.ana.UserID); err != nil {
		t.Fatal(err)
	}
	if got := itemStates(t, r, r.ana.UserID)["k-salt"]; got != WatchLimit {
		t.Fatalf("limit entry %q", got)
	}
	none := 0
	if err := SetPersonLimit(ctx, r.st, r.ana.UserID, PersonLimit{Movies: &none}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Pass(ctx, r.ana.UserID); err != nil {
		t.Fatal(err)
	}
	if got := itemStates(t, r, r.ana.UserID)["k-salt"]; got != WatchAsked {
		t.Fatalf("retried entry %q", got)
	}
}

func TestWatchlistKidAndRefusedToken(t *testing.T) {
	r, f, w := watchRig(t)
	ctx := context.Background()
	r.svc.Metadata = &metadata.Static{Movies: []metadata.Movie{{Name: "Grim", Year: 2020, Rating: "R", IDs: map[string]string{"tmdb": "900099"}}}}
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE users SET max_rating = 'PG' WHERE id = ?", r.ana.UserID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := SaveWatchlistSwitches(ctx, r.st, r.ana.UserID, WatchlistSwitches{Movies: true}); err != nil {
		t.Fatal(err)
	}
	f.set("tok-ana", plexMovie("k-grim", "Grim", 2020, "tmdb://900099"))
	if _, err := w.Pass(ctx, r.ana.UserID); err != nil {
		t.Fatal(err)
	}
	if got := itemStates(t, r, r.ana.UserID)["k-grim"]; got != WatchSkipped {
		t.Fatalf("kid entry %q", got)
	}
	var n int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM asks").Scan(&n)
	if n != 0 {
		t.Fatal("a Kid's watchlist asked above the limit")
	}
	// Plex refuses the token: the person sees why.
	f.bad["tok-ana"] = true
	if _, err := w.Pass(ctx, r.ana.UserID); err == nil {
		t.Fatal("no error for a refused token")
	}
	st, _ := ReadWatchlist(ctx, r.st.Read(), r.ana.UserID)
	if !strings.Contains(st.Error, "link your Plex account again") {
		t.Fatalf("error %q", st.Error)
	}
	// No token at all: no call, and the person is told to link Plex.
	w.Token = func(context.Context, int64) (string, error) { return "", nil }
	calls := f.count()
	if _, err := w.Pass(ctx, r.ana.UserID); err != nil || f.count() != calls {
		t.Fatalf("pass without a token: %v, %d calls", err, f.count()-calls)
	}
	st, _ = ReadWatchlist(ctx, r.st.Read(), r.ana.UserID)
	if !strings.Contains(st.Error, "Link your Plex account") {
		t.Fatalf("error %q", st.Error)
	}
	// The Requests setting off: nobody is read.
	set, _ := ReadSettings(ctx, r.st.Read())
	set.PlexWatchlists = false
	_ = SaveSettings(ctx, r.st, set, domain.System)
	if need, _, _ := w.Probe(ctx, r.clock); need {
		t.Fatal("probe wants a pass with the Requests setting off")
	}
}

func TestWatchlistLookupErrorIsRetriedNoIDStays(t *testing.T) {
	r, f, w := watchRig(t)
	ctx := context.Background()
	if err := SaveWatchlistSwitches(ctx, r.st, r.ana.UserID, WatchlistSwitches{Movies: true}); err != nil {
		t.Fatal(err)
	}
	// Both entries come without ids. Plex's lookup of the first fails once
	// and then gives a TMDB id; the second's lookup answers with no ids.
	f.set("tok-ana",
		map[string]any{"ratingKey": "k-orchard", "type": "movie", "title": "Quiet Orchard", "hidden_guid": "tmdb://900003"},
		map[string]any{"ratingKey": "k-none", "type": "movie", "title": "Nowhere"},
	)
	f.mu.Lock()
	f.lookupFails["k-orchard"] = 1
	f.mu.Unlock()
	if _, err := w.Pass(ctx, r.ana.UserID); err != nil {
		t.Fatal(err)
	}
	got := itemStates(t, r, r.ana.UserID)
	if got["k-orchard"] != WatchFailed || got["k-none"] != WatchNoMatch {
		t.Fatalf("first pass %v", got)
	}
	var n int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM asks").Scan(&n)
	if n != 0 {
		t.Fatal("an entry was asked for without ids")
	}
	// Next pass: the lookup works and the entry is asked; no_match stays,
	// and its page is not looked up again.
	calls := f.count()
	if res, err := w.Pass(ctx, r.ana.UserID); err != nil || res.Asked != 1 {
		t.Fatalf("second pass %+v %v", res, err)
	}
	got = itemStates(t, r, r.ana.UserID)
	if got["k-orchard"] != WatchAsked || got["k-none"] != WatchNoMatch {
		t.Fatalf("second pass %v", got)
	}
	if f.count() != calls+2 {
		t.Fatalf("second pass made %d calls, want 2 (list + one lookup)", f.count()-calls)
	}
	// A lookup that keeps failing stops after the usual number of tries.
	f.set("tok-ana", map[string]any{"ratingKey": "k-down", "type": "movie", "title": "Down", "hidden_guid": "tmdb://900004"})
	f.mu.Lock()
	f.lookupFails["k-down"] = 100
	f.mu.Unlock()
	for i := 0; i < watchlistTries+2; i++ {
		if _, err := w.Pass(ctx, r.ana.UserID); err != nil {
			t.Fatal(err)
		}
	}
	var tries int
	_ = r.st.Read().QueryRow("SELECT tries FROM plex_watchlist_items WHERE user_id = ? AND plex_key = 'k-down'", r.ana.UserID).Scan(&tries)
	if got := itemStates(t, r, r.ana.UserID)["k-down"]; got != WatchFailed || tries != watchlistTries {
		t.Fatalf("k-down %q after %d tries", got, tries)
	}
}

func TestWatchlistPeopleEveryoneSwitch(t *testing.T) {
	r, f, w := watchRig(t)
	ctx := context.Background()
	tokens := map[int64]string{r.ana.UserID: "tok-ana", r.bo.UserID: "tok-bo"}
	w.Token = func(_ context.Context, id int64) (string, error) { return tokens[id], nil }
	has := func(ids []int64, id int64) bool {
		for _, x := range ids {
			if x == id {
				return true
			}
		}
		return false
	}
	// ana turns it on, bo turns it off, luis chooses nothing.
	if err := SaveWatchlistSwitches(ctx, r.st, r.ana.UserID, WatchlistSwitches{Movies: true}); err != nil {
		t.Fatal(err)
	}
	if err := SaveWatchlistSwitches(ctx, r.st, r.bo.UserID, WatchlistSwitches{}); err != nil {
		t.Fatal(err)
	}
	// Switch off: only those who turned it on.
	ids, err := w.people(ctx)
	if err != nil || len(ids) != 1 || ids[0] != r.ana.UserID {
		t.Fatalf("switch off: %v %v", ids, err)
	}
	set, _ := ReadSettings(ctx, r.st.Read())
	set.PlexWatchlistsEveryone = true
	if err := SaveSettings(ctx, r.st, set, domain.System); err != nil {
		t.Fatal(err)
	}
	if set, _ := ReadSettings(ctx, r.st.Read()); !set.PlexWatchlistsEveryone {
		t.Fatal("everyone switch not kept")
	}
	// Switch on: luis has no token and is left out; bo turned it off.
	ids, err = w.people(ctx)
	if err != nil || !has(ids, r.ana.UserID) || has(ids, r.bo.UserID) || has(ids, r.luis.UserID) {
		t.Fatalf("switch on, no token: %v %v", ids, err)
	}
	// A pass for someone without a token says nothing and calls nobody.
	calls := f.count()
	if _, err := w.Pass(ctx, r.luis.UserID); err != nil || f.count() != calls {
		t.Fatalf("quiet pass: %v", err)
	}
	if st, _ := ReadWatchlist(ctx, r.st.Read(), r.luis.UserID); st.Error != "" || st.CheckedAt != nil {
		t.Fatalf("quiet pass wrote %+v", st)
	}
	// With a token luis is read for movies and shows, and stays in after a pass.
	tokens[r.luis.UserID] = "tok-luis"
	f.set("tok-luis", plexMovie("k-l1", "Quiet Orchard", 2022, "tmdb://900003"))
	ids, _ = w.people(ctx)
	if !has(ids, r.luis.UserID) || has(ids, r.bo.UserID) {
		t.Fatalf("switch on, token: %v", ids)
	}
	if _, err := w.Pass(ctx, r.luis.UserID); err != nil {
		t.Fatal(err)
	}
	if ids, _ = w.people(ctx); !has(ids, r.luis.UserID) {
		t.Fatalf("luis dropped after a pass: %v", ids)
	}
	// luis turns it off himself: left out from then on.
	if err := SaveWatchlistSwitches(ctx, r.st, r.luis.UserID, WatchlistSwitches{}); err != nil {
		t.Fatal(err)
	}
	if ids, _ = w.people(ctx); has(ids, r.luis.UserID) {
		t.Fatalf("luis still read after turning off: %v", ids)
	}
}
