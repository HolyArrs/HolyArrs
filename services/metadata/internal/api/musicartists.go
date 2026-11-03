package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/discog"
	"github.com/CoderLuii/holyarrs-metadata/internal/musicdata"
	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
)

// Batch artist data for the app's scored Music rows (plan 2026-10-22 §5,
// endpoints 4 and 5), artists' top songs and discography, and albums'
// details. A batch goes in the query (GET only); each list is capped.
// Nothing about a member is sent or kept.

// MusicArtistData is the artist side of the similar source.
type MusicArtistData interface {
	Scores(ctx context.Context, seed string) ([]musictrend.SimilarScore, error)
	Data(ctx context.Context, id string) (*musictrend.ArtistData, error)
}

// MusicData answers top songs, discography and album details
// (musicdata.Service).
type MusicData interface {
	TopSongs(ctx context.Context, mbid string) (musicdata.TopSongs, error)
	Discography(ctx context.Context, mbid string) (musicdata.Discography, error)
	AlbumDetails(ctx context.Context, ids []string) map[string]musicdata.Album
}

const (
	musicSeedsMax   = 12
	musicArtistsMax = 30
	musicAlbumsMax  = 20
)

// idList reads a comma list of artist ids, at most n; false on a bad id.
func idList(raw string, n int) ([]string, bool) {
	var out []string
	seen := map[string]bool{}
	if raw == "" {
		return nil, false
	}
	for _, a := range strings.Split(strings.ToLower(raw), ",") {
		if !mbidRe.MatchString(a) || len(out) >= n {
			return nil, false
		}
		if !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out, true
}

func (s *Server) artistData(w http.ResponseWriter) (MusicArtistData, bool) {
	d, ok := s.opts.Similar.(MusicArtistData)
	if !ok || s.opts.Similar == nil {
		problem(w, 404, "list_unavailable", 0)
		return nil, false
	}
	return d, true
}

func (s *Server) musicData(w http.ResponseWriter) (MusicData, bool) {
	if s.opts.MusicData == nil {
		problem(w, 404, "list_unavailable", 0)
		return nil, false
	}
	return s.opts.MusicData, true
}

// serveMusicTopSongs answers GET /v1/music/top-songs/{artist mbid}: the
// artist's top songs from Spotify, else Apple Music, else Deezer.
func (s *Server) serveMusicTopSongs(w http.ResponseWriter, r *http.Request) {
	mbid := strings.ToLower(strings.TrimPrefix(r.URL.Path, "/v1/music/top-songs/"))
	if len(r.URL.Query()) > 0 || !mbidPattern.MatchString(mbid) {
		problem(w, 400, "invalid_query", 0)
		return
	}
	d, ok := s.musicData(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	t, err := d.TopSongs(ctx, mbid)
	switch {
	case errors.Is(err, musicdata.ErrNone):
		problem(w, 404, "not_found", 0)
		return
	case err != nil:
		problem(w, 503, "provider_unavailable", time.Minute)
		return
	}
	writeMusicJSON(w, r, map[string]any{"schema_version": 1, "mbid": mbid, "source": t.Source, "songs": t.Songs}, 86400)
}

// serveMusicDiscography answers GET /v1/music/discography/{artist mbid}:
// "musicbrainz" with every release group stored (release_groups, with
// release_group_count and release_groups_complete; albums empty) when
// MusicBrainz has any, else the artist's albums from Apple Music, else
// Spotify (no MusicBrainz ids). A list not stored yet is walked at P0
// and waited for; past the wait the answer is 202 with Retry-After.
func (s *Server) serveMusicDiscography(w http.ResponseWriter, r *http.Request) {
	mbid := strings.ToLower(strings.TrimPrefix(r.URL.Path, "/v1/music/discography/"))
	if len(r.URL.Query()) > 0 || !mbidPattern.MatchString(mbid) {
		problem(w, 400, "invalid_query", 0)
		return
	}
	if a, ok, waited := s.storedDiscography(r, mbid); ok && len(a.Groups) > 0 {
		s.writeDiscography(w, r, mbid, a)
		return
	} else if waited {
		w.Header().Set("Retry-After", "2")
		writeJSON(w, 202, s.withQueueState(r.Context(), discog.ArtistKey(mbid), map[string]any{"schema_version": 1, "mbid": mbid, "source": "musicbrainz", "partial": true, "albums": []musicdata.DiscAlbum{}, "release_groups": []any{}, "release_groups_complete": false}))
		return
	}
	d, ok := s.musicData(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	disc, err := d.Discography(ctx, mbid)
	switch {
	case errors.Is(err, musicdata.ErrNone):
		problem(w, 404, "not_found", 0)
		return
	case err != nil:
		problem(w, 503, "provider_unavailable", time.Minute)
		return
	}
	writeMusicJSON(w, r, map[string]any{"schema_version": 1, "mbid": mbid, "source": disc.Source, "albums": disc.Albums}, 86400)
}

// serveMusicAlbumDetails answers GET /v1/music/album-details?albums=a,b
// (release-group ids, up to 20): each album's label, track count,
// barcode, explicit, single and complete marks, with each field's source.
// Albums no source knows, or not answered in time, are left out.
func (s *Server) serveMusicAlbumDetails(w http.ResponseWriter, r *http.Request) {
	if !onlyParams(w, r, "albums") {
		return
	}
	ids, ok := idList(r.URL.Query().Get("albums"), musicAlbumsMax)
	if !ok {
		problem(w, 400, "invalid_query", 0)
		return
	}
	d, ok := s.musicData(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	got := d.AlbumDetails(ctx, ids)
	list := []musicdata.Album{}
	for _, id := range ids {
		if a, ok := got[id]; ok {
			list = append(list, a)
		}
	}
	writeMusicJSON(w, r, map[string]any{"schema_version": 1, "albums": list}, 86400)
}

// serveMusicSimilarSet answers GET /v1/music/similar-set?seeds=a,b: per
// seed, the similar artists with S normalised by the seed's top score.
func (s *Server) serveMusicSimilarSet(w http.ResponseWriter, r *http.Request) {
	if !onlyParams(w, r, "seeds") {
		return
	}
	ids, ok := idList(r.URL.Query().Get("seeds"), musicSeedsMax)
	if !ok {
		problem(w, 400, "invalid_query", 0)
		return
	}
	ids = s.slimKnown(r, ids)
	d, ok := s.artistData(w)
	if !ok {
		return
	}
	type seed struct {
		MBID    string                    `json:"mbid"`
		Artists []musictrend.SimilarScore `json:"artists"`
	}
	out := make([]seed, len(ids))
	fails := make([]bool, len(ids))
	parallel(r.Context(), ids, 4, func(ctx context.Context, i int, id string) {
		list, err := d.Scores(ctx, id)
		out[i] = seed{MBID: id, Artists: list}
		fails[i] = err != nil
	})
	kept := []seed{}
	for i, x := range out {
		if !fails[i] {
			kept = append(kept, x)
		}
	}
	if len(kept) == 0 && len(ids) > 0 {
		problem(w, 503, "provider_unavailable", time.Minute)
		return
	}
	writeMusicJSON(w, r, map[string]any{"schema_version": 1, "seeds": kept}, 86400)
}

// serveMusicArtistData answers GET /v1/music/artist-data?artists=a,b: per
// artist, its genre votes and top release groups by listeners. Artists
// the source does not answer in time are left out (asked again later).
func (s *Server) serveMusicArtistData(w http.ResponseWriter, r *http.Request) {
	if !onlyParams(w, r, "artists") {
		return
	}
	ids, ok := idList(r.URL.Query().Get("artists"), musicArtistsMax)
	if !ok {
		problem(w, 400, "invalid_query", 0)
		return
	}
	ids = s.slimKnown(r, ids)
	d, ok := s.artistData(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	got := make([]*musictrend.ArtistData, len(ids))
	parallel(ctx, ids, 4, func(ctx context.Context, i int, id string) {
		got[i], _ = d.Data(ctx, id)
	})
	list := []*musictrend.ArtistData{}
	for _, x := range got {
		if x != nil {
			list = append(list, x)
		}
	}
	writeMusicJSON(w, r, map[string]any{"schema_version": 1, "source": "listenbrainz", "artists": list}, 86400)
}

// parallel runs f over ids, n at a time.
func parallel(ctx context.Context, ids []string, n int, f func(context.Context, int, string)) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, n)
	for i, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			f(ctx, i, id)
		}()
	}
	wg.Wait()
}

// slimKnown drops, for a self-registered or anonymous caller, the ids the
// loaded slim copy does not hold: each one would be a live ListenBrainz
// call on the server's shared budget for an artist that cannot exist.
func (s *Server) slimKnown(r *http.Request, ids []string) []string {
	if c, _ := r.Context().Value(callerKey{}).(caller); !c.registered {
		return ids
	}
	kept := ids[:0:0]
	for _, id := range ids {
		if !s.SlimAbsent(r.Context(), "artist", id) {
			kept = append(kept, id)
		}
	}
	return kept
}
