package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

const movieDetailsBody = `{"id":5,"title":"Five","release_date":"2024-03-01","runtime":121,
"poster_path":"/p.jpg","backdrop_path":"/b.jpg","genres":[{"id":1,"name":"Drama"},{"id":2,"name":"Adventure"}],
"release_dates":{"results":[
 {"iso_3166_1":"GB","release_dates":[{"release_date":"2024-03-01T00:00:00.000Z","type":3,"certification":"15"}]},
 {"iso_3166_1":"US","release_dates":[{"release_date":"2024-02-01T00:00:00.000Z","type":1,"certification":""},{"release_date":"2024-03-01T00:00:00.000Z","type":3,"certification":"PG-13"}]}]},
"images":{"logos":[{"file_path":"/de.png","iso_639_1":"de"},{"file_path":"/en.svg","iso_639_1":"en"},{"file_path":"/en.png","iso_639_1":"en"},{"file_path":"/none.png","iso_639_1":null}]},
"credits":{"cast":[{"name":"A1"},{"name":"A2"},{"name":"A3"},{"name":"A4"},{"name":"A5"},{"name":"A6"},{"name":"A7"},{"name":"A8"},{"name":"A9"},{"name":"A10"},{"name":"A11"}],
 "crew":[{"name":"D1","job":"Director"},{"name":"W1","job":"Writer"},{"name":"A1","job":"Director"}]}}`

func TestTMDBMovieDetailsExtras(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("append_to_response") != "release_dates,images,credits,translations,alternative_titles" || q.Get("include_image_language") != "en,null" {
			t.Errorf("query %v", q)
		}
		_, _ = w.Write([]byte(movieDetailsBody))
	}))
	defer srv.Close()
	m, err := newTMDB(srv, fakeCred).Movie(context.Background(), "tmdb:5")
	if err != nil {
		t.Fatal(err)
	}
	if m.Backdrop != ImageBase+"/w1280/b.jpg" {
		t.Errorf("backdrop %q", m.Backdrop)
	}
	if m.Logo != ImageBase+"/w500/en.png" {
		t.Errorf("logo %q", m.Logo)
	}
	if !reflect.DeepEqual(m.Genres, []string{"Drama", "Adventure"}) {
		t.Errorf("genres %v", m.Genres)
	}
	if m.Rating != "PG-13" || m.Runtime != 121 {
		t.Errorf("rating %q runtime %d", m.Rating, m.Runtime)
	}
	want := []string{"A1", "A2", "A3", "A4", "A5", "A6", "A7", "A8", "A9", "A10", "D1"}
	if !reflect.DeepEqual(m.People, want) {
		t.Errorf("people %v", m.People)
	}
}

func TestTMDBMovieDetailsExtrasMissing(t *testing.T) {
	cases := []struct{ name, body string }{
		{"bare", `{"id":5,"title":"Five"}`},
		{"only foreign logo", `{"id":5,"title":"Five","images":{"logos":[{"file_path":"/de.png","iso_639_1":"de"}]}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(c.body)) }))
			defer srv.Close()
			m, err := newTMDB(srv, fakeCred).Movie(context.Background(), "tmdb:5")
			if err != nil {
				t.Fatal(err)
			}
			if m.Backdrop != "" || m.Logo != "" || m.Rating != "" || len(m.Genres) != 0 || len(m.People) != 0 {
				t.Fatalf("got %+v", m)
			}
		})
	}
}

func TestTMDBShowDetailsExtras(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/season/") {
			_, _ = w.Write([]byte(`{"episodes":[]}`))
			return
		}
		q := r.URL.Query()
		if q.Get("append_to_response") != "external_ids,images,credits,content_ratings,translations,alternative_titles" || q.Get("include_image_language") != "en,null" {
			t.Errorf("query %v", q)
		}
		_, _ = w.Write([]byte(`{"id":9,"name":"Nine","backdrop_path":"/sb.jpg","genres":[{"name":"Comedy"}],
			"content_ratings":{"results":[{"iso_3166_1":"DE","rating":"12"},{"iso_3166_1":"US","rating":"TV-14"}]},
			"images":{"logos":[{"file_path":"/x.png","iso_639_1":null}]},
			"credits":{"cast":[{"name":"C1"}],"crew":[{"name":"Dir","job":"Director"}]},
			"seasons":[{"season_number":1,"name":"S1"}]}`))
	}))
	defer srv.Close()
	sh, err := newTMDB(srv, fakeCred).Show(context.Background(), "tmdb:9")
	if err != nil {
		t.Fatal(err)
	}
	if sh.Backdrop != ImageBase+"/w1280/sb.jpg" || sh.Logo != ImageBase+"/w500/x.png" || sh.Rating != "TV-14" {
		t.Errorf("show %+v", sh)
	}
	if !reflect.DeepEqual(sh.Genres, []string{"Comedy"}) || !reflect.DeepEqual(sh.People, []string{"C1", "Dir"}) {
		t.Errorf("genres %v people %v", sh.Genres, sh.People)
	}
}
