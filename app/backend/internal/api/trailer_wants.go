package api

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/explore"
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
	t.KindsOn, t.Wants, t.Videos = a.kindsOn, trailerHost{a}, artistVideoHooks{a}
	return t
}

// Trailers is the trailer reader, pass and routes (for the health check
// and the job loops).
func (a *API) Trailers() *metadata.Trailers { return a.trailers }

// trailerHost gives the trailers the look settings, who asks, and the
// titles Explore shows.
type trailerHost struct{ a *API }

func (h trailerHost) AutoplayTrailers(ctx context.Context) bool {
	return h.a.uiSettings(ctx).AutoplayTrailers
}

func (h trailerHost) UserID(r *http.Request) int64 { return h.a.person(r).UserID }

func (h trailerHost) PassTitles(ctx context.Context) [][2]string { return h.a.trailerPassTitles(ctx) }

func (h trailerHost) PassLanguages(ctx context.Context) []string {
	return h.a.trailerPassLanguages(ctx)
}

func (h trailerHost) Warm(lang string, titles [][2]string) { h.a.warmTitles(lang, titles) }

// artistVideoHooks gives the trailers the artists' music videos.
type artistVideoHooks struct{ a *API }

func (v artistVideoHooks) On() bool { return v.a.artistVideosOn() }

func (v artistVideoHooks) Asks(ctx context.Context, now time.Time) []string {
	return v.a.artistAsks(ctx, now)
}

func (v artistVideoHooks) Due(ctx context.Context, mbid string, now time.Time) (bool, bool, bool) {
	return v.a.artistVideoDue(ctx, mbid, now)
}

func (v artistVideoHooks) Pass(ctx context.Context, mbid string, renew bool) error {
	return v.a.passArtistVideo(ctx, mbid, renew)
}

func (v artistVideoHooks) SaveAsks(ctx context.Context) { v.a.saveArtistAsks(ctx) }

func (v artistVideoHooks) Ready(r *http.Request, mbid string) (any, bool) {
	e, kept := v.a.keptVideos(r.Context(), mbid)
	if !kept {
		return nil, false
	}
	vid, s, ready := v.a.readyVideo(r.Context(), e.list)
	if !ready {
		return nil, false
	}
	view, ok := v.a.videoView(r, vid, s)
	return view, ok
}

func (v artistVideoHooks) Note(ctx context.Context, mbids []string) { v.a.noteArtistVideos(ctx, mbids) }

// fillTrailerKeys sets trailer_key on Explore cards whose key is kept.
func (a *API) fillTrailerKeys(r *http.Request, items []discoverItemView) {
	lang := metadata.TrailerLanguage(r)
	// Every listed title is warmed ahead (details, extras, trailer key), so
	// opening or hovering it never makes the first outside call.
	warm := make([][2]string, 0, len(items))
	for _, v := range items {
		if v.Kind == "movie" || v.Kind == "show" {
			warm = append(warm, [2]string{v.Kind, v.TMDBID})
		}
	}
	a.warmTitles(lang, warm)
	if a.kidMax(r) != "" {
		return
	}
	for i := range items {
		if v := &items[i]; (v.Kind == "movie" || v.Kind == "show") && v.TMDBID != "" {
			v.TrailerKey = a.trailers.KnownKey(r.Context(), v.Kind, v.TMDBID, lang)
		}
	}
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
	var keys []poolKey
	for rows.Next() {
		var k poolKey
		if rows.Scan(&k.member, &k.sw) == nil {
			keys = append(keys, k)
		}
	}
	rows.Close()
	now := a.exploreNow()
	day := explore.Day(now)
	var heroes [][2]string
	var cards []explore.Item
	for _, k := range keys {
		m, err := a.loadMember(ctx, k.member)
		if err != nil || m.kid {
			continue
		}
		// Today's pool, else the newest older one, as Explore shows it.
		pool, _, err := a.readPool(ctx, k.member, k.sw, day, false)
		if err == nil && pool == nil {
			pool, _, err = a.readPool(ctx, k.member, k.sw, day, true)
		}
		if err != nil || pool == nil || pool.Kid {
			continue
		}
		pg := a.poolPage(*pool, k.member, k.sw, m.region, explore.Moment{Local: now.Local()})
		heroes = append(heroes, exploreTrailerTitles(heroCandidates(pg, k.sw == "all"), nil)...)
		for _, row := range append(append([]explore.Row{}, pg.Head...), pg.Tail...) {
			if row.Type == explore.TypeTopServices {
				// The page fills this row itself, from the kept lists.
				cards = append(cards, a.keptServiceTop(strings.ToUpper(m.region), k.sw)...)
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

// trailerPassLanguages are the languages pages asked for lately (most card
// pictures kept first, then the latest asked), at most metadata.TrailerPassLangs.
// Before any page asked (just after a start) the card pictures' languages
// count instead; "en" when none.
func (a *API) trailerPassLanguages(ctx context.Context) []string {
	var asked []string
	at := map[string]time.Time{}
	c := &a.explore.cards
	c.mu.Lock()
	for l, t := range c.langs {
		if time.Since(t) < cardArtLangTTL {
			asked = append(asked, l)
			at[l] = t
		}
	}
	c.mu.Unlock()
	slices.SortFunc(asked, func(x, y string) int { return at[y].Compare(at[x]) })
	var out []string
	add := func(l string) {
		if len(out) < metadata.TrailerPassLangs && len(l) == 2 && metadata.ValidTrailerLanguage(l) && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	if rows, err := a.s.Store.Read().QueryContext(ctx, "SELECT lang FROM card_art GROUP BY lang ORDER BY count(*) DESC, lang LIMIT 20"); err == nil {
		for rows.Next() {
			var l string
			// The filler keeps English pictures for every title; they count
			// only when a page asked for English.
			if rows.Scan(&l) == nil && (len(asked) == 0 || at[l] != (time.Time{})) {
				add(l)
			}
		}
		rows.Close()
	}
	for _, l := range asked {
		add(l)
	}
	if len(out) == 0 {
		out = []string{"en"}
	}
	return out
}

// artistVideosOn says whether artist music videos can play here: music on,
// a music source and a metadata server that lists videos.
func (a *API) artistVideosOn() bool {
	p, _ := parts.ForTitleKind("artist")
	return a.partIsOn(p) && a.s.Library != nil && a.s.Library.Artists != nil && a.s.ArtistVideos != nil
}
