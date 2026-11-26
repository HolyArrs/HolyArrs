package metadata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

func omdbStandIn(t *testing.T, status int, body string) (*OMDb, *[]string) {
	t.Helper()
	outbound.Reset()
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.RawQuery)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &OMDb{Base: srv.URL, Key: func(context.Context) (string, error) { return "omdb-secret-fixture", nil },
		HTTP: &http.Client{Timeout: 5 * time.Second}, Gap: time.Millisecond}, &asked
}

func TestOMDbScores(t *testing.T) {
	o, asked := omdbStandIn(t, 200, `{"Title":"The Matrix","imdbRating":"8.7","imdbVotes":"2,123,456","Metascore":"73",
		"Ratings":[{"Source":"Internet Movie Database","Value":"8.7/10"},{"Source":"Rotten Tomatoes","Value":"83%"},{"Source":"Metacritic","Value":"73/100"}],"Response":"True"}`)
	sc, err := o.Scores(context.Background(), "tt0133093")
	if err != nil || *sc.IMDbRating != 8.7 || sc.IMDbVotes != 2123456 || *sc.RottenTomatoes != 83 || *sc.Metacritic != 73 {
		t.Fatalf("%+v %v", sc, err)
	}
	if len(*asked) != 1 || !strings.Contains((*asked)[0], "i=tt0133093") || !strings.Contains((*asked)[0], "apikey=omdb-secret-fixture") {
		t.Fatalf("asked %v", *asked)
	}
	// The answer is kept: asking again costs no request.
	if _, err := o.Scores(context.Background(), "tt0133093"); err != nil || len(*asked) != 1 {
		t.Fatalf("asked again: %v %v", *asked, err)
	}
	if _, err := o.Scores(context.Background(), "603"); !errors.Is(err, ErrNotFound) || len(*asked) != 1 {
		t.Fatalf("a TMDB id is not an IMDb id: %v", err)
	}
}

func TestOMDbScoresNotKnown(t *testing.T) {
	// A show often has no Rotten Tomatoes or Metacritic score; N/A is none.
	o, _ := omdbStandIn(t, 200, `{"imdbRating":"N/A","imdbVotes":"N/A","Metascore":"N/A","Ratings":[],"Response":"True"}`)
	sc, err := o.Scores(context.Background(), "tt0000001")
	if err != nil || sc.IMDbRating != nil || sc.IMDbVotes != 0 || sc.RottenTomatoes != nil || sc.Metacritic != nil {
		t.Fatalf("%+v %v", sc, err)
	}
	// The Metascore field stands in when Ratings lacks Metacritic.
	o, _ = omdbStandIn(t, 200, `{"imdbRating":"6.1","imdbVotes":"900","Metascore":"55","Response":"True"}`)
	if sc, err := o.Scores(context.Background(), "tt0000002"); err != nil || *sc.Metacritic != 55 || sc.RottenTomatoes != nil {
		t.Fatalf("%+v %v", sc, err)
	}
}

func TestOMDbScoresErrors(t *testing.T) {
	for _, c := range []struct {
		status int
		body   string
		want   error
	}{
		{401, `{"Response":"False","Error":"Request limit reached!"}`, ErrNoAccess},
		{401, `{"Response":"False","Error":"Invalid API key!"}`, ErrNoAccess},
		{200, `{"Response":"False","Error":"Request limit reached!"}`, ErrNoAccess},
		{200, `{"Response":"False","Error":"Incorrect IMDb ID."}`, ErrNotFound},
		{200, `{"Response":"False","Error":"Movie not found!"}`, ErrNotFound},
		{503, `down`, ErrServiceUnavailable},
		{200, `not json`, ErrServiceInvalid},
	} {
		o, _ := omdbStandIn(t, c.status, c.body)
		_, err := o.Scores(context.Background(), "tt0133093")
		if !errors.Is(err, c.want) {
			t.Errorf("%d %s: %v, want %v", c.status, c.body, err, c.want)
		}
		if err != nil && strings.Contains(err.Error(), "omdb-secret-fixture") {
			t.Errorf("the key is in the error: %v", err)
		}
	}
	none := &OMDb{Key: func(context.Context) (string, error) { return " ", nil }}
	if _, err := none.Scores(context.Background(), "tt0133093"); !errors.Is(err, ErrSourceOff) {
		t.Fatalf("no key: %v", err)
	}
}

