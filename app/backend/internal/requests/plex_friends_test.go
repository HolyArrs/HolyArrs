package requests

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// fakeFriends stands in for plex.tv (the owner's friends), Plex's community
// service (a friend's watchlist) and the discover service (an entry's ids).
// Every call must carry the owner's token in the header, and only there.
type fakeFriends struct {
	mu      sync.Mutex
	friends []map[string]any            // the friends answer
	lists   map[string][]map[string]any // watchlist nodes per friend uuid
	hidden  map[string]bool             // friends whose watchlist Plex does not show
	status  map[string]int              // friends whose watchlist read answers this status
	looks   map[string]map[string]any   // entry key -> its own page
	calls   map[string]int              // "friends", "watchlist", "lookup"
	leaked  bool
	// usersStatus makes /api/users answer this status instead of the list.
	usersStatus int
	srv         *httptest.Server
}

const ownerTok = "tok-owner"

var friendUUID = regexp.MustCompile(`user\(id: "([^"]*)"\)`)

func newFakeFriends(t *testing.T) *fakeFriends {
	f := &fakeFriends{lists: map[string][]map[string]any{}, hidden: map[string]bool{}, status: map[string]int{}, looks: map[string]map[string]any{}, calls: map[string]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(r.URL.String(), ownerTok) || strings.Contains(string(body), ownerTok) {
			f.leaked = true
		}
		accept := "application/json"
		if r.URL.Path == "/api/users" {
			accept = "application/xml"
		}
		if r.Header.Get("X-Plex-Token") != ownerTok || r.Header.Get("X-Plex-Product") != "HolyArrs" || r.Header.Get("Accept") != accept {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/friends":
			f.calls["v2"]++
			http.Error(w, "gone", http.StatusGone)
		case r.Method == http.MethodGet && r.URL.Path == "/api/users":
			f.calls["friends"]++
			if f.usersStatus != 0 {
				http.Error(w, "no", f.usersStatus)
				return
			}
			_, _ = w.Write([]byte(friendsXML(f.friends)))
		case r.Method == http.MethodPost && r.URL.Path == "/api":
			f.calls["watchlist"]++
			var q struct {
				Query string `json:"query"`
			}
			if json.Unmarshal(body, &q) != nil || r.Header.Get("Content-Type") != "application/json" {
				http.Error(w, "body", http.StatusBadRequest)
				return
			}
			m := friendUUID.FindStringSubmatch(q.Query)
			if m == nil || !strings.Contains(q.Query, "watchlist(first: 50)") {
				http.Error(w, "query", http.StatusBadRequest)
				return
			}
			if code := f.status[m[1]]; code != 0 {
				http.Error(w, "no", code)
				return
			}
			if f.hidden[m[1]] {
				_, _ = w.Write([]byte(`{"data":{"user":null},"errors":[{"message":"not allowed"}]}`))
				return
			}
			nodes := f.lists[m[1]]
			if nodes == nil {
				nodes = []map[string]any{}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"user": map[string]any{"watchlist": map[string]any{"nodes": nodes}}}})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/library/metadata/"):
			f.calls["lookup"]++
			key := strings.TrimPrefix(r.URL.Path, "/library/metadata/")
			var out []map[string]any
			if m, ok := f.looks[key]; ok {
				out = append(out, m)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"Metadata": out}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// friendsXML is /api/users as Plex gives it: no uuid field, the uuid only
// inside the picture address. A "thumb" is used as given, else one is made
// from "uuid".
func friendsXML(fs []map[string]any) string {
	esc := func(v string) string {
		var b strings.Builder
		_ = xml.EscapeText(&b, []byte(v))
		return b.String()
	}
	var b strings.Builder
	b.WriteString(`<MediaContainer friendlyName="myPlex">`)
	for _, f := range fs {
		thumb, _ := f["thumb"].(string)
		if u, _ := f["uuid"].(string); thumb == "" && u != "" {
			thumb = "https://plex.tv/users/" + u + "/avatar?c=1"
		}
		home := "0"
		if h, _ := f["home"].(bool); h {
			home = "1"
		}
		b.WriteString(`<User id="` + esc(fmt.Sprint(f["id"])) + `" title="x" username="x" thumb="` + esc(thumb) + `" home="` + home + `">`)
		if ss, ok := f["sharedServers"].([]map[string]string); ok {
			for _, s := range ss {
				b.WriteString(`<Server machineIdentifier="` + esc(s["machineIdentifier"]) + `"/>`)
			}
		}
		b.WriteString(`</User>`)
	}
	b.WriteString(`</MediaContainer>`)
	return b.String()
}

func (f *fakeFriends) setFriends(fs ...map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.friends = fs
}

func (f *fakeFriends) setList(uuid string, nodes ...map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists[uuid] = nodes
}

func (f *fakeFriends) count(kind string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[kind]
}

// friendNode is a watchlist entry as the community service gives it, with
// its own page (ids and year) kept for the lookup.
func (f *fakeFriends) friendNode(key, kind, title string, year int, guid string) map[string]any {
	f.mu.Lock()
	f.looks[key] = map[string]any{"ratingKey": key, "type": strings.ToLower(kind), "title": title, "year": year,
		"Guid": []map[string]string{{"id": guid}}}
	f.mu.Unlock()
	return map[string]any{"id": key, "title": title, "type": kind}
}

// friendsRig: the show rig with Plex watchlists on, ana and bo imported from
// the owner's Plex account (Plex ids 9001 and 9002), no one's own token kept,
// and the owner's link kept for the chosen server "srv-1".
func friendsRig(t *testing.T) (*rig, *fakeFriends, *Watchlists, *string) {
	t.Helper()
	r := showRig(t)
	ctx := context.Background()
	f := newFakeFriends(t)
	owner := ownerTok
	w := &Watchlists{Asks: r.svc, Base: f.srv.URL, PlexTV: f.srv.URL, Community: f.srv.URL,
		Token:    func(context.Context, int64) (string, error) { return "", nil },
		Owner:    func(context.Context) (string, string) { return owner, "srv-1" },
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
		if _, err := tx.Exec(`UPDATE users SET perms = '["ask"]' WHERE role = 'member'`); err != nil {
			return err
		}
		for _, l := range []struct {
			id  int64
			ext string
		}{{r.ana.UserID, "9001"}, {r.bo.UserID, "9002"}} {
			if _, err := tx.Exec("INSERT INTO user_links (user_id, provider, external_id, external_name, created_at) VALUES (?, 'plex', ?, ?, '2026-10-01T00:00:00Z')",
				l.id, l.ext, "plex-"+l.ext); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	f.setFriends(
		map[string]any{"id": 9001, "uuid": "aaa111", "username": "anaplex", "sharedServers": []map[string]string{{"machineIdentifier": "srv-1"}}},
		map[string]any{"id": "9002", "uuid": "bbb222", "username": "boplex", "restricted": true, "home": true},
	)
	return r, f, w, &owner
}

func setFriendsSwitch(t *testing.T, r *rig, on bool) {
	t.Helper()
	ctx := context.Background()
	set, err := ReadSettings(ctx, r.st.Read())
	if err != nil {
		t.Fatal(err)
	}
	set.PlexFriends = on
	if err := SaveSettings(ctx, r.st, set, domain.System); err != nil {
		t.Fatal(err)
	}
}

func runWatchlists(t *testing.T, w *Watchlists) {
	t.Helper()
	if err := w.Run(context.Background(), &jobs.Run{Job: domain.Job{Params: map[string]any{}}}); err != nil {
		t.Fatal(err)
	}
}

func countAsks(t *testing.T, r *rig) int {
	t.Helper()
	var n int
	if err := r.st.Read().QueryRow("SELECT count(*) FROM asks").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestFriendsSwitchOffChangesNothing(t *testing.T) {
	r, f, w, _ := friendsRig(t)
	ctx := context.Background()
	f.setList("aaa111", f.friendNode("k-dune", "MOVIE", "Dune: Part Two", 2024, "tmdb://693134"))
	if set, _ := ReadSettings(ctx, r.st.Read()); set.PlexFriends {
		t.Fatal("Include Plex friends is on by default")
	}
	if need, _, err := w.Probe(ctx, r.clock); need || err != nil {
		t.Fatalf("probe wants a pass with the friends switch off: %v", err)
	}
	runWatchlists(t, w)
	if f.count("friends")+f.count("watchlist")+f.count("lookup") != 0 || countAsks(t, r) != 0 {
		t.Fatalf("switch off: calls %v, %d asks", f.calls, countAsks(t, r))
	}
	// The friends switch alone does nothing while Plex watchlists are off.
	setFriendsSwitch(t, r, true)
	set, _ := ReadSettings(ctx, r.st.Read())
	set.PlexWatchlists = false
	if err := SaveSettings(ctx, r.st, set, domain.System); err != nil {
		t.Fatal(err)
	}
	if need, _, _ := w.Probe(ctx, r.clock); need {
		t.Fatal("probe wants a pass with Plex watchlists off")
	}
	runWatchlists(t, w)
	if f.count("friends") != 0 || countAsks(t, r) != 0 {
		t.Fatal("friends read with Plex watchlists off")
	}
}

func TestFriendsWatchlistBecomesAskUnderFriend(t *testing.T) {
	r, f, w, _ := friendsRig(t)
	ctx := context.Background()
	setFriendsSwitch(t, r, true)
	if need, _, err := w.Probe(ctx, r.clock); !need || err != nil {
		t.Fatalf("probe skips imported friends: %v", err)
	}
	f.setList("aaa111",
		f.friendNode("k-dune", "MOVIE", "Dune: Part Two", 2024, "tmdb://693134"),
		f.friendNode("k-show", "SHOW", "Lab Show", 2025, "tvdb://200"),
		map[string]any{"id": "k-ep", "title": "An Episode", "type": "EPISODE"},
	)
	runWatchlists(t, w)
	if f.leaked {
		t.Fatal("the owner's token left the header")
	}
	if f.count("friends") != 1 {
		t.Fatalf("friends read %d times in one pass, want 1", f.count("friends"))
	}
	got := itemStates(t, r, r.ana.UserID)
	if got["k-dune"] != WatchAsked || got["k-show"] != WatchAsked || len(got) != 2 {
		t.Fatalf("ana's entries %v", got)
	}
	vs, err := r.svc.Views(ctx, "a.id IN (SELECT ask_id FROM ask_people WHERE user_id = ?)", r.ana.UserID)
	if err != nil || len(vs) != 2 {
		t.Fatalf("views %v %+v", err, vs)
	}
	for _, v := range vs {
		if v.State != domain.AskAsked || len(v.People) != 1 || v.People[0].ID != r.ana.UserID || !v.People[0].FromWatchlist {
			t.Fatalf("ask %+v", v)
		}
	}
	st, err := ReadWatchlist(ctx, r.st.Read(), r.ana.UserID)
	if err != nil || st.CheckedAt == nil || st.Error != "" || len(st.Items) != 2 {
		t.Fatalf("ana's state %+v %v", st, err)
	}
	for _, it := range st.Items {
		if it.Year == nil || *it.Year == 0 {
			t.Fatalf("entry without its year: %+v", it)
		}
	}
	// A second pass asks for nothing again and looks up no ids again.
	looks := f.count("lookup")
	runWatchlists(t, w)
	if countAsks(t, r) != 2 || f.count("lookup") != looks {
		t.Fatalf("second pass: %d asks, %d more lookups", countAsks(t, r), f.count("lookup")-looks)
	}
	// bo's list is empty: Plex may be hiding it, and he is told so.
	st, _ = ReadWatchlist(ctx, r.st.Read(), r.bo.UserID)
	if st.CheckedAt == nil || st.Error != friendWords["empty"] {
		t.Fatalf("bo's state %+v", st)
	}
}

func TestFriendsKidKeepsKidLimits(t *testing.T) {
	r, f, w, _ := friendsRig(t)
	ctx := context.Background()
	setFriendsSwitch(t, r, true)
	r.svc.Metadata = &metadata.Static{Movies: []metadata.Movie{
		{Name: "Grim", Year: 2020, Rating: "R", IDs: map[string]string{"tmdb": "900099"}},
		{Name: "Sunny", Year: 2021, Rating: "G", IDs: map[string]string{"tmdb": "900098"}},
	}}
	r.svc.Library.Metadata = r.svc.Metadata
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE users SET max_rating = 'PG' WHERE id = ?", r.bo.UserID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	f.setList("bbb222",
		f.friendNode("k-grim", "MOVIE", "Grim", 2020, "tmdb://900099"),
		f.friendNode("k-sunny", "MOVIE", "Sunny", 2021, "tmdb://900098"),
	)
	runWatchlists(t, w)
	got := itemStates(t, r, r.bo.UserID)
	if got["k-grim"] != WatchSkipped || got["k-sunny"] != WatchAsked {
		t.Fatalf("kid's entries %v", got)
	}
	vs, _ := r.svc.Views(ctx, "a.tmdb_id = ?", "900098")
	if len(vs) != 1 || vs[0].State != domain.AskAsked || vs[0].AutoApproved {
		t.Fatalf("a Kid's ask did not wait for approval: %+v", vs)
	}
	// The owner's list of friends flags the Kid.
	people, err := ReadPlexFriends(ctx, r.st.Read())
	if err != nil || len(people) != 2 {
		t.Fatalf("friends %+v %v", people, err)
	}
	for _, p := range people {
		if p.Kid != (p.UserID == r.bo.UserID) {
			t.Fatalf("kid flag %+v", p)
		}
		if p.UserID == r.bo.UserID && (p.Asked != 1 || p.PlexName != "plex-9002" || p.CheckedAt == nil) {
			t.Fatalf("bo's row %+v", p)
		}
	}
}

func TestFriendsRemovedShareStops(t *testing.T) {
	r, f, w, _ := friendsRig(t)
	ctx := context.Background()
	setFriendsSwitch(t, r, true)
	f.setList("aaa111", f.friendNode("k-dune", "MOVIE", "Dune: Part Two", 2024, "tmdb://693134"))
	runWatchlists(t, w)
	if countAsks(t, r) != 1 {
		t.Fatalf("first pass: %d asks", countAsks(t, r))
	}
	// ana no longer shares the chosen server; bo is no longer a friend at all.
	f.setFriends(map[string]any{"id": 9001, "uuid": "aaa111", "sharedServers": []map[string]string{{"machineIdentifier": "srv-other"}}})
	f.setList("aaa111",
		f.friendNode("k-dune", "MOVIE", "Dune: Part Two", 2024, "tmdb://693134"),
		f.friendNode("k-paper", "MOVIE", "Paper Harbour", 2023, "tmdb://900002"))
	f.setList("bbb222", f.friendNode("k-salt", "MOVIE", "Salt Choir", 2021, "tmdb://900005"))
	reads := f.count("watchlist")
	runWatchlists(t, w)
	if countAsks(t, r) != 1 || f.count("watchlist") != reads {
		t.Fatalf("after the share went: %d asks, %d watchlist reads", countAsks(t, r), f.count("watchlist")-reads)
	}
	for _, id := range []int64{r.ana.UserID, r.bo.UserID} {
		if st, _ := ReadWatchlist(ctx, r.st.Read(), id); st.Error != friendWords["gone"] {
			t.Fatalf("user %d: %q", id, st.Error)
		}
	}
	// Plex still lists ana after her share went, with no servers and outside
	// the owner's Home: she is not read.
	f.setFriends(map[string]any{"id": 9001, "uuid": "aaa111", "home": false, "sharedServers": []map[string]string{}})
	runWatchlists(t, w)
	if countAsks(t, r) != 1 || f.count("watchlist") != reads {
		t.Fatalf("empty share: %d asks, %d watchlist reads", countAsks(t, r), f.count("watchlist")-reads)
	}
	// With no server chosen, a friend who shares nothing listed is read again.
	w.Owner = func(context.Context) (string, string) { return ownerTok, "" }
	runWatchlists(t, w)
	if got := itemStates(t, r, r.ana.UserID)["k-paper"]; got != WatchAsked {
		t.Fatalf("ana read again: %q", got)
	}
}

func TestFriendsOwnerUnlinkStops(t *testing.T) {
	r, f, w, owner := friendsRig(t)
	ctx := context.Background()
	setFriendsSwitch(t, r, true)
	f.setList("aaa111", f.friendNode("k-dune", "MOVIE", "Dune: Part Two", 2024, "tmdb://693134"))
	*owner = ""
	if need, _, _ := w.Probe(ctx, r.clock); need {
		t.Fatal("probe wants a pass without the owner's Plex link")
	}
	runWatchlists(t, w)
	if f.count("friends")+f.count("watchlist") != 0 || countAsks(t, r) != 0 {
		t.Fatalf("friends read without the owner's link: %v", f.calls)
	}
	// Plex refusing the owner's link reads no friend either.
	*owner = "tok-old"
	runWatchlists(t, w)
	if f.count("watchlist") != 0 || countAsks(t, r) != 0 {
		t.Fatal("friends read with a refused owner link")
	}
}

func TestFriendsHiddenListAndOwnChoice(t *testing.T) {
	r, f, w, _ := friendsRig(t)
	ctx := context.Background()
	setFriendsSwitch(t, r, true)
	f.mu.Lock()
	f.hidden["aaa111"] = true
	f.mu.Unlock()
	f.setList("bbb222", f.friendNode("k-salt", "MOVIE", "Salt Choir", 2021, "tmdb://900005"))
	// bo turned his watchlist off himself: he is left alone.
	if err := SaveWatchlistSwitches(ctx, r.st, r.bo.UserID, WatchlistSwitches{}); err != nil {
		t.Fatal(err)
	}
	runWatchlists(t, w)
	if st, _ := ReadWatchlist(ctx, r.st.Read(), r.ana.UserID); st.Error != friendWords["hidden"] {
		t.Fatalf("ana's hidden list: %q", st.Error)
	}
	if countAsks(t, r) != 0 || len(itemStates(t, r, r.bo.UserID)) != 0 {
		t.Fatal("a friend who turned it off was read")
	}
	// bo turns on shows only: a movie on his list waits.
	if err := SaveWatchlistSwitches(ctx, r.st, r.bo.UserID, WatchlistSwitches{Shows: true}); err != nil {
		t.Fatal(err)
	}
	runWatchlists(t, w)
	if countAsks(t, r) != 0 {
		t.Fatal("a movie was asked for with only shows on")
	}
}

func TestFriendsSkipPeopleReadWithOwnToken(t *testing.T) {
	r, f, w, _ := friendsRig(t)
	ctx := context.Background()
	setFriendsSwitch(t, r, true)
	own := newFakeWatchlist(t)
	own.set("tok-ana", plexMovie("k-dune", "Dune: Part Two", 2024, "tmdb://693134"))
	w.Base = own.srv.URL
	w.Token = func(_ context.Context, id int64) (string, error) {
		if id == r.ana.UserID {
			return "tok-ana", nil
		}
		return "", nil
	}
	if err := SaveWatchlistSwitches(ctx, r.st, r.ana.UserID, WatchlistSwitches{Movies: true}); err != nil {
		t.Fatal(err)
	}
	f.setList("aaa111", f.friendNode("k-paper", "MOVIE", "Paper Harbour", 2023, "tmdb://900002"))
	runWatchlists(t, w)
	got := itemStates(t, r, r.ana.UserID)
	if got["k-dune"] != WatchAsked || got["k-paper"] != "" {
		t.Fatalf("ana read through the owner's link too: %v", got)
	}
}

func TestFriendsSettingKept(t *testing.T) {
	r, _, _, _ := friendsRig(t)
	setFriendsSwitch(t, r, true)
	set, err := ReadSettings(context.Background(), r.st.Read())
	if err != nil || !set.PlexFriends || !set.PlexWatchlists {
		t.Fatalf("settings %+v %v", set, err)
	}
	b, _ := json.Marshal(set)
	if !strings.Contains(string(b), `"plex_friends":true`) {
		t.Fatalf("json %s", b)
	}
}

func TestFriendsRefusedReadNotes(t *testing.T) {
	r, f, w, _ := friendsRig(t)
	ctx := context.Background()
	setFriendsSwitch(t, r, true)
	// Plex lists both friends with the owner's token, then refuses one
	// watchlist (403: kept from friends) and the token itself for the other.
	f.mu.Lock()
	f.status["aaa111"], f.status["bbb222"] = http.StatusForbidden, http.StatusUnauthorized
	f.mu.Unlock()
	runWatchlists(t, w)
	if st, _ := ReadWatchlist(ctx, r.st.Read(), r.ana.UserID); st.Error != friendWords["hidden"] {
		t.Fatalf("403: %q", st.Error)
	}
	if st, _ := ReadWatchlist(ctx, r.st.Read(), r.bo.UserID); st.Error != friendWords["refused"] {
		t.Fatalf("401: %q", st.Error)
	}
}

func TestFriendsCraftedUUIDNeverQueried(t *testing.T) {
	r, f, w, _ := friendsRig(t)
	setFriendsSwitch(t, r, true)
	f.setFriends(map[string]any{"id": 9001, "uuid": `x") { id } } #`, "sharedServers": []map[string]string{{"machineIdentifier": "srv-1"}}})
	runWatchlists(t, w)
	if f.count("watchlist") != 0 {
		t.Fatalf("a crafted uuid reached the community service: %v", f.calls)
	}
	if _, err := w.fetchFriendList(context.Background(), ownerTok, `x") { id } } #`); err == nil || f.count("watchlist") != 0 {
		t.Fatalf("fetchFriendList sent a crafted uuid: %v", err)
	}
}

func TestFriendsOwnerNeverReadAsFriend(t *testing.T) {
	r, f, w, _ := friendsRig(t)
	ctx := context.Background()
	setFriendsSwitch(t, r, true)
	var owner int64
	if err := r.st.Read().QueryRow("SELECT id FROM users WHERE role = 'owner'").Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO user_links (user_id, provider, external_id, external_name, created_at) VALUES (?, 'plex', '9003', 'me', '2026-10-01T00:00:00Z')", owner)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	f.setFriends(map[string]any{"id": 9003, "uuid": "ccc333", "sharedServers": []map[string]string{{"machineIdentifier": "srv-1"}}})
	f.setList("ccc333", f.friendNode("k-dune", "MOVIE", "Dune: Part Two", 2024, "tmdb://693134"))
	runWatchlists(t, w)
	if f.count("watchlist") != 0 || countAsks(t, r) != 0 {
		t.Fatalf("the owner was read as a friend: %v", f.calls)
	}
}

func TestFriendsGoneOnlyForPeopleReadBefore(t *testing.T) {
	r, f, w, _ := friendsRig(t)
	ctx := context.Background()
	setFriendsSwitch(t, r, true)
	// ana was never the owner's friend: no note at all.
	f.setFriends(map[string]any{"id": "9002", "uuid": "bbb222", "home": true})
	runWatchlists(t, w)
	if st, _ := ReadWatchlist(ctx, r.st.Read(), r.ana.UserID); st.Error != "" || st.CheckedAt != nil {
		t.Fatalf("never-read ana got %q", st.Error)
	}
	// bo was read, then dropped: he gets the note once.
	f.setFriends()
	runWatchlists(t, w)
	st, _ := ReadWatchlist(ctx, r.st.Read(), r.bo.UserID)
	if st.Error != friendWords["gone"] || st.CheckedAt == nil {
		t.Fatalf("bo: %q", st.Error)
	}
	r.clock = r.clock.Add(time.Hour)
	runWatchlists(t, w)
	if again, _ := ReadWatchlist(ctx, r.st.Read(), r.bo.UserID); !again.CheckedAt.Equal(*st.CheckedAt) {
		t.Fatal("the same gone note was written again")
	}
}

func TestFriendsPrivateListIsNotAFailure(t *testing.T) {
	r, f, w, _ := friendsRig(t)
	ctx := context.Background()
	setFriendsSwitch(t, r, true)
	f.mu.Lock()
	f.hidden["aaa111"] = true
	f.status["bbb222"] = http.StatusForbidden
	f.mu.Unlock()
	for id, uuid := range map[int64]string{r.ana.UserID: "aaa111", r.bo.UserID: "bbb222"} {
		if _, err := w.friendPass(ctx, id, uuid, ownerTok); err != nil {
			t.Fatalf("user %d: a private list counted as a failure: %v", id, err)
		}
	}
}

func TestFriendsTokenNeverFollowsRedirectToAnotherHost(t *testing.T) {
	r, _, w, _ := friendsRig(t)
	setFriendsSwitch(t, r, true)
	var got string
	other := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		got = req.Header.Get("X-Plex-Token")
		_, _ = rw.Write([]byte(`[]`))
	}))
	t.Cleanup(other.Close)
	hop := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		http.Redirect(rw, req, strings.Replace(other.URL, "127.0.0.1", "localhost", 1)+req.URL.Path, http.StatusFound)
	}))
	t.Cleanup(hop.Close)
	w.PlexTV = hop.URL
	_, _, _ = w.fetchFriends(context.Background(), ownerTok, "srv-1")
	if got != "" {
		t.Fatal("the owner's token followed a redirect to another host")
	}
}

