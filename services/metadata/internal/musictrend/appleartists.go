package musictrend

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"
)

// appleItemsBatch is how many catalog ids one Apple call asks for.
const appleItemsBatch = 50

// appleItemFresh is how long a kept item's artist answer holds before it
// is asked again (with a chart refresh).
const appleItemFresh = 30 * 24 * time.Hour

// fillAppleArtists gives chart rows without an artist id the Apple Music
// artist of their album or song: kept answers first, the rest asked from
// Apple's catalog in batches (ids=, appleItemsBatch at a time, through the
// server's Apple client) and kept for good. An item credited to several
// artists gets none (the name rules apply to it).
func (s *Service) fillAppleArtists(ctx context.Context, st string, rows []Entry) error {
	if s.opts.Apple == nil {
		return nil
	}
	now := time.Now()
	need := map[string][]string{} // kind: ids to ask
	for i := range rows {
		e := &rows[i]
		if e.AppleArtist != "" || !digits(e.AppleID) || (e.AppleKind != "albums" && e.AppleKind != "songs") {
			continue
		}
		var artist string
		var at int64
		if s.db.QueryRowContext(ctx, `SELECT artist, at FROM apple_item_artists WHERE kind=? AND id=?`, e.AppleKind, e.AppleID).Scan(&artist, &at) == nil && now.Sub(time.Unix(at, 0)) < appleItemFresh {
			e.AppleArtist = artist
			continue
		}
		need[e.AppleKind] = append(need[e.AppleKind], e.AppleID)
	}
	for _, kind := range []string{"albums", "songs"} {
		ids := unique(need[kind])
		for start := 0; start < len(ids); start += appleItemsBatch {
			part := ids[start:min(len(ids), start+appleItemsBatch)]
			var d struct {
				Data []struct {
					ID            string `json:"id"`
					Relationships struct {
						Artists struct {
							Data []struct {
								ID string `json:"id"`
							} `json:"data"`
						} `json:"artists"`
					} `json:"relationships"`
				} `json:"data"`
			}
			ok, err := s.opts.Apple(ctx, "/v1/catalog/"+url.PathEscape(st)+"/"+kind, url.Values{"ids": {strings.Join(part, ",")}}, &d)
			if err != nil {
				return err
			}
			if !ok {
				// Apple answered "no such thing" (400/404) for the batch.
				return fmt.Errorf("%s %s: no answer for %d ids", st, kind, len(part))
			}
			got := map[string]string{}
			for _, x := range d.Data {
				artist := ""
				if a := x.Relationships.Artists.Data; len(a) == 1 && digits(a[0].ID) {
					artist = a[0].ID
				}
				got[x.ID] = artist
			}
			for _, id := range part {
				// An id Apple did not answer is kept empty too: asked again
				// only after appleItemFresh.
				_, _ = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO apple_item_artists(kind, id, artist, at) VALUES(?,?,?,?)`, kind, id, got[id], now.Unix())
			}
			for i := range rows {
				if rows[i].AppleKind == kind && rows[i].AppleArtist == "" {
					if a, ok := got[rows[i].AppleID]; ok {
						rows[i].AppleArtist = a
					}
				}
			}
		}
	}
	return nil
}

func digits(s string) bool { return s != "" && len(s) <= 20 && strings.Trim(s, "0123456789") == "" }

func unique(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// fillAllAppleArtists fills every storefront's chart rows (fillAppleArtists)
// and ranks its artists again; it reports whether a row gained an id.
func (s *Service) fillAllAppleArtists(ctx context.Context) bool {
	s.amu.Lock()
	stores := make([]string, 0, len(s.apple))
	for st := range s.apple {
		stores = append(stores, st)
	}
	s.amu.Unlock()
	changed := false
	fails, filled := 0, 0
	var last error
	defer func() {
		if fails > 0 || filled > 0 {
			log.Printf("musictrend: apple artist ids: %d storefronts gained ids, %d failed (last: %v)", filled, fails, last)
		}
	}()
	for _, st := range stores {
		s.amu.Lock()
		c := s.apple[st]
		albums, songs := append([]Entry(nil), c.albums...), append([]Entry(nil), c.songs...)
		s.amu.Unlock()
		before := withArtist(albums) + withArtist(songs)
		if err := s.fillAppleArtists(ctx, st, albums); err != nil {
			fails++
			last = err
			continue
		}
		if err := s.fillAppleArtists(ctx, st, songs); err != nil {
			fails++
			last = err
			continue
		}
		if withArtist(albums)+withArtist(songs) == before {
			continue
		}
		s.amu.Lock()
		s.apple[st] = chart{artists: RankArtists(songs, albums), albums: albums, songs: songs}
		s.amu.Unlock()
		changed = true
		filled++
	}
	return changed
}

func withArtist(rows []Entry) int {
	n := 0
	for _, e := range rows {
		if e.AppleArtist != "" {
			n++
		}
	}
	return n
}
