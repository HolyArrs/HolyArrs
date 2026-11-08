package music

import (
	"context"
	"sort"
	"strings"
	"time"

	musicscore "github.com/HolyArrs/holyarrs/app/backend/internal/music/score"
)

// risingGenres is how many of the member's top G genres Rising reads.
const risingGenres = 5

// risingRow is Explore → Music row 13 (plan 22 October 2026 §2.6, Phase 6):
// albums climbing a genre chart, from artists whose first release is at
// most 3 years old, in the member's top-5 G genres; 0.4·G + 0.4·ΔR +
// 0.2·F60. Hidden while any of those genre charts has fewer than 7 days
// of kept ranks. Items with no rank gain, or an artist with no known
// first release, are left out.
func (h *Handlers) risingRow(ctx context.Context, region string, now time.Time, in personalIn) {
	type gv struct {
		g string
		v float64
	}
	var gs []gv
	for g, v := range in.G {
		if g != "" && v > 0 {
			gs = append(gs, gv{g, v})
		}
	}
	sort.Slice(gs, func(i, j int) bool {
		if gs[i].v != gs[j].v {
			return gs[i].v > gs[j].v
		}
		return gs[i].g < gs[j].g
	})
	type cand struct {
		c      MusicCard
		g, dr  float64
		f60    float64
		genre  string
		artist string
	}
	var list []cand
	got := map[string]bool{}
	var artists []string
	charts := 0
	for _, x := range gs[:min(risingGenres, len(gs))] {
		slug := ""
		for _, t := range in.tiles {
			if musicscore.ItemGenre(map[string]float64{x.g: 1}, []string{t.Name, t.Slug}) > 0 {
				slug = t.Slug
				break
			}
		}
		if slug == "" {
			continue
		}
		chart := h.musicGenreChartOf(ctx, region, slug, now)
		if !musicscore.RanksReady(albumRanks(chart)) {
			return // ΔR undefined: the whole row stays hidden
		}
		charts++
		L := len(chart)
		for _, it := range chart {
			dr := musicscore.RankGain(*it.RankAgo, it.Rank, L)
			c := in.chartCard(it)
			if dr <= 0 || c.MBID == "" || got[c.MBID] || in.hadAlbum[c.MBID] || c.ArtistMBID == "" || in.hadMBID[c.ArtistMBID] {
				continue
			}
			got[c.MBID] = true
			c.Note = climbNote(*it.RankAgo, it.Rank)
			list = append(list, cand{c, x.v, dr, musicscore.Freshness(it.Date, 60, now), x.g, c.ArtistMBID})
			artists = append(artists, c.ArtistMBID)
		}
	}
	if charts == 0 {
		return
	}
	data := h.musicArtistDataOf(ctx, artists, now)
	var cards []MusicCard
	var sc []musicscore.Scored
	for _, x := range list {
		d := data[x.artist]
		if d == nil {
			continue
		}
		var dates []string
		for _, al := range d.Albums {
			if strings.EqualFold(firstOr(al.Artists), x.artist) {
				dates = append(dates, al.Date)
			}
		}
		if !musicscore.NewArtist(dates, now) {
			continue
		}
		cards = append(cards, x.c)
		sc = append(sc, in.scored(x.c, musicscore.ScoreRising(x.g, x.dr, x.f60), in.affMBID[x.artist], x.g, x.genre))
	}
	in.add(MusicRow{ID: "rising", Type: "rising", Title: "Rising in your genres", Reason: "Artists with a first release in the last 3 years, moving up your genres' charts this week", Shape: "square"},
		musicscore.Row{Prior: musicscore.PriorPersonal}, cards, sc, true, true)
}

func firstOr(l []string) string {
	if len(l) == 0 {
		return ""
	}
	return l[0]
}
