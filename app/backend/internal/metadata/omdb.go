package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OMDbHost is where OMDb answers.
const OMDbHost = "www.omdbapi.com"

// OMDb reads IMDb, Rotten Tomatoes and Metacritic scores by IMDb id with
// the owner's own OMDb key. Without a key it asks nothing (ErrSourceOff).
type OMDb struct {
	Base string // default https://www.omdbapi.com
	Key  func(ctx context.Context) (string, error)
	HTTP *http.Client
	// Gap is the least time between requests (default one second).
	Gap time.Duration

	once sync.Once
	g    *getter
}

// OMDbScores are one title's scores; nil when OMDb has none.
type OMDbScores struct {
	IMDbRating     *float64
	IMDbVotes      int
	RottenTomatoes *int // critics, percent
	Metacritic     *int // 0-100
}

// Answers are kept an hour, so a retry costs nothing from the daily
// allowance.
func (o *OMDb) getter() *getter {
	o.once.Do(func() {
		gap := o.Gap
		if gap <= 0 {
			gap = time.Second
		}
		o.g = &getter{name: "OMDb", http: o.HTTP, timeout: 10 * time.Second, gap: gap, ttl: time.Hour}
	})
	return o.g
}

// KeyNow is the saved key ("" when none).
func (o *OMDb) KeyNow(ctx context.Context) string { return ownKey(ctx, o.Key) }

var imdbID = regexp.MustCompile(`^tt[0-9]{5,10}$`)

// Scores reads one title's scores. A refused key or a used-up day answers
// ErrNoAccess; a title OMDb does not know answers ErrNotFound.
func (o *OMDb) Scores(ctx context.Context, id string) (OMDbScores, error) {
	key := o.KeyNow(ctx)
	if key == "" {
		return OMDbScores{}, ErrSourceOff
	}
	if !imdbID.MatchString(id) {
		return OMDbScores{}, ErrNotFound
	}
	u := ownBase(o.Base, "https://"+OMDbHost) + "/?" + url.Values{"i": {id}, "apikey": {key}}.Encode()
	b, err := o.getter().do(ctx, []string{OMDbHost}, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	})
	var es errStatus
	switch {
	case errors.As(err, &es) && (es.code == http.StatusUnauthorized || es.code == http.StatusForbidden):
		// OMDb answers 401 both to a wrong key and to "Request limit reached!".
		return OMDbScores{}, ErrNoAccess
	case errors.As(err, &es) && es.code == http.StatusNotFound:
		return OMDbScores{}, ErrNotFound
	case errors.As(err, &es):
		return OMDbScores{}, ErrServiceUnavailable
	case err != nil:
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // the address holds the key
		}
		return OMDbScores{}, err
	}
	return parseOMDb(b)
}

func parseOMDb(b []byte) (OMDbScores, error) {
	var raw struct {
		Response   string `json:"Response"`
		Error      string `json:"Error"`
		IMDbRating string `json:"imdbRating"`
		IMDbVotes  string `json:"imdbVotes"`
		Metascore  string `json:"Metascore"`
		Ratings    []struct {
			Source string `json:"Source"`
			Value  string `json:"Value"`
		} `json:"Ratings"`
	}
	if json.Unmarshal(b, &raw) != nil {
		return OMDbScores{}, ErrServiceInvalid
	}
	if !strings.EqualFold(raw.Response, "true") {
		low := strings.ToLower(raw.Error)
		if strings.Contains(low, "limit") || strings.Contains(low, "api key") {
			return OMDbScores{}, ErrNoAccess
		}
		return OMDbScores{}, ErrNotFound
	}
	var out OMDbScores
	if v, err := strconv.ParseFloat(raw.IMDbRating, 64); err == nil && v > 0 && v <= 10 {
		out.IMDbRating = &v
		if n, err := strconv.Atoi(strings.ReplaceAll(raw.IMDbVotes, ",", "")); err == nil && n > 0 {
			out.IMDbVotes = n
		}
	}
	percent := func(s string, suffix string) *int {
		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), suffix)))
		if err != nil || n < 0 || n > 100 {
			return nil
		}
		return &n
	}
	for _, r := range raw.Ratings {
		switch r.Source {
		case "Rotten Tomatoes":
			out.RottenTomatoes = percent(r.Value, "%")
		case "Metacritic":
			out.Metacritic = percent(r.Value, "/100")
		}
	}
	if out.Metacritic == nil {
		out.Metacritic = percent(raw.Metascore, "")
	}
	return out, nil
}
