package music

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	musicscore "github.com/HolyArrs/holyarrs/app/backend/internal/music/score"
)

// Explore → Music, flag music_rows_v7 (plan 2026-10-23): the v3 rows plus
// mixes, Because you have ×4, Deep cuts, Turning N this month and Together
// with, ordered once a day into a plan of up to 30 rows and served 6 at a
// time behind a signed cursor. Off keeps the v3/v6 page.

// musicV7Cand is one build's raw candidate rows with their cards.
type musicV7Cand struct {
	rows  []musicscore.Row
	metas map[string]MusicRow
	cards map[string]map[string]MusicCard
	seen  map[string]bool
}

// musicV7Head is the plan's fixed head (the Spotlight sits above it).
var musicV7Head = []string{"new_from", "genres"}

// musicV7Max is each family's row maximum (others: 1).
var musicV7Max = map[string]int{"mix": musicscore.MixMax, "because": 4, "genre_shelf": 3, "together": 2,
	// Row template families (music_lists_v1 engine).
	"tpl_yr": 1, "tpl_ch": 5, "tpl_co": 3, "tpl_po": 3, "tpl_nw": 5, "tpl_cl": 5, "tpl_de": 3, "tpl_su": 4, "tpl_cr": 6, "tpl_se": 3, "tpl_ar": 4, "tpl_vi": 1, "tpl_di": 3, "tpl_pe": 6}

// musicV7Few is the row count a household answers in one response.
const musicV7Few = 12

