package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

// Cache audit 2026-10-23: empty, partial and placeholder answers are kept
// minutes only, never with stale-while-revalidate; full answers keep the
// long policy.

func shortEdge(cc string) bool {
	return cc == "no-store" || (strings.Contains(cc, "s-maxage=60") && !strings.Contains(cc, "stale-while-revalidate") && !strings.Contains(cc, "immutable"))
}

// Finding 1: an artist answer whose video job has not run yet is partial.
func TestArtistAnswerWithoutVideoJobIsShort(t *testing.T) {
	s, _, _ := setup(Options{Burst: 100, Videos: fakeVideos{}})
	w := req(s, "/v1/music/musicbrainz/artist/"+queuedArtist, "")
	if w.Code != 200 || !shortEdge(w.Header().Get("Cache-Control")) {
		t.Fatalf("artist without videos yet %d %q", w.Code, w.Header().Get("Cache-Control"))
	}
	s, _, _ = setup(Options{Burst: 100, Videos: fakeVideos{queuedArtist: `{"videos":[{"youtube_id":"AAAAAAAAAAA"}]}`}})
	w = req(s, "/v1/music/musicbrainz/artist/"+queuedArtist, "")
	if w.Code != 200 || !strings.Contains(w.Header().Get("Cache-Control"), "s-maxage=86400") {
		t.Fatalf("full artist %d %q", w.Code, w.Header().Get("Cache-Control"))
	}
}

// Finding 1/6: an empty video list is not kept for days at the edge.
func TestEmptyVideoListIsShort(t *testing.T) {
	s, _, _ := setup(Options{Burst: 100, Videos: fakeVideos{queuedArtist: `{"videos":[]}`}})
	w := req(s, "/v1/music/videos/"+queuedArtist, "")
	if w.Code != 200 || !shortEdge(w.Header().Get("Cache-Control")) {
		t.Fatalf("empty list %d %q", w.Code, w.Header().Get("Cache-Control"))
	}
}

// Finding 3: the grey placeholder is never stored by browsers or the edge.
func TestPlaceholderNoStore(t *testing.T) {
	s, _, _ := setup(Options{Burst: 100, Images: &Images{Store: fakeImages{}}})
	w := req(s, "/v1/img/artist/c1392450-e666-3926-a536-22c65f834433/250.jpg", "")
	if w.Code != 200 || w.Header().Get("X-Metadata-Placeholder") != "true" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("placeholder %d %v", w.Code, w.Header())
	}
}

// Finding 8: an unversioned picture is re-checked within the hour.
func TestUnversionedPictureShort(t *testing.T) {
	file := filepath.Join(t.TempDir(), "500.webp")
	_ = os.WriteFile(file, []byte("jpegbytes"), 0o644)
	s, _, _ := setup(Options{Burst: 100, Images: &Images{Store: fakeImages{"album/" + queuedArtist: file}}})
	w := req(s, "/v1/img/album/"+queuedArtist+"/500.webp", "")
	cc := w.Header().Get("Cache-Control")
	if w.Code != 200 || cc != "public, max-age=600, s-maxage=3600" || w.Header().Get("ETag") == "" {
		t.Fatalf("unversioned %d %q", w.Code, cc)
	}
}

// Finding 9: the purge list is every edge prefix.
func TestEdgePrefixesMatchPolicies(t *testing.T) {
	got := EdgePrefixes()
	if len(got) != len(edgePolicies)+len(edgeAuthPolicies) {
		t.Fatalf("prefixes %v", got)
	}
	for _, want := range []string{"/v1/art/", "/v1/music/musicbrainz/artist/", "/v1/music/videos/", "/v1/music/cover/", "/v1/img/", "/v1/music/genres/tile/"} {
		found := false
		for _, p := range got {
			found = found || p == want
		}
		if !found {
			t.Fatalf("missing %s in %v", want, got)
		}
	}
}

// A show answer with no trailer even in its seasons is kept minutes only.
func TestShowWithoutTrailerKeptBriefly(t *testing.T) {
	s, _, _ := setup(Options{Burst: 100})
	q := model.Query{Provider: "tmdb", Kind: "raw", Operation: model.OperationTMDB, Path: "/tv/1416", Params: "append_to_response=videos"}
	if d := s.expiry(q, model.Result{Data: []byte(`{"videos":{"results":[]}}`)}); d > 15*time.Minute {
		t.Fatalf("empty kept %v", d)
	}
	if d := s.expiry(q, model.Result{Data: []byte(`{"videos":{"results":[{"key":"k","site":"YouTube","type":"Trailer"}]}}`)}); d <= 15*time.Minute {
		t.Fatalf("full kept only %v", d)
	}
}
