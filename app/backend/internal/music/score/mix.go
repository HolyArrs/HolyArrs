package musicscore

import (
	"math"
	"sort"
	"strings"
	"time"
)

const (
	// MixMax is the most mixes per member.
	MixMax = 6
	// MixTaste is how many top artists by affinity form the taste set.
	MixTaste = 60
	// mixMerge is the lowest average link weight at which two clusters merge.
	mixMerge = 0.45
	// mixMinArtists is the fewest artists a mix keeps.
	mixMinArtists = 3
)

// MixArtist is one taste-set artist with its affinity and genre share
// vector, which is empty when the artist has no genre votes.
type MixArtist struct {
	ID    string
	A     float64
	Share map[string]float64
}

// Mix is one cluster of artists with its weighted genre centroid, naming
// genres (empty means it is named after Lead) and total affinity.
type Mix struct {
	Members  []string
	Centroid map[string]float64
	Genres   []string
	Lead     string
	Weight   float64
}

// Cos returns the cosine of two genre share vectors, or 0 if either is empty.
func Cos(u, v map[string]float64) float64 {
	var dot, nu, nv float64
	for k, x := range u {
		nu += x * x
		dot += x * v[k]
	}
	for _, y := range v {
		nv += y * y
	}
	if nu == 0 || nv == 0 {
		return 0
	}
	return clamp01(dot / math.Sqrt(nu*nv))
}

// Clusters groups the taste set into mixes. sim returns the symmetric
// similarity of two artists, or 0 when unknown. Artists without genre votes
// join the cluster of their most similar neighbour after grouping, so they
// never shape it.
func Clusters(set []MixArtist, sim func(a, b string) float64) []Mix {
	set = append([]MixArtist{}, set...)
	sort.SliceStable(set, func(i, j int) bool {
		if set[i].A != set[j].A {
			return set[i].A > set[j].A
		}
		return set[i].ID < set[j].ID
	})
	set = set[:min(MixTaste, len(set))]
	by := map[string]MixArtist{}
	var voted, bare []string
	for _, a := range set {
		by[a.ID] = a
		if len(a.Share) > 0 {
			voted = append(voted, a.ID)
		} else {
			bare = append(bare, a.ID)
		}
	}
	sort.Strings(voted)
	n := len(voted)
	w := make([][]float64, n)
	for i := range w {
		w[i] = make([]float64, n)
		for j := range w[i] {
			if i != j {
				// Genre votes count a bit more than the similarity feed.
				w[i][j] = 0.6*Cos(by[voted[i]].Share, by[voted[j]].Share) + 0.4*clamp01(sim(voted[i], voted[j]))
			}
		}
	}
	// Greedy average-linkage merging; ties go to the lowest cluster index.
	cl := make([][]int, n)
	for i := range cl {
		cl[i] = []int{i}
	}
	link := func(x, y []int) float64 {
		s := 0.0
		for _, i := range x {
			for _, j := range y {
				s += w[i][j]
			}
		}
		return s / float64(len(x)*len(y))
	}
	for {
		bi, bj, best := -1, -1, mixMerge
		for i := 0; i < len(cl); i++ {
			for j := i + 1; j < len(cl); j++ {
				// The small margin keeps float noise from breaking ties.
				if l := link(cl[i], cl[j]); l > best+1e-12 || bi < 0 && l >= best-1e-12 {
					bi, bj, best = i, j, l
				}
			}
		}
		if bi < 0 {
			break
		}
		cl[bi] = append(cl[bi], cl[bj]...)
		sort.Ints(cl[bi])
		cl = append(cl[:bj], cl[bj+1:]...)
	}
	var mixes []Mix
	for _, c := range cl {
		m := Mix{}
		for _, i := range c {
			m.Members = append(m.Members, voted[i])
		}
		mixes = append(mixes, m)
	}
	// Unvoted artists only add members, they do not move a cluster.
	for _, id := range bare {
		bi, best := -1, 0.0
		for i, m := range mixes {
			for _, x := range m.Members {
				if len(by[x].Share) == 0 {
					continue
				}
				if s := sim(id, x); s > best {
					bi, best = i, s
				}
			}
		}
		if bi >= 0 {
			mixes[bi].Members = append(mixes[bi].Members, id)
		}
	}
	var out []Mix
	for _, m := range mixes {
		if len(m.Members) < mixMinArtists {
			continue
		}
		out = append(out, finishMix(m, by))
	}
	sortMixes(out)
	// Two mixes that would share a name become one.
	var merged []Mix
	for _, m := range out {
		dup := -1
		for i, x := range merged {
			if mixName(x) == mixName(m) {
				dup = i
				break
			}
		}
		if dup < 0 {
			merged = append(merged, m)
			continue
		}
		merged[dup] = finishMix(Mix{Members: append(merged[dup].Members, m.Members...)}, by)
	}
	sortMixes(merged)
	return merged[:min(MixMax, len(merged))]
}

// MixTitle names a mix after its genres, or "{lead} and more" when no genre
// is strong enough to name it.
func MixTitle(m Mix, leadName string) string {
	if len(m.Genres) == 0 {
		return leadName + " and more"
	}
	parts := make([]string, len(m.Genres))
	for i, g := range m.Genres {
		parts[i] = GenreName(g)
	}
	return strings.Join(parts, " & ") + " mix"
}

