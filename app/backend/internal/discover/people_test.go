package discover

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

type fakePeople struct {
	*metadata.Static
	fail bool
}

func (f *fakePeople) SearchPeople(_ context.Context, query string, page int) (metadata.PersonPage, error) {
	if f.fail {
		return metadata.PersonPage{}, metadata.ErrNoAccess
	}
	return metadata.PersonPage{People: []metadata.Person{
		{ID: "11", Name: "Alex Kim", Department: "Acting", KnownFor: []metadata.Movie{{Name: "One", IDs: map[string]string{"tmdb": "101"}}}},
		{ID: "12", Name: "Alex Kim", Department: "Directing", KnownFor: []metadata.Movie{{Name: "Two", IDs: map[string]string{"tmdb": "102"}}}},
	}, Page: page, TotalPages: 3, TotalResults: 42}, nil
}

func (f *fakePeople) Person(_ context.Context, id string) (metadata.Person, error) {
	if f.fail {
		return metadata.Person{}, metadata.ErrNoAccess
	}
	if id != "tmdb:11" {
		return metadata.Person{}, metadata.ErrNotFound
	}
	return metadata.Person{ID: "11", Name: "Alex Kim", Department: "Acting", KnownFor: []metadata.Movie{}}, nil
}

func (f *fakePeople) PersonMovies(_ context.Context, id string) (metadata.PersonMoviePage, error) {
	if f.fail {
		return metadata.PersonMoviePage{}, metadata.ErrNoAccess
	}
	if id != "tmdb:11" {
		return metadata.PersonMoviePage{}, metadata.ErrNotFound
	}
	movies := []metadata.PersonMovieCredit{
		{Movie: metadata.Movie{Name: "Alpha", Year: 2025, IDs: map[string]string{"tmdb": "101"}}, Roles: []string{"Actor", "Director"}, Characters: []string{"Lee"}},
		{Movie: metadata.Movie{Name: "Beta", Year: 2024, IDs: map[string]string{"tmdb": "102"}}, Roles: []string{"Actor"}},
		{Movie: metadata.Movie{Name: "Gamma", Year: 2023, IDs: map[string]string{"tmdb": "103"}}, Roles: []string{"Director"}},
	}
	return metadata.PersonMoviePage{Movies: movies, Complete: false, ProviderPages: 1}, nil
}

func (f *fakePeople) MoviePeople(_ context.Context, id string) ([]metadata.MoviePersonCredit, error) {
	if f.fail {
		return nil, metadata.ErrNoAccess
	}
	if id != "tmdb:101" {
		return nil, metadata.ErrNotFound
	}
	return []metadata.MoviePersonCredit{{Person: metadata.Person{ID: "11", Name: "Alex Kim", KnownFor: []metadata.Movie{}}, Roles: []string{"Actor", "Director"}, Characters: []string{"Lee"}}}, nil
}

func (f *fakePeople) ShowPeople(_ context.Context, id string) ([]metadata.MoviePersonCredit, error) {
	if id != "tmdb:201" {
		return nil, metadata.ErrNotFound
	}
	return []metadata.MoviePersonCredit{{Person: metadata.Person{ID: "12", Name: "Sam Lane", KnownFor: []metadata.Movie{}}, Roles: []string{"Actor"}, Characters: []string{"Jo"}}}, nil
}

