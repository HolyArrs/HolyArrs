package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/musicdata"
	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
)

const (
	tA1 = "a74b1b7f-71a5-4011-9441-d0b5e4122711"
	tA2 = "b10bbbfc-cf9e-42e0-be17-e2c3e1d2600d"
)

type fakeArtistData struct{ fakeSimilarBody }

type fakeSimilarBody struct{}

func (fakeSimilarBody) Artists(ctx context.Context, mbid string) ([]byte, string, error) {
	return nil, "", musictrend.ErrNoSimilar
}

func (fakeArtistData) Scores(ctx context.Context, seed string) ([]musictrend.SimilarScore, error) {
	return []musictrend.SimilarScore{{MBID: tA2, Name: "B", S: 1, Source: "listenbrainz"}}, nil
}

func (fakeArtistData) Data(ctx context.Context, id string) (*musictrend.ArtistData, error) {
	return &musictrend.ArtistData{MBID: id, Genres: []musictrend.GenreVote{}, Albums: []musictrend.TopAlbum{}}, nil
}

func TestMusicArtistBatches(t *testing.T) {
	bare := &Server{}
	for _, c := range []struct {
		url  string
		code int
		fn   func(*Server, http.ResponseWriter, *http.Request)
	}{
		{"/v1/music/top-songs/nope", 400, (*Server).serveMusicTopSongs},
		{"/v1/music/top-songs/" + tA1, 404, (*Server).serveMusicTopSongs},
		{"/v1/music/album-details?albums=" + tA1 + "&x=1", 400, (*Server).serveMusicAlbumDetails},
		{"/v1/music/similar-set?seeds=" + tA1 + "&x=1", 400, (*Server).serveMusicSimilarSet},
		{"/v1/music/artist-data?artists=" + tA1, 404, (*Server).serveMusicArtistData},
	} {
		w := httptest.NewRecorder()
		c.fn(bare, w, httptest.NewRequest("GET", c.url, nil))
		if w.Code != c.code {
			t.Errorf("%s: %d, want %d", c.url, w.Code, c.code)
		}
	}
	s := &Server{opts: Options{Similar: fakeArtistData{}}}
	w := httptest.NewRecorder()
	s.serveMusicSimilarSet(w, httptest.NewRequest("GET", "/v1/music/similar-set?seeds="+tA1, nil))
	if w.Code != 200 {
		t.Fatalf("similar-set %d", w.Code)
	}
	w = httptest.NewRecorder()
	s.serveMusicArtistData(w, httptest.NewRequest("GET", "/v1/music/artist-data?artists="+tA1, nil))
	if w.Code != 200 {
		t.Fatalf("artist-data %d", w.Code)
	}
}

type fakeMusicData struct{ failTop bool }

func (f fakeMusicData) TopSongs(ctx context.Context, mbid string) (musicdata.TopSongs, error) {
	switch {
	case f.failTop:
		return musicdata.TopSongs{}, musicdata.ErrLater
	case mbid == tA2:
		return musicdata.TopSongs{}, musicdata.ErrNone
	}
	return musicdata.TopSongs{Source: "spotify", Songs: []musicdata.Song{{Title: "Nights", LengthMS: 1000}}}, nil
}

func (fakeMusicData) Discography(ctx context.Context, mbid string) (musicdata.Discography, error) {
	if mbid == tA2 {
		return musicdata.Discography{}, musicdata.ErrNone
	}
	return musicdata.Discography{Source: "applemusic", Albums: []musicdata.DiscAlbum{{Title: "One", Type: "album"}}}, nil
}

func (fakeMusicData) AlbumDetails(ctx context.Context, ids []string) map[string]musicdata.Album {
	return map[string]musicdata.Album{tA2: {MBID: tA2, Label: "L", Sources: map[string]string{"label": "applemusic"}}}
}

