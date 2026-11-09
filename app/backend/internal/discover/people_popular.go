package discover

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// TMDB's popular people follow short page-view spikes, so Search re-ranks
// the first popularPages pages by fame and serves its own list, kept a day.
const (
	popularPages    = 8
	popularPageSize = 20
	popularTTL      = 24 * time.Hour
	// A person needs one known_for title with at least this many votes.
	popularMinVotes = 1000
	// A known_for title with this many votes counts as a hit.
	popularHitVotes = 5000
)

// fameScore sums log(votes) over a person's known_for titles, with a boost
// for each hit.
func fameScore(votes []int) float64 {
	s := 0.0
	for _, v := range votes {
		if v > 0 {
			s += math.Log(float64(v))
		}
		if v >= popularHitVotes {
			s += 4
		}
	}
	return s
}

// famous is the gate: a real picture, not adult, one well-known title.
func famous(p metadata.Person) bool {
	if p.Adult || p.Profile == "" {
		return false
	}
	for _, v := range p.KnownForVotes {
		if v >= popularMinVotes {
			return true
		}
	}
	return false
}

// rankPopular gates and orders people: fame first, TMDB's popularity a
// little, and a boost for people in the Library or today's trending casts.
func rankPopular(people []metadata.Person, boost map[string]bool) []metadata.Person {
	type scored struct {
		p metadata.Person
		s float64
	}
	var list []scored
	seen := map[string]bool{}
	for _, p := range people {
		if seen[p.ID] || !famous(p) {
			continue
		}
		seen[p.ID] = true
		s := fameScore(p.KnownForVotes) + math.Log1p(math.Max(p.Popularity, 0))
		if boost[p.ID] {
			s += 8
		}
		list = append(list, scored{p, s})
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].s > list[j].s })
	out := make([]metadata.Person, len(list))
	for i, x := range list {
		out[i] = x.p
	}
	return out
}

// popularPool pulls TMDB's popular pages 1..popularPages, re-ranks the whole
// pool and keeps it for a day.
func (h *Handlers) popularPool(ctx context.Context, p metadata.PeopleProvider, pop metadata.PopularPeopler) ([]metadata.Person, error) {
	const key = "peoplepopular|v1"
	now := time.Now()
	c := metadata.LookupCache{Store: h.Store}
	if b, ok, _ := c.Get(ctx, key, now); ok {
		var out []metadata.Person
		if json.Unmarshal(b, &out) == nil {
			return out, nil
		}
	}
	pages := make([][]metadata.Person, popularPages)
	errs := make([]error, popularPages)
	eachLimited(popularPages, func(i int) {
		got, err := pop.PopularPeople(ctx, i+1)
		pages[i], errs[i] = got.People, err
	})
	if errs[0] != nil {
		return nil, errs[0]
	}
	ttl := popularTTL
	var all []metadata.Person
	for i, pg := range pages {
		if errs[i] != nil {
			// A page that failed is tried again soon, not in a day.
			ttl = 15 * time.Minute
		}
		all = append(all, pg...)
	}
	boost := map[string]bool{}
	for _, id := range []string{"library", "hot"} {
		if s, err := h.peopleShelf(ctx, id, "", p); err == nil {
			for _, sp := range s.People {
				boost[sp.ID] = true
			}
		}
	}
	out := rankPopular(all, boost)
	if b, err := json.Marshal(out); err == nil {
		_ = c.Put(context.WithoutCancel(ctx), key, b, now, now.Add(ttl))
	}
	return out, nil
}
