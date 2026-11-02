package musictrend

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Artist data for the app's scored Music rows (plan 2026-10-22 §5,
// endpoints 4 and 5): normalised similarity per seed, and each artist's
// genre votes and top release groups from ListenBrainz (keyless). Answers
// are kept a week; a failed fetch is retried after an hour. Requests carry
// only artist ids, never anything about a member.

// ListenBrainzAPIURL is the ListenBrainz API base.
const ListenBrainzAPIURL = "https://api.listenbrainz.org"

// SimilarScore is one artist like a seed with S = score / the seed's top
// score (ListenBrainz), so the best match is 1.
type SimilarScore struct {
	MBID   string  `json:"mbid"`
	Name   string  `json:"name"`
	S      float64 `json:"s"`
	Source string  `json:"source"`
}

// GenreVote is one genre tag on an artist and its vote count.
type GenreVote struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// TopAlbum is one release group of an artist's, by listeners.
type TopAlbum struct {
	MBID      string   `json:"mbid"`
	Title     string   `json:"title"`
	Type      string   `json:"type"`
	Date      string   `json:"date,omitempty"`
	Listeners int64    `json:"listeners"`
	Artists   []string `json:"artists"` // credited artist ids, primary first
	Names     []string `json:"artist_names"`
}

// ArtistData is one artist's genre votes and top release groups.
type ArtistData struct {
	MBID   string      `json:"mbid"`
	Genres []GenreVote `json:"genres"`
	Albums []TopAlbum  `json:"albums"`
}

type artistDataEntry struct {
	data    *ArtistData
	dataErr bool
	dataAt  time.Time
}

// artistStore keeps artist data, per artist id.
type artistStore struct {
	mu sync.Mutex
	m  map[string]*artistDataEntry
}

func (s *Similar) apiBase() string {
	if s.APIURL != "" {
		return strings.TrimSuffix(s.APIURL, "/")
	}
	return ListenBrainzAPIURL
}

func (s *Similar) client() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (s *Similar) entry(id string) *artistDataEntry {
	if s.store.m == nil {
		s.store.m = map[string]*artistDataEntry{}
	}
	e := s.store.m[id]
	if e == nil {
		if len(s.store.m) >= s.max() {
			s.store.m = map[string]*artistDataEntry{}
		}
		e = &artistDataEntry{}
		s.store.m[id] = e
	}
	return e
}

