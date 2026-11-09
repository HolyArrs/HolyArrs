package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// infoTMDB answers a movie (70) and a show (80) with the facts a title page shows under its story.
func infoTMDB(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/genre/"):
			_, _ = w.Write([]byte(`{"genres":[]}`))
		case strings.HasSuffix(r.URL.Path, "/movie/70"):
			_, _ = w.Write([]byte(`{"id":70,"title":"Le Film","original_title":"Le Film Original","release_date":"2014-10-10",
				"status":"Released","tagline":"  Not quite my tempo. ","budget":3300000,"revenue":49000000,
				"production_companies":[{"name":"Bold Films"},{"name":"Blumhouse"},{"name":"Right of Way"},{"name":"Fourth"}],
				"release_dates":{"results":[
					{"iso_3166_1":"FR","release_dates":[{"release_date":"2014-09-01T00:00:00.000Z","type":3}]},
					{"iso_3166_1":"US","release_dates":[{"release_date":"2014-10-10T00:00:00.000Z","type":3},{"release_date":"2014-10-01T00:00:00.000Z","type":2},{"release_date":"2015-01-13T00:00:00.000Z","type":4},{"release_date":"2015-02-17T00:00:00.000Z","type":5}]}]},
				"keywords":{"keywords":[{"name":"drummer"},{"name":"music school"}]}}`))
		case strings.HasSuffix(r.URL.Path, "/tv/80"):
			_, _ = w.Write([]byte(`{"id":80,"name":"Eighty","original_name":"Eighty","status":"Returning Series","type":"Miniseries","episode_run_time":[55,60],"spoken_languages":[{"english_name":"English"},{"english_name":"Dothraki"}],
				"first_air_date":"2011-04-17","last_air_date":"2026-10-20",
				"next_episode_to_air":{"season_number":3,"episode_number":4,"name":" The Return ","air_date":"2026-11-02"},
				"production_companies":[{"name":"HBO"}],"keywords":{"results":[{"name":"dragon"}]}}`))
		case strings.HasSuffix(r.URL.Path, "/movie/71"):
			_, _ = w.Write([]byte(`{"id":71,"title":"Elsewhere","release_dates":{"results":[{"iso_3166_1":"JP","release_dates":[{"release_date":"2020-05-05T00:00:00.000Z","type":3}]}]}}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestTMDBDetailsInfo(t *testing.T) {
	p := newTMDB(infoTMDB(t), fakeCred)
	ctx := context.Background()

	m, err := p.Details(ctx, "movie", "70")
	if err != nil {
		t.Fatal(err)
	}
	if m.Status != "Released" || m.Tagline != "Not quite my tempo." || m.OriginalTitle != "Le Film Original" || m.Budget != 3300000 || m.Revenue != 49000000 {
		t.Fatalf("movie facts %+v", m)
	}
	if !reflect.DeepEqual(m.Studios, []string{"Bold Films", "Blumhouse", "Right of Way"}) {
		t.Fatalf("studios: the first three, %v", m.Studios)
	}
	// US dates win over other countries; the earliest of theatrical types 2 and 3 counts.
	if m.Dates == nil || *m.Dates != (DetailDates{Theaters: "2014-10-01", Digital: "2015-01-13", Physical: "2015-02-17"}) {
		t.Fatalf("movie dates %+v", m.Dates)
	}
	if !reflect.DeepEqual(m.Keywords, []string{"drummer", "music school"}) || m.NextEpisode != nil {
		t.Fatalf("keywords %v next %v", m.Keywords, m.NextEpisode)
	}

	s, err := p.Details(ctx, "show", "80")
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != "Returning Series" || s.OriginalTitle != "" || !reflect.DeepEqual(s.Studios, []string{"HBO"}) {
		t.Fatalf("show facts %+v", s)
	}
	if s.ShowType != "Miniseries" || s.EpisodeRuntime != 55 || !reflect.DeepEqual(s.SpokenLanguages, []string{"English", "Dothraki"}) {
		t.Fatalf("show kind %q runtime %d spoken %v", s.ShowType, s.EpisodeRuntime, s.SpokenLanguages)
	}
	if s.Dates == nil || *s.Dates != (DetailDates{FirstAir: "2011-04-17", LastAir: "2026-10-20"}) {
		t.Fatalf("show dates %+v", s.Dates)
	}
	if s.NextEpisode == nil || *s.NextEpisode != (DetailNextEpisode{Season: 3, Episode: 4, Name: "The Return", AirDate: "2026-11-02"}) {
		t.Fatalf("next episode %+v", s.NextEpisode)
	}

	// No US entry: the earliest anywhere.
	e, err := p.Details(ctx, "movie", "71")
	if err != nil {
		t.Fatal(err)
	}
	if e.Dates == nil || e.Dates.Theaters != "2020-05-05" || e.Dates.Digital != "" {
		t.Fatalf("fallback dates %+v", e.Dates)
	}
}