func friendNotes(t *testing.T, r *rig) map[int64]string {
	t.Helper()
	out := map[int64]string{}
	for _, id := range []int64{r.ana.UserID, r.bo.UserID} {
		st, err := ReadWatchlist(context.Background(), r.st.Read(), id)
		if err != nil {
			t.Fatal(err)
		}
		out[id] = st.Error
	}
	return out
}

func TestFriendsReadFromUsersListWithUUIDFromPicture(t *testing.T) {
	r, f, w, _ := friendsRig(t)
	setFriendsSwitch(t, r, true)
	f.setList("aaa111", f.friendNode("k1", "movie", "Heat", 1995, "tmdb://949"))
	runWatchlists(t, w)
	if f.count("v2") != 0 || f.count("friends") != 1 || f.count("watchlist") == 0 {
		t.Fatalf("calls %v, want /api/users once, no /api/v2/friends, a watchlist read", f.calls)
	}
}

func TestFriendWithoutUUIDIsSkippedAndSaid(t *testing.T) {
	r, f, w, _ := friendsRig(t)
	setFriendsSwitch(t, r, true)
	f.setFriends(
		map[string]any{"id": 9001, "thumb": "https://plex.tv/users/aaa111/avatar", "sharedServers": []map[string]string{{"machineIdentifier": "srv-1"}}},
		map[string]any{"id": "9002", "thumb": "https://example.com/pic.png", "home": true},
	)
	f.setList("aaa111", f.friendNode("k1", "movie", "Heat", 1995, "tmdb://949"))
	runWatchlists(t, w)
	notes := friendNotes(t, r)
	if notes[r.bo.UserID] != friendWords["nocode"] || notes[r.ana.UserID] == friendWords["nocode"] {
		t.Fatalf("notes %v: only the person without a uuid should say so", notes)
	}
	if f.count("watchlist") != 1 {
		t.Fatalf("watchlist read %d times, want 1 (only the one with a uuid)", f.count("watchlist"))
	}
}

func TestFriendsListFailingSaysWhyOnEachPerson(t *testing.T) {
	for _, c := range []struct {
		status int
		want   string
	}{{http.StatusGone, errPlexGone.Error()}, {http.StatusUnauthorized, friendWords["refused"]}, {http.StatusInternalServerError, "Plex answered 500; HolyArrs tries again at the next check"}} {
		r, f, w, _ := friendsRig(t)
		setFriendsSwitch(t, r, true)
		f.usersStatus = c.status
		runWatchlists(t, w)
		for id, note := range friendNotes(t, r) {
			if note != c.want {
				t.Fatalf("status %d: user %d note %q, want %q", c.status, id, note, c.want)
			}
		}
		if f.count("watchlist") != 0 || countAsks(t, r) != 0 {
			t.Fatalf("status %d: something was read or asked", c.status)
		}
	}
}
