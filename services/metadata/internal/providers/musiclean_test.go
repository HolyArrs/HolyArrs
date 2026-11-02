package providers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

const relA, relB = "aaaaaaaa-1234-1234-1234-123456789abc", "bbbbbbbb-1234-1234-1234-123456789abc"

// Music speed plan section 3: an artist page costs 2 MusicBrainz calls
// (artist with url-rels, then one release-group browse of up to 100).
func TestArtistIsTwoCallsWithBrowse(t *testing.T) {
	var calls atomic.Int32
	c := fixtureClient(t, "musicbrainz", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/artist/"+mbid:
			if strings.Contains(q.Get("inc"), "release-groups") || !strings.Contains(q.Get("inc"), "url-rels") {
				t.Errorf("artist inc %q", q.Get("inc"))
			}
			fmt.Fprintf(w, `{"id":%q,"name":"Synthetic","relations":[]}`, mbid)
		case r.URL.Path == "/release-group" && q.Get("artist") == mbid && q.Get("limit") == "100":
			fmt.Fprintf(w, `{"release-group-count":130,"release-group-offset":0,"release-groups":[{"id":%q,"title":"New","first-release-date":"2020"},{"id":%q,"title":"Old","first-release-date":"1990"}]}`, relA, relB)
		default:
			t.Errorf("unexpected %s", r.URL)
			w.WriteHeader(404)
		}
	}, nil)
	r, err := c.Fetch(context.Background(), query("musicbrainz", "artist", "detail"))
	if err != nil {
		t.Fatal(err)
	}
	src := dataObject(t, r)["source_data"].(map[string]any)
	if rg, _ := src["release-groups"].([]any); len(rg) != 2 || src["release-group-count"] != float64(130) {
		t.Fatalf("release groups not merged: %v", src)
	}
	if calls.Load() != 2 {
		t.Fatalf("%d calls, want 2", calls.Load())
	}
}

// An album page costs 2 calls: the release group, then its first official
// release (earliest date) with recordings, so the tracklist comes along.
func TestAlbumIsTwoCallsWithTracklist(t *testing.T) {
	var calls atomic.Int32
	c := fixtureClient(t, "musicbrainz", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/release-group/" + mbid:
			fmt.Fprintf(w, `{"id":%q,"title":"Album","releases":[{"id":%q,"status":"Bootleg","date":"1999"},{"id":%q,"status":"Official","date":"2001-05-01"}]}`, mbid, relA, relB)
		case "/release/" + relB:
			if !strings.Contains(r.URL.Query().Get("inc"), "recordings") {
				t.Errorf("release inc %q", r.URL.Query().Get("inc"))
			}
			fmt.Fprintf(w, `{"id":%q,"media":[{"position":1,"tracks":[{"id":"t1","title":"One","position":1}]}]}`, relB)
		default:
			t.Errorf("unexpected %s", r.URL)
			w.WriteHeader(404)
		}
	}, nil)
	r, err := c.Fetch(context.Background(), query("musicbrainz", "release-group", "detail"))
	if err != nil {
		t.Fatal(err)
	}
	src := dataObject(t, r)["source_data"].(map[string]any)
	if m, _ := src["media"].([]any); len(m) != 1 || src["tracklist_release"] != relB {
		t.Fatalf("tracklist not merged: %v", src)
	}
	if calls.Load() != 2 {
		t.Fatalf("%d calls, want 2", calls.Load())
	}
}

// A failed browse fails the artist answer (retried by the queue) rather
// than storing an artist with no albums for a day.
func TestArtistBrowseFailureFails(t *testing.T) {
	c := fixtureClient(t, "musicbrainz", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/release-group" {
			w.WriteHeader(503)
			return
		}
		fmt.Fprintf(w, `{"id":%q,"name":"Synthetic"}`, mbid)
	}, nil)
	if _, err := c.Fetch(context.Background(), query("musicbrainz", "artist", "detail")); err == nil {
		t.Fatal("artist without its albums accepted")
	}
}
