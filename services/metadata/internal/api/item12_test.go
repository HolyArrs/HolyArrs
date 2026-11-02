package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/kworb"
)

// GET /v1/music/popularity answers each asked artist's kept figures (null
// when none), anonymous or not, an hour at the edge.
func TestPopularityRoute(t *testing.T) {
	const other = "b1392450-e666-3926-a536-22c65f834433"
	change := int64(12)
	f := &fakePopularity{known: map[string]*kworb.Popularity{queuedArtist: {MonthlyListeners: 1000, WorldRank: 7, ListenersChange: &change, AsOf: "2026-10-27"}}}
	s, _, _ := setup(Options{Burst: 100, Popularity: f})
	w := req(s, "/v1/music/popularity?artists="+queuedArtist+","+strings.ToUpper(other), "")
	var got struct {
		Version int    `json:"schema_version"`
		Source  string `json:"source"`
		AsOf    string `json:"as_of"`
		Artists []struct {
			MBID       string            `json:"mbid"`
			Popularity *kworb.Popularity `json:"popularity"`
		} `json:"artists"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if got.Version != 1 || got.Source != "kworb" || got.AsOf != "2026-10-27" || len(got.Artists) != 2 || got.Artists[0].Popularity.MonthlyListeners != 1000 || got.Artists[1].MBID != other || got.Artists[1].Popularity != nil {
		t.Fatalf("answer %s", w.Body)
	}
	if !strings.Contains(w.Body.String(), `"popularity":null`) {
		t.Fatalf("unknown artist not null: %s", w.Body)
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "s-maxage=3600") || !strings.HasPrefix(cc, "public") {
		t.Fatalf("cache %q", cc)
	}
	if len(f.wanted) != 0 {
		t.Fatal("a popularity ask counted as an opened page")
	}
	for _, bad := range []string{"/v1/music/popularity", "/v1/music/popularity?artists=nope", "/v1/music/popularity?artists=" + many(51), "/v1/music/popularity?artists=" + queuedArtist + "&x=1"} {
		if w := req(s, bad, ""); w.Code != 400 {
			t.Fatalf("%s: %d", bad, w.Code)
		}
	}
}

func many(n int) string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("%08d-e666-3926-a536-22c65f834433", i)
	}
	return strings.Join(ids, ",")
}

// An artist or album answer whose picture is not stored yet (and not known
// missing) is marked partial: a minute at the edge.
func TestPicturePendingMarksPartial(t *testing.T) {
	pending := true
	s, _, _ := setup(Options{Burst: 100,
		Pictures:       func(context.Context, string, string) map[string]imgstore.Info { return nil },
		PicturePending: func(_ context.Context, kind, mbid string) bool { return pending && mbid == queuedArtist },
	})
	w := req(s, "/v1/music/musicbrainz/artist/"+queuedArtist, "")
	if w.Code != 200 || w.Header().Get("Cache-Control") != partialPolicy {
		t.Fatalf("pending picture: %d %q", w.Code, w.Header().Get("Cache-Control"))
	}
	pending = false
	w = req(s, "/v1/music/musicbrainz/artist/"+queuedArtist, "")
	if cc := w.Header().Get("Cache-Control"); w.Code != 200 || !strings.Contains(cc, "s-maxage=86400") {
		t.Fatalf("stored picture: %d %q", w.Code, cc)
	}
}

// Top songs, discography and album details are kept at the edge, but a
// miss still needs a credential.
func TestCredentialedEdgeRoutes(t *testing.T) {
	for _, p := range []string{"/v1/music/top-songs/x", "/v1/music/discography/x", "/v1/music/album-details"} {
		if edgePolicy(p) != "" || !strings.HasPrefix(edgeAuthPolicy(p), "public") {
			t.Fatalf("%s: public %q auth %q", p, edgePolicy(p), edgeAuthPolicy(p))
		}
	}
	s, _, _ := setup(Options{Burst: 100})
	if w := req(s, "/v1/music/top-songs/"+queuedArtist, ""); w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("anonymous miss %d %q", w.Code, w.Header().Get("Cache-Control"))
	}
}
