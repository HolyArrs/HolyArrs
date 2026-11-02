package wikidata

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TVmazeURL is TVmaze's person search (free, no key; about 20 calls per 10 s).
const TVmazeURL = "https://api.tvmaze.com/search/people"

// TVmazePhoto is a person's photo from TVmaze, for people neither TMDB nor
// Wikidata has one of. TVmaze has no TMDB ids for people, so the match is by
// name: exactly one person of exactly that name (letter case aside), with a
// photo on static.tvmaze.com. Answers, found or not, are kept per TMDB id and
// name (found for Fresh, a miss for Miss), so each person is asked at most once a while.
func (s *Service) TVmazePhoto(ctx context.Context, tmdb int64, name string) string {
	name = strings.TrimSpace(name)
	if tmdb <= 0 || name == "" || len(name) > 100 {
		return ""
	}
	// The name comes from the caller, so it is part of the key: a wrong name can only ever
	// answer for itself, never change the photo stored for the right one.
	key := "tvmaze:" + strconv.FormatInt(tmdb, 10) + ":" + strings.ToLower(name)
	now := s.opts.Now()
	var body []byte
	var at int64
	if s.db.QueryRowContext(ctx, `SELECT body, fetched_at FROM answers WHERE k=?`, key).Scan(&body, &at) == nil {
		age := now.Sub(time.Unix(at, 0))
		if body == nil && age < s.opts.Miss || body != nil && age < s.opts.Fresh {
			return string(body)
		}
	}
	img, err := s.tvmazeSearch(ctx, name)
	if err != nil {
		return "" // a failed call is asked again next time
	}
	var stored any
	if img != "" {
		stored = []byte(img)
	}
	_, _ = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO answers VALUES (?, ?, ?)`, key, stored, now.Unix())
	return img
}

func (s *Service) tvmazeSearch(ctx context.Context, name string) (string, error) {
	base := s.opts.TVmazeURL
	if base == "" {
		base = TVmazeURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?q="+url.QueryEscape(name), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", s.opts.UserAgent)
	resp, err := s.opts.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", sql.ErrNoRows
	}
	var got []struct {
		Person struct {
			Name  string `json:"name"`
			Image *struct {
				Medium string `json:"medium"`
			} `json:"image"`
		} `json:"person"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&got); err != nil {
		return "", err
	}
	match := ""
	n := 0
	for _, g := range got {
		if !strings.EqualFold(strings.TrimSpace(g.Person.Name), name) {
			continue
		}
		n++
		if g.Person.Image != nil && strings.HasPrefix(g.Person.Image.Medium, "https://static.tvmaze.com/") {
			match = g.Person.Image.Medium
		}
	}
	if n != 1 {
		return "", nil // nobody, or two people with that name: no guess
	}
	return match, nil
}