func (f *fakePeople) PopularPeople(_ context.Context, page int) (metadata.PersonPage, error) {
	if f.fail {
		return metadata.PersonPage{}, metadata.ErrNoAccess
	}
	if page > 1 {
		return metadata.PersonPage{Page: page, TotalPages: 8}, nil
	}
	return metadata.PersonPage{People: []metadata.Person{
		{ID: "30", Name: "Spike Lowvote", Department: "Acting", Profile: "/p/s.jpg", Popularity: 400, KnownForVotes: []int{1200, 40}, KnownFor: []metadata.Movie{}},
		{ID: "31", Name: "No Picture", Department: "Acting", Popularity: 90, KnownForVotes: []int{30000}, KnownFor: []metadata.Movie{}},
		{ID: "32", Name: "Never Known", Department: "Acting", Profile: "/p/n.jpg", Popularity: 300, KnownForVotes: []int{900}, KnownFor: []metadata.Movie{}},
		{ID: "33", Name: "Adult One", Department: "Acting", Profile: "/p/a.jpg", Adult: true, KnownForVotes: []int{9000}, KnownFor: []metadata.Movie{}},
		{ID: "21", Name: "Pat Star", Department: "Acting", Profile: "/p/p.jpg", Popularity: 20, KnownForVotes: []int{25000, 18000, 9000}, KnownFor: []metadata.Movie{}},
	}, Page: page, TotalPages: 8, TotalResults: 160}, nil
}

// peopleRig serves the discover routes on a fresh database with the
// given provider, wired the way api wires them for a signed-out owner.
func peopleRig(t *testing.T, p metadata.Provider) (*httptest.Server, *database.Store) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	m := http.NewServeMux()
	testHandlers(s, p).PeopleRoutes(m)
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	return srv, s
}

// testHandlers are discover Handlers on s and p with the program's wiring for a
// signed-out owner (user 1, no age limit, every part on).
func testHandlers(s *database.Store, p metadata.Provider) *Handlers {
	mf := &mediafiles.Handlers{Deps: mediafiles.Deps{Store: s, Metadata: p, KidMax: func(*http.Request) string { return "" }}}
	e := &Explore{}
	e.Store = func() *database.Store { return s }
	e.Metadata = func() metadata.Provider { return p }
	e.KnownRating, e.LookupRating = mf.KnownRating, mf.LookupRating
	h := &Handlers{Deps: Deps{Store: s, Metadata: p, Cache: &Cache{}, Explore: e,
		KidMax: func(*http.Request) string { return "" }, UserID: func(*http.Request) int64 { return 1 },
		Actor:  func(*http.Request) domain.Actor { return domain.System },
		PartOn: func(pt parts.Part) bool { return parts.PartOn(nil, pt) }, TitleAllowed: mf.TitleAllowed,
		LibraryRating: mf.LibraryRating, KidItems: mf.KidItems, KidKnownFor: mf.KidKnownFor, LibraryMovieAllowed: mf.LibraryMovieAllowed,
		KidMovies: mf.KidMovies, AllowedTitleIDs: mf.AllowedTitleIDs,
		OpenAsk: func(ctx context.Context, id string, me int64) (*requests.AskBrief, error) {
			return requests.OpenMovieAsk(ctx, s.Read(), id, me, false)
		},
		OpenShowAsk: func(ctx context.Context, ids map[string]string, me int64) (*requests.AskBrief, error) {
			return requests.OpenShowAsk(ctx, s.Read(), ids, me)
		},
		LiveSource: func(ctx context.Context) Lister {
			var l metadata.ExploreLister
			if x, ok := metadata.ExploreListerFor(ctx, p); ok {
				l = x
			}
			return ExploreSource{E: e, Lister: l, Calls: &engine.Calls{}}
		}}}
	return h
}

func do(t *testing.T, method, url, body string, want int) []byte {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		t.Fatalf("%s %s: %d, want %d: %s", method, url, resp.StatusCode, want, b)
	}
	return b
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestPeopleSearchDisambiguatesAndProviderFailure(t *testing.T) {
	t.Parallel()
	p := &fakePeople{Static: metadata.Lab()}
	srv, _ := peopleRig(t, p)
	b := string(do(t, "GET", srv.URL+"/api/v1/people/search?q=alex&page=2", "", 200))
	if strings.Count(b, `"name":"Alex Kim"`) != 2 || !strings.Contains(b, `"department":"Acting"`) || !strings.Contains(b, `"department":"Directing"`) || !strings.Contains(b, `"total_pages":3`) {
		t.Fatalf("search %s", b)
	}
	do(t, "GET", srv.URL+"/api/v1/people/search?q=&page=1", "", 400)
	do(t, "GET", srv.URL+"/api/v1/people/search?q=alex&page=501", "", 400)
	p.fail = true
	b = string(do(t, "GET", srv.URL+"/api/v1/people/search?q=alex", "", 503))
	if !strings.Contains(b, "people results are unavailable") {
		t.Fatalf("failure %s", b)
	}
	srv2, _ := peopleRig(t, metadata.Lab())
	do(t, "GET", srv2.URL+"/api/v1/people/search?q=alex", "", 503)
}