// musicV7Family answers a row's family and whether it is a discovery row.
func musicV7Family(m MusicRow) (string, bool) {
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

// musicV7Rows adds the v7 families to the build (personal households).
// yesterday holds yesterday's seeds.
func (h *Handlers) musicV7Rows(ctx context.Context, member int64, date string, lib *musicLib, acts *musicActs, now time.Time, in personalIn, yesterday map[string]bool) {
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
		h.musicv2.seedLog.put(member, date, seedIDs)
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
	// 25th or 30th anniversary falls in −7..+30 days (Luis: not-had only).
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
	// artists (Luis, 2026-10-23: allowed); their albums not had.
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

// musicV7Cursor is one page's position in a pinned plan.
type musicV7Cursor struct {
	Member  int64
	Date    string
	Region  string
	Version int
	Offset  int
}

// musicCursorSecret is the cursor signing key: 32 random bytes kept in the
// settings table, made on first use.
func (h *Handlers) musicCursorSecret(ctx context.Context) ([]byte, error) {
	h.musicv2.cursorMu.Lock()
	defer h.musicv2.cursorMu.Unlock()
	if h.musicv2.cursorKey != nil {
		return h.musicv2.cursorKey, nil
	}
	const key = "music_cursor_key"
	var v string
	err := h.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", key).Scan(&v)
	if err == sql.ErrNoRows || err == nil && v == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		v = base64.RawURLEncoding.EncodeToString(b)
		now := database.FormatTime(time.Now())
		by, _ := json.Marshal(domain.System)
		err = h.Store.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?) ON CONFLICT (grp, key) DO NOTHING`, key, v, now, string(by), now, string(by))
			return err
		})
		if err == nil {
			err = h.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", key).Scan(&v)
		}
	}
	if err != nil {
		return nil, err
	}
	b, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil || len(b) < 32 {
		return nil, fmt.Errorf("music cursor key unreadable")
	}
	h.musicv2.cursorKey = b
	return b, nil
}

func (h *Handlers) musicCursorSign(ctx context.Context, c musicV7Cursor) (string, error) {
	k, err := h.musicCursorSecret(ctx)
	if err != nil {
		return "", err
	}
	body := fmt.Sprintf("%d|%s|%s|%d|%d", c.Member, c.Date, c.Region, c.Version, c.Offset)
	m := hmac.New(sha256.New, k)
	m.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString([]byte(body)) + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil)), nil
}

func (h *Handlers) musicCursorOpen(ctx context.Context, s string) (musicV7Cursor, bool) {
	var c musicV7Cursor
	k, err := h.musicCursorSecret(ctx)
	if err != nil {
		return c, false
	}
	body64, sig64, ok := strings.Cut(s, ".")
	if !ok {
		return c, false
	}
	body, err1 := base64.RawURLEncoding.DecodeString(body64)
	sig, err2 := base64.RawURLEncoding.DecodeString(sig64)
	if err1 != nil || err2 != nil {
		return c, false
	}
	m := hmac.New(sha256.New, k)
	m.Write(body)
	if !hmac.Equal(sig, m.Sum(nil)) {
		return c, false
	}
	f := strings.Split(string(body), "|")
	if len(f) != 5 {
		return c, false
	}
	c.Member, err1 = strconv.ParseInt(f[0], 10, 64)
	c.Date, c.Region = f[1], f[2]
	c.Version, err2 = strconv.Atoi(f[3])
	off, err3 := strconv.Atoi(f[4])
	c.Offset = off
	return c, err1 == nil && err2 == nil && err3 == nil && off > 0 && off%musicscore.PlanBatch == 0
}

// exploreMusicV7 answers GET /api/v1/explore/music with flag music_rows_v7:
// page 1 (rows 1–6, the Spotlight and the asks row) or, with ?cursor=, the
// next 6 rows of the same pinned plan.
func (h *Handlers) exploreMusicV7(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now().UTC()
	if h.musicv2.now != nil {
		now = h.musicv2.now().UTC()
	}
	member := h.Person(r).UserID
	region := strings.ToUpper(r.URL.Query().Get("region"))
	if region == "" || region == "GLOBAL" || !isCountry(region) {
		region = "global"
	}
	key := musicscore.PlanKey{Member: member, Date: musicscore.Day(now), View: region}
	lib, err := h.musicLibrary(ctx, now)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	acts, err := h.musicActsOf(ctx, member)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	offset := 0
	var p *musicscore.Plan
	if cur := r.URL.Query().Get("cursor"); cur != "" {
		c, ok := h.musicCursorOpen(ctx, cur)
		if ok && c.Member == member && c.Region == region {
			// The plan's own day: a last good plan of yesterday pages on.
			p, ok = h.musicv2.plans.Get(musicscore.PlanKey{Member: member, Date: c.Date, View: region}, c.Version, now)
		}
		if !ok || p == nil {
			webserver.Fail(w, http.StatusBadRequest, "these rows expired; load Music again")
			return
		}
		offset = c.Offset
	} else {
		h.rememberMusicV7Ask(member, region, now)
		p = h.musicV7PlanFor(ctx, member, region, key, lib, acts, now)
	}
	ord := p.Meta.(*musicV2Order)
	var rows []musicscore.Row
	more := false
	if p.Version == 0 {
		// A plan cut short at the visitor's deadline is not kept: no cursor.
		rows = p.All(nil)
	} else if offset == 0 && p.Total() <= musicV7Few {
		rows = p.All(nil)
	} else {
		rows, more = p.Page(offset/musicscore.PlanBatch, nil)
	}
	state := func(c MusicCard) MusicCard {
		c.State = ""
		if c.MBID != "" && c.Kind == "album" {
			if s := acts.albumState[c.MBID]; s != "" {
				c.State = s
			}
		}
		if c.State == "" && c.ArtistMBID != "" {
			if s := acts.artistState[c.ArtistMBID]; s != "" {
				c.State = s
			}
		}
		if c.Kind == "album" && c.PartID != 0 && lib.hadParts[c.PartID] {
			c.State = "have"
		}
		return c
	}
	out := make([]MusicRow, 0, len(rows)+1)
	head := 0
	for _, row := range rows {
		m := ord.v7.metas[row.ID]
		m.Items = make([]MusicCard, 0, len(row.Items))
		for _, it := range row.Items {
			m.Items = append(m.Items, state(ord.v7.cards[row.ID][it.Key]))
		}
		out = append(out, m)
		if offset == 0 && (row.ID == "new_from" || row.ID == "genres") && len(out) == head+1 {
			head++
		}
	}
	if offset == 0 {
		asksRow := MusicRow{ID: "your_asks", Type: "your_asks", Title: "Your asks in progress", Reason: "Albums you asked for, newest first", Shape: "square"}
		for _, m := range acts.mine {
			c := MusicCard{Kind: "album", MBID: m.albumMBID, Name: m.album, Artist: m.artist, ArtistMBID: m.artistMBID, State: m.state, Date: dayOfText(m.at), Genres: []string{}}
			c.Key = "album:" + m.albumMBID
			if m.albumMBID == "" {
				c.Kind, c.MBID, c.Name, c.Key = "artist", m.artistMBID, m.artist, "artist:"+m.artistMBID
			} else {
				c.Image = "/api/v1/music-trending/art/album/" + m.albumMBID
			}
			if len(asksRow.Items) < musicscore.RowMax {
				asksRow.Items = append(asksRow.Items, c)
			}
		}
		if len(asksRow.Items) > 0 {
			out = append(out[:head], append([]MusicRow{asksRow}, out[head:]...)...)
		}
	}
	sinkAsked(out)
	next := ""
	if more {
		next, err = h.musicCursorSign(ctx, musicV7Cursor{Member: member, Date: p.Date, Region: region, Version: p.Version, Offset: offset + musicscore.PlanBatch})
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
	}
	h.rememberMusicCards(out)
	if more {
		// The next page's albums and artists are fetched ahead while this
		// one is read (nothing while Music's loop is off).
		nextPage := offset/musicscore.PlanBatch + 1
		go func() { h.queueAhead(musicV7PagesAhead(p, nextPage, 1)) }()
	}
	h.ExploreCaching(w, r, "private, no-cache")
	if offset > 0 {
		h.WriteMusicJSON(w, r, map[string]any{"v2": true, "v7": true, "rows": out, "next": next, "total": p.Total()})
		return
	}
	var hero *MusicCard
	if ord.Hero != nil {
		hv := state(*ord.Hero)
		hero = &hv
	}
	alts := make([]MusicCard, 0, len(ord.Alts))
	for _, c := range ord.Alts {
		alts = append(alts, state(c))
	}
	regions := ord.Regions
	if regions == nil {
		regions = []string{}
	}
	if hero != nil {
		h.rememberMusicCards(nil, append([]MusicCard{*hero}, alts...)...)
	}
	h.WriteMusicJSON(w, r, map[string]any{"v2": true, "v7": true, "kid": false, "cold": ord.Cold, "partial": p.Partial, "genres": []string{}, "genre": "", "region": region, "regions": regions, "hero": hero, "hero_alts": alts, "rows": out, "next": next, "total": p.Total()})
}

// prewarmMusicV7 runs a second after Music starts and after each Music
// read-ahead prebuild: one household build (member 0, worldwide) fills
// layer 1 (memory and disk) with the artist data, similarity and listeners
// the v7 families read, so a member's first page of the day needs few or no
// metadata requests. The household plan is kept like a visit's.
func (h *Handlers) prewarmMusicV7(ctx context.Context) {
	if !h.MusicRowsV7 || !h.MusicRowsV3 || h.Store == nil {
		return
	}
	now := time.Now().UTC()
	lib, err := h.musicLibrary(ctx, now)
	if err != nil {
		return
	}
	acts, err := h.musicActsOf(ctx, 0)
	if err != nil {
		return
	}
	key := musicscore.PlanKey{Member: 0, Date: musicscore.Day(now), View: "global"}
	p := h.buildMusicV7Plan(ctx, 0, "global", key, lib, acts, now)
	// The member and region pairs actually asked for lately are built ahead
	// too, so their first visit of the day reads a ready plan.
	defer h.prewarmMusicV7Asked(ctx, key.Date, lib, now)
	if p != nil && p.Meta != nil {
		// The first pages' albums and artists: details and pictures ahead.
		pages := musicV7AheadPages
		if h.MusicListsV1 {
			// Every row of the plan: list rows show art at once on any page.
			pages = musicscore.PlanMax / musicscore.PlanBatch
		}
		h.queueAhead(musicV7PagesAhead(p, 0, pages))
	}
}

// musicV7AheadPages is how many v7 pages the prewarm fetches ahead.
const musicV7AheadPages = 3

// prewarmMusicV7Asked builds today's plan of each recently asked member and
// region that has none yet.
func (h *Handlers) prewarmMusicV7Asked(ctx context.Context, date string, lib *musicLib, now time.Time) {
	for _, k := range h.musicV7Asked(now) {
		if ctx.Err() != nil {
			return
		}
		pk := musicscore.PlanKey{Member: k.member, Date: date, View: k.region}
		if _, ok := h.musicv2.plans.Current(pk, now); ok || (k.member == 0 && k.region == "global") {
			continue
		}
		if acts, err := h.musicActsOf(ctx, k.member); err == nil {
			h.buildMusicV7Plan(ctx, k.member, k.region, pk, lib, acts, now)
		}
	}
}

// musicV7Wait is the longest a visitor waits for a plan build; past it the
// last good plan (or, with none, what the build has so far) is answered.
var musicV7Wait = 1500 * time.Millisecond

// musicV7PartialRetry is how soon a plan that lost a source is rebuilt
// behind while it keeps being served.
const musicV7PartialRetry = time.Minute

// musicV7PlanFor answers page 1's plan without building inline while any
// earlier plan exists: the current plan; else the last good plan (stale,
// partial or yesterday's, in memory or on disk) with a rebuild behind;
// else a build cut at musicV7Wait.
func (h *Handlers) musicV7PlanFor(ctx context.Context, member int64, region string, key musicscore.PlanKey, lib *musicLib, acts *musicActs, now time.Time) *musicscore.Plan {
	if p, ok := h.musicv2.plans.Current(key, now); ok {
		if p.Partial && now.Sub(p.Built) >= musicV7PartialRetry {
			h.refreshMusicV7(member, region, key, now)
		}
		return p
	}
	if old, ok := h.musicv2.plans.Newest(member, region, now); ok {
		done := h.refreshMusicV7(member, region, key, now)
		if p := h.waitMusicV7(done, key, now); p != nil {
			return p
		}
		return old
	}
	if snap, current, ok := h.loadMusicV7Snap(ctx, member, region, key.Date, lib, now); ok {
		// Kept on disk (a restart): served at once, rebuilt behind when old
		// (the rebuild is answered when it is done within the deadline).
		if !current {
			if p := h.waitMusicV7(h.refreshMusicV7(member, region, key, now), key, now); p != nil {
				return p
			}
		}
		ord := snap.order()
		p := musicscore.NewPlan(ord.v7.rows, h.musicV7HeadRows(), musicV7Max, member, key.Date, ord.v7.seen)
		p.Meta = ord
		h.musicv2.plans.Put(key, p, now)
		if !current {
			h.musicv2.plans.MarkStaleKey(key)
			h.refreshMusicV7(member, region, key, now)
		}
		return p
	}
	// Nothing kept at all: the full build runs behind; the visitor gets it
	// if it is done in time, else a build cut at the deadline.
	done := h.refreshMusicV7(member, region, key, now)
	dctx, cancel := context.WithTimeout(ctx, musicV7Wait)
	defer cancel()
	// Most builds read kept data and are done well within the deadline; only
	// a slow one gets a second, cut build beside it.
	grace := time.NewTimer(musicV7Wait / 3)
	defer grace.Stop()
	select {
	case <-done:
		if p, ok := h.musicv2.plans.Current(key, now); ok {
			return p
		}
	case <-grace.C:
	}
	cut := make(chan *musicscore.Plan, 1)
	go func() {
		ord := h.buildMusicV3(context.WithValue(dctx, musicMissKey{}, &musicMiss{}), member, region, lib, acts, now)
		if ord.v7 == nil {
			ord.v7 = &musicV7Cand{metas: map[string]MusicRow{}, cards: map[string]map[string]MusicCard{}}
		}
		p := musicscore.NewPlan(ord.v7.rows, h.musicV7HeadRows(), musicV7Max, member, key.Date, ord.v7.seen)
		p.Meta = ord
		p.Partial = true
		cut <- p
	}()
	// The full build is preferred while the deadline lasts.
	select {
	case <-done:
		if p, ok := h.musicv2.plans.Current(key, now); ok {
			return p
		}
	case <-dctx.Done():
	}
	t := time.NewTimer(300 * time.Millisecond)
	defer t.Stop()
	select {
	case p := <-cut:
		return p
	case <-t.C:
		p := musicscore.NewPlan(nil, nil, musicV7Max, member, key.Date, nil)
		p.Meta = &musicV2Order{v7: &musicV7Cand{metas: map[string]MusicRow{}, cards: map[string]map[string]MusicCard{}}}
		p.Partial = true
		return p
	}
}

// waitMusicV7 waits up to musicV7Wait for a rebuild and answers its plan.
func (h *Handlers) waitMusicV7(done <-chan struct{}, key musicscore.PlanKey, now time.Time) *musicscore.Plan {
	t := time.NewTimer(musicV7Wait)
	defer t.Stop()
	select {
	case <-done:
		if p, ok := h.musicv2.plans.Current(key, now); ok {
			return p
		}
	case <-t.C:
	}
	return nil
}

// musicV7Recent is how long an asked member and region stay prewarmed.
const (
	musicV7Recent    = 7 * 24 * time.Hour
	musicV7RecentMax = 16
)

type musicV7AskKey struct {
	member int64
	region string
}

// rememberMusicV7Ask notes that member asked for region's rows, so the
// prewarm builds that plan ahead next time.
func (h *Handlers) rememberMusicV7Ask(member int64, region string, now time.Time) {
	h.musicv2.asked.Store(musicV7AskKey{member, region}, now)
}

// musicV7Asked answers the recently asked member and region pairs, newest
// first, at most musicV7RecentMax.
func (h *Handlers) musicV7Asked(now time.Time) []musicV7AskKey {
	type at struct {
		k musicV7AskKey
		t time.Time
	}
	var l []at
	h.musicv2.asked.Range(func(k, v any) bool {
		if t := v.(time.Time); now.Sub(t) < musicV7Recent {
			l = append(l, at{k.(musicV7AskKey), t})
		} else {
			h.musicv2.asked.Delete(k)
		}
		return true
	})
	sort.Slice(l, func(i, j int) bool { return l[i].t.After(l[j].t) })
	out := make([]musicV7AskKey, 0, min(len(l), musicV7RecentMax))
	for _, e := range l[:min(len(l), musicV7RecentMax)] {
		out = append(out, e.k)
	}
	return out
}

// writeJSONTagged writes v with a content ETag and answers 304 when the
// visitor already has it.
func writeJSONTagged(w http.ResponseWriter, r *http.Request, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	sum := sha256.Sum256(buf.Bytes())
	tag := `"` + hex.EncodeToString(sum[:12]) + `"`
	w.Header().Set("ETag", tag)
	for _, t := range strings.Split(r.Header.Get("If-None-Match"), ",") {
		if t = strings.TrimPrefix(strings.TrimSpace(t), "W/"); t == tag || t == "*" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}
