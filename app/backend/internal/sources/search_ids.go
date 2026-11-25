package sources

import (
	"context"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// search is Search that also says whether the Source was asked by an id,
// which names the title however a release is named.
//
// A Source asked by an id answers with that title, under whatever name a
// release carries (a French site names a movie in French). Its answer is kept
// when a release carries the asked id, or names the title under one of its
// names. Otherwise the site may have ignored the id and sent its newest
// releases, so it is asked again by the title's words: what that finds is
// kept in place of the id answer when it names the title or every id
// answer carries an id that is not the asked one, and beside it otherwise. When the word search finds nothing or
// fails, the id answer is kept and the judge says which releases are not the
// title, unless every release carries an id and none is the asked one: then
// the site plainly ignored the id, and its answer is dropped and counted.
func (c *Client) search(ctx context.Context, q Query) ([]Item, bool, error) {
	if c.cfg.Format == "site" {
		items, err := c.siteSearch(ctx, q, c.cfg.Observe)
		return items, false, err
	}
	c.mu.Lock()
	caps := c.caps
	c.mu.Unlock()
	if caps == nil {
		got, err := c.Caps(ctx)
		if err != nil {
			return nil, false, err
		}
		caps = &got
	}
	v, ids, err := searchValues(q, *caps, true)
	if err != nil {
		return nil, false, err
	}
	asked := askedIDs(v)
	items, err := c.searchWith(ctx, q, *caps, v)
	if err != nil || !ids {
		return items, false, err
	}
	if len(items) == 0 {
		// A site that needs every id to match may answer nothing: ask by words.
		wv, _, werr := searchValues(q, *caps, false)
		if werr != nil {
			return items, true, nil
		}
		byName, err := c.searchWith(ctx, q, *caps, wv)
		return byName, false, err
	}
	verdict := idAnswer(items, asked)
	if verdict == idsMatch || verdict == idsUnknown && namesTitle(q, items) {
		return items, true, nil
	}
	wrong := verdict == idsOther
	if wv, _, werr := searchValues(q, *caps, false); werr == nil {
		byName, err := c.searchWith(ctx, q, *caps, wv)
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		if err == nil && len(byName) > 0 {
			if !wrong && !namesTitle(q, byName) {
				// Nothing names the title either way and a word search is
				// loose: keep both and let the judge decide.
				return append(items, byName...), false, nil
			}
			q.drops.add("", len(items))
			return byName, false, nil
		}
	}
	if wrong {
		q.drops.add("", len(items))
		return nil, true, nil
	}
	return items, true, nil
}

// idVerdict is what the ids on an id search's answers say.
type idVerdict int

const (
	idsUnknown idVerdict = iota // no release carries an id the search asked
	idsMatch                    // a release carries an asked id
	idsOther                    // every release carries ids, none an asked one
)

// askedIDs are the ids an id search sends, keyed imdb, tmdb and tvdb, with
// IMDb as digits with no leading zeros, as parseItems reads them.
func askedIDs(v url.Values) map[string]string {
	out := map[string]string{}
	if id := strings.TrimLeft(v.Get("imdbid"), "0"); id != "" {
		out["imdb"] = id
	}
	for _, k := range []string{"tmdb", "tvdb"} {
		if id := strings.TrimLeft(v.Get(k+"id"), "0"); id != "" {
			out[k] = id
		}
	}
	return out
}

func idAnswer(items []Item, asked map[string]string) idVerdict {
	if len(asked) == 0 {
		return idsUnknown
	}
	all := true
	for _, it := range items {
		have := map[string]string{"imdb": it.IMDb}
		if it.TMDb > 0 {
			have["tmdb"] = strconv.Itoa(it.TMDb)
		}
		if it.TVDB > 0 {
			have["tvdb"] = strconv.Itoa(it.TVDB)
		}
		carries := false
		for k, id := range asked {
			if have[k] == "" {
				continue
			}
			carries = true
			if have[k] == id {
				return idsMatch
			}
		}
		all = all && carries
	}
	if all {
		return idsOther
	}
	return idsUnknown
}

// namesTitle says whether a release names the title or one of its other names.
func namesTitle(q Query, items []Item) bool {
	name := nameWords(q.Title)
	if len(name) == 0 {
		return true
	}
	names := [][]string{name}
	for _, a := range q.Aliases {
		if w := nameWords(a); len(w) > 0 {
			names = append(names, w)
		}
	}
	for _, it := range items {
		for _, n := range names {
			if namesAll(it.Title, n) {
				return true
			}
		}
	}
	return false
}

// dropTally counts, by Source name, the answers dropped as not the title. A
// Client adds under "", and askAll moves that under the Source's name.
type dropTally struct {
	mu sync.Mutex
	by map[string]int
}

func (d *dropTally) add(source string, n int) {
	if d == nil || n <= 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.by == nil {
		d.by = map[string]int{}
	}
	d.by[source] += n
}

// take gives the count under "" and clears it.
func (d *dropTally) take() int {
	if d == nil {
		return 0
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	n := d.by[""]
	delete(d.by, "")
	return n
}

// total is how many answers were dropped, and the Sources that sent them.
func (d *dropTally) total() (int, []string) {
	if d == nil {
		return 0, nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	n, names := 0, make([]string, 0, len(d.by))
	for k, v := range d.by {
		n += v
		names = append(names, k)
	}
	sort.Strings(names)
	return n, names
}
