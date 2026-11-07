package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

// MadeByLister lists the movies one person directed or the shows they made.
type MadeByLister interface {
	MadeBy(ctx context.Context, kind, personID string) ([]DiscoverItem, error)
}

// MadeByListerFor returns the made-by surface selected for ctx.
func MadeByListerFor(ctx context.Context, provider Provider) (MadeByLister, bool) {
	if sw, ok := provider.(*Switch); ok {
		provider = sw.pick(ctx)
	}
	c, ok := provider.(MadeByLister)
	return c, ok
}

// maxCredits caps the titles MadeBy answers.
const maxCredits = 40

// MadeBy reads a person's movies (kind movie: directed) or shows (kind
// show: created, executive produced or directed), most popular first,
// each title once.
func (t *TMDB) MadeBy(ctx context.Context, kind, personID string) ([]DiscoverItem, error) {
	n, err := strconv.ParseInt(personID, 10, 64)
	if err != nil || n <= 0 || (kind != "movie" && kind != "show") {
		return nil, ErrNotFound
	}
	names, err := t.genreNames(ctx, kind)
	if err != nil {
		return nil, err
	}
	path := "/person/" + strconv.FormatInt(n, 10) + "/movie_credits"
	if kind == "show" {
		path = "/person/" + strconv.FormatInt(n, 10) + "/tv_credits"
	}
	b, err := t.get(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	var res struct {
		Crew []struct {
			tmdbListItem
			Job string `json:"job"`
		} `json:"crew"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	seen := map[int64]bool{}
	var list []tmdbListItem
	for _, c := range res.Crew {
		ok := c.Job == "Director"
		if kind == "show" {
			ok = c.Job == "Creator" || c.Job == "Executive Producer" || c.Job == "Director"
		}
		if !ok || seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		list = append(list, c.tmdbListItem)
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].Popularity > list[j].Popularity })
	return t.items(kind, list, names, maxCredits), nil
}
