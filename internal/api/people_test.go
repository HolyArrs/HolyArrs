package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
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

func peopleRig(t *testing.T, p metadata.Provider) (*httptest.Server, *store.Store) {
	t.Helper()
	s, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	a := New(Server{Store: s, Metadata: p, Version: "test"})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	return srv, s
}

func TestPeopleSearchDisambiguatesAndProviderFailure(t *testing.T) {
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
	p := &fakePeople{Static: metadata.Lab()}
	srv, s := peopleRig(t, p)
	year := 2025
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (1, 't', '{}', 't', '{}', 'owner', 'owner', 'x')`); err != nil {
			return err
		}
		title := domain.Title{Kind: "movie", Name: "Alpha", SortName: "alpha", Year: &year, SourceIDs: map[string]string{"tmdb": "101"}, MetadataSource: "tmdb", NewPartsPolicy: "all", WhyAdded: "by_hand", State: "active", ReleaseGate: "released", GateDriver: "none", Status: "released"}
		if _, err := store.Insert(tx, "titles", &title, domain.System); err != nil {
			return err
		}
		never := domain.NeverAdd{Kind: "movie", SourceIDs: map[string]string{"tmdb": "102"}, Name: "Beta", Reason: "test", CreatedAt: time.Now()}
		if _, err := store.Insert(tx, "never_add", &never, domain.System); err != nil {
			return err
		}
		ask := domain.Ask{Kind: "movie", TMDBID: "103", Name: "Gamma", State: domain.AskAsked, AskedBy: 1, AskedAt: time.Now(), SourceIDs: map[string]string{"tmdb": "103"}}
		if _, err := store.Insert(tx, "asks", &ask, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, 1, ?, 1)", ask.ID, store.FormatTime(time.Now()))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	b := string(do(t, "GET", srv.URL+"/api/v1/people/11?role=all&page=1&page_size=2", "", 200))
	if !strings.Contains(b, `"title_id":1`) || !strings.Contains(b, `"never_add":true`) || !strings.Contains(b, `"complete":false`) || !strings.Contains(b, `"total_pages":2`) {
		t.Fatalf("filmography %s", b)
	}
	b = string(do(t, "GET", srv.URL+"/api/v1/people/11?role=all&page=2&page_size=2", "", 200))
	if !strings.Contains(b, `"state":"asked"`) {
		t.Fatalf("second filmography page %s", b)
	}
	b = string(do(t, "GET", srv.URL+"/api/v1/people/11?role=director&page=1&page_size=10", "", 200))
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
	p := &fakePeople{Static: metadata.Lab()}
	srv, s := peopleRig(t, p)
	year := 2025
	var title domain.Title
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		title = domain.Title{Kind: "movie", Name: "Alpha", SortName: "alpha", Year: &year, SourceIDs: map[string]string{"tmdb": "101"}, MetadataSource: "tmdb", NewPartsPolicy: "all", WhyAdded: "by_hand", State: "active", ReleaseGate: "released", GateDriver: "none", Status: "released"}
		_, err := store.Insert(tx, "titles", &title, domain.System)
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

func TestPeopleErrorsStayBounded(t *testing.T) {
	p := &fakePeople{Static: metadata.Lab(), fail: true}
	srv, _ := peopleRig(t, p)
	b := string(do(t, "GET", srv.URL+"/api/v1/people/11", "", 503))
	if strings.Contains(b, "test-read-value") || !strings.Contains(b, "people results are unavailable") {
		t.Fatalf("error %s", b)
	}
	_ = errors.Is(metadata.ErrNoAccess, metadata.ErrNoAccess)
}
