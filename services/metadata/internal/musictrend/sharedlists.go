package musictrend

import (
	"context"
	"sort"
	"strings"
	"time"
)

// SharedMembers lists the MusicBrainz ids shown on the shared lists that
// are built from stored Apple Music chart data (no upstream call): every
// region's album chart (release groups and their artists) and, per Apple
// genre, up to perGenre artists charted in it across all regions, best
// chart rank first. The list source's members are filled apart (lists).
func (s *Service) SharedMembers(ctx context.Context, perGenre int, now time.Time) (albums, artists []string, genres map[string][]string) {
	best := map[string]map[string]int{}
	seenAlbum := map[string]bool{}
	seenArtist := map[string]bool{}
	for _, st := range Storefronts() {
		for _, kind := range []string{"albums", "songs"} {
			items, ok := s.appleChart(ctx, st, kind, "", now)
			if !ok {
				continue
			}
			for _, it := range items {
				a := strings.ToLower(it.ArtistMBID)
				if kind == "albums" && validMBID(it.MBID) && !seenAlbum[it.MBID] {
					seenAlbum[it.MBID] = true
					albums = append(albums, strings.ToLower(it.MBID))
				}
				if !validMBID(a) {
					continue
				}
				if !seenArtist[a] {
					seenArtist[a] = true
					artists = append(artists, a)
				}
				for _, g := range it.Genres {
					sl := GenreSlug(g)
					if sl == "" {
						continue
					}
					if best[sl] == nil {
						best[sl] = map[string]int{}
					}
					if r, ok := best[sl][a]; !ok || it.ChartRank < r {
						best[sl][a] = it.ChartRank
					}
				}
			}
		}
	}
	genres = map[string][]string{}
	for sl, m := range best {
		ids := make([]string, 0, len(m))
		for id := range m {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool {
			if m[ids[i]] != m[ids[j]] {
				return m[ids[i]] < m[ids[j]]
			}
			return ids[i] < ids[j]
		})
		if len(ids) > perGenre {
			ids = ids[:perGenre]
		}
		genres[sl] = ids
	}
	return albums, artists, genres
}

// SharedMembers lists the new-release and coming-soon albums (release
// groups) and their artists, from the stored windows.
func (n *NewMusic) SharedMembers(now time.Time) (albums, artists []string) {
	seen := map[string]bool{}
	for _, it := range n.Releases("0000-00-00", "9999-12-31", nil, now) {
		if validMBID(it.MBID) {
			albums = append(albums, strings.ToLower(it.MBID))
		}
		if a := strings.ToLower(it.ArtistMBID); validMBID(a) && !seen[a] {
			seen[a] = true
			artists = append(artists, a)
		}
	}
	return albums, artists
}
