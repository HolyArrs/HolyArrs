package music

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	musicscore "github.com/HolyArrs/holyarrs/app/backend/internal/music/score"
)

// Explore → Music, flag music_rows_v7 (plan 23 October 2026): the v3 rows plus
// mixes, Because you have ×4, Deep cuts, Turning N this month and Together
// with, ordered once a day into a plan of up to 30 rows and served 6 at a
// time behind a signed cursor. Off keeps the v3/v6 page.

// musicCand is one build's raw candidate rows with their cards.
type musicCand struct {
	rows  []musicscore.Row
	metas map[string]MusicRow
	cards map[string]map[string]MusicCard
	seen  map[string]bool
}

// musicHead is the plan's fixed head (the Spotlight sits above it).
var musicHead = []string{"new_from", "genres"}

// musicPlanMax is each family's row maximum (others: 1).
var musicPlanMax = map[string]int{"mix": musicscore.MixMax, "because": 4, "genre_shelf": 3, "together": 2,
	// Row template families (music_lists_v1 engine).
	"tpl_yr": 1, "tpl_ch": 5, "tpl_co": 3, "tpl_po": 3, "tpl_nw": 5, "tpl_cl": 5, "tpl_de": 3, "tpl_su": 4, "tpl_cr": 6, "tpl_se": 3, "tpl_ar": 4, "tpl_vi": 1, "tpl_di": 3, "tpl_pe": 6}

// musicFew is the row count a household answers in one response.
const musicFew = 12

// musicFamily answers a row's family and whether it is a discovery row.
func musicFamily(m MusicRow) (string, bool) {
	if strings.HasPrefix(m.Type, "tpl_") {
		// Row template families; discovery rows count as such.
		return m.Type, m.Type == "tpl_di"
	}
	switch m.Type {
	case "top_albums", "mix", "because", "near_taste", "rising", "together", "since_have":
		return m.Type, true
	}
	return m.Type, false
}

// chartExplicit marks the charted albums with explicit lyrics.
func chartExplicit(l []metadata.MusicChartItem) map[string]bool {
	out := map[string]bool{}
	for _, it := range l {
		if it.Explicit && it.MBID != "" {
			out[strings.ToLower(it.MBID)] = true
		}
	}
	return out
}

