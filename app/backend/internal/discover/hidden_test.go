package discover

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// hiddenRig serves the hide routes and the people routes on a fresh
// database with an owner (1), an admin (2) and a Kid (3).
func hiddenRig(t *testing.T) (*httptest.Server, *Handlers, *database.Store) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (1, 't', '{}', 't', '{}', 'owner', 'owner', 'x')`,
			`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (2, 't', '{}', 't', '{}', 'ana', 'admin', 'x')`,
			`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (3, 't', '{}', 't', '{}', 'kid', 'member', 'x')`,
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	h := testHandlers(s, &fakePeople{Static: &metadata.Static{}})
	h.CardFresh = func(context.Context, metadata.DiscoverItem, int64) (*engine.Fresh, error) { return nil, nil }
	m := http.NewServeMux()
	h.HiddenRoutes(m)
	h.PeopleRoutes(m)
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	return srv, h, s
}

func as(user int64, role string, perms ...webserver.Perm) context.Context {
	return webserver.ContextWith(context.Background(), webserver.Caller{UserID: user, Role: role, Perms: perms})
}

func items(ids ...string) []metadata.DiscoverItem {
	var out []metadata.DiscoverItem
	for _, id := range ids {
		kind, rest, _ := strings.Cut(id, ":")
		tmdb, genres, _ := strings.Cut(rest, "/")
		it := metadata.DiscoverItem{Kind: kind, TMDBID: tmdb}
		if genres != "" {
			it.Genres = strings.Split(genres, ",")
		}
		out = append(out, it)
	}
	return out
}

func keys(list []metadata.DiscoverItem) string {
	var out []string
	for _, it := range list {
		out = append(out, it.Kind+":"+it.TMDBID)
	}
	return strings.Join(out, " ")
}

func TestHideRoutesAddListShowAgain(t *testing.T) {
	srv, _, _ := hiddenRig(t)
	var first Hide
	if err := json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/hidden", `{"kind": "movie", "tmdb_id": "101", "name": "Alpha"}`, http.StatusCreated), &first); err != nil {
		t.Fatal(err)
	}
	if first.ID == 0 || first.Name != "Alpha" || first.UserID != 0 || first.CreatedAt == "" {
		t.Fatalf("hide = %+v", first)
	}
	// The same hide again is the same row (a second tap, an Undo race).
	var again Hide
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/hidden", `{"kind": "movie", "tmdb_id": "101"}`, http.StatusOK), &again)
	if again.ID != first.ID {
		t.Fatalf("again = %+v, want id %d", again, first.ID)
	}
	do(t, "POST", srv.URL+"/api/v1/hidden", `{"kind": "movie", "genre": "Horror", "user_id": 3}`, http.StatusCreated)
	for _, bad := range []string{
		`{"kind": "artist", "tmdb_id": "1"}`,
		`{"kind": "movie"}`,
		`{"kind": "movie", "tmdb_id": "1", "genre": "Horror"}`,
		`{"kind": "movie", "tmdb_id": "01"}`,
		`{"kind": "movie", "tmdb_id": "x"}`,
		`{"kind": "show", "genre": "Horror"}`,
		`{"kind": "movie", "genre": "Horror", "user_id": 99}`,
		`{"kind": "movie", "genre": "Horror", "user_id": -1}`,
	} {
		do(t, "POST", srv.URL+"/api/v1/hidden", bad, http.StatusBadRequest)
	}
	var list struct {
		Hidden []Hide `json:"hidden"`
	}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/hidden", "", http.StatusOK), &list)
	if len(list.Hidden) != 2 {
		t.Fatalf("list = %+v", list)
	}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/hidden?user_id=3", "", http.StatusOK), &list)
	if len(list.Hidden) != 1 || list.Hidden[0].Genre != "Horror" || list.Hidden[0].UserID != 3 {
		t.Fatalf("person list = %+v", list)
	}
	do(t, "GET", srv.URL+"/api/v1/hidden?user_id=x", "", http.StatusBadRequest)
	do(t, "DELETE", srv.URL+"/api/v1/hidden/"+strconv.FormatInt(first.ID, 10), "", http.StatusNoContent)
	do(t, "DELETE", srv.URL+"/api/v1/hidden/"+strconv.FormatInt(first.ID, 10), "", http.StatusNotFound)
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/hidden?user_id=0", "", http.StatusOK), &list)
	if len(list.Hidden) != 0 {
		t.Fatalf("after show again = %+v", list)
	}
}

func TestHiddenLeavesBrowsingPerPerson(t *testing.T) {
	srv, h, _ := hiddenRig(t)
	do(t, "POST", srv.URL+"/api/v1/hidden", `{"kind": "movie", "tmdb_id": "101"}`, http.StatusCreated)
	do(t, "POST", srv.URL+"/api/v1/hidden", `{"kind": "movie", "genre": "Horror", "user_id": 3}`, http.StatusCreated)
	list := items("movie:101", "movie:102/Horror,Thriller", "show:101", "show:103/Horror", "movie:104/Comedy")
	for _, c := range []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"owner", as(1, webserver.RoleOwner), "movie:102 show:101 show:103 movie:104"},
		{"kid", as(3, webserver.RoleMember), "show:101 show:103 movie:104"},
		{"background job", context.Background(), "movie:102 show:101 show:103 movie:104"},
	} {
		got, err := h.DropNeverAdd(c.ctx, list)
		if err != nil {
			t.Fatal(err)
		}
		if keys(got) != c.want {
			t.Errorf("%s: DropNeverAdd = %q, want %q", c.name, keys(got), c.want)
		}
		joined, err := h.JoinItems(c.ctx, list, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		var ids []metadata.DiscoverItem
		for _, v := range joined {
			ids = append(ids, v.DiscoverItem)
		}
		if keys(ids) != c.want {
			t.Errorf("%s: JoinItems = %q, want %q", c.name, keys(ids), c.want)
		}
	}
	// Managers still find a hidden title to add it, marked; others do not.
	if hidden, drop := HiddenMark(as(3, webserver.RoleMember), h.Store, "movie", "102", []string{"Horror"}); !hidden || !drop {
		t.Errorf("kid mark = %v %v", hidden, drop)
	}
	if hidden, drop := HiddenMark(as(1, webserver.RoleOwner), h.Store, "movie", "101", nil); !hidden || drop {
		t.Errorf("owner mark = %v %v", hidden, drop)
	}
	if hidden, drop := HiddenMark(as(2, webserver.RoleAdmin, webserver.PermManageLibrary), h.Store, "movie", "104", nil); hidden || drop {
		t.Errorf("not hidden mark = %v %v", hidden, drop)
	}
}

// No hides: browsing reads nothing more than before (the empty set is kept
// in memory), and a change is seen on the next request.
func TestHiddenKeptInMemory(t *testing.T) {
	srv, h, s := hiddenRig(t)
	ctx := as(1, webserver.RoleOwner)
	if _, err := h.dropHidden(ctx, items("movie:1")); err != nil {
		t.Fatal(err)
	}
	gen := hiddenGen(s)
	// With the table gone, only a kept set can answer.
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`ALTER TABLE hidden RENAME TO hidden_away`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, err := h.dropHidden(ctx, items("movie:1")); err != nil || len(got) != 1 {
		t.Fatalf("kept answer = %v, %v", got, err)
	}
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`ALTER TABLE hidden_away RENAME TO hidden`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	do(t, "POST", srv.URL+"/api/v1/hidden", `{"kind": "movie", "tmdb_id": "1"}`, http.StatusCreated)
	if hiddenGen(s) == gen {
		t.Error("a change did not move the generation on")
	}
	if got, _ := h.dropHidden(ctx, items("movie:1")); len(got) != 0 {
		t.Errorf("after the hide = %v", got)
	}
}

func TestHiddenOffPeoplePages(t *testing.T) {
	srv, _, _ := hiddenRig(t)
	do(t, "POST", srv.URL+"/api/v1/hidden", `{"kind": "movie", "tmdb_id": "102"}`, http.StatusCreated)
	var page struct {
		Movies []struct {
			IDs map[string]string `json:"ids"`
		} `json:"movies"`
		Total int `json:"total_results"`
	}
	if err := json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/people/11", "", http.StatusOK), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Fatalf("total = %d, want 2", page.Total)
	}
	for _, m := range page.Movies {
		if m.IDs["tmdb"] == "102" {
			t.Fatal("the hidden movie is on the people page")
		}
	}
	var found struct {
		People []metadata.Person `json:"people"`
	}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/people/search?q=alex", "", http.StatusOK), &found)
	if len(found.People) != 2 || len(found.People[1].KnownFor) != 0 || len(found.People[0].KnownFor) != 1 {
		t.Fatalf("known for = %+v", found.People)
	}
}

func TestTitleHidesOnTitlePage(t *testing.T) {
	srv, h, _ := hiddenRig(t)
	do(t, "POST", srv.URL+"/api/v1/hidden", `{"kind": "movie", "genre": "Horror", "user_id": 3}`, http.StatusCreated)
	it := metadata.DiscoverItem{Kind: "movie", TMDBID: "7", Genres: []string{"Horror"}}
	req := func(ctx context.Context) *http.Request {
		return httptest.NewRequest("GET", "/x", nil).WithContext(ctx)
	}
	hidden, hides, err := h.titleHides(req(as(3, webserver.RoleMember)), it)
	if err != nil || !hidden || hides != nil {
		t.Fatalf("kid: %v %v %v", hidden, hides, err)
	}
	hidden, hides, err = h.titleHides(req(as(2, webserver.RoleAdmin)), it)
	if err != nil || hidden || len(hides) != 1 || hides[0].UserID != 3 {
		t.Fatalf("admin: %v %v %v", hidden, hides, err)
	}
	// A read-only API key sees its own mark, not whose hides they are
	// (the hide list itself needs an admin key).
	key := webserver.ContextWith(context.Background(), webserver.Caller{UserID: 1, Role: webserver.RoleOwner, Via: "api_key", Scopes: []string{webserver.ScopeRead}})
	if _, hides, err = h.titleHides(req(key), it); err != nil || hides != nil {
		t.Fatalf("read key: %v %v", hides, err)
	}
}

// Every genre TMDB puts on a title page can be hidden (Talk shows, TV
// movies), not only the ones the genre picker offers; a genre of the
// other kind still cannot.
func TestHideTakesEveryTMDBGenre(t *testing.T) {
	srv, _, _ := hiddenRig(t)
	for _, body := range []string{
		`{"kind": "movie", "genre": "TV Movie"}`,
		`{"kind": "show", "genre": "Talk"}`,
		`{"kind": "show", "genre": "News"}`,
		`{"kind": "show", "genre": "Soap"}`,
	} {
		do(t, "POST", srv.URL+"/api/v1/hidden", body, http.StatusCreated)
	}
	do(t, "POST", srv.URL+"/api/v1/hidden", `{"kind": "movie", "genre": "Talk"}`, http.StatusBadRequest)
}

// A genre hide reaches the browse categories and the genre rows: their
// titles carry their genres to the filter.
func TestHiddenGenreLeavesBrowseAndGenreRows(t *testing.T) {
	srv, h, _ := hiddenRig(t)
	do(t, "POST", srv.URL+"/api/v1/hidden", `{"kind": "movie", "genre": "Horror"}`, http.StatusCreated)
	ctx := as(1, webserver.RoleOwner)
	list := []engine.Item{{Kind: "movie", TMDBID: "1", Genres: []string{"Horror"}}, {Kind: "movie", TMDBID: "2", Genres: []string{"Comedy"}}}
	got, err := h.browseDropNever(ctx, list)
	if err != nil || len(got) != 1 || got[0].TMDBID != "2" {
		t.Fatalf("browse = %+v, %v", got, err)
	}
	pg, err := GenreRowPage(ctx, engine.Page{Head: []engine.Row{{ID: "r", Items: list}}}, "", h.KidItems, h.DropNeverAdd)
	if err != nil || len(pg.Head) != 1 || len(pg.Head[0].Items) != 1 || pg.Head[0].Items[0].TMDBID != "2" {
		t.Fatalf("genre rows = %+v, %v", pg.Head, err)
	}
}
