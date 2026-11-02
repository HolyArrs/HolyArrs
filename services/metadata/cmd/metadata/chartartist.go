package main

import (
	"context"
	"slices"
	"sync"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/mbread"
	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
)

// chartArtists resolves a chart row's own artist id (Apple Music or
// Spotify, as the chart source gives it) or its album barcode to one
// MusicBrainz artist, exactly and without any outside call:
//  1. the MusicBrainz artist whose link holds that id;
//  2. else the artist our store holds that id for, found by a MusicBrainz
//     link, a barcode or Wikidata (never by name);
//  3. else the one artist the barcode's release credits alone, when that
//     artist is named like the row's artist or its credit's first artist
//     (its name, sort name or an alias; a soundtrack's "Various Artists"
//     or a duo's album is not one member's).
//
// Two answers at a step, a malformed id or barcode, or none: "" (the
// placeholder stays; no guess).
type chartArtists struct {
	linked  func(ctx context.Context, source, id string) []string
	holders func(source, id string) []string
	barcode func(ctx context.Context, code string) string
	// names answers an artist's MusicBrainz name, sort name and aliases.
	names func(ctx context.Context, mbid string) []string
	// groups answers an artist's release-group count (a duplicate entry
	// usually has none).
	groups func(ctx context.Context, mbid string) int
	// counts: asked, and resolved by link, stored match or barcode.
	mu     sync.Mutex
	counts map[string]int
}

// count notes one outcome.
func (c *chartArtists) count(k string) {
	c.mu.Lock()
	if c.counts == nil {
		c.counts = map[string]int{}
	}
	c.counts[k]++
	c.mu.Unlock()
}

// Counts answers the outcomes so far.
func (c *chartArtists) Counts() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]int{}
	for k, v := range c.counts {
		out[k] = v
	}
	return out
}

func (c *chartArtists) resolve(ctx context.Context, source, id, name string, codes []string) (out string) {
	c.count("asked")
	defer func() {
		if out == "" {
			c.count("unresolved")
		}
	}()
	return c.resolveOnce(ctx, source, id, name, codes)
}

func (c *chartArtists) resolveOnce(ctx context.Context, source, id, name string, codes []string) string {
	if imgstore.ValidSourceID(source, id) {
		if l := unique(c.linked(ctx, source, id)); len(l) == 1 {
			c.count("link")
			return l[0]
		} else if len(l) > 1 {
			// Linked from two entries (a duplicate): the one our exact
			// match holds, else the only one with release groups.
			if x := c.oneOf(ctx, source, id, l); x != "" {
				c.count("link")
				return x
			}
			return ""
		}
		if c.holders != nil {
			if h := unique(c.holders(source, id)); len(h) == 1 {
				c.count("stored")
				return h[0]
			} else if len(h) > 1 {
				return ""
			}
		}
	}
	found, first := "", musictrend.FirstArtist(name)
	for _, code := range codes {
		if !imgstore.ValidBarcode(code) {
			continue
		}
		g := c.barcode(ctx, code)
		if g == "" || !c.named(ctx, g, name, first) {
			continue
		}
		if found != "" && g != found {
			return ""
		}
		found = g
	}
	if found != "" {
		c.count("barcode")
	}
	return found
}

// named reports the artist carries the row's name, or the first artist of
// the row's credit ("Drake feat. Future": Drake).
func (c *chartArtists) named(ctx context.Context, mbid, credit, first string) bool {
	if credit == "" || c.names == nil {
		return false
	}
	for _, n := range c.names(ctx, mbid) {
		if imgstore.SameName(n, credit) || imgstore.SameName(n, first) {
			return true
		}
	}
	return false
}

func unique(ids []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// newChartArtists links chart rows by the slim copy's Apple Music and
// Spotify artist links (linkIndex, kept in dir) and starts reading them.
func newChartArtists(slim *mbread.Reader, imgs *imgstore.Store, dir string) *chartArtists {
	idx := &linkIndex{date: slim.Date, links: slim.ArtistLinks, dir: dir}
	c := &chartArtists{barcode: slim.ArtistByBarcode,
		names:  slim.ArtistNames,
		groups: func(ctx context.Context, mbid string) int { return len(slim.GroupTitles(ctx, mbid)) },
		linked: func(ctx context.Context, source, id string) []string { return idx.get(ctx, source+":"+id) }}
	if imgs != nil {
		c.holders = imgs.ExactHolders
	}
	go idx.refresh(context.Background())
	return c
}

// sharedName reports a name two or more MusicBrainz artists carry (holders
// is mbread Reader.NameHolders), each name asked once per copy (version is
// the copy in use, "" when none): with no copy, or a read cut short, the
// answer is not kept.
func sharedName(holders func(ctx context.Context, name string) int, version func() string) func(ctx context.Context, name string) bool {
	memo := newCopyMemo[bool](version, 20000)
	return func(ctx context.Context, name string) bool {
		return memo.get(ctx, name, func() bool { return holders(ctx, name) >= 2 })
	}
}

// oneOf settles an id MusicBrainz links from several artists: the one our
// store holds it for exactly, else the only one with release groups; ""
// when that does not single one out.
func (c *chartArtists) oneOf(ctx context.Context, source, id string, cands []string) string {
	if c.holders != nil {
		var hit []string
		for _, h := range unique(c.holders(source, id)) {
			if slices.Contains(cands, h) {
				hit = append(hit, h)
			}
		}
		if len(hit) == 1 {
			return hit[0]
		}
	}
	if c.groups == nil {
		return ""
	}
	var with []string
	for _, x := range cands {
		if c.groups(ctx, x) > 0 {
			with = append(with, x)
		}
	}
	if len(with) == 1 {
		return with[0]
	}
	return ""
}
