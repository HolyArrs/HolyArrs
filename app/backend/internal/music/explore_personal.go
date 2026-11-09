package music

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	musicscore "github.com/HolyArrs/holyarrs/app/backend/internal/music/score"
)

func (h *Handlers) personalMusicRows(ctx context.Context, member int64, region, date string, lib *musicLib, acts *musicActs, now time.Time, in personalIn) {
	today := dayAt(now, 0)
	asked := func(mbid string) bool { return mbid != "" && acts.albumState[mbid] != "" }

	// Seeds: the top-10 A artists (ties by the day's hash).
	type seed struct {
		ar musicLibArtist
		a  float64
	}
	var seeds []seed
	for _, ar := range lib.artists {
		if ar.mbid != "" && in.aff[ar.id] > 0 {
			seeds = append(seeds, seed{ar, in.aff[ar.id]})
		}
	}
	sort.SliceStable(seeds, func(i, j int) bool {
		if seeds[i].a != seeds[j].a {
			return seeds[i].a > seeds[j].a
		}
		return musicscore.Tie(member, date, seeds[i].ar.mbid) < musicscore.Tie(member, date, seeds[j].ar.mbid)
	})
	seeds = seeds[:min(musicSeedsTop, len(seeds))]
	var seedIDs []string
	for _, s := range seeds {
		seedIDs = append(seedIDs, s.ar.mbid)
	}
	sim := h.musicSimilarOf(ctx, seedIDs, now)
	// MusicBrainz relations (members, the artist's own groups, side
	// projects, charity and collective credits) are not taste: they only
	// rule artists out. S comes from ListenBrainz similarity alone, and
	// never names an artist already in the Library (had or followed).
	var libIDs []string
	for _, ar := range lib.artists {
		if ar.mbid != "" {
			libIDs = append(libIDs, ar.mbid)
		}
	}
	related := map[string]bool{}
	for _, l := range h.linksOf(libIDs) {
		for _, x := range l {
			related[strings.ToLower(x.MBID)] = true
		}
	}
	// S per seed and artist: ListenBrainz (normalised).
	S := map[string]map[string]float64{}
	names := map[string]string{}
	for _, s := range seeds {
		m := map[string]float64{}
		for _, x := range sim[s.ar.mbid] {
			id := strings.ToLower(x.MBID)
			if !nearTasteOK(id, x.Name, s.ar.mbid, lib.byMBID, related) {
				continue
			}
			m[id] = musicscore.Similarity(x.S, false)
			names[id] = x.Name
		}
		S[s.ar.mbid] = m
	}

	// Similar artists not had: their data (top album, genres) and listeners.
	var pool []string
	inPool := map[string]bool{}
	for _, s := range seeds {
		for id := range S[s.ar.mbid] {
			if !in.hadMBID[id] && !inPool[id] {
				inPool[id] = true
				pool = append(pool, id)
			}
		}
	}
	sort.Strings(pool)
	listeners := h.musicListenersOf(ctx, pool, now)
	// The Since-you-have seeds, logged so tomorrow picks others.
	yesterday := map[string]bool{}
	for _, id := range h.exploreCache.seedLog.get(member, dayBefore(date)) {
		yesterday[id] = true
	}
	var since []seed
	for _, s := range seeds {
		if !yesterday[s.ar.mbid] && len(since) < 2 {
			since = append(since, s)
		}
	}
	var sinceIDs []string
	for _, s := range since {
		sinceIDs = append(sinceIDs, s.ar.mbid)
	}
	h.exploreCache.seedLog.put(member, date, sinceIDs)
	pOf := func(ids []string) map[string]float64 {
		var top int64
		for _, id := range ids {
			top = max(top, in.albumListeners[id])
		}
		out := map[string]float64{}
		for _, id := range ids {
			if p, ok := in.chartP[id]; ok {
				out[id] = p
			} else {
				out[id] = musicscore.Uncharted(musicscore.Lnorm(in.albumListeners[id], top))
			}
		}
		return out
	}

	// Row 10: Finish the set, artists with at least half their studio
	// albums had; fewest missing first, then A.
	{
		type fin struct {
			ar      musicLibArtist
			missing int
			al      musicLibAlbum
		}
		missingOf := map[int64][]musicLibAlbum{}
		for _, al := range lib.albums {
			if al.kind == "album" && !al.secondary && al.date != "" && al.date <= today && !lib.hadParts[al.partID] && !asked(al.mbid) {
				missingOf[al.titleID] = append(missingOf[al.titleID], al)
			}
		}
		var list []fin
		for _, ar := range lib.artists {
			n := in.released[ar.id]
			miss := missingOf[ar.id]
			had := 0
			for _, al := range lib.albums {
				if al.titleID == ar.id && al.kind == "album" && !al.secondary && al.date != "" && al.date <= today && lib.hadParts[al.partID] {
					had++
				}
			}
			if n == 0 || len(miss) == 0 || had*2 < n {
				continue
			}
			sort.SliceStable(miss, func(i, j int) bool { return miss[i].date > miss[j].date })
			list = append(list, fin{ar, len(miss), miss[0]})
		}
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].missing != list[j].missing {
				return list[i].missing < list[j].missing
			}
			if in.aff[list[i].ar.id] != in.aff[list[j].ar.id] {
				return in.aff[list[i].ar.id] > in.aff[list[j].ar.id]
			}
			return musicscore.Tie(member, date, list[i].ar.mbid) < musicscore.Tie(member, date, list[j].ar.mbid)
		})
		var cards []MusicCard
		var sc []musicscore.Scored
		for i, f := range list {
			c := in.albumCard(f.al)
			c.Note = fmt.Sprintf("%s · %d left", f.ar.name, f.missing)
			cards = append(cards, c)
			sc = append(sc, in.scored(c, 1-float64(i)/float64(len(list)), in.aff[f.ar.id], 0, ""))
		}
		in.add(MusicRow{ID: "finish_set", Type: "finish_set", Title: "Finish the set", Reason: "Artists you have most of: the studio albums still missing", Shape: "square"},
			musicscore.Row{Prior: musicscore.PriorPersonal}, cards, sc, false, false)
	}

	// Row 11: Artists near your taste, norm(Σ A_i·S_i) + 0.2·P, capped 1.
	{
		sum := map[string]float64{}
		for _, s := range seeds {
			for id, v := range S[s.ar.mbid] {
				if !in.hadMBID[id] {
					sum[id] += s.a * v
				}
			}
		}
		ids := make([]string, 0, len(sum))
		for id := range sum {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		vals := make([]float64, len(ids))
		var top int64
		for i, id := range ids {
			vals[i] = sum[id]
			top = max(top, listeners[id])
		}
		normed := musicscore.Norm(vals)
		var cards []MusicCard
		var sc []musicscore.Scored
		for i, id := range ids {
			_, charted := in.pcBest[id]
			p := musicscore.ArtistPopularity(in.pcBest[id], charted, musicscore.Lnorm(listeners[id], top))
			c := MusicCard{Key: "artist:" + id, Kind: "artist", MBID: id, Name: names[id], Artist: names[id], ArtistMBID: id, Image: "/api/v1/music-trending/art/artist/" + id, Genres: []string{}}
			if c.Name == "" {
				continue
			}
			if j, ok := lib.byMBID[id]; ok {
				c.TitleID = lib.artists[j].id
			}
			cards = append(cards, c)
			sc = append(sc, in.scored(c, musicscore.ScoreNearTaste(normed[i], p), in.affMBID[id], 0, ""))
		}
		in.add(MusicRow{ID: "near_taste", Type: "near_taste", Title: "Artists near your taste", Reason: "Close to the artists you have and follow", Shape: "round"},
			musicscore.Row{Prior: musicscore.PriorPersonal, Min: 5, ArtistRow: true}, cards, sc, true, true)
	}

	// Row 12: genre shelves for the top-3 G genres, 0.5·P + 0.3·G + 0.2·M.
	{
		type gv struct {
			g string
			v float64
		}
		var gs []gv
		for g, v := range in.G {
			gs = append(gs, gv{g, v})
		}
		sort.Slice(gs, func(i, j int) bool {
			if gs[i].v != gs[j].v {
				return gs[i].v > gs[j].v
			}
			return gs[i].g < gs[j].g
		})
		for _, x := range gs[:min(3, len(gs))] {
			if x.g == "" {
				continue
			}
			name, slug := x.g, ""
			for _, t := range in.tiles {
				if musicscore.ItemGenre(map[string]float64{x.g: 1}, []string{t.Name, t.Slug}) > 0 {
					name, slug = t.Name, t.Slug
					break
				}
			}
			var cards []MusicCard
			var sc []musicscore.Scored
			var ids []string
			type pending struct {
				c MusicCard
				m float64
			}
			var list []pending
			if slug != "" {
				for _, it := range h.musicGenreChartOf(ctx, region, slug, now) {
					c := in.chartCard(it)
					if in.hadAlbum[c.MBID] {
						continue
					}
					list = append(list, pending{c, musicscore.Bool(!asked(c.MBID))})
					ids = append(ids, c.MBID)
				}
			}
			for _, al := range lib.albums {
				ar := in.artistOf[al.titleID]
				if in.shares[ar.mbid][x.g] <= 0 || al.kind != "album" || al.secondary || al.date == "" || al.date > today || lib.hadParts[al.partID] {
					continue
				}
				list = append(list, pending{in.albumCard(al), musicscore.Bool(!asked(al.mbid))})
				ids = append(ids, al.mbid)
			}
			P := pOf(ids)
			for _, p := range list {
				cards = append(cards, p.c)
				sc = append(sc, in.scored(p.c, musicscore.ScoreGenreShelf(P[p.c.MBID], x.v, p.m), in.affMBID[p.c.ArtistMBID], x.v, ""))
			}
			if name == "" {
				continue
			}
			label := musicscore.Label(name)
			if slug == "" {
				label = musicscore.GenreName(x.g)
			}
			in.add(MusicRow{ID: "shelf:" + x.g, Type: "genre_shelf", Title: label + " for you", Reason: "One of your top genres: its chart and your artists in it", Shape: "square"},
				musicscore.Row{Prior: musicscore.PriorMixed}, cards, sc, true, false)
		}
	}

	// Row 13 (Phase 6, flag music_rows_v6): Rising in your genres.
	h.risingRow(ctx, region, now, in)

	// Row 14: Guest spots, release groups crediting a Library artist who is
	// not the primary artist, not had; 0.6·A + 0.4·P.
	{
		type guest struct {
			lib musicLibArtist
			al  metadata.MusicTopAlbum
		}
		var list []guest
		var ids []string
		got := map[string]bool{}
		for _, ar := range lib.artists {
			d := in.data[ar.mbid]
			if d == nil {
				continue
			}
			for _, al := range d.Albums {
				if !musicGuestTypes[al.Type] || len(al.Artists) < 2 || al.Artists[0] == ar.mbid || in.hadAlbum[al.MBID] || asked(al.MBID) || got[al.MBID] {
					continue
				}
				credited := false
				for _, x := range al.Artists[1:] {
					credited = credited || x == ar.mbid
				}
				if !credited {
					continue
				}
				got[al.MBID] = true
				list = append(list, guest{ar, al})
				ids = append(ids, al.MBID)
			}
		}
		P := pOf(ids)
		var cards []MusicCard
		var sc []musicscore.Scored
		for _, g := range list {
			al := g.al
			c := MusicCard{Key: "album:" + al.MBID, Kind: "album", MBID: al.MBID, Name: al.Title, ArtistMBID: al.Artists[0], Date: al.Date, Image: "/api/v1/music-trending/art/album/" + al.MBID, Note: "With " + g.lib.name, Genres: []string{}}
			if len(al.ArtistNames) > 0 && al.ArtistNames[0] != "" {
				c.Artist = al.ArtistNames[0]
				c.Note = c.Artist + " · with " + g.lib.name
			}
			cards = append(cards, c)
			sc = append(sc, in.scored(c, musicscore.ScoreGuest(in.aff[g.lib.id], P[al.MBID]), in.aff[g.lib.id], 0, in.primary[g.lib.mbid]))
		}
		in.add(MusicRow{ID: "guest_spots", Type: "guest_spots", Title: "Guest spots", Reason: "Releases your artists appear on", Shape: "square"},
			musicscore.Row{Prior: musicscore.PriorPersonal}, cards, sc, true, true)
	}

	h.musicPlanRows(ctx, member, date, lib, acts, now, in, yesterday)
}