func TestPersonFilmographyJoinsOwnedNeverAddAndAskWithStablePages(t *testing.T) {
	t.Parallel()
	p := &fakePeople{Static: metadata.Lab()}
	srv, s := peopleRig(t, p)
	year := 2025
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (1, 't', '{}', 't', '{}', 'owner', 'owner', 'x')`); err != nil {
			return err
		}
		title := mediafiles.Title{Kind: "movie", Name: "Alpha", SortName: "alpha", Year: &year, SourceIDs: map[string]string{"tmdb": "101"}, MetadataSource: "tmdb", NewPartsPolicy: "all", WhyAdded: "by_hand", State: "active", ReleaseGate: "released", GateDriver: "none", Status: "released"}
		if _, err := database.Insert(tx, "titles", &title, domain.System); err != nil {
			return err
		}
		never := mediafiles.NeverAdd{Kind: "movie", SourceIDs: map[string]string{"tmdb": "102"}, Name: "Beta", Reason: "test", CreatedAt: time.Now()}
		if _, err := database.Insert(tx, "never_add", &never, domain.System); err != nil {
			return err
		}
		ask := domain.Ask{Kind: "movie", TMDBID: "103", Name: "Gamma", State: domain.AskAsked, AskedBy: 1, AskedAt: time.Now(), SourceIDs: map[string]string{"tmdb": "103"}}
		if _, err := database.Insert(tx, "asks", &ask, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, 1, ?, 1)", ask.ID, database.FormatTime(time.Now()))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	b := string(do(t, "GET", srv.URL+"/api/v1/people/11?role=all&sort=newest&page=1&page_size=2", "", 200))
	if !strings.Contains(b, `"title_id":1`) || !strings.Contains(b, `"never_add":true`) || !strings.Contains(b, `"complete":false`) || !strings.Contains(b, `"total_pages":2`) {
		t.Fatalf("filmography %s", b)
	}
	b = string(do(t, "GET", srv.URL+"/api/v1/people/11?role=all&sort=newest&page=2&page_size=2", "", 200))
	if !strings.Contains(b, `"state":"asked"`) {
		t.Fatalf("second filmography page %s", b)
	}
	b = string(do(t, "GET", srv.URL+"/api/v1/people/11?role=director&sort=newest&page=1&page_size=10", "", 200))
	// Only the filmography page counts here; the personal rows repeat titles.
	b = b[strings.Index(b, `"movies":`):strings.Index(b, `"often_with":`)]
	if strings.Contains(b, `"name":"Beta"`) || strings.Count(b, `"name":"Alpha"`) != 1 || strings.Count(b, `"name":"Gamma"`) != 1 {
		t.Fatalf("director filter %s", b)
	}
	do(t, "GET", srv.URL+"/api/v1/people/11?role=writer", "", 400)
	before := s.WriteCount()
	b = string(do(t, "POST", srv.URL+"/api/v1/people/11/movies/review", `{"tmdb_ids":["103","102","103"]}`, 200))
	if strings.Count(b, `"tmdb":"103"`) != 1 || strings.Count(b, `"tmdb":"102"`) != 1 || !strings.Contains(b, `"never_add":true`) {
		t.Fatalf("review %s", b)
	}
	if after := s.WriteCount(); after != before {
		t.Fatalf("review wrote %d -> %d", before, after)
	}
	do(t, "POST", srv.URL+"/api/v1/people/11/movies/review", `{"tmdb_ids":["999"]}`, 409)
}

func TestMoviePeopleUsesStoredTMDBIdentity(t *testing.T) {
	t.Parallel()
	p := &fakePeople{Static: metadata.Lab()}
	srv, s := peopleRig(t, p)
	year := 2025
	var title mediafiles.Title
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		title = mediafiles.Title{Kind: "movie", Name: "Alpha", SortName: "alpha", Year: &year, SourceIDs: map[string]string{"tmdb": "101"}, MetadataSource: "tmdb", NewPartsPolicy: "all", WhyAdded: "by_hand", State: "active", ReleaseGate: "released", GateDriver: "none", Status: "released"}
		_, err := database.Insert(tx, "titles", &title, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	b := string(do(t, "GET", srv.URL+"/api/v1/titles/"+itoa(title.ID)+"/people", "", 200))
	if !strings.Contains(b, `"name":"Alex Kim"`) || !strings.Contains(b, `"Director"`) {
		t.Fatalf("credits %s", b)
	}
	do(t, "GET", srv.URL+"/api/v1/titles/999/people", "", 404)
}

func TestShowPeopleUsesStoredTMDBIdentity(t *testing.T) {
	t.Parallel()
	p := &fakePeople{Static: metadata.Lab()}
	srv, s := peopleRig(t, p)
	var title mediafiles.Title
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		title = mediafiles.Title{Kind: "show", Name: "Show", SortName: "show", SourceIDs: map[string]string{"tmdb": "201"}, MetadataSource: "tmdb", NewPartsPolicy: "all", WhyAdded: "by_hand", State: "active", ReleaseGate: "released", GateDriver: "none", Status: "running"}
		_, err := database.Insert(tx, "titles", &title, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	b := string(do(t, "GET", srv.URL+"/api/v1/titles/"+itoa(title.ID)+"/people", "", 200))
	if !strings.Contains(b, `"name":"Sam Lane"`) || !strings.Contains(b, `"Jo"`) {
		t.Fatalf("show credits %s", b)
	}
}

func TestPeopleErrorsStayBounded(t *testing.T) {
	t.Parallel()
	p := &fakePeople{Static: metadata.Lab(), fail: true}
	srv, _ := peopleRig(t, p)
	b := string(do(t, "GET", srv.URL+"/api/v1/people/11", "", 503))
	if strings.Contains(b, "test-read-value") || !strings.Contains(b, "people results are unavailable") {
		t.Fatalf("error %s", b)
	}
	_ = errors.Is(metadata.ErrNoAccess, metadata.ErrNoAccess)
}

// route: GET /api/v1/people/popular
func TestPopularPeople(t *testing.T) {
	t.Parallel()
	p := &fakePeople{Static: metadata.Lab()}
	srv, _ := peopleRig(t, p)
	b := string(do(t, "GET", srv.URL+"/api/v1/people/popular", "", 200))
	star, spike := strings.Index(b, "Pat Star"), strings.Index(b, "Spike Lowvote")
	if star < 0 || spike < star || strings.Contains(b, "No Picture") || strings.Contains(b, "Never Known") || strings.Contains(b, "Adult One") || !strings.Contains(b, `"total_pages":1`) {
		t.Fatalf("popular %s", b)
	}
	b = string(do(t, "GET", srv.URL+"/api/v1/people/popular?page=2", "", 200))
	if !strings.Contains(b, `"people":[]`) || !strings.Contains(b, `"page":2`) {
		t.Fatalf("popular page 2 %s", b)
	}
	do(t, "GET", srv.URL+"/api/v1/people/popular?page=0", "", 400)
	do(t, "GET", srv.URL+"/api/v1/people/popular?page=8", "", 200)
	do(t, "GET", srv.URL+"/api/v1/people/popular?page=9", "", 400)
	p.fail = true
	srv, _ = peopleRig(t, p)
	do(t, "GET", srv.URL+"/api/v1/people/popular", "", 503)
	srv2, _ := peopleRig(t, metadata.Lab())
	do(t, "GET", srv2.URL+"/api/v1/people/popular", "", 503)
}
