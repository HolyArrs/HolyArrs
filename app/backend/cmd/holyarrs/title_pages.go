package main

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
)

// newTitlePages builds the metadata title-page routes on this API's age
// limits, Explore cache and music answers.
func (a *handlers) newTitlePages() *metadata.TitlePages {
	return &metadata.TitlePages{
		Store: a.s.Store,
		Server: func() metadata.ServerReads {
			return metadata.ServerReads{Art: a.s.ExtraArt, Ratings: a.s.ExtraRatings, WikiTitle: a.s.WikiTitle,
				WikiPerson: a.s.WikiPerson, AwardWinners: a.s.AwardWinners, ArtLink: a.serverArtLink}
		},
		Ages:    titleAges{a},
		Explore: titleExplore{a},
		Music:   titleMusic{a},
		PartOn: func(kind string) bool {
			p, _ := parts.ForTitleKind(kind)
			return a.partIsOn(p)
		},
	}
}

// titleAges gives the title pages the asker's Kid limit and the Library's
// ratings.
type titleAges struct{ a *handlers }

func (t titleAges) Max(r *http.Request) string { return t.a.kidMax(r) }

func (t titleAges) Limited(w http.ResponseWriter, r *http.Request) bool {
	return t.a.disc.KidLimited(w, r)
}

func (t titleAges) LibraryRating(ctx context.Context, kind, tmdbID string) (string, bool) {
	return t.a.libraryRating(ctx, kind, tmdbID)
}

// titleExplore gives the title pages the shared Explore cache.
type titleExplore struct{ a *handlers }

// Details reads a title's details through the discover cache, fetching
// them when missing or old.
func (t titleExplore) Details(ctx context.Context, kind, id string) (metadata.Details, error) {
	return discover.TitleDetails(ctx, &t.a.discover, t.a.s.Metadata, kind, id)
}

func (t titleExplore) Caching(w http.ResponseWriter, r *http.Request, cache string) {
	t.a.disc.Caching(w, r, cache)
}

// titleMusic gives the title pages the music answers and artist pictures.
type titleMusic struct{ a *handlers }

func (t titleMusic) WriteJSON(w http.ResponseWriter, r *http.Request, v any) {
	t.a.writeMusicJSON(w, r, v)
}

func (t titleMusic) ArtistPicture(ctx context.Context, kind, mbid, size string) (string, bool) {
	if _, _, ok := t.a.musicTrendArtFile(ctx, kind, mbid, size); ok {
		return music.ArtPrefix + kind + "/" + mbid, true
	}
	return "", false
}
