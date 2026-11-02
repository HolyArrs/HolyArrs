package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/discog"
	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

type fakeDiscog struct {
	artists map[string]discog.Artist
	albums  map[string]discog.Album
	due     bool
}

func (f *fakeDiscog) Artist(_ context.Context, id string) (discog.Artist, bool) {
	a, ok := f.artists[id]
	return a, ok
}
func (f *fakeDiscog) Album(_ context.Context, id string) (discog.Album, bool) {
	a, ok := f.albums[id]
	return a, ok
}
func (f *fakeDiscog) ArtistDue(discog.Artist) bool { return f.due }
func (f *fakeDiscog) AlbumDue(discog.Album) bool   { return f.due }

// keyQueue records keys and, for a waited job, runs fill then answers.
type keyQueue struct {
	keys []string
	p    []int
	fill func(key string)
}

func (q *keyQueue) Enqueue(_ context.Context, key string, p int, wait bool) <-chan error {
	q.keys, q.p = append(q.keys, key), append(q.p, p)
	if !wait {
		return nil
	}
	ch := make(chan error, 1)
	if q.fill != nil {
		q.fill(key)
	}
	ch <- nil
	return ch
}

const discArtist = "f9133869-f87c-459c-9aa7-2f176cda7e06"
const discAlbum = "22222222-2222-2222-2222-222222222222"

func groups(n int) []discog.Group {
	out := make([]discog.Group, n)
	for i := range out {
		out[i] = discog.Group{ID: strings.Replace(discAlbum, "2222", "3333", 1)[:24] + strings.Repeat("0", 12-len(itoa(i+1))) + itoa(i+1), Title: "G", SecondaryTypes: []string{}}
	}
	return out
}