func TestOMDbScoresStayInWhileLookupsAreOff(t *testing.T) {
	outbound.Reset()
	asked := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { asked++ }))
	t.Cleanup(srv.Close)
	m := NewModes(envOf(map[string]string{EnvMode: "off"}))
	o := &OMDb{Base: srv.URL, Key: func(context.Context) (string, error) { return "k", nil }, HTTP: m.Client(true, NotOff), Gap: time.Millisecond}
	if _, err := o.Scores(context.Background(), "tt0133093"); !errors.Is(err, ErrSourceOff) || asked != 0 {
		t.Fatalf("off: %v, asked %d", err, asked)
	}
}

func TestOMDbIsATestableKeyAndAnAddress(t *testing.T) {
	all := func(string) bool { return true }
	none := func(string) bool { return false }
	if SourceOn("omdb", ModeServer, none) || !SourceOn("omdb", ModeServer, all) || !SourceOn("omdb", ModeOwn, all) || SourceOn("omdb", ModeOff, all) {
		t.Fatal("OMDb needs its key, in every mode but off")
	}
	if !strings.Contains(strings.Join(ModeHosts(ModeServer, all), " "), OMDbHost) || strings.Contains(strings.Join(ModeHosts(ModeServer, none), " "), OMDbHost) {
		t.Fatal("the firewall list names OMDb only with its key")
	}
	h, srv := modeRig(t, nil)
	st, a := modeCall(t, srv, "PUT", "/api/v1/settings/metadata", `{"keys":{"omdb_key":"omdb-fixture"}}`)
	if st != 200 || a["keys"].(map[string]any)["omdb_key"] != true || strings.Contains(srvBody(t, srv), "omdb-fixture") {
		t.Fatalf("saved: %d %v", st, a)
	}
	if v, _ := h.Vault.Setting(context.Background(), KeyGroup, KeyOMDb); v != "omdb-fixture" {
		t.Fatalf("vault: %q", v)
	}
	if _, a := modeCall(t, srv, "POST", "/api/v1/settings/metadata/test", `{"key":"omdb_key","value":"bad"}`); a["message"] != "Key refused" {
		t.Fatalf("test: %v", a)
	}
	// Without the key nothing reads as off: OMDb is an extra the owner adds.
	modeCall(t, srv, "PUT", "/api/v1/settings/metadata", `{"keys":{"omdb_key":""}}`)
	if _, a := modeCall(t, srv, "GET", "/api/v1/settings/metadata", ""); a["note"] != nil {
		t.Fatalf("a missing OMDb key is not a row that is off: %v", a["note"])
	}
}

func TestOMDbKeyTest(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Query().Get("apikey"))
		if r.URL.Query().Get("apikey") != "good" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"Response":"False","Error":"Invalid API key!"}`))
			return
		}
		_, _ = w.Write([]byte(`{"Response":"False","Error":"Incorrect IMDb ID."}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(EnvOMDbURL, srv.URL)
	outbound.Reset()
	h := &Hosted{}
	if err := h.TestKey(context.Background(), KeyOMDb, "good", ""); err != nil {
		t.Fatalf("good key: %v", err)
	}
	if err := h.TestKey(context.Background(), KeyOMDb, "bad", ""); !errors.Is(err, ErrNoAccess) {
		t.Fatalf("bad key: %v", err)
	}
	if len(asked) != 2 {
		t.Fatalf("asked %v", asked)
	}
}

func TestKeptScoresFillMissingSources(t *testing.T) {
	f, n := 7.9, 9
	got := withKept(TitleExtras{Scores: []OutsideScore{{Source: "imdb", Value: 8.1, Max: 10, Votes: 5000}}},
		KeptScores{IMDbRating: &f, IMDbVotes: 250000, RottenTomatoes: &n, Metacritic: &n})
	if len(got.Scores) != 3 || got.Scores[0].Value != 8.1 || got.Scores[1].Source != "rt_critics" || got.Scores[2].Source != "metacritic" || got.Scores[2].Max != 100 {
		t.Fatalf("the outside ratings answer wins, kept scores fill the rest: %+v", got.Scores)
	}
	got = withKept(TitleExtras{Scores: []OutsideScore{}}, KeptScores{IMDbRating: &f, IMDbVotes: 12})
	if len(got.Scores) != 0 {
		t.Fatalf("an IMDb score with too few votes stays hidden: %+v", got.Scores)
	}
	if got := withKept(TitleExtras{Scores: []OutsideScore{}}, KeptScores{}); len(got.Scores) != 0 {
		t.Fatal("nothing kept adds nothing")
	}
}
