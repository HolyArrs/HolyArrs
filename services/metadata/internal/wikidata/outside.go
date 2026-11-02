package wikidata

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

// OutsideIDs are an artist item's ids at the music sources: Apple Music
// artist ID (P2850), Deezer artist ID (P2722), Spotify artist ID (P1902).
type OutsideIDs struct {
	AppleMusic string `json:"applemusic,omitempty"`
	Deezer     string `json:"deezer,omitempty"`
	Spotify    string `json:"spotify,omitempty"`
}

// OutsideIDs answers the outside ids of the given items. Each answer is
// kept (found or empty, ArtistFresh long): items asked before cost no
// call; the rest are read 50 at a time. An item Wikidata does not have is
// answered empty.
func (s *Service) OutsideIDs(ctx context.Context, qids []string) (map[string]OutsideIDs, error) {
	out := map[string]OutsideIDs{}
	now := s.opts.Now()
	var need []string
	for _, q := range qids {
		if _, done := out[q]; done || !qidOnly.MatchString(q) {
			continue
		}
		var body []byte
		var at int64
		if s.db.QueryRowContext(ctx, `SELECT body, fetched_at FROM answers WHERE k=?`, "outside:"+q).Scan(&body, &at) == nil &&
			now.Unix()-at < int64(s.opts.ArtistFresh.Seconds()) {
			var ids OutsideIDs
			if json.Unmarshal(body, &ids) == nil {
				out[q] = ids
				continue
			}
		}
		need = append(need, q)
		out[q] = OutsideIDs{}
	}
	for len(need) > 0 {
		n := min(50, len(need))
		var got struct {
			Entities map[string]item `json:"entities"`
		}
		if err := s.api(ctx, url.Values{"action": {"wbgetentities"}, "ids": {strings.Join(need[:n], "|")}, "props": {"claims"}}, &got); err != nil {
			return out, err
		}
		for _, q := range need[:n] {
			var ids OutsideIDs
			if it, ok := got.Entities[q]; ok && it.Missing == nil {
				ids = OutsideIDs{AppleMusic: best(it.Claims["P2850"]).str(), Deezer: best(it.Claims["P2722"]).str(), Spotify: best(it.Claims["P1902"]).str()}
			}
			out[q] = ids
			if b, err := json.Marshal(ids); err == nil {
				_, _ = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO answers VALUES (?, ?, ?)`, "outside:"+q, b, now.Unix())
			}
		}
		need = need[n:]
	}
	return out, nil
}

// qidOnly is a bare item id.
var qidOnly = regexp.MustCompile(`^Q[0-9]+$`)
