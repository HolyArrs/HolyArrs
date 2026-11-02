package kworb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Spotify artist ids are matched to MusicBrainz ids by links only, never
// by name: the Spotify link on a MusicBrainz artist (a batched URL lookup,
// 100 a call, or the url-relations of a stored artist answer, or the
// picture chains' match when it came from that link), else Wikidata's
// Spotify artist ID (P1902) on an item with one MusicBrainz artist id
// (P434), 100 a query. A pair is kept for good; an id neither source links
// is asked again after missRetry.

// Default endpoints.
const (
	MusicBrainzURL = "https://musicbrainz.org/ws/2"
	SPARQLURL      = "https://query.wikidata.org/sparql"
	spotifyPage    = "https://open.spotify.com/artist/"
)

// missRetry is how long an id no source links waits to be asked again.
const missRetry = 7 * 24 * time.Hour

// matchBatch is how many ids one MusicBrainz lookup or Wikidata query asks;
// mbBusyWait is the rest before a busy MusicBrainz is asked once more.
const (
	matchBatch = 100
	mbBusyWait = 10 * time.Second
)

var (
	spotifyID   = regexp.MustCompile(`^[0-9A-Za-z]{22}$`)
	spotifyLink = regexp.MustCompile(`^https?://open\.spotify\.com/(?:intl-[a-z]+/)?artist/([0-9A-Za-z]{22})`)
	mbidRe      = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// statusErr is a source's answer other than 200 or 404 (asked again later).
type statusErr struct {
	name string
	code int
}

func (e *statusErr) Error() string { return fmt.Sprintf("kworb: %s answered %d", e.name, e.code) }

func (s *Service) getJSON(ctx context.Context, c *http.Client, name, u, accept string, out any) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", s.o.UserAgent)
	req.Header.Set("Accept", accept)
	resp, err := c.Do(req)
	if err != nil {
		return false, fmt.Errorf("kworb: %s: request failed", name)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return false, nil
	case resp.StatusCode != http.StatusOK:
		return false, &statusErr{name, resp.StatusCode}
	}
	return true, json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
}

type mbURL struct {
	Resource  string `json:"resource"`
	Relations []struct {
		Target string `json:"target-type"`
		Artist struct {
			ID string `json:"id"`
		} `json:"artist"`
	} `json:"relations"`
}

// oneArtist is the single artist a URL is linked to ("" for none or two).
func (u mbURL) oneArtist() string {
	id := ""
	for _, r := range u.Relations {
		if r.Target != "artist" || !mbidRe.MatchString(r.Artist.ID) {
			continue
		}
		if id != "" && id != r.Artist.ID {
			return ""
		}
		id = r.Artist.ID
	}
	return id
}

// mbByLinks looks up the Spotify artist pages of ids on MusicBrainz in one
// call and answers the artist each is linked to.
func (s *Service) mbByLinks(ctx context.Context, ids []string) (map[string]string, error) {
	q := url.Values{"inc": {"artist-rels"}, "fmt": {"json"}}
	for _, id := range ids {
		q.Add("resource", spotifyPage+id)
	}
	var raw json.RawMessage
	ok, err := s.getJSON(ctx, s.o.MBHTTP, "musicbrainz", s.o.MBURL+"/url?"+q.Encode(), "application/json", &raw)
	out := map[string]string{}
	if err != nil || !ok {
		return out, err
	}
	// One resource answers the URL itself; more answer a list of them.
	var many struct {
		URLs []mbURL `json:"urls"`
	}
	var one mbURL
	if json.Unmarshal(raw, &many) != nil || many.URLs == nil {
		if err := json.Unmarshal(raw, &one); err != nil {
			return out, err
		}
		many.URLs = []mbURL{one}
	}
	for _, u := range many.URLs {
		m := spotifyLink.FindStringSubmatch(u.Resource)
		if m == nil {
			continue
		}
		if a := u.oneArtist(); a != "" {
			out[m[1]] = a
		}
	}
	return out, nil
}

type sparqlRows struct {
	Results struct {
		Bindings []map[string]struct {
			Value string `json:"value"`
		} `json:"bindings"`
	} `json:"results"`
}

// wikidataPairs asks Wikidata for items carrying both ids: from Spotify ids
// (fromSpotify) or from MusicBrainz ids. A value linked to two of the
// other kind is left out.
func (s *Service) wikidataPairs(ctx context.Context, values []string, fromSpotify bool) (map[string]string, error) {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = `"` + v + `"`
	}
	from := "?mb"
	if fromSpotify {
		from = "?sp"
	}
	query := "SELECT ?sp ?mb WHERE { VALUES " + from + " { " + strings.Join(quoted, " ") + " } ?item wdt:P1902 ?sp ; wdt:P434 ?mb . }"
	var d sparqlRows
	if _, err := s.getJSON(ctx, s.o.WDHTTP, "wikidata", s.o.SPARQLURL+"?"+url.Values{"query": {query}}.Encode(), "application/sparql-results+json", &d); err != nil {
		return nil, err
	}
	key, val := "mb", "sp"
	if fromSpotify {
		key, val = "sp", "mb"
	}
	got := map[string]map[string]bool{}
	for _, b := range d.Results.Bindings {
		k, v := b[key].Value, b[val].Value
		if got[k] == nil {
			got[k] = map[string]bool{}
		}
		got[k][v] = true
	}
	out := map[string]string{}
	for k, vs := range got {
		if len(vs) != 1 {
			continue
		}
		for v := range vs {
			if (fromSpotify && mbidRe.MatchString(v)) || (!fromSpotify && spotifyID.MatchString(v)) {
				out[k] = v
			}
		}
	}
	return out, nil
}

