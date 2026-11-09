package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// kidEntry is one title the fake TMDB knows: id, name and US rating ("" for
// none).
type kidEntry struct{ id, name, rating string }

var (
	kidMovies = []kidEntry{
		{"4101", "Gentle Meadow", "G"}, {"4102", "Paper Kite", "PG"}, {"4203", "Night Courier", "PG-13"},
		{"4204", "Iron Verdict", "R"}, {"4205", "Quiet Unknown", ""},
		{"4111", "Sunny Pond", "G"}, {"4211", "Dark Alley", "R"}, {"4212", "Nameless Reel", ""},
	}
	kidShows = []kidEntry{
		{"4121", "Harbor Friends", "TV-PG"}, {"4221", "Cold Precinct", "TV-MA"},
		{"4131", "Puppet Lane", "TV-Y"}, {"4231", "Blood Coast", "TV-MA"},
	}
	// blockedNames are every title a Kid limited to PG must never see.
	blockedNames = []string{"Night Courier", "Iron Verdict", "Quiet Unknown", "Cold Precinct", "Loud Static",
		"Dark Alley", "Nameless Reel", "Blood Coast"}
	// blockedTMDB are their TMDB ids.
	blockedTMDB = []string{"4203", "4204", "4205", "4221", "4211", "4212", "4231"}
)

// kidTMDB is a fake TMDB that knows the titles above with their US
// ratings (release_dates for movies, content_ratings for shows), a person
// credited in some of them, search, Explore rows and recommendations.
type kidTMDB struct {
	srv     *httptest.Server
	details atomic.Int32 // movie and show details answered
	light   atomic.Int32 // release_dates and content_ratings answered
	// extra are more movies a test adds; trending, when set, replaces the
	// trending movies row. Both are set before the first request.
	extra    []kidEntry
	trending []string
}

