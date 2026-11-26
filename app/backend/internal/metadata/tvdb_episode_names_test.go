package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TVDB's default episode list names a Japanese show's episodes in Japanese.
// They are named in English, like the show; an episode with no English name
// keeps its own, and a failed English list keeps every name (#769).
func TestTVDBEpisodeNamesInEnglish(t *testing.T) {
	cases := []struct {
		lang, english string
		want          []string
		asked         bool
	}{
		{"jpn", `{"data":{"episodes":[{"id":1,"name":"Odd Taxi"},{"id":2,"name":"  "}]},"links":{"next":null}}`,
			[]string{"Odd Taxi", "二話"}, true},
		{"jpn", `broken`, []string{"一話", "二話"}, true},
		{"jpn", `{odd`, []string{"一話", "二話"}, true},
		{"eng", ``, []string{"一話", "二話"}, false},
	}
	for _, tc := range cases {
		asked := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v4/login":
				_, _ = w.Write([]byte(`{"data":{"token":"s"}}`))
			case "/v4/series/7/extended":
				_, _ = w.Write([]byte(`{"data":{"id":7,"name":"オッドタクシー","year":"2021","originalLanguage":"` + tc.lang + `","nameTranslations":["jpn","eng"],"aliases":[]}}`))
			case "/v4/series/7/episodes/default":
				_, _ = w.Write([]byte(`{"data":{"episodes":[{"id":1,"seasonNumber":1,"number":1,"name":"一話"},{"id":2,"seasonNumber":1,"number":2,"name":"二話"}]},"links":{"next":null}}`))
			case "/v4/series/7/episodes/default/eng":
				asked = true
				if tc.english == "broken" {
					w.WriteHeader(500)
					return
				}
				_, _ = w.Write([]byte(tc.english))
			default:
				w.WriteHeader(404)
			}
		}))
		p := &TVDB{BaseURL: srv.URL + "/v4", Key: func(context.Context) (string, error) { return fakeTVDBKey, nil }, Gap: time.Microsecond}
		s, err := p.Show(context.Background(), "tvdb:7")
		srv.Close()
		if err != nil {
			t.Fatalf("%s %s: %v", tc.lang, tc.english, err)
		}
		var got []string
		for _, e := range s.Episodes {
			got = append(got, e.Name)
		}
		if len(got) != 2 || got[0] != tc.want[0] || got[1] != tc.want[1] || asked != tc.asked {
			t.Fatalf("%s %s: names %q asked %v, want %q asked %v", tc.lang, tc.english, got, asked, tc.want, tc.asked)
		}
	}
}
