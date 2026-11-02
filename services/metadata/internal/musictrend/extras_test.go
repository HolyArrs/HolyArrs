package musictrend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtrasListsFreshRisingClassic(t *testing.T) {
	lb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/1/stats/sitewide/artists" && q.Get("range") == "week":
			_, _ = w.Write([]byte(`{"payload":{"last_updated":5,"artists":[{"artist_mbid":"` + artistA + `","artist_name":"Old Star","listen_count":9},{"artist_mbid":"` + artistB + `","artist_name":"New Star","listen_count":8}]}}`))
		case r.URL.Path == "/1/stats/sitewide/artists" && q.Get("range") == "year":
			_, _ = w.Write([]byte(`{"payload":{"artists":[{"artist_mbid":"` + artistA + `","artist_name":"Old Star"}]}}`))
		case r.URL.Path == "/1/stats/sitewide/release-groups" && q.Get("range") == "week":
			_, _ = w.Write([]byte(`{"payload":{"last_updated":5,"release_groups":[{"release_group_mbid":"` + groupA + `","release_group_name":"G","artist_name":"Old Star","artist_mbids":["` + artistA + `"]}]}}`))
		case r.URL.Path == "/1/stats/sitewide/release-groups" && q.Get("range") == "all_time" && q.Get("offset") == "0":
			_, _ = w.Write([]byte(`{"payload":{"release_groups":[{"release_group_mbid":"` + groupA + `","release_group_name":"Classic","artist_name":"Old Star","artist_mbids":["` + artistA + `"],"listen_count":99}]}}`))
		case r.URL.Path == "/1/stats/sitewide/release-groups":
			w.WriteHeader(204)
		case r.URL.Path == "/1/explore/fresh-releases/":
			_, _ = w.Write([]byte(`{"payload":{"releases":[
				{"artist_credit_name":"New Star","artist_mbids":["` + artistB + `"],"caa_release_mbid":null,"listen_count":3,"release_date":"2026-10-15","release_group_mbid":"` + release + `","release_group_primary_type":"Album","release_name":"Fresh","release_tags":["Chill"," chill","ambient"]},
				{"artist_credit_name":"New Star","artist_mbids":["` + artistB + `"],"listen_count":30,"release_group_mbid":"` + singer + `","release_group_primary_type":"Single","release_name":"A single"}]}}`))
		case r.URL.Path == "/1/metadata/release_group/":
			if !strings.Contains(q.Get("release_group_mbids"), groupA) {
				t.Errorf("meta ids %s", q.Get("release_group_mbids"))
			}
			_, _ = w.Write([]byte(`{"` + groupA + `":{"release_group":{"date":"1997-05-21"},"tag":{"artist":[{"tag":"rock","count":9}],"release_group":[{"tag":"alternative rock","count":3}]}}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer lb.Close()
	s, err := Open(filepath.Join(t.TempDir(), "m.sqlite"), Options{Extras: true, ListenBrainzURL: lb.URL + "/1/stats/sitewide", Pause: -1, Search: &fakeSearch{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_ = s.Refresh(context.Background())
	read := func(kind string) map[string]any {
		b, _, ok := s.List(kind, "global")
		if !ok {
			t.Fatalf("no %s list", kind)
		}
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		return m
	}
	fresh := read(KindFresh)["items"].([]any)
	if len(fresh) != 1 || fresh[0].(map[string]any)["title"] != "Fresh" || fresh[0].(map[string]any)["release_date"] != "2026-10-15" || len(fresh[0].(map[string]any)["tags"].([]any)) != 2 {
		t.Fatalf("fresh %v", fresh)
	}
	rising := read(KindRising)["items"].([]any)
	if len(rising) != 1 || rising[0].(map[string]any)["name"] != "New Star" {
		t.Fatalf("rising %v", rising)
	}
	classic := read(KindClassic)["items"].([]any)[0].(map[string]any)
	if classic["title"] != "Classic" || classic["release_date"] != "1997-05-21" || classic["tags"].([]any)[0] != "alternative rock" {
		t.Fatalf("classic %v", classic)
	}
}
