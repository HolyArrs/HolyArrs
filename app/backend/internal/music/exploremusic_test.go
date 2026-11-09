package music

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// genreArtists is fakeArtists plus MusicBrainz genres.
type genreArtists struct {
	*fakeArtists
	calls atomic.Int32
}

func (g *genreArtists) ArtistGenres(_ context.Context, mbid string) ([]string, error) {
	g.calls.Add(1)
	switch {
	case strings.HasPrefix(mbid, "a"), strings.HasPrefix(mbid, "5"):
		return []string{"rock"}, nil
	case strings.HasPrefix(mbid, "b"):
		return []string{"rock", "pop"}, nil
	}
	return []string{"jazz"}, nil
}

func (g *genreArtists) ArtistGenresLinks(ctx context.Context, mbid string) ([]string, []metadata.ArtistLink, error) {
	gs, _ := g.ArtistGenres(ctx, mbid)
	var links []metadata.ArtistLink
	if mbid == mid('a', 1) {
		for i := 1; i <= 4; i++ {
			links = append(links, metadata.ArtistLink{MBID: mid('9', i), Name: fmt.Sprintf("Member %d", i), Type: "member of band"})
		}
		links = append(links, metadata.ArtistLink{MBID: mid('b', 2), Name: "Second Band"}) // owned: left out
	}
	return gs, links, nil
}

func mid(prefix byte, n int) string {
	return fmt.Sprintf("%c%07d-0000-4000-8000-%012d", prefix, n, n)
}

type musicRowsAnswer struct {
	Kid    bool        `json:"kid"`
	Cold   bool        `json:"cold"`
	Genres []string    `json:"genres"`
	Hero   *MusicCard  `json:"hero"`
	Alts   []MusicCard `json:"hero_alts"`
	Rows   []MusicRow  `json:"rows"`
}

func (m musicRowsAnswer) row(id string) *MusicRow {
	for i := range m.Rows {
		if m.Rows[i].ID == id || m.Rows[i].Type == id {
			return &m.Rows[i]
		}
	}
	return nil
}