// MixSeeds picks 3 seeds for the day from the mix's top 6 members by
// affinity. The pick is rotated by a daily hash so it changes each day but
// not within one.
func MixSeeds(m Mix, member int64, date string) []string {
	top := append([]string{}, m.Members[:min(6, len(m.Members))]...)
	sort.SliceStable(top, func(i, j int) bool {
		return Tie(member, date, m.Lead+top[i]) < Tie(member, date, m.Lead+top[j])
	})
	return top[:min(3, len(top))]
}

// ScoreMix scores a mix item as 0.45*normS + 0.25*cos + 0.2*p + 0.1*f60.
func ScoreMix(normS, cos, p, f60 float64) float64 {
	return 0.45*normS + 0.25*cos + 0.2*p + 0.1*f60
}

// MixKeeps reports whether an item clears the quality bar: score at least
// 0.35 and either a close similar artist or a close genre match.
func MixKeeps(score, sMax, cos float64) bool {
	return score >= 0.35 && (sMax >= 0.3 || cos >= 0.5)
}

// ScoreDeepCut scores a deep cut as 0.5*a + 0.3*rankFrac + 0.2*cover.
func ScoreDeepCut(a, rankFrac, cover float64) float64 { return 0.5*a + 0.3*rankFrac + 0.2*cover }

// DeepRankFrac returns (r-1)/max(1, n-1) for the 1-based listeners rank r.
func DeepRankFrac(r, n int) float64 { return clamp01(float64(r-1) / math.Max(1, float64(n-1))) }

// ScoreAnniversary scores an anniversary as 0.5*a + 0.3*lnorm + 0.2*round.
func ScoreAnniversary(a, lnorm, round float64) float64 { return 0.5*a + 0.3*lnorm + 0.2*round }

// ScoreTogether scores a played-together item as 0.5*rel + 0.3*p + 0.2*aSeed.
func ScoreTogether(rel, p, aSeed float64) float64 { return 0.5*rel + 0.3*p + 0.2*aSeed }

// Rel weighs a MusicBrainz artist link: 1 for members and founders, 0.8 for
// collaborations, and 0 for any other type since it says nothing of taste.
func Rel(linkType string) float64 {
	switch linkType {
	case "member of band", "founder":
		return 1
	case "collaboration":
		return 0.8
	}
	return 0
}

// Anniversary reports the 10th, 20th, 25th or 30th anniversary of a release
// date when it falls within 7 days back to 30 days ahead of now (UTC). It
// returns the years and the anniversary day.
func Anniversary(date string, now time.Time) (int, time.Time, bool) {
	if len(date) < 10 {
		return 0, time.Time{}, false
	}
	d, err := time.Parse("2006-01-02", date[:10])
	if err != nil {
		return 0, time.Time{}, false
	}
	today := now.UTC().Truncate(24 * time.Hour)
	for _, n := range []int{10, 20, 25, 30} {
		on := d.AddDate(n, 0, 0)
		days := int(on.Sub(today).Hours() / 24)
		if days >= -7 && days <= 30 {
			return n, on, true
		}
	}
	return 0, time.Time{}, false
}

func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }

// sortMixes orders mixes by weight, then by first member for a stable result.
func sortMixes(l []Mix) {
	sort.SliceStable(l, func(i, j int) bool {
		if l[i].Weight != l[j].Weight {
			return l[i].Weight > l[j].Weight
		}
		return l[i].Members[0] < l[j].Members[0]
	})
}

// finishMix orders members by affinity, then works out the centroid, the
// naming genres and the weight.
func finishMix(m Mix, by map[string]MixArtist) Mix {
	sort.SliceStable(m.Members, func(i, j int) bool {
		a, b := by[m.Members[i]], by[m.Members[j]]
		if a.A != b.A {
			return a.A > b.A
		}
		return a.ID < b.ID
	})
	m.Centroid = map[string]float64{}
	m.Weight = 0
	total := 0.0
	for _, id := range m.Members {
		a := by[id]
		m.Weight += a.A
		for g, s := range a.Share {
			m.Centroid[g] += a.A * s
			total += a.A * s
		}
	}
	m.Lead = m.Members[0]
	type gw struct {
		g string
		w float64
	}
	var gs []gw
	for g, v := range m.Centroid {
		gs = append(gs, gw{g, v})
	}
	sort.Slice(gs, func(i, j int) bool {
		if gs[i].w != gs[j].w {
			return gs[i].w > gs[j].w
		}
		return gs[i].g < gs[j].g
	})
	m.Genres = nil
	if total > 0 && len(gs) > 0 && gs[0].w/total >= 0.4 {
		m.Genres = []string{gs[0].g}
		if len(gs) > 1 && gs[1].w/total >= 0.25 {
			m.Genres = append(m.Genres, gs[1].g)
		}
	}
	return m
}

// mixName returns the key two mixes share when they would get one name.
func mixName(m Mix) string {
	if len(m.Genres) > 0 {
		return strings.Join(m.Genres, "&")
	}
	return "lead:" + m.Lead
}