// Scores answers the artists like seed with S normalised by the seed's
// top score. A seed with no answer or a top score of 0 has none.
func (s *Similar) Scores(ctx context.Context, seed string) ([]SimilarScore, error) {
	body, _, err := s.Artists(ctx, seed)
	if errors.Is(err, ErrNoSimilar) {
		return []SimilarScore{}, nil
	}
	if err != nil {
		return nil, err
	}
	var v struct {
		Source  string          `json:"source"`
		Artists []SimilarArtist `json:"artists"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, err
	}
	return NormaliseSimilar(v.Artists, v.Source), nil
}

// NormaliseSimilar divides each score by the top one; a top of 0 (or an
// empty list) gives no rows. source is where the list came from.
func NormaliseSimilar(list []SimilarArtist, source string) []SimilarScore {
	var top int64
	for _, a := range list {
		top = max(top, a.Score)
	}
	out := []SimilarScore{}
	if top <= 0 {
		return out
	}
	for _, a := range list {
		if a.Score <= 0 {
			continue
		}
		out = append(out, SimilarScore{MBID: a.MBID, Name: a.Name, S: float64(a.Score) / float64(top), Source: source})
	}
	return out
}

// Data answers one artist's genre votes and top release groups (cached).
func (s *Similar) Data(ctx context.Context, id string) (*ArtistData, error) {
	id = strings.ToLower(id)
	if !validMBID(id) {
		return nil, ErrNoSimilar
	}
	now := s.now()
	s.mu.Lock()
	e := s.entry(id)
	if e.dataAt.IsZero() && s.loadDataLocked(id) {
		e = s.entry(id)
	}
	if !e.dataAt.IsZero() {
		ttl := s.ttl(similarEntry{})
		if e.dataErr {
			ttl = s.ttl(similarEntry{err: errors.New("miss")})
		}
		if now.Sub(e.dataAt) < ttl {
			d, bad := e.data, e.dataErr
			s.mu.Unlock()
			if bad && d == nil {
				return nil, ErrNoSimilar
			}
			return d, nil
		}
	}
	s.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.apiBase()+"/1/popularity/top-release-groups-for-artist/"+id, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.UserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := s.client().Do(req)
	if err != nil {
		s.report("artist_data_failed")
		return nil, err
	}
	defer resp.Body.Close()
	var d *ArtistData
	switch {
	case resp.StatusCode == http.StatusNotFound:
	case resp.StatusCode != http.StatusOK:
		s.report("artist_data_failed")
		return nil, errors.New("musictrend: artist data answered " + resp.Status)
	default:
		d, err = ParseLBArtistData(io.LimitReader(resp.Body, 16<<20), id)
		if err != nil {
			return nil, err
		}
	}
	s.mu.Lock()
	e = s.entry(id)
	e.data, e.dataErr, e.dataAt = d, d == nil, now
	s.keepDataLocked(id, d, now)
	s.mu.Unlock()
	if d == nil {
		return nil, ErrNoSimilar
	}
	return d, nil
}

func (s *Similar) report(code string) {
	if s.Report != nil {
		s.Report("listenbrainz", code)
	}
}

// artistDataAlbums caps the release groups kept per artist.
const artistDataAlbums = 40

// ParseLBArtistData reads the top release groups answer: the artist's
// genre votes (tags that are MusicBrainz genres, with at least one vote)
// and up to 40 release groups by listeners.
func ParseLBArtistData(r io.Reader, id string) (*ArtistData, error) {
	var rows []struct {
		Artist struct {
			Artists []struct {
				MBID string `json:"artist_mbid"`
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"artist"`
		RG struct {
			Name string `json:"name"`
			Date string `json:"date"`
			Type string `json:"type"`
		} `json:"release_group"`
		RGID  string `json:"release_group_mbid"`
		Users int64  `json:"total_user_count"`
		Tag   struct {
			Artist []struct {
				MBID  string `json:"artist_mbid"`
				Count int    `json:"count"`
				Tag   string `json:"tag"`
				Genre string `json:"genre_mbid"`
			} `json:"artist"`
		} `json:"tag"`
	}
	if err := json.NewDecoder(r).Decode(&rows); err != nil {
		return nil, err
	}
	d := &ArtistData{MBID: id, Genres: []GenreVote{}, Albums: []TopAlbum{}}
	votes := map[string]int{}
	var order []string
	seen := map[string]bool{}
	for _, x := range rows {
		for _, t := range x.Tag.Artist {
			name := strings.ToLower(strings.TrimSpace(t.Tag))
			if strings.ToLower(t.MBID) != id || t.Genre == "" || t.Count < 1 || name == "" {
				continue
			}
			if _, ok := votes[name]; !ok {
				order = append(order, name)
			}
			votes[name] = max(votes[name], t.Count)
		}
		rg := strings.ToLower(x.RGID)
		if !validMBID(rg) || seen[rg] || len(d.Albums) >= artistDataAlbums || strings.TrimSpace(x.RG.Name) == "" {
			continue
		}
		seen[rg] = true
		al := TopAlbum{MBID: rg, Title: strings.TrimSpace(x.RG.Name), Type: strings.ToLower(x.RG.Type), Date: x.RG.Date, Listeners: max(0, x.Users), Artists: []string{}, Names: []string{}}
		for _, a := range x.Artist.Artists {
			if m := strings.ToLower(a.MBID); validMBID(m) {
				al.Artists = append(al.Artists, m)
				al.Names = append(al.Names, strings.TrimSpace(a.Name))
			}
		}
		d.Albums = append(d.Albums, al)
	}
	for _, n := range order {
		d.Genres = append(d.Genres, GenreVote{Name: n, Count: votes[n]})
	}
	return d, nil
}