// musicPlanRows adds the v7 families to the build (personal households).
// yesterday holds yesterday's seeds.
func (h *Handlers) musicPlanRows(ctx context.Context, member int64, date string, lib *musicLib, acts *musicActs, now time.Time, in personalIn, yesterday map[string]bool) {
	today := dayAt(now, 0)
	asked := func(mbid string) bool { return mbid != "" && acts.albumState[mbid] != "" }
	nameOf := map[string]string{}
	var libIDs []string
	for _, ar := range lib.artists {
		if ar.mbid != "" {
			nameOf[ar.mbid] = ar.name
			libIDs = append(libIDs, ar.mbid)
		}
	}
	related := map[string]bool{}
	links := h.linksOf(libIDs)
	for _, l := range links {
		for _, x := range l {
			related[strings.ToLower(x.MBID)] = true
		}
	}
	// P of album candidates of one row: charted P, else min(0.3, 0.3·Lnorm).
	pOf := func(ids []string, listeners map[string]int64) map[string]float64 {
		var mx int64
		for _, id := range ids {
			mx = max(mx, listeners[id])
		}
		out := map[string]float64{}
		for _, id := range ids {
			if p, ok := in.chartP[id]; ok {
				out[id] = p
			} else {
				out[id] = musicscore.Uncharted(musicscore.Lnorm(listeners[id], mx))
			}
		}
		return out
	}
	albumOf := func(id, name string, al metadata.MusicTopAlbum) MusicCard {
		c := MusicCard{Key: "album:" + al.MBID, Kind: "album", MBID: al.MBID, Name: al.Title, Artist: name, ArtistMBID: id, Date: al.Date, Image: "/api/v1/music-trending/art/album/" + al.MBID, Explicit: in.explicit[al.MBID], Genres: []string{}}
		if len(al.ArtistNames) > 0 && al.ArtistNames[0] != "" {
			c.Artist = al.ArtistNames[0]
		}
		if i, ok := lib.byMBID[id]; ok {
			c.TitleID = lib.artists[i].id
		}
		return c
	}
	// topNotHad is an artist's best-known studio album not had or asked.
	topNotHad := func(id string, d *metadata.MusicArtistData) *metadata.MusicTopAlbum {
		if d == nil {
			return nil
		}
		for i, al := range d.Albums {
			if al.Type == "album" && !nonStudioTitle(al.Title) && len(al.Artists) > 0 && al.Artists[0] == id && !in.hadAlbum[al.MBID] && !asked(al.MBID) {
				return &d.Albums[i]
			}
		}
		return nil
	}

	// Taste set T and its similarity (symmetric max).
	var set []musicscore.MixArtist
	for _, ar := range lib.artists {
		if ar.mbid != "" && in.aff[ar.id] > 0 {
			set = append(set, musicscore.MixArtist{ID: ar.mbid, A: in.aff[ar.id], Share: in.shares[ar.mbid]})
		}
	}
	sort.SliceStable(set, func(i, j int) bool {
		if set[i].A != set[j].A {
			return set[i].A > set[j].A
		}
		return set[i].ID < set[j].ID
	})
	set = set[:min(musicscore.MixTaste, len(set))]
	var tIDs []string
	for _, x := range set {
		tIDs = append(tIDs, x.ID)
	}
	sim := h.musicSimilarOf(ctx, tIDs, now)
	pair := map[[2]string]float64{}
	for s, l := range sim {
		for _, x := range l {
			id := strings.ToLower(x.MBID)
			k := [2]string{min(s, id), max(s, id)}
			pair[k] = max(pair[k], x.S)
		}
	}
	S := func(x, y string) float64 { return pair[[2]string{min(x, y), max(x, y)}] }
	seedUsed := map[string]bool{}
	// byA is the taste set as Library artists, highest A first: the list
	// Because and Together pick their next unused artist from.
	byA := make([]musicLibArtist, 0, len(set))
	for _, x := range set {
		byA = append(byA, lib.artists[lib.byMBID[x.ID]])
	}
	mixes := musicscore.Clusters(set, S)
	// mixCands is one mix's seeds and its candidates by ΣS (top 40).
	mixCands := func(m musicscore.Mix) ([]string, map[string]float64, map[string]float64, map[string]string, []string) {
		seeds := musicscore.MixSeeds(m, member, date)
		sum, smax, names := map[string]float64{}, map[string]float64{}, map[string]string{}
		for _, sd := range seeds {
			for _, x := range sim[sd] {
				id := strings.ToLower(x.MBID)
				if x.S < musicSimilarS || id == "" || id == sd || collectiveName.MatchString(x.Name) {
					continue
				}
				sum[id] += x.S
				smax[id] = max(smax[id], x.S)
				names[id] = x.Name
			}
		}
		ids := make([]string, 0, len(sum))
		for id := range sum {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool {
			if sum[ids[i]] != sum[ids[j]] {
				return sum[ids[i]] > sum[ids[j]]
			}
			return ids[i] < ids[j]
		})
		return seeds, sum, smax, names, ids[:min(40, len(ids))]
	}
	// One read of artist data for every family's candidates (layer 1 keeps
	// it; the per-row reads below are then cache hits).
	{
		var want []string
		for _, m := range mixes {
			_, _, _, _, ids := mixCands(m)
			want = append(want, ids...)
		}
		for _, ar := range byA[:min(12, len(byA))] {
			for _, x := range sim[ar.mbid] {
				if x.S >= musicSimilarS {
					want = append(want, strings.ToLower(x.MBID))
				}
			}
		}
		for _, ar := range byA {
			for _, l := range links[ar.mbid] {
				if musicscore.Rel(l.Type) > 0 {
					want = append(want, strings.ToLower(l.MBID))
				}
			}
		}
		sort.Strings(want)
		h.musicArtistDataOf(ctx, want, now)
	}

	// Mixes (§2).
	for mi, m := range mixes {
		seeds, sum, smax, names, ids := mixCands(m)
		for _, sd := range seeds {
			seedUsed[sd] = true
		}
		data := h.musicArtistDataOf(ctx, ids, now)
		vals := make([]float64, len(ids))
		for i, id := range ids {
			vals[i] = sum[id]
		}
		normed := musicscore.Norm(vals)
		type cand struct {
			id  string
			al  *metadata.MusicTopAlbum
			n   float64
			cos float64
			sh  map[string]float64
			top string
		}
		var list []cand
		var alIDs []string
		listeners := map[string]int64{}
		for i, id := range ids {
			al := topNotHad(id, data[id])
			if al == nil {
				continue
			}
			sh, g := in.sharesOf(data[id])
			list = append(list, cand{id, al, normed[i], musicscore.Cos(sh, m.Centroid), sh, g})
			alIDs = append(alIDs, al.MBID)
			listeners[al.MBID] = al.Listeners
		}
		P := pOf(alIDs, listeners)
		var cards []MusicCard
		var sc []musicscore.Scored
		for _, x := range list {
			score := musicscore.ScoreMix(x.n, x.cos, P[x.al.MBID], musicscore.Freshness(x.al.Date, 60, now))
			if !musicscore.MixKeeps(score, smax[x.id], x.cos) {
				continue
			}
			c := albumOf(x.id, names[x.id], *x.al)
			g := 0.0
			for k := range x.sh {
				g = max(g, in.G[k])
			}
			cards = append(cards, c)
			sc = append(sc, in.scored(c, score, in.affMBID[x.id], g, x.top))
		}
		var from []string
		for _, sd := range seeds {
			from = append(from, nameOf[sd])
		}
		in.add(MusicRow{ID: "mix:" + m.Lead, Type: "mix", Title: musicscore.MixTitle(m, nameOf[m.Lead]), Reason: "From " + joinNames(from) + " in your Library", Shape: "square"},
			// Mixes keep the member's ΣA order: each later mix's prior is a
			// step lower, so a member's own taste leads.
			musicscore.Row{Prior: musicscore.PriorPersonal - 0.05*float64(mi), Min: 8}, cards, sc, true, true)
	}

	// Because you have {X} ×4: top-A artists, day-rotated among the top 8,
	// never yesterday's seeds or a mix's.
	{
		pick := append([]musicLibArtist{}, byA[:min(8, len(byA))]...)
		sort.SliceStable(pick, func(i, j int) bool {
			return musicscore.Tie(member, date, "because"+pick[i].mbid) < musicscore.Tie(member, date, "because"+pick[j].mbid)
		})
		// Mixes took their seeds first: the list goes on down the A order.
		pick = append(pick, byA[min(8, len(byA)):]...)
		var seeds []musicLibArtist
		var seedIDs []string
		for _, ar := range pick {
			if len(seeds) < 4 && !yesterday[ar.mbid] && !seedUsed[ar.mbid] {
				seeds = append(seeds, ar)
				seedIDs = append(seedIDs, ar.mbid)
				seedUsed[ar.mbid] = true
			}
		}
		h.exploreCache.seedLog.put(member, date, seedIDs)
		bySim := sim
		var want []string
		near := map[string]map[string]float64{}
		names := map[string]string{}
		for _, s := range seeds {
			m := map[string]float64{}
			for _, x := range bySim[s.mbid] {
				id := strings.ToLower(x.MBID)
				if x.S < musicSimilarS || in.hadMBID[id] || !nearTasteOK(id, x.Name, s.mbid, lib.byMBID, related) {
					continue
				}
				m[id] = musicscore.Similarity(x.S, false)
				names[id] = x.Name
			}
			near[s.mbid] = m
			ids := make([]string, 0, len(m))
			for id := range m {
				ids = append(ids, id)
			}
			sort.Slice(ids, func(i, j int) bool {
				if m[ids[i]] != m[ids[j]] {
					return m[ids[i]] > m[ids[j]]
				}
				return ids[i] < ids[j]
			})
			want = append(want, ids[:min(15, len(ids))]...)
		}
		data := h.musicArtistDataOf(ctx, want, now)
		for _, s := range seeds {
			type cand struct {
				id string
				al *metadata.MusicTopAlbum
			}
			var list []cand
			var ids []string
			listeners := map[string]int64{}
			for id := range near[s.mbid] {
				if al := topNotHad(id, data[id]); al != nil {
					list = append(list, cand{id, al})
					ids = append(ids, al.MBID)
					listeners[al.MBID] = al.Listeners
				}
			}
			sort.Slice(list, func(i, j int) bool { return list[i].id < list[j].id })
			P := pOf(ids, listeners)
			var cards []MusicCard
			var sc []musicscore.Scored
			for _, x := range list {
				sh, g0 := in.sharesOf(data[x.id])
				g := 0.0
				for k := range sh {
					g = max(g, in.G[k])
				}
				c := albumOf(x.id, names[x.id], *x.al)
				cards = append(cards, c)
				sc = append(sc, in.scored(c, musicscore.ScoreSinceYouHave(near[s.mbid][x.id], P[x.al.MBID], g), in.affMBID[x.id], g, g0))
			}
			in.add(MusicRow{ID: "because:" + s.mbid, Type: "because", Title: "Because you have " + s.name, Reason: "Artists close to " + s.name + ": their best-known album", Shape: "square"},
				musicscore.Row{Prior: musicscore.PriorPersonal, Seed: s.mbid}, cards, sc, true, true)
		}
	}

	// Deep cuts: studio albums of artists with A ≥ 0.5, not had, not the
	// artist's top 2 by listeners, out more than 2 years.
	{
		cutoff := now.AddDate(-2, 0, 0).Format("2006-01-02")
		byArtist := map[int64][]musicLibAlbum{}
		for _, al := range lib.albums {
			if al.kind == "album" && !al.secondary && al.mbid != "" && al.date != "" && al.date <= today && !nonStudioTitle(al.name) {
				byArtist[al.titleID] = append(byArtist[al.titleID], al)
			}
		}
		var cards []MusicCard
		var sc []musicscore.Scored
		for _, ar := range lib.artists {
			l := byArtist[ar.id]
			if in.aff[ar.id] < 0.5 || len(l) <= 2 {
				continue
			}
			sort.SliceStable(l, func(i, j int) bool {
				x, y := in.albumListeners[l[i].mbid], in.albumListeners[l[j].mbid]
				if x != y {
					return x > y
				}
				return l[i].date < l[j].date
			})
			for r, al := range l {
				if r < 2 || lib.hadParts[al.partID] || asked(al.mbid) || al.date > cutoff {
					continue
				}
				c := in.albumCard(al)
				c.Explicit = in.explicit[al.mbid]
				cards = append(cards, c)
				sc = append(sc, in.scored(c, musicscore.ScoreDeepCut(in.aff[ar.id], musicscore.DeepRankFrac(r+1, len(l)), musicscore.Bool(al.mbid != "")), in.aff[ar.id], 0, in.primary[ar.mbid]))
			}
		}
		in.add(MusicRow{ID: "deep_cuts", Type: "deep_cuts", Title: "Deep cuts", Reason: "Lesser-known studio albums by artists you have most", Shape: "square"},
			musicscore.Row{Prior: musicscore.PriorPersonal}, cards, sc, true, false)
	}

	// Turning N this month: not-had albums of A > 0 artists whose 10th, 20th,
	// 25th or 30th anniversary falls in −7..+30 days (not-had only).
	{
		type cand struct {
			al musicLibAlbum
			n  int
			on time.Time
		}
		var list []cand
		var ids []string
		for _, al := range lib.albums {
			if al.kind != "album" || al.secondary || al.mbid == "" || in.aff[al.titleID] <= 0 || lib.hadParts[al.partID] || asked(al.mbid) {
				continue
			}
			if n, on, ok := musicscore.Anniversary(al.date, now); ok {
				list = append(list, cand{al, n, on})
				ids = append(ids, al.mbid)
			}
		}
		var mx int64
		for _, id := range ids {
			mx = max(mx, in.albumListeners[id])
		}
		var cards []MusicCard
		var sc []musicscore.Scored
		years := map[int]bool{}
		for _, x := range list {
			c := in.albumCard(x.al)
			c.Explicit = in.explicit[x.al.mbid]
			c.Note = fmt.Sprintf("%d years on %s", x.n, x.on.Format("Jan 2"))
			years[x.n] = true
			cards = append(cards, c)
			ar := in.artistOf[x.al.titleID]
			sc = append(sc, in.scored(c, musicscore.ScoreAnniversary(in.aff[x.al.titleID], musicscore.Lnorm(in.albumListeners[x.al.mbid], mx), musicscore.Bool(x.n == 10 || x.n == 20)), in.aff[x.al.titleID], 0, in.primary[ar.mbid]))
		}
		var ns []string
		for _, n := range []int{10, 20, 25, 30} {
			if years[n] {
				ns = append(ns, strconv.Itoa(n))
			}
		}
		title := "Turning " + joinNames(ns) + " this month"
		in.add(MusicRow{ID: "anniversary", Type: "anniversary", Title: title, Reason: "Albums by your artists with a round birthday around now, not in your Library yet", Shape: "square"},
			musicscore.Row{Prior: musicscore.PriorPersonal}, cards, sc, true, false)
	}

	// Together with {X}: members, founders and collaborations of top-A
	// artists; their albums not had.
	{
		made := 0
		for _, s := range byA {
			if made >= 2 {
				break
			}
			if seedUsed[s.mbid] {
				continue
			}
			rel := map[string]float64{}
			names := map[string]string{}
			for _, l := range links[s.mbid] {
				id := strings.ToLower(l.MBID)
				if v := musicscore.Rel(l.Type); v > 0 && id != s.mbid && !collectiveName.MatchString(l.Name) {
					if _, inLib := lib.byMBID[id]; !inLib {
						rel[id] = max(rel[id], v)
						names[id] = l.Name
					}
				}
			}
			if len(rel) == 0 {
				continue
			}
			ids := make([]string, 0, len(rel))
			for id := range rel {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			data := h.musicArtistDataOf(ctx, ids, now)
			type cand struct {
				id string
				al metadata.MusicTopAlbum
			}
			var list []cand
			var alIDs []string
			listeners := map[string]int64{}
			for _, id := range ids {
				d := data[id]
				if d == nil {
					continue
				}
				for _, al := range d.Albums {
					if al.Type == "album" && !nonStudioTitle(al.Title) && len(al.Artists) > 0 && al.Artists[0] == id && !in.hadAlbum[al.MBID] && !asked(al.MBID) {
						list = append(list, cand{id, al})
						alIDs = append(alIDs, al.MBID)
						listeners[al.MBID] = al.Listeners
					}
				}
			}
			P := pOf(alIDs, listeners)
			var cards []MusicCard
			var sc []musicscore.Scored
			for _, x := range list {
				c := albumOf(x.id, names[x.id], x.al)
				c.Note = names[x.id]
				cards = append(cards, c)
				sc = append(sc, in.scored(c, musicscore.ScoreTogether(rel[x.id], P[x.al.MBID], in.aff[s.id]), in.affMBID[x.id], 0, ""))
			}
			if len(cards) < 6 {
				continue
			}
			seedUsed[s.mbid] = true
			made++
			in.add(MusicRow{ID: "together:" + s.mbid, Type: "together", Title: "Together with " + s.name, Reason: "Bands, members and partners of " + s.name, Shape: "square"},
				musicscore.Row{Prior: musicscore.PriorPersonal, Seed: s.mbid}, cards, sc, true, true)
		}
	}
}

// joinNames joins names as "A, B and C".
func joinNames(l []string) string {
	switch len(l) {
	case 0:
		return ""
	case 1:
		return l[0]
	}
	return strings.Join(l[:len(l)-1], ", ") + " and " + l[len(l)-1]
}

// musicCursor is one page's position in a pinned plan.
type musicCursor struct {
	Member  int64
	Date    string
	Region  string
	Version int
	Offset  int
}