func TestArtistAnswerCarriesFullList(t *testing.T) {
	d := &fakeDiscog{artists: map[string]discog.Artist{}}
	q := &keyQueue{}
	s, _, _ := setup(Options{Burst: 100, Discog: d, Pictures: func(_ context.Context, kind, id string) map[string]imgstore.Info {
		if kind == "release-group" && id == groups(1)[0].ID {
			return map[string]imgstore.Info{"cover": {Version: "v1"}}
		}
		return nil
	}})
	req(s, "/v1/music/musicbrainz/artist/"+discArtist, "") // the artist answer is stored
	s.SetQueue(q)
	// No list stored yet: the walk is queued (P1), the answer says so and
	// is kept a minute only at the edge.
	w := req(s, "/v1/music/musicbrainz/artist/"+discArtist, "")
	var body struct {
		Data struct {
			Complete *bool `json:"release_groups_complete"`
			Count    int   `json:"release_group_count"`
			Source   struct {
				Groups []map[string]any `json:"release-groups"`
				Count  int              `json:"release-group-count"`
			} `json:"source_data"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != 200 || body.Data.Complete == nil || *body.Data.Complete || !shortEdge(w.Header().Get("Cache-Control")) || len(q.keys) != 1 || q.keys[0] != discog.ArtistKey(discArtist) || q.p[0] != fetchq.P1Finish {
		t.Fatalf("no list %d %s %v %v", w.Code, w.Body, q.keys, w.Header())
	}
	d.artists[discArtist] = discog.Artist{MBID: discArtist, Groups: groups(259), Count: 259, Complete: true, Walked: time.Now(), Checked: time.Now()}
	w = req(s, "/v1/music/musicbrainz/artist/"+discArtist, "")
	body.Data.Complete = nil
	json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != 200 || body.Data.Complete == nil || !*body.Data.Complete || body.Data.Count != 259 || body.Data.Source.Count != 259 || len(body.Data.Source.Groups) != 259 || !strings.Contains(w.Header().Get("Cache-Control"), "s-maxage=86400") {
		t.Fatalf("full list %d %d %d %v", w.Code, body.Data.Count, len(body.Data.Source.Groups), w.Header())
	}
	if c, _ := body.Data.Source.Groups[0]["cover"].(map[string]any); c["v"] != "v1" {
		t.Fatalf("cover pointer missing %v", body.Data.Source.Groups[0])
	}
	if len(q.keys) != 1 {
		t.Fatalf("fresh list queued again %v", q.keys)
	}
	d.due = true
	req(s, "/v1/music/musicbrainz/artist/"+discArtist, "")
	if len(q.keys) != 2 || q.p[1] != fetchq.P3Refresh {
		t.Fatalf("due list not checked %v %v", q.keys, q.p)
	}
}

func TestDiscographyRouteWalksAndAnswers(t *testing.T) {
	d := &fakeDiscog{artists: map[string]discog.Artist{}}
	q := &keyQueue{fill: func(key string) {
		d.artists[strings.TrimPrefix(key, discog.ArtistPrefix)] = discog.Artist{Groups: groups(30), Count: 30, Complete: true, Walked: time.Now()}
	}}
	s, _, _ := setup(Options{Burst: 100, Discog: d, MusicData: fakeMusicData{}})
	s.SetQueue(q)
	w := req(s, "/v1/music/discography/"+discArtist, "synthetic-test-token")
	var body struct {
		Source   string            `json:"source"`
		Groups   []json.RawMessage `json:"release_groups"`
		Count    int               `json:"release_group_count"`
		Complete bool              `json:"release_groups_complete"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != 200 || body.Source != "musicbrainz" || len(body.Groups) != 30 || body.Count != 30 || !body.Complete || q.p[0] != fetchq.P0Waiting {
		t.Fatalf("discography %d %s %v", w.Code, w.Body, q.p)
	}
}

func TestAlbumRoute(t *testing.T) {
	d := &fakeDiscog{albums: map[string]discog.Album{}}
	q := &keyQueue{}
	s, _, _ := setup(Options{Burst: 100, Discog: d})
	s.SetQueue(q)
	old := discogWait
	discogWait = 50 * time.Millisecond
	defer func() { discogWait = old }()
	if w := req(s, "/v1/music/album/nope", ""); w.Code != 400 {
		t.Fatalf("bad id %d", w.Code)
	}
	// Not stored and the walk does not end: 202, never kept at the edge.
	q.fill = func(string) {}
	if w := req(s, "/v1/music/album/"+discAlbum, ""); w.Code != 202 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Retry-After") == "" {
		t.Fatalf("not stored %d %v", w.Code, w.Header())
	} else if !strings.Contains(w.Body.String(), `"state":"fetching"`) || !strings.Contains(w.Body.String(), `"message":`) {
		t.Fatalf("not stored: no state %s", w.Body)
	}
	q.fill = func(key string) {
		id := strings.TrimPrefix(key, discog.AlbumPrefix)
		d.albums[id] = discog.Album{MBID: id, Title: "Globalization", Edition: &discog.Edition{ID: "e"}, Tracks: []discog.Track{{Disc: 1, Position: 1, Number: "1", Title: "Fireball", LengthMS: 235000}}, TrackCount: 1, Editions: []discog.Edition{{ID: "e"}}, EditionCount: 1}
	}
	w := req(s, "/v1/music/album/"+discAlbum, "")
	var a struct {
		SchemaVersion int    `json:"schema_version"`
		Title         string `json:"title"`
		Edition       struct {
			ID string `json:"id"`
		} `json:"edition"`
		Tracks []struct {
			Title    string `json:"title"`
			LengthMS int    `json:"length_ms"`
		} `json:"tracks"`
		EditionCount int `json:"edition_count"`
	}
	json.Unmarshal(w.Body.Bytes(), &a)
	if w.Code != 200 || a.SchemaVersion != 1 || a.Title != "Globalization" || a.Edition.ID != "e" || len(a.Tracks) != 1 || a.Tracks[0].LengthMS != 235000 || a.EditionCount != 1 || !strings.Contains(w.Header().Get("Cache-Control"), "s-maxage=86400") {
		t.Fatalf("album %d %s %v", w.Code, w.Body, w.Header())
	}
	n := len(q.keys)
	if w := req(s, "/v1/music/album/"+discAlbum, ""); w.Code != 200 || len(q.keys) != n {
		t.Fatalf("stored album queued again %v", q.keys)
	}
}
