package metadata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func peopleTMDB(t *testing.T) *TMDB {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeCred {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/3/search/person":
			if q := r.URL.Query(); q.Get("query") != "alex" || q.Get("include_adult") != "false" || q.Get("page") != "2" {
				t.Fatalf("person search query %v", q)
			}
			_, _ = w.Write([]byte(`{"page":2,"total_pages":4,"total_results":62,"results":[
				{"id":11,"name":"Alex Kim","known_for_department":"Acting","profile_path":"/alex.jpg","known_for":[{"id":101,"title":"North Wind","release_date":"2022-01-02"}]},
				{"id":12,"name":"Alex Kim","known_for_department":"Directing","known_for":[{"id":102,"title":"South Wind","release_date":"2020-05-01"}]}
			]}`))
		case "/3/person/11":
			_, _ = w.Write([]byte(`{"id":11,"name":"Alex Kim","known_for_department":"Acting","biography":"A short biography.","birthday":"1980-02-03","deathday":null,"place_of_birth":"Halifax","profile_path":"/alex.jpg"}`))
		case "/3/person/11/movie_credits":
			_, _ = w.Write([]byte(`{
				"cast":[
					{"id":101,"title":"North Wind","release_date":"2022-01-02","character":"Lee"},
					{"id":101,"title":"North Wind","release_date":"2022-01-02","character":"Narrator"},
					{"id":103,"title":"Undated","release_date":"","character":"Sam"}],
				"crew":[
					{"id":101,"title":"North Wind","release_date":"2022-01-02","job":"Director"},
					{"id":104,"title":"Other Job","release_date":"2023-01-01","job":"Producer"},
					{"id":102,"title":"South Wind","release_date":"2020-05-01","job":"Director"}]
			}`))
		case "/3/person/11/tv_credits":
			_, _ = w.Write([]byte(`{"cast":[{"id":201,"name":"Ice Road"},{"id":201,"name":"Ice Road"},{"id":202,"name":"Harbor"}],
				"crew":[{"id":203,"name":"Lantern","job":"Director"},{"id":204,"name":"Paper","job":"Producer"}]}`))
		case "/3/movie/101/credits":
			_, _ = w.Write([]byte(`{
				"cast":[{"id":11,"name":"Alex Kim","known_for_department":"Acting","character":"Lee","profile_path":"/alex.jpg"},{"id":11,"name":"Alex Kim","character":"Narrator"}],
				"crew":[{"id":11,"name":"Alex Kim","job":"Director"},{"id":12,"name":"Alex Kim","job":"Producer"},{"id":13,"name":"Bea Ray","job":"Director"}]
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return &TMDB{BaseURL: srv.URL + "/3", Access: func(context.Context) (string, error) { return fakeCred, nil }, Gap: time.Millisecond, Images: srv.URL + "/images"}
}

func TestTMDBPeopleSearchAndDetails(t *testing.T) {
	p := peopleTMDB(t)
	var _ PeopleProvider = p
	page, err := p.SearchPeople(context.Background(), "alex", 2)
	if err != nil {
		t.Fatal(err)
	}
	if page.Page != 2 || page.TotalPages != 4 || page.TotalResults != 62 || len(page.People) != 2 {
		t.Fatalf("page %+v", page)
	}
	if page.People[0].ID != "11" || page.People[0].Name != "Alex Kim" || page.People[0].Department != "Acting" || page.People[0].Profile != p.Images+"/w185/alex.jpg" || page.People[0].KnownFor[0].IDs["tmdb"] != "101" {
		t.Fatalf("first result %+v", page.People[0])
	}
	if page.People[1].Department != "Directing" || page.People[1].KnownFor[0].Name != "South Wind" {
		t.Fatalf("same-name disambiguation %+v", page.People[1])
	}
	person, err := p.Person(context.Background(), "tmdb:11")
	if err != nil || person.Birthday != "1980-02-03" || person.Deathday != "" || person.Birthplace != "Halifax" {
		t.Fatalf("person %+v %v", person, err)
	}
	for _, bad := range []string{"11", "tmdb:0", "tmdb:no", "tmdb:+11", "tmdb:1234567890123", "imdb:11"} {
		if _, err := p.Person(context.Background(), bad); err == nil {
			t.Fatalf("bad id %q accepted", bad)
		}
	}
}

func TestTMDBPersonFilmographyDeduplicatesAndRetainsRoles(t *testing.T) {
	p := peopleTMDB(t)
	credits, err := p.PersonMovies(context.Background(), "tmdb:11")
	if err != nil {
		t.Fatal(err)
	}
	if credits.Complete || credits.ProviderPages != 1 || len(credits.Movies) != 3 {
		t.Fatalf("credits %+v", credits)
	}
	if got := []string{credits.Movies[0].Movie.Name, credits.Movies[1].Movie.Name, credits.Movies[2].Movie.Name}; !reflect.DeepEqual(got, []string{"North Wind", "South Wind", "Undated"}) {
		t.Fatalf("order %v", got)
	}
	first := credits.Movies[0]
	if !reflect.DeepEqual(first.Roles, []string{"Actor", "Director"}) || !reflect.DeepEqual(first.Characters, []string{"Lee", "Narrator"}) {
		t.Fatalf("merged credit %+v", first)
	}
	for _, c := range credits.Movies {
		if c.Movie.Name == "Other Job" {
			t.Fatal("non-director crew credit included")
		}
	}
	b, _ := json.Marshal(credits)
	if string(b) == "" {
		t.Fatal("credits did not encode")
	}
}

func TestTMDBMoviePeopleDeduplicatesAndKeepsDirectorsOnly(t *testing.T) {
	p := peopleTMDB(t)
	got, err := p.MoviePeople(context.Background(), "tmdb:101")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Person.ID != "11" || got[1].Person.ID != "13" {
		t.Fatalf("people %+v", got)
	}
	if !reflect.DeepEqual(got[0].Roles, []string{"Actor", "Director"}) || !reflect.DeepEqual(got[0].Characters, []string{"Lee", "Narrator"}) || !reflect.DeepEqual(got[1].Roles, []string{"Director"}) {
		t.Fatalf("roles %+v", got)
	}
}

func TestTMDBPersonShowCount(t *testing.T) {
	p := peopleTMDB(t)
	var _ PersonShowCounter = p
	// Each show once (repeat cast rows are episodes); directing counts, producing does not.
	if n, err := p.PersonShowCount(context.Background(), "tmdb:11"); err != nil || n != 3 {
		t.Fatalf("shows %d %v", n, err)
	}
}
