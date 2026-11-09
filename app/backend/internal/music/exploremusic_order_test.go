package music

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// orderFixed is the fixed clock of the Phase 3 golden (ties hash the date).
var orderFixed = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

// fakeArtistSource answers the metadata server's artist batches for the
// household fixture: each band has nine similar artists, rock and pop
// votes, three back albums with listeners and three guest spots.
func fakeArtistSource(calls *int) (func(context.Context, []string) (map[string]int64, error), func(context.Context, []string) (map[string][]metadata.MusicSimilarScore, error), func(context.Context, []string) ([]metadata.MusicArtistData, error)) {
	band := func(id string) int {
		var k int
		if _, err := fmt.Sscanf(id, "a000000%d-", &k); err != nil || k < 1 || k > 3 {
			return 0
		}
		return k
	}
	listeners := func(_ context.Context, ids []string) (map[string]int64, error) {
		*calls++
		out := map[string]int64{}
		for i, id := range ids {
			out[id] = int64(1000 * (len(id) + i%7))
		}
		return out, nil
	}
	similar := func(_ context.Context, seeds []string) (map[string][]metadata.MusicSimilarScore, error) {
		*calls++
		out := map[string][]metadata.MusicSimilarScore{}
		for _, s := range seeds {
			k := band(s)
			if k == 0 {
				continue
			}
			for i := 1; i <= 9; i++ {
				out[s] = append(out[s], metadata.MusicSimilarScore{MBID: mid('b', 10*k+i), Name: fmt.Sprintf("Near %d-%d", k, i), S: 1 - 0.08*float64(i-1), Source: "listenbrainz"})
			}
		}
		return out, nil
	}
	data := func(_ context.Context, ids []string) ([]metadata.MusicArtistData, error) {
		*calls++
		var out []metadata.MusicArtistData
		for _, id := range ids {
			d := metadata.MusicArtistData{MBID: id}
			if k := band(id); k > 0 {
				d.Genres = []metadata.MusicGenreVote{{Name: "Rock", Count: 10 - k}, {Name: "pop", Count: 2 + k}}
				for j := 0; j < 3; j++ {
					d.Albums = append(d.Albums, metadata.MusicTopAlbum{MBID: fmt.Sprintf("c00001%d%d-0000-4000-8000-0000000001%d%d", k, j, k, j), Title: fmt.Sprintf("Back %d-%d", k, j), Type: "album", Listeners: int64(5000 * (3 - j)), Artists: []string{id}})
				}
				for j := 1; j <= 3; j++ {
					d.Albums = append(d.Albums, metadata.MusicTopAlbum{MBID: mid('d', 10*k+j), Title: fmt.Sprintf("Guest %d-%d", k, j), Type: "single", Listeners: int64(100 * j), Artists: []string{mid('e', 10*k+j), id}, ArtistNames: []string{fmt.Sprintf("Host %d-%d", k, j), "Band"}})
				}
			} else if strings.HasPrefix(id, "b") {
				d.Genres = []metadata.MusicGenreVote{{Name: "rock", Count: 3}}
				if id[len(id)-1]%2 == 0 {
					d.Genres = []metadata.MusicGenreVote{{Name: "jazz", Count: 3}}
				}
				d.Albums = []metadata.MusicTopAlbum{{MBID: "f" + id[1:], Title: "Best of " + id[:8], Type: "album", Listeners: int64(len(id) * 100), Artists: []string{id}, ArtistNames: []string{"Near"}}}
			}
			out = append(out, d)
		}
		return out, nil
	}
	return listeners, similar, data
}

func orderRig(t *testing.T) (*askRig, *int) {
	r := newMusicAskRig(t)
	loadHousehold(t, r.st, orderFixed)
	r.api.exploreCache.now = func() time.Time { return orderFixed }
	charts, genres := fakeCharts(orderFixed)
	r.api.s.MusicGenres = genres
	// Genre charts: their own albums (the region chart otherwise).
	r.api.s.MusicCharts = func(ctx context.Context, region, kind, genre string) (metadata.MusicChart, error) {
		c, err := charts(ctx, region, kind, genre)
		if genre == "" {
			return c, err
		}
		for i := range c.Items {
			c.Items[i].MBID = mid('9', 50+i)
			c.Items[i].Title = genre + " chart " + fmt.Sprint(i+1)
			c.Items[i].Explicit = false
		}
		return c, err
	}
	calls := 0
	r.api.s.MusicListeners, r.api.s.MusicSimilarSet, r.api.s.MusicArtistData = fakeArtistSource(&calls)
	return r, &calls
}