func TestMusicDataRoutes(t *testing.T) {
	s := &Server{opts: Options{MusicData: fakeMusicData{}}}
	w := httptest.NewRecorder()
	s.serveMusicTopSongs(w, httptest.NewRequest("GET", "/v1/music/top-songs/"+tA1, nil))
	var top struct {
		Source string           `json:"source"`
		Songs  []musicdata.Song `json:"songs"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &top) != nil || top.Source != "spotify" || top.Songs[0].Title != "Nights" {
		t.Fatalf("top songs %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	s.serveMusicTopSongs(w, httptest.NewRequest("GET", "/v1/music/top-songs/"+tA2, nil))
	if w.Code != 404 {
		t.Fatalf("no songs: %d", w.Code)
	}
	w = httptest.NewRecorder()
	(&Server{opts: Options{MusicData: fakeMusicData{failTop: true}}}).serveMusicTopSongs(w, httptest.NewRequest("GET", "/v1/music/top-songs/"+tA1, nil))
	if w.Code != 503 {
		t.Fatalf("ask again later: %d", w.Code)
	}
	w = httptest.NewRecorder()
	s.serveMusicDiscography(w, httptest.NewRequest("GET", "/v1/music/discography/"+tA1, nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"source":"applemusic"`) {
		t.Fatalf("discography %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	s.serveMusicDiscography(w, httptest.NewRequest("GET", "/v1/music/discography/"+tA2, nil))
	if w.Code != 404 {
		t.Fatalf("no discography: %d", w.Code)
	}
	w = httptest.NewRecorder()
	s.serveMusicAlbumDetails(w, httptest.NewRequest("GET", "/v1/music/album-details?albums="+tA1+","+tA2, nil))
	var al struct {
		Albums []musicdata.Album `json:"albums"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &al) != nil || len(al.Albums) != 1 || al.Albums[0].Label != "L" {
		t.Fatalf("album details %d %s", w.Code, w.Body.String())
	}
}

type pendingSimilar struct{}

func (pendingSimilar) Artists(ctx context.Context, mbid string) ([]byte, string, error) {
	return nil, "miss", musictrend.ErrPending
}

func TestMusicSimilarPending(t *testing.T) {
	s := &Server{opts: Options{Similar: pendingSimilar{}}}
	w := httptest.NewRecorder()
	s.serveMusicSimilar(w, httptest.NewRequest("GET", "/v1/music/similar/"+tA1, nil))
	if w.Code != 202 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
}

// nowSimilar would block on Artists; the route must use ArtistsNow.
type nowSimilar struct{}

func (nowSimilar) Artists(ctx context.Context, mbid string) ([]byte, string, error) {
	select {} // a page open must never get here
}

func (nowSimilar) ArtistsNow(ctx context.Context, mbid string) ([]byte, string, error) {
	return nil, "miss", musictrend.ErrPending
}

func TestMusicSimilarNeverWaitsOnSource(t *testing.T) {
	s := &Server{opts: Options{Similar: nowSimilar{}}}
	w := httptest.NewRecorder()
	s.serveMusicSimilar(w, httptest.NewRequest("GET", "/v1/music/similar/"+tA1, nil))
	if w.Code != 202 {
		t.Fatalf("%d", w.Code)
	}
}

type countingArtistData struct {
	fakeArtistData
	asked *[]string
}

func (c countingArtistData) Data(ctx context.Context, id string) (*musictrend.ArtistData, error) {
	*c.asked = append(*c.asked, id)
	return c.fakeArtistData.Data(ctx, id)
}

func (c countingArtistData) Scores(ctx context.Context, seed string) ([]musictrend.SimilarScore, error) {
	*c.asked = append(*c.asked, seed)
	return c.fakeArtistData.Scores(ctx, seed)
}

// Security 34: a self-registered caller's ids the slim copy lacks never
// become live ListenBrainz calls; a full install credential still asks.
func TestMusicArtistBatchesSkipSlimMissing(t *testing.T) {
	var asked []string
	s := &Server{opts: Options{Similar: countingArtistData{asked: &asked},
		SlimMissing: func(_ context.Context, _, id string) bool { return id == tA1 }}}
	reg := func(u string, registered bool) *http.Request {
		r := httptest.NewRequest("GET", u, nil)
		return r.WithContext(context.WithValue(r.Context(), callerKey{}, caller{registered: registered, who: "x"}))
	}
	s.serveMusicArtistData(httptest.NewRecorder(), reg("/v1/music/artist-data?artists="+tA1+","+tA2, true))
	s.serveMusicSimilarSet(httptest.NewRecorder(), reg("/v1/music/similar-set?seeds="+tA1, true))
	if len(asked) != 1 || asked[0] != tA2 {
		t.Fatalf("asked %v", asked)
	}
	asked = nil
	s.serveMusicArtistData(httptest.NewRecorder(), reg("/v1/music/artist-data?artists="+tA1, false))
	if len(asked) != 1 {
		t.Fatalf("full install not asked %v", asked)
	}
}
