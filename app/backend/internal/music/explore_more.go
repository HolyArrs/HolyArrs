package music

import (
	"context"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// More Explore → Music rows: new this week, rising artists, classics,
// popular in a country, anniversaries, live albums and EPs, collaborators,
// debuts, decades, moods and soundtracks for the Library's movies and
// shows. Every row needs only the Library, the metadata server's lists
// (ListenBrainz, Deezer) and MusicBrainz, all keyless.

type genreLinkSource interface {
	ArtistGenresLinks(ctx context.Context, mbid string) ([]string, []metadata.ArtistLink, error)
}

type soundtrackSource interface {
	SoundtrackFor(ctx context.Context, title string, year int) ([]metadata.Album, error)
}

// musicCtx is what the main build hands the extra rows.
type musicCtx struct {
	now          time.Time
	hour         int
	region       string
	artists      []*musicArtist
	byTitle      map[int64]*musicArtist
	owned        map[string]int64
	genres       map[string][]string
	trendAlbums  []MusicCard
	trendArtists []MusicCard
	pool         map[string]MusicCard
	top          []string
	tag          func(MusicCard) MusicCard
	albumCard    func(*musicArtist, mediafiles.Part) MusicCard
	names        *[]nameKey
	cold         bool
}

func init() {
	for t, n := range map[string]int{"fresh": 4, "rising": 4, "classic": 4, "popular_country": 4, "anniversary": 4,
		"live_eps": 4, "collabs": 4, "debut": 3, "decade": 4, "mood": 4} {
		musicRowMin[t] = n
	}
}

// linksOf answers the known collaborators of each artist id.
func (h *Handlers) linksOf(ids []string) map[string][]metadata.ArtistLink {
	c := &h.musicx
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string][]metadata.ArtistLink{}
	for _, id := range ids {
		if e, ok := c.genres[id]; ok {
			out[id] = e.links
		}
	}
	return out
}

func studio(p mediafiles.Part) bool {
	return strings.EqualFold(derefStr(p.ReleaseType), "album") && len(p.SecondaryTypes) == 0
}
