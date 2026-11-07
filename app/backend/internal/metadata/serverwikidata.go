package metadata

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// WikiAward is one award received or nomination (Wikidata, CC0).
type WikiAward struct {
	Name    string `json:"name"`
	Family  string `json:"family"`
	Won     bool   `json:"won"`
	Year    int    `json:"year"`
	ForWork string `json:"for_work"`
}

// WikiTally counts one major award family (oscar, emmy, golden_globe,
// bafta, grammy).
type WikiTally struct {
	Family      string `json:"family"`
	Wins        int    `json:"wins"`
	Nominations int    `json:"nominations"`
}

// WikiSummary is Wikipedia's short summary with its attribution
// (CC BY-SA 4.0).
type WikiSummary struct {
	Title       string `json:"title"`
	Extract     string `json:"extract"`
	URL         string `json:"url"`
	License     string `json:"license"`
	LicenseURL  string `json:"license_url"`
	Attribution string `json:"attribution"`
}

// WikiEntity is the metadata server's Wikidata answer for a title or person.
type WikiEntity struct {
	QID        string       `json:"qid"`
	Awards     []WikiAward  `json:"awards"`
	Tallies    []WikiTally  `json:"tallies"`
	Born       string       `json:"born"`
	Died       string       `json:"died"`
	BirthPlace string       `json:"birth_place"`
	DeathPlace string       `json:"death_place"`
	Wikipedia  string       `json:"wikipedia"`
	Summary    *WikiSummary `json:"summary"`
	// Image is a person's photo for when TMDB has none: the server's stored
	// copy of the Wikimedia Commons photo ({server}/v1/img/person/...), or a
	// TVmaze link.
	Image string `json:"image"`
}

// AwardWinner is one title on the server's award-winner list.
type AwardWinner struct {
	TMDB int64 `json:"tmdb"`
	Wins int   `json:"wins"`
}

// WikiTitle reads awards for a TMDB movie or show.
func (s *Service) WikiTitle(ctx context.Context, kind, id string) (WikiEntity, error) {
	if kind == "show" {
		kind = "tv"
	}
	if (kind != "movie" && kind != "tv") || !serviceID("tmdb", id) {
		return WikiEntity{}, ErrServiceInvalid
	}
	return s.wikiEntity(ctx, "/v1/wikidata/title/"+kind+"/"+id)
}

// WikiPerson reads awards, life facts and the Wikipedia bio for a TMDB person;
// with the person's name, the server also looks for a photo on TVmaze when Wikidata has none.
func (s *Service) WikiPerson(ctx context.Context, id, name string) (WikiEntity, error) {
	if !serviceID("tmdb", id) {
		return WikiEntity{}, ErrServiceInvalid
	}
	path := "/v1/wikidata/person/" + id
	if name = strings.TrimSpace(name); name != "" && len(name) <= 100 {
		path += "?name=" + url.QueryEscape(name)
	}
	return s.wikiEntity(ctx, path)
}

func (s *Service) wikiEntity(ctx context.Context, path string) (WikiEntity, error) {
	var out WikiEntity
	if err := s.wikiGet(ctx, path, 1<<20, &out); err != nil {
		return WikiEntity{}, err
	}
	if out.Summary != nil && (!wikipediaURL(out.Summary.URL) || out.Summary.Extract == "") {
		out.Summary = nil
	}
	if out.Summary != nil && !strings.HasPrefix(out.Summary.LicenseURL, "https://creativecommons.org/") {
		out.Summary.LicenseURL = ccBySA
	}
	return out, nil
}

const ccBySA = "https://creativecommons.org/licenses/by-sa/4.0/"

// wikipediaURL reports an https link on a *.wikipedia.org host.
func wikipediaURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return u.Scheme == "https" && u.User == nil && (h == "wikipedia.org" || strings.HasSuffix(h, ".wikipedia.org"))
}

// AwardWinners reads the server's award-winning TMDB IDs for a kind (movie
// or show), most wins first.
func (s *Service) AwardWinners(ctx context.Context, kind string) ([]AwardWinner, error) {
	if kind == "show" {
		kind = "tv"
	}
	if kind != "movie" && kind != "tv" {
		return nil, ErrServiceInvalid
	}
	var out struct {
		Results []AwardWinner `json:"results"`
	}
	if err := s.wikiGet(ctx, "/v1/wikidata/award-winners?kind="+kind, 4<<20, &out); err != nil {
		return nil, err
	}
	return out.Results, nil
}

func (s *Service) wikiGet(ctx context.Context, path string, limit int64, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	tok, err := s.access(ctx)
	if err != nil || tok == "" || strings.ContainsAny(tok, "\r\n") {
		return ErrServiceAccess
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.base+path, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return ErrServiceUnavailable
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		return ErrServiceBusy
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrServiceAccess
	case resp.StatusCode != http.StatusOK:
		return ErrServiceUnavailable
	}
	if json.NewDecoder(io.LimitReader(resp.Body, limit)).Decode(out) != nil {
		return ErrServiceInvalid
	}
	return nil
}

// AwardBadges turns tallies into short labels, most notable first: "Oscar
// winner", "3 Emmys", "Golden Globe nominee".
func AwardBadges(ts []WikiTally) []string {
	names := map[string][2]string{"oscar": {"Oscar", "Oscars"}, "emmy": {"Emmy", "Emmys"}, "golden_globe": {"Golden Globe", "Golden Globes"}, "bafta": {"BAFTA", "BAFTAs"}, "grammy": {"Grammy", "Grammys"}}
	var won, nominated []string
	for _, t := range ts {
		n, ok := names[t.Family]
		switch {
		case !ok:
		case t.Wins == 1:
			won = append(won, n[0]+" winner")
		case t.Wins > 1:
			won = append(won, strconv.Itoa(t.Wins)+" "+n[1])
		case t.Nominations > 0:
			nominated = append(nominated, n[0]+" nominee")
		}
	}
	return append(won, nominated...)
}