func newKidTMDB(t *testing.T) *kidTMDB {
	t.Helper()
	f := &kidTMDB{}
	find := func(list []kidEntry, id string) (kidEntry, bool) {
		for _, e := range list {
			if e.id == id {
				return e, true
			}
		}
		return kidEntry{}, false
	}
	movies := func() []kidEntry { return append(slices.Clone(kidMovies), f.extra...) }
	item := func(id string) string {
		if m, ok := find(movies(), id); ok {
			return fmt.Sprintf(`{"id":%s,"title":%q,"release_date":"2024-05-01","poster_path":"/p.jpg","genre_ids":[18]}`, m.id, m.name)
		}
		s, _ := find(kidShows, id)
		return fmt.Sprintf(`{"id":%s,"name":%q,"first_air_date":"2022-01-01","poster_path":"/p.jpg","genre_ids":[18]}`, s.id, s.name)
	}
	list := func(ids ...string) string {
		var out []string
		for _, id := range ids {
			out = append(out, item(id))
		}
		return `{"results":[` + strings.Join(out, ",") + `],"page":1,"total_pages":1,"total_results":` + fmt.Sprint(len(ids)) + `}`
	}
	movieRx := regexp.MustCompile(`^/3/movie/(\d+)(/credits|/recommendations|/release_dates)?$`)
	showRx := regexp.MustCompile(`^/3/tv/(\d+)(/season/1|/recommendations|/content_ratings)?$`)
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch p {
		case "/3/watch/providers/movie", "/3/watch/providers/tv":
			_, _ = io.WriteString(w, `{"results":[{"provider_id":8,"provider_name":"Netflix","display_priorities":{"US":1}}]}`)
			return
		case "/3/genre/movie/list", "/3/genre/tv/list":
			_, _ = io.WriteString(w, `{"genres":[{"id":18,"name":"Drama"}]}`)
			return
		case "/3/trending/movie/week":
			if f.trending != nil {
				_, _ = io.WriteString(w, list(f.trending...))
				return
			}
			_, _ = io.WriteString(w, list("4211", "4111", "4212", "4204", "4101", "4203"))
			return
		case "/3/trending/tv/week":
			_, _ = io.WriteString(w, list("4231", "4131", "4221", "4121"))
			return
		case "/3/movie/popular", "/3/movie/upcoming", "/3/discover/movie":
			_, _ = io.WriteString(w, list("4211", "4111"))
			return
		case "/3/tv/popular", "/3/tv/on_the_air", "/3/discover/tv":
			_, _ = io.WriteString(w, list("4231", "4131"))
			return
		case "/3/search/movie":
			_, _ = io.WriteString(w, list("4111", "4211", "4212", "4203", "4101"))
			return
		case "/3/search/tv":
			_, _ = io.WriteString(w, list("4131", "4231", "4221", "4121"))
			return
		case "/3/search/person":
			_, _ = io.WriteString(w, `{"results":[{"id":4901,"name":"Ann Actor","known_for_department":"Acting","known_for":[`+
				item("4204")+`,`+item("4101")+`,`+item("4111")+`]}],"page":1,"total_pages":1,"total_results":1}`)
			return
		case "/3/person/4901":
			_, _ = io.WriteString(w, `{"id":4901,"name":"Ann Actor","known_for_department":"Acting"}`)
			return
		case "/3/person/4901/movie_credits":
			_, _ = io.WriteString(w, `{"cast":[{"id":4101,"title":"Gentle Meadow","release_date":"2024-05-01","character":"Hero"},
				{"id":4204,"title":"Iron Verdict","release_date":"2023-05-01","character":"Judge"},
				{"id":4111,"title":"Sunny Pond","release_date":"2022-05-01","character":"Friend"}],"crew":[]}`)
			return
		}
		if m := movieRx.FindStringSubmatch(p); m != nil {
			e, ok := find(movies(), m[1])
			if !ok {
				w.WriteHeader(404)
				return
			}
			switch m[2] {
			case "/credits":
				_, _ = io.WriteString(w, `{"cast":[{"id":4901,"name":"Ann Actor","character":"Hero"}],"crew":[]}`)
			case "/recommendations":
				_, _ = io.WriteString(w, list("4111", "4211", "4203"))
			case "/release_dates":
				f.light.Add(1)
				_, _ = io.WriteString(w, `{"id":`+e.id+`,"results":[{"iso_3166_1":"US","release_dates":[{"type":3,"certification":"`+e.rating+`"}]}]}`)
			default:
				f.details.Add(1)
				cert := ""
				if e.rating != "" {
					cert = `,"release_dates":{"results":[{"iso_3166_1":"GB","release_dates":[{"type":3,"certification":"U"}]},` +
						`{"iso_3166_1":"US","release_dates":[{"type":3,"certification":"` + e.rating + `","release_date":"2024-05-01T00:00:00.000Z"}]}]}`
				}
				_, _ = fmt.Fprintf(w, `{"id":%s,"title":%q,"release_date":"2024-05-01","runtime":90,"genres":[{"name":"Drama"}]%s,
					"credits":{"cast":[{"name":"Ann Actor","character":"Hero"}]},"recommendations":%s}`, e.id, e.name, cert, list("4111", "4211", "4203"))
			}
			return
		}
		if m := showRx.FindStringSubmatch(p); m != nil {
			e, ok := find(kidShows, m[1])
			if !ok {
				w.WriteHeader(404)
				return
			}
			switch m[2] {
			case "/season/1":
				_, _ = io.WriteString(w, `{"episodes":[{"id":1,"season_number":1,"episode_number":1,"name":"Pilot","air_date":"2022-01-01"}]}`)
			case "/recommendations":
				_, _ = io.WriteString(w, list("4131", "4231"))
			case "/content_ratings":
				f.light.Add(1)
				_, _ = io.WriteString(w, `{"id":`+e.id+`,"results":[{"iso_3166_1":"US","rating":"`+e.rating+`"}]}`)
			default:
				f.details.Add(1)
				rating := ""
				if e.rating != "" {
					rating = `,"content_ratings":{"results":[{"iso_3166_1":"US","rating":"` + e.rating + `"}]}`
				}
				_, _ = fmt.Fprintf(w, `{"id":%s,"name":%q,"first_air_date":"2022-01-01","status":"Returning Series",
					"seasons":[{"season_number":1,"episode_count":1,"air_date":"2022-01-01"}]%s,"recommendations":%s}`, e.id, e.name, rating, list("4131", "4231"))
			}
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *kidTMDB) provider() *metadata.TMDB {
	return &metadata.TMDB{BaseURL: f.srv.URL + "/3", Access: func(context.Context) (string, error) { return "fake-read-value", nil }, Gap: time.Millisecond}
}

type kidRig struct {
	srv   *httptest.Server
	st    *database.Store
	tmdb  *kidTMDB
	owner *signedIn
	ana   *signedIn // a member without a limit
	kai   *signedIn // a Kid limited to PG
	api   *handlers
	ids   map[string]int64
}

// blockedIDs are the Library ids of the titles above PG (and the artist);
// they are moved to 9101 and up so a body check cannot mistake them.
var blockedIDs = []int64{9101, 9102, 9103, 9104, 9105}

// newKidRig runs the real API with sign-in and a fake TMDB: the owner luis,
// the member ana and the Kid kai (PG). The Library holds movies rated G, PG,
// PG-13, R and none, a TV-PG show, a TV-MA show and an artist.
func newKidRig(t *testing.T, off ...parts.Part) *kidRig {
	t.Helper()
	dir := t.TempDir()
	st, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	if err := st.Write(ctx, insertFolder(t)); err != nil {
		t.Fatal(err)
	}
	v, err := secrets.Open(ctx, st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	f := newKidTMDB(t)
	prov := f.provider()
	q := jobs.New(st, jobs.Options{Workers: 1})
	hub := webserver.NewLiveHub(webserver.LiveOptions{})
	lib := &library.Service{Store: st, Queue: q, Metadata: prov, Live: hub, Shows: metadata.ShowSources{"tmdb": prov}}
	var pc *parts.Control
	if len(off) > 0 {
		pc = &parts.Control{Store: st, Reg: jobs.NewRegistry(), Drain: time.Second, Logf: func(string, ...any) {}}
		if err := pc.Load(ctx); err != nil {
			t.Fatal(err)
		}
		for _, part := range off {
			if err := pc.Set(ctx, part, false, domain.System); err != nil {
				t.Fatal(err)
			}
		}
	}
	a := buildHandlers(services{Store: st, Queue: q, Metadata: prov, Version: "test", Auth: accounts.New(st, accounts.Options{Vault: v}), Secrets: v, Live: hub, Library: lib,
		Parts: pc})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	k := &kidRig{srv: srv, st: st, tmdb: f, ids: map[string]int64{}, api: a}
	browser := func() *signedIn {
		jar, _ := cookiejar.New(nil)
		return &signedIn{t: t, base: srv.URL, c: &http.Client{Jar: jar}}
	}
	read := func(b []byte) (string, string) {
		var x struct {
			CSRF string `json:"csrf"`
			Path string `json:"path"`
		}
		_ = json.Unmarshal(b, &x)
		return x.CSRF, x.Path
	}
	k.owner = browser()
	k.owner.csrf, _ = read(k.owner.call("POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`, 201))
	join := func(name, invite string) *signedIn {
		_, link := read(k.owner.call("POST", "/api/v1/users/invites", invite, 201))
		b := browser()
		b.csrf, _ = read(b.call("POST", "/api/v1/auth/invites/"+strings.TrimPrefix(link, "/invite/"), `{"name":"`+name+`","password":"a long test passphrase"}`, 201))
		var me struct {
			ID int64 `json:"id"`
		}
		_ = json.Unmarshal(b.call("GET", "/api/v1/auth/me", "", 200), &me)
		b.id = me.ID
		return b
	}
	k.ana = join("ana", `{}`)
	k.kai = join("kai", `{"max_rating":"PG"}`)

	soon := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, 2)
	k.add(t, "movie", "Gentle Meadow", "4101", "G", 0, &soon)
	k.add(t, "movie", "Paper Kite", "4102", "PG", 0, nil)
	k.add(t, "show", "Harbor Friends", "4121", "TV-PG", 0, nil)
	k.add(t, "movie", "Night Courier", "4203", "PG-13", 9101, nil)
	k.add(t, "movie", "Iron Verdict", "4204", "R", 9102, &soon)
	k.add(t, "movie", "Quiet Unknown", "4205", "", 9103, nil)
	k.add(t, "show", "Cold Precinct", "4221", "TV-MA", 9104, nil)
	k.add(t, "artist", "Loud Static", "", "", 9105, nil)
	err = st.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO movie_collections(id, source_id, name) VALUES (1, 'tmdb:51', 'Shelf One'), (2, 'tmdb:52', 'Verdict Saga')"); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO movie_collection_titles(collection_id, title_id) VALUES (1, ?), (1, 9102), (2, 9102)", k.ids["Gentle Meadow"])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// add puts one title in the Library; a movie also gets a missing copy
// (Home's "missing" list). id moves the title to that id when not 0.
func (k *kidRig) add(t *testing.T, kind, name, tmdbID, rating string, id int64, cinema *time.Time) {
	t.Helper()
	err := k.st.Write(context.Background(), func(tx *sql.Tx) error {
		ids := map[string]string{"tmdb": tmdbID}
		src := "tmdb"
		if kind == "artist" {
			ids, src = map[string]string{"musicbrainz": "5b11f4ce-a62d-471e-81fc-a69a8278c7da"}, "musicbrainz"
		}
		title := mediafiles.Title{Kind: kind, Name: name, SortName: strings.ToLower(name), SourceIDs: ids, MetadataSource: src, Rating: rating,
			WhyAdded: "by_hand", State: "active", NewPartsPolicy: "all", ReleaseGate: "released", GateDriver: "none", Status: "released",
			CinemaDate: cinema, KeepUpToDate: true}
		if _, err := database.Insert(tx, "titles", &title, domain.System); err != nil {
			return err
		}
		if id != 0 {
			if _, err := tx.Exec("UPDATE titles SET id = ? WHERE id = ?", id, title.ID); err != nil {
				return err
			}
			title.ID = id
		}
		k.ids[name] = title.ID
		if kind != "movie" {
			return nil
		}
		var rulesID, folderID int64
		if err := tx.QueryRow("SELECT id FROM pick_rules ORDER BY id LIMIT 1").Scan(&rulesID); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT id FROM library_folders WHERE kind = 'movie' ORDER BY id LIMIT 1").Scan(&folderID); err != nil {
			return err
		}
		item := mediafiles.Item{TitleID: title.ID, Kind: "movie", Name: &title.Name}
		if _, err := database.Insert(tx, "items", &item, domain.System); err != nil {
			return err
		}
		version := mediafiles.Version{TitleID: title.ID, Label: "Main", IsMain: true, LibraryFolderID: folderID, PickRulesID: rulesID}
		if _, err := database.Insert(tx, "versions", &version, domain.System); err != nil {
			return err
		}
		_, err := database.Insert(tx, "slots", &mediafiles.Slot{ItemID: item.ID, VersionID: version.ID, State: mediafiles.SlotWanted}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

var digitsAround = regexp.MustCompile(`\d`)

// mentionsBlocked names the first blocked title body mentions: a name, a
// Library id or a TMDB id (ids only when no digit touches them).
func mentionsBlocked(body string) string {
	for _, n := range blockedNames {
		if strings.Contains(body, n) {
			return n
		}
	}
	ids := append([]string{}, blockedTMDB...)
	for _, id := range blockedIDs {
		ids = append(ids, fmt.Sprint(id))
	}
	for _, id := range ids {
		for i := strings.Index(body, id); i >= 0; {
			before := i == 0 || !digitsAround.MatchString(body[i-1:i])
			end := i + len(id)
			after := end == len(body) || !digitsAround.MatchString(body[end:end+1])
			if before && after {
				return id
			}
			next := strings.Index(body[i+1:], id)
			if next < 0 {
				break
			}
			i += 1 + next
		}
	}
	return ""
}

func namesIn(t *testing.T, body string, list string) []string {
	t.Helper()
	var x map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &x); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	var items []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(x[list], &items); err != nil {
		t.Fatalf("%s: %v: %s", list, err, body)
	}
	out := []string{}
	for _, it := range items {
		out = append(out, it.Name)
	}
	slices.Sort(out)
	return out
}

func sameNames(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("%s: got %q, want %q", what, got, want)
	}
}

func TestKidTitleList(t *testing.T) {
	t.Parallel()
	k := newKidRig(t)
	sameNames(t, "kid titles", namesIn(t, string(k.kai.call("GET", "/api/v1/titles", "", 200)), "titles"), "Gentle Meadow", "Paper Kite", "Harbor Friends")
	sameNames(t, "kid search", namesIn(t, string(k.kai.call("GET", "/api/v1/titles?q=Iron", "", 200)), "titles"))
}

func TestKidTitleDetailIs404(t *testing.T) {
	t.Parallel()
	k := newKidRig(t)
	for _, id := range blockedIDs {
		for _, sub := range []string{"", "/seasons", "/episodes", "/albums", "/people"} {
			code, body := k.kai.get(fmt.Sprintf("/api/v1/titles/%d%s", id, sub))
			if code != 404 || strings.TrimSpace(body) != `{"error":"not found"}` {
				t.Errorf("title %d%s: %d %s", id, sub, code, body)
			}
		}
	}
	// A title that is not there answers the same, so a Kid cannot tell.
	if code, body := k.kai.get("/api/v1/titles/9999"); code != 404 || strings.TrimSpace(body) != `{"error":"not found"}` {
		t.Errorf("missing title: %d %s", code, body)
	}
	for _, name := range []string{"Gentle Meadow", "Paper Kite", "Harbor Friends"} {
		k.kai.call("GET", fmt.Sprintf("/api/v1/titles/%d", k.ids[name]), "", 200)
	}
	k.kai.call("GET", fmt.Sprintf("/api/v1/titles/%d/seasons", k.ids["Harbor Friends"]), "", 200)
}

// A part that is off does not tell a Kid that a blocked title exists.
func TestKidBlockedTitleIs404WithItsPartOff(t *testing.T) {
	t.Parallel()
	k := newKidRig(t, parts.Music, parts.Shows)
	for _, id := range []int64{9104, 9105} {
		if code, body := k.kai.get(fmt.Sprintf("/api/v1/titles/%d", id)); code != 404 || strings.TrimSpace(body) != `{"error":"not found"}` {
			t.Errorf("title %d with its part off: %d %s", id, code, body)
		}
	}
	if code, _ := k.ana.get("/api/v1/titles/9105"); code != 409 {
		t.Fatalf("member, music off: %d", code)
	}
}

func TestKidCalendar(t *testing.T) {
	t.Parallel()
	k := newKidRig(t)
	// The API refuses manage_library on a Kid; an account from before that
	// rule could still hold it, so it is written straight to the row here.
	k.owner.call("PUT", fmt.Sprintf("/api/v1/users/%d", k.kai.id), `{"perms":["ask","manage_library"]}`, 400)
	if err := k.st.WriteCore(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE users SET perms = '["ask","manage_library"]' WHERE id = ?`, k.kai.id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	b := string(k.kai.call("GET", "/api/v1/calendar", "", 200))
	if !strings.Contains(b, "Gentle Meadow") || mentionsBlocked(b) != "" {
		t.Fatalf("kid calendar: %s", b)
	}
	k.ana.call("GET", "/api/v1/calendar", "", 200) // every signed-in person may read the calendar
}

func TestKidExploreRows(t *testing.T) {
	t.Parallel()
	k := newKidRig(t)
	for _, q := range []string{"row=trending", "row=trending&kind=movie", "row=popular", "row=upcoming&kind=show",
		fmt.Sprintf("row=similar&id=%d", k.ids["Gentle Meadow"]), "row=genre&g=Drama"} {
		b := string(k.kai.call("GET", "/api/v1/discover?"+q, "", 200))
		if hit := mentionsBlocked(b); hit != "" {
			t.Errorf("%s shows %s: %s", q, hit, b)
		}
	}
	sameNames(t, "kid trending", namesIn(t, string(k.kai.call("GET", "/api/v1/discover?row=trending", "", 200)), "items"),
		"Sunny Pond", "Gentle Meadow", "Puppet Lane", "Harbor Friends")
	// A blocked Library title cannot seed a row.
	if code, _ := k.kai.get("/api/v1/discover?row=similar&id=9102"); code != 404 {
		t.Fatalf("similar to a blocked title: %d", code)
	}
}

// TestKidExploreRowFullAfterNeverAdd: Never-add items do not use up a Kid's
// row; with 3 of them among the first allowed and 23 allowed in all, the
// Kid still gets a full row of 20.
func TestKidExploreRowFullAfterNeverAdd(t *testing.T) {
	t.Parallel()
	k := newKidRig(t)
	var ids []string
	for i := range 23 {
		id := fmt.Sprint(4300 + i)
		k.tmdb.extra = append(k.tmdb.extra, kidEntry{id, "Calm Field " + id, "G"})
		ids = append(ids, id)
	}
	k.tmdb.trending = ids
	if err := k.st.Write(context.Background(), func(tx *sql.Tx) error {
		for _, id := range ids[:3] {
			if _, err := tx.Exec(`INSERT INTO never_add (created_at, created_by, updated_at, updated_by, kind, name, source_ids)
				VALUES ('x', '{}', 'x', '{}', 'movie', 'Never', json_object('tmdb', ?))`, id); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got := namesIn(t, string(k.kai.call("GET", "/api/v1/discover?row=trending&kind=movie", "", 200)), "items")
	if len(got) != 20 {
		t.Fatalf("kid row has %d items, want 20: %v", len(got), got)
	}
	for _, n := range got {
		for _, id := range ids[:3] {
			if n == "Calm Field "+id {
				t.Fatalf("Never-add item %s shown", n)
			}
		}
	}
}

func TestKidMetadataSearch(t *testing.T) {
	t.Parallel()
	k := newKidRig(t)
	sameNames(t, "kid search", namesIn(t, string(k.kai.call("GET", "/api/v1/metadata/search?q=any", "", 200)), "movies"), "Sunny Pond", "Gentle Meadow")
	sameNames(t, "kid lookup", namesIn(t, string(k.kai.call("GET", "/api/v1/titles/lookup?term=any", "", 200)), "movies"), "Sunny Pond", "Gentle Meadow")
	sameNames(t, "kid lookup by id", namesIn(t, string(k.kai.call("GET", "/api/v1/titles/lookup?term=tmdb:4211", "", 200)), "movies"))
	sameNames(t, "kid shows", namesIn(t, string(k.kai.call("GET", "/api/v1/shows/lookup?term=any", "", 200)), "shows"), "Puppet Lane", "Harbor Friends")
	sameNames(t, "kid artists", namesIn(t, string(k.kai.call("GET", "/api/v1/artists/lookup?term=loud", "", 200)), "artists"))
}

func TestKidMetadataDetailsIs403(t *testing.T) {
	t.Parallel()
	k := newKidRig(t)
	const above = `{"error":"This title is rated above what this account may see."}`
	for _, p := range []string{"movie/4211", "movie/4212", "movie/4204", "movie/4205", "show/4231", "show/4221"} {
		if code, body := k.kai.get("/api/v1/metadata/" + p); code != 403 || strings.TrimSpace(body) != above {
			t.Errorf("%s: %d %s", p, code, body)
		}
	}
	b := string(k.kai.call("GET", "/api/v1/metadata/movie/4101", "", 200))
	sameNames(t, "kid similar", namesIn(t, b, "similar"), "Sunny Pond")
	if hit := mentionsBlocked(b); hit != "" {
		t.Fatalf("details show %s: %s", hit, b)
	}
	k.kai.call("GET", "/api/v1/metadata/show/4131", "", 200)
}

// A Kid's More from row: the source title's rating is gated like its
// details, on a cold request and on one the cache already holds.
func TestKidMoreFromIs403(t *testing.T) {
	t.Parallel()
	k := newKidRig(t)
	const above = `{"error":"This title is rated above what this account may see."}`
	// Cold: nothing cached yet (R in the Library; R and unrated from TMDB).
	for _, p := range []string{"movie/4204", "movie/4211", "movie/4212"} {
		if code, body := k.kai.get("/api/v1/metadata/" + p + "/more-from"); code != 403 || strings.TrimSpace(body) != above {
			t.Errorf("cold %s: %d %s", p, code, body)
		}
	}
	// Cached: a member warms the row first.
	for _, p := range []string{"movie/4211", "movie/4204"} {
		k.ana.call("GET", "/api/v1/metadata/"+p+"/more-from", "", 200)
		if code, body := k.kai.get("/api/v1/metadata/" + p + "/more-from"); code != 403 || strings.TrimSpace(body) != above {
			t.Errorf("cached %s: %d %s", p, code, body)
		}
	}
	k.kai.call("GET", "/api/v1/metadata/movie/4101/more-from", "", 200)
}

func TestKidAsksAboveLimitAre403(t *testing.T) {
	t.Parallel()
	k := newKidRig(t)
	const upTo = `{"error":"This account can ask only for titles rated up to PG."}`
	// Someone else's open ask is not joined either.
	k.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:4211"}`, 201)
	for _, body := range []string{`{"id":"tmdb:4211"}`, `{"id":"tmdb:4212"}`, `{"id":"tmdb:4204"}`, `{"id":"tmdb:4205"}`,
		`{"kind":"show","id":"tmdb:4231"}`, `{"kind":"show","id":"tmdb:4221"}`} {
		if got := strings.TrimSpace(string(k.kai.call("POST", "/api/v1/asks", body, 403))); got != upTo {
			t.Errorf("%s: %s", body, got)
		}
	}
	if got := strings.TrimSpace(string(k.kai.call("POST", "/api/v1/asks", `{"kind":"artist","id":"mbid:5b11f4ce-a62d-471e-81fc-a69a8278c7da"}`, 403))); got != `{"error":"This account cannot ask for music, because music has no age ratings."}` {
		t.Errorf("music: %s", got)
	}
	k.kai.call("POST", "/api/v1/asks", `{"id":"tmdb:4111"}`, 201)
	k.kai.call("POST", "/api/v1/asks", `{"kind":"show","id":"tmdb:4131"}`, 201)
	var n int
	if err := k.st.Read().QueryRow("SELECT count(*) FROM ask_people WHERE user_id = ?", k.kai.id).Scan(&n); err != nil || n != 2 {
		t.Fatalf("kid asks stored: %d %v", n, err)
	}
}

func TestMemberWithoutLimitSeesEverything(t *testing.T) {
	t.Parallel()
	k := newKidRig(t)
	sameNames(t, "member titles", namesIn(t, string(k.ana.call("GET", "/api/v1/titles", "", 200)), "titles"),
		"Gentle Meadow", "Paper Kite", "Harbor Friends", "Night Courier", "Iron Verdict", "Quiet Unknown", "Cold Precinct", "Loud Static")
	sameNames(t, "member search", namesIn(t, string(k.ana.call("GET", "/api/v1/metadata/search?q=any", "", 200)), "movies"),
		"Sunny Pond", "Dark Alley", "Nameless Reel", "Night Courier", "Gentle Meadow")
	sameNames(t, "member trending", namesIn(t, string(k.ana.call("GET", "/api/v1/discover?row=trending", "", 200)), "items"),
		"Dark Alley", "Sunny Pond", "Nameless Reel", "Iron Verdict", "Gentle Meadow", "Night Courier", "Blood Coast", "Puppet Lane", "Cold Precinct", "Harbor Friends")
	k.ana.call("GET", "/api/v1/titles/9102", "", 200)
	k.ana.call("GET", "/api/v1/metadata/movie/4211", "", 200)
	if b := string(k.ana.call("GET", "/api/v1/people/4901", "", 200)); !strings.Contains(b, "Iron Verdict") || !strings.Contains(b, "Sunny Pond") {
		t.Fatalf("member person: %s", b)
	}
	if b := string(k.ana.call("GET", "/api/v1/collections", "", 200)); !strings.Contains(b, "Verdict Saga") {
		t.Fatalf("member collections: %s", b)
	}
	k.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:4211"}`, 201)
	// The owner, too, sees everything.
	sameNames(t, "owner titles", namesIn(t, string(k.owner.call("GET", "/api/v1/titles", "", 200)), "titles"),
		"Gentle Meadow", "Paper Kite", "Harbor Friends", "Night Courier", "Iron Verdict", "Quiet Unknown", "Cold Precinct", "Loud Static")
}

func TestKidPeopleAndCollections(t *testing.T) {
	t.Parallel()
	k := newKidRig(t)
	// Credits: only Library titles with an allowed rating.
	b := string(k.kai.call("GET", "/api/v1/people/4901", "", 200))
	sameNames(t, "kid credits", namesIn(t, b, "movies"), "Gentle Meadow")
	if !strings.Contains(b, `"total_results":1`) {
		t.Fatalf("credit count: %s", b)
	}
	b = string(k.kai.call("GET", "/api/v1/people/search?q=ann", "", 200))
	if !strings.Contains(b, "Ann Actor") || !strings.Contains(b, "Gentle Meadow") || strings.Contains(b, "Sunny Pond") || mentionsBlocked(b) != "" {
		t.Fatalf("kid people search: %s", b)
	}
	if code, _ := k.kai.get("/api/v1/people/4901?role=actor"); code != 200 {
		t.Fatalf("role filter: %d", code)
	}
	if code, body := k.kai.get("/api/v1/collections"); code != 200 || !strings.Contains(body, "Shelf One") || !strings.Contains(body, `"owned_titles":1`) || strings.Contains(body, "Verdict Saga") {
		t.Fatalf("kid collections: %d %s", code, body)
	}
	b = string(k.kai.call("GET", "/api/v1/collections/1", "", 200))
	sameNames(t, "kid collection", namesIn(t, b, "titles"), "Gentle Meadow")
	if !strings.Contains(b, `"owned_titles":1`) {
		t.Fatalf("kid collection counts blocked titles: %s", b)
	}
	if code, _ := k.kai.get("/api/v1/collections/2"); code != 404 {
		t.Fatalf("collection with only blocked titles: %d", code)
	}
}

// TestKidCannotReachBlockedTitleAnywhere walks every GET route a member
// who may ask can reach, calls it with each blocked title's id (and the
// plain routes with a query that would find them) and checks that no 2xx
// answer names or numbers a blocked title.
func TestKidCannotReachBlockedTitleAnywhere(t *testing.T) {
	t.Parallel()
	k := newKidRig(t)
	queries := map[string][]string{
		"/api/v1/titles":          {"", "?q=Iron", "?q=Cold", "?q=Loud"},
		"/api/v1/titles/lookup":   {"?term=any", "?term=tmdb:4204", "?term=tmdb:4211"},
		"/api/v1/shows/lookup":    {"?term=any", "?term=tmdb:4221", "?term=tmdb:4231"},
		"/api/v1/artists/lookup":  {"?term=loud"},
		"/api/v1/metadata/search": {"?q=any"},
		"/api/v1/people/search":   {"?q=ann"},
		"/api/v1/discover":        {"?row=trending", "?row=popular", "?row=upcoming", "?row=genre&g=Drama", "?row=similar&id=9102", "?row=similar&id=9104", fmt.Sprintf("?row=similar&id=%d", k.ids["Gentle Meadow"])},
		"/api/v1/asks":            {"", "?mine=1"},
		"/api/v1/collections":     {""},
	}
	ok := map[string]bool{}
	checked := 0
	for pattern, need := range accounts.RoutePerms {
		method, path, _ := strings.Cut(pattern, " ")
		if method != "GET" || !strings.HasPrefix(path, "/api/v1/") {
			continue
		}
		if need != webserver.PermAsk && string(need) != "signed_in" && string(need) != "self" {
			continue
		}
		var urls []string
		switch {
		case strings.Contains(path, "{kind}/{tmdb_id}"):
			for _, p := range []string{"movie/4203", "movie/4204", "movie/4205", "movie/4211", "movie/4212", "show/4221", "show/4231"} {
				urls = append(urls, strings.Replace(path, "{kind}/{tmdb_id}", p, 1))
			}
		case strings.HasPrefix(path, "/api/v1/people/{id}"):
			urls = append(urls, strings.Replace(path, "{id}", "4901", 1))
		case strings.Contains(path, "{id}"):
			for _, id := range blockedIDs {
				urls = append(urls, strings.Replace(path, "{id}", fmt.Sprint(id), 1))
			}
		case strings.Contains(path, "{"):
			urls = append(urls, regexp.MustCompile(`\{[a-z_]+\}`).ReplaceAllString(path, "x"))
		default:
			qs := queries[path]
			if qs == nil {
				qs = []string{""}
			}
			for _, q := range qs {
				urls = append(urls, path+q)
			}
		}
		for _, u := range urls {
			// The walk covers every route, more lookups than the Kid's minute
			// limit allows (kidlookup_test checks that limit); start each fresh.
			k.api.discover.ResetHits()
			code, body := k.kai.get(u)
			if code < 200 || code > 299 {
				continue
			}
			checked++
			ok[pattern] = true
			if hit := mentionsBlocked(body); hit != "" {
				t.Errorf("%s (%s) shows %s: %.300s", u, pattern, hit, body)
			}
		}
	}
	for _, must := range []string{"GET /api/v1/titles", "GET /api/v1/discover", "GET /api/v1/metadata/search",
		"GET /api/v1/titles/lookup", "GET /api/v1/shows/lookup", "GET /api/v1/artists/lookup", "GET /api/v1/people/search",
		"GET /api/v1/people/{id}", "GET /api/v1/collections", "GET /api/v1/asks"} {
		if !ok[must] {
			t.Errorf("%s never answered 2xx, so it was not checked", must)
		}
	}
	if checked < 20 {
		t.Fatalf("only %d answers checked", checked)
	}
}

// slowDetails is a Discoverer whose details take a while, counting how
// many run at once.
type slowDetails struct{ now, peak, calls atomic.Int32 }

func (s *slowDetails) Search(context.Context, string, int) ([]metadata.Movie, error) { return nil, nil }
func (s *slowDetails) Movie(context.Context, string) (metadata.Movie, error) {
	return metadata.Movie{}, metadata.ErrNotFound
}
func (s *slowDetails) Discover(context.Context, string, string, string, string) ([]metadata.DiscoverItem, error) {
	return nil, nil
}
func (s *slowDetails) Details(_ context.Context, kind, id string) (metadata.Details, error) {
	s.calls.Add(1)
	n := s.now.Add(1)
	for p := s.peak.Load(); n > p && !s.peak.CompareAndSwap(p, n); p = s.peak.Load() {
	}
	time.Sleep(30 * time.Millisecond)
	s.now.Add(-1)
	rating := "G"
	if kind == "show" {
		rating = "TV-Y"
	}
	return metadata.Details{DiscoverItem: metadata.DiscoverItem{TMDBID: id, Kind: kind}, Rating: rating}, nil
}

func TestKidRatingLookupsAtMostFour(t *testing.T) {
	t.Parallel()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	slow := &slowDetails{}
	a := buildHandlers(services{Store: st, Metadata: slow, Version: "test"})
	var refs []ratingRef
	for i := range 12 {
		refs = append(refs, ratingRef{Kind: "movie", TMDBID: fmt.Sprint(7000 + i)})
	}
	got := a.kidAllowed(context.Background(), "PG", refs, 0)
	if len(got) != 12 || slices.Contains(got, false) {
		t.Fatalf("allowed %v", got)
	}
	if p := slow.peak.Load(); p > 4 || p < 2 {
		t.Fatalf("%d details lookups ran at once, want 2 to 4", p)
	}
	// The answers are kept in the details cache: no second lookup.
	a.kidAllowed(context.Background(), "PG", refs, 0)
	if c := slow.calls.Load(); c != 12 {
		t.Fatalf("%d details lookups for 12 titles asked twice", c)
	}
	// Stop early once enough are allowed.
	more := []ratingRef{{Kind: "show", TMDBID: "8001"}, {Kind: "show", TMDBID: "8002"}, {Kind: "show", TMDBID: "8003"}, {Kind: "show", TMDBID: "8004"},
		{Kind: "show", TMDBID: "8005"}, {Kind: "show", TMDBID: "8006"}, {Kind: "show", TMDBID: "8007"}, {Kind: "show", TMDBID: "8008"}}
	if got := a.kidAllowed(context.Background(), "PG", more, 2); !got[0] || !got[1] || got[7] {
		t.Fatalf("stop early: %v", got)
	}
	if c := slow.calls.Load(); c > 12+4 {
		t.Fatalf("%d lookups after asking for 2 of 8", c-12)
	}
}
