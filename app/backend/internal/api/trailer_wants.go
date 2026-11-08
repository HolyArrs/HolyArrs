package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"

	engine "github.com/HolyArrs/holyarrs/app/backend/internal/discover/rows"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
)

// newTrailers builds the metadata trailers on this API's settings, age
// limits, Explore pools and music videos.
func (a *API) newTrailers() *metadata.Trailers {
	t := &metadata.Trailers{}
	t.Store, t.Version = a.s.Store, a.s.Version
	t.Provider = func() metadata.Provider { return a.s.Metadata }
	t.Pick = func() func(ctx context.Context, kind, id, lang string) (string, error) { return a.s.TrailerPick }
	t.Explore, t.Ages, t.Settings, t.People = titleExplore{a}, titleAges{a}, trailerHost{a}, trailerHost{a}
	t.KindsOn, t.Wants, t.Videos = a.kindsOn, trailerHost{a}, a.mh().VideoHooks()
	return t
}

// Trailers is the trailer reader, pass and routes (for the health check
// and the job loops).
func (a *API) Trailers() *metadata.Trailers { return a.trailers }

// trailerHost gives the trailers the look settings, who asks, and the
// titles Explore shows.
type trailerHost struct{ a *API }

func (h trailerHost) AutoplayTrailers(ctx context.Context) bool {
	return health.AutoplayOn(ctx, h.a.s.Store)
}

func (h trailerHost) UserID(r *http.Request) int64 { return h.a.person(r).UserID }

func (h trailerHost) PassTitles(ctx context.Context) [][2]string { return h.a.trailerPassTitles(ctx) }

func (h trailerHost) PassLanguages(ctx context.Context) []string {
	return h.a.trailerPassLanguages(ctx)
}

func (h trailerHost) Warm(lang string, titles [][2]string) { h.a.warmTitles(lang, titles) }

// fillTrailerKeys sets trailer_key on Explore cards whose key is kept
// (discover.FillTrailerKeys).
func (a *API) fillTrailerKeys(r *http.Request, items []discoverItemView) {
	discover.FillTrailerKeys(r, items, a.kidMax(r) != "", a.warmTitles, a.trailers.KnownKey)
}

// trailerPassTitles are the movies and shows of every stored Explore pool
// (Kid pools left out) as each page shows them: every pool's hero
// candidates first, then every card of its head and tail rows in page order
// (Trending on streaming from its kept lists), each title once and none on
// the Never-add list.
func (a *API) trailerPassTitles(ctx context.Context) [][2]string {
	rows, err := a.s.Store.Read().QueryContext(ctx, "SELECT DISTINCT member_id, type_switch FROM explore_pool ORDER BY member_id, type_switch")
	if err != nil {
		return nil
	}
	var keys []discover.PoolKey
	for rows.Next() {
		var k discover.PoolKey
		if rows.Scan(&k.Member, &k.Switch) == nil {
			keys = append(keys, k)
		}
	}
	rows.Close()
	now := a.explore.Now()
	day := engine.Day(now)
	var heroes [][2]string
	var cards []engine.Item
	for _, k := range keys {
		m, err := a.explore.LoadMember(ctx, k.Member)
		if err != nil || m.Kid {
			continue
		}
		// Today's pool, else the newest older one, as Explore shows it.
		pool, _, err := a.explore.ReadPool(ctx, k.Member, k.Switch, day, false)
		if err == nil && pool == nil {
			pool, _, err = a.explore.ReadPool(ctx, k.Member, k.Switch, day, true)
		}
		if err != nil || pool == nil || pool.Kid {
			continue
		}
		pg := a.poolPage(*pool, k.Member, k.Switch, m.Region, engine.Moment{Local: now.Local()})
		heroes = append(heroes, discover.ExploreTrailerTitles(discover.HeroCandidates(pg, k.Switch == "all"), nil)...)
		for _, row := range append(append([]engine.Row{}, pg.Head...), pg.Tail...) {
			if row.Type == engine.TypeTopServices {
				// The page fills this row itself, from the kept lists.
				cards = append(cards, a.keptServiceTop(strings.ToUpper(m.Region), k.Switch)...)
				continue
			}
			cards = append(cards, row.Items...)
		}
	}
	items := make([]metadata.DiscoverItem, 0, len(heroes)+len(cards))
	for _, t := range heroes {
		items = append(items, metadata.DiscoverItem{Kind: t[0], TMDBID: t[1]})
	}
	for _, it := range cards {
		items = append(items, metadata.DiscoverItem{Kind: it.Kind, TMDBID: it.TMDBID})
	}
	items, err = a.dropNeverAdd(ctx, items)
	if err != nil {
		return nil
	}
	var out [][2]string
	seen := map[[2]string]bool{}
	for _, it := range items {
		t := [2]string{it.Kind, it.TMDBID}
		if (it.Kind == "movie" || it.Kind == "show") && it.TMDBID != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// trailerPassLanguages are the languages the trailer pass fills
// (discover.TrailerPassLanguages).
func (a *API) trailerPassLanguages(ctx context.Context) []string {
	return discover.TrailerPassLanguages(ctx, a.s.Store, a.cardArt().LangsAsked())
}

// artistVideosOn says whether artist music videos can play here: music on,
// a music source and a metadata server that lists videos.
func (a *API) artistVideosOn() bool {
	p, _ := parts.ForTitleKind("artist")
	return a.partIsOn(p) && a.s.Library != nil && a.s.Library.Artists != nil && a.s.ArtistVideos != nil
}