// pairBySpotify and pairByMBID answer a kept pair from either side.
func (s *Service) pairBySpotify(id string) string {
	var mb string
	_ = s.db.QueryRow(`SELECT mbid FROM pairs WHERE spotify = ?`, id).Scan(&mb)
	return mb
}

func (s *Service) pairByMBID(mbid string) string {
	var sp string
	_ = s.db.QueryRow(`SELECT spotify FROM pairs WHERE mbid = ? ORDER BY at LIMIT 1`, mbid).Scan(&sp)
	return sp
}

func (s *Service) keepPair(sp, mbid, how string) {
	_, _ = s.db.Exec(`INSERT OR IGNORE INTO pairs (spotify, mbid, how, at) VALUES (?, ?, ?, ?)`, sp, mbid, how, s.o.Now().Unix())
	_, _ = s.db.Exec(`DELETE FROM tried WHERE k IN (?, ?)`, "sp:"+sp, "mb:"+mbid)
}

func (s *Service) triedRecently(k string) bool {
	var until int64
	return s.db.QueryRow(`SELECT until FROM tried WHERE k = ?`, k).Scan(&until) == nil && s.o.Now().Unix() < until
}

func (s *Service) markTried(k string) {
	_, _ = s.db.Exec(`INSERT OR REPLACE INTO tried (k, until) VALUES (?, ?)`, k, s.o.Now().Add(missRetry).Unix())
}

// matchSpotify pairs Spotify ids (best first) with MusicBrainz ids. It
// stops at the first failed call (the rest are asked on the next run).
func (s *Service) matchSpotify(ctx context.Context, ids []string) (matched int, err error) {
	var todo []string
	for _, id := range ids {
		if s.pairBySpotify(id) != "" || s.triedRecently("sp:"+id) {
			continue
		}
		if s.o.KeptArtist != nil {
			if mb, ok := s.o.KeptArtist(id); ok && mbidRe.MatchString(mb) {
				s.keepPair(id, mb, "picture")
				matched++
				continue
			}
		}
		todo = append(todo, id)
	}
	for len(todo) > 0 && ctx.Err() == nil {
		n := min(matchBatch, len(todo))
		batch := todo[:n]
		todo = todo[n:]
		got := map[string]string{}
		if s.o.MBHTTP != nil {
			got, err = s.mbByLinks(ctx, batch)
			var se *statusErr
			if errors.As(err, &se) && se.code == http.StatusServiceUnavailable && s.o.sleep(ctx, mbBusyWait) == nil {
				// MusicBrainz is busy now and then: one more try after a rest.
				got, err = s.mbByLinks(ctx, batch)
			}
			if err != nil {
				s.count(func(st *Status) { st.MatchFailures++ })
				return matched, err
			}
		}
		var rest []string
		for _, id := range batch {
			if mb := got[id]; mb != "" {
				s.keepPair(id, mb, "musicbrainz")
				matched++
			} else {
				rest = append(rest, id)
			}
		}
		if len(rest) == 0 || s.o.WDHTTP == nil {
			continue
		}
		wd, err := s.wikidataPairs(ctx, rest, true)
		if err != nil {
			s.count(func(st *Status) { st.MatchFailures++ })
			return matched, err
		}
		for _, id := range rest {
			if mb := wd[id]; mb != "" {
				s.keepPair(id, mb, "wikidata")
				matched++
			} else {
				s.markTried("sp:" + id)
			}
		}
	}
	return matched, ctx.Err()
}

// localSpotify answers an artist's Spotify id without a call: the kept
// pair, the picture chains' link match, then the stored answer's Spotify
// link (two different links name neither).
func (s *Service) localSpotify(ctx context.Context, mbid string) string {
	if sp := s.pairByMBID(mbid); sp != "" {
		return sp
	}
	if s.o.KeptSpotify != nil {
		if sp, ok := s.o.KeptSpotify(mbid); ok && spotifyID.MatchString(sp) {
			s.keepPair(sp, mbid, "picture")
			return sp
		}
	}
	if s.o.StoredLinks == nil {
		return ""
	}
	found := ""
	for _, l := range s.o.StoredLinks(ctx, mbid) {
		if m := spotifyLink.FindStringSubmatch(l); m != nil {
			if found != "" && found != m[1] {
				return ""
			}
			found = m[1]
		}
	}
	if found != "" {
		s.keepPair(found, mbid, "musicbrainz")
	}
	return found
}

// wikidataSpotify asks Wikidata for the Spotify ids of artists no link
// names yet, 100 a query; those it does not know wait missRetry.
func (s *Service) wikidataSpotify(ctx context.Context, mbids []string) error {
	if s.o.WDHTTP == nil {
		return nil
	}
	for len(mbids) > 0 && ctx.Err() == nil {
		n := min(matchBatch, len(mbids))
		got, err := s.wikidataPairs(ctx, mbids[:n], false)
		if err != nil {
			s.count(func(st *Status) { st.MatchFailures++ })
			return err
		}
		for _, mb := range mbids[:n] {
			if sp := got[mb]; sp != "" {
				s.keepPair(sp, mb, "wikidata")
			} else {
				s.markTried("mb:" + mb)
			}
		}
		mbids = mbids[n:]
	}
	return ctx.Err()
}
