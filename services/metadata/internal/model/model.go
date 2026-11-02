package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Query struct {
	Provider  string `json:"provider"`
	Kind      string `json:"kind"`
	Operation string `json:"operation"`
	ID        string `json:"id,omitempty"`
	Season    int    `json:"season,omitempty"`
	Q         string `json:"q,omitempty"`
	Language  string `json:"language,omitempty"`
	Region    string `json:"region,omitempty"`
	Page      int    `json:"page,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	// Path and Params are the TMDB path and canonical parameters of an
	// OperationTMDB read.
	Path   string `json:"path,omitempty"`
	Params string `json:"params,omitempty"`
}

func (q Query) Key() string {
	b, _ := json.Marshal(struct {
		Version int
		Query   Query
	}{1, q})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (q Query) Validate() error {
	if q.Operation == OperationTMDB || q.Path != "" || q.Params != "" {
		if q.Operation != OperationTMDB || q.Provider != "tmdb" || q.Kind != "raw" || q.ID != "" || q.Q != "" {
			return fmt.Errorf("invalid TMDB read")
		}
		v, err := url.ParseQuery(q.Params)
		if err != nil {
			return fmt.Errorf("invalid TMDB read")
		}
		canon, err := TMDBRead(q.Path, v)
		if err != nil || canon != q.Params {
			return fmt.Errorf("invalid TMDB read")
		}
		return nil
	}
	if len(q.Q) > 256 || len(q.ID) > 80 || strings.ContainsAny(q.ID, "/\\?#%") || len(q.Language) > 12 || len(q.Region) > 8 || q.Page < 1 || q.Page > 100 || q.Limit < 1 || q.Limit > 100 || q.Season < 0 || q.Season > 1000 {
		return fmt.Errorf("invalid query bounds")
	}
	allowed := (q.Provider == "tmdb" && (q.Kind == "movie" || q.Kind == "show")) || (q.Provider == "tvmaze" && q.Kind == "show") || (q.Provider == "musicbrainz" && (q.Kind == "artist" || q.Kind == "release-group" || q.Kind == "release" || q.Kind == "recording"))
	if !allowed {
		return fmt.Errorf("unsupported provider or kind")
	}
	if q.Operation == "search" && q.Provider == "tvmaze" && q.Page != 1 {
		return fmt.Errorf("provider has no search pages")
	}
	if q.Operation != "search" {
		if q.Provider == "musicbrainz" {
			if len(q.ID) != 36 {
				return fmt.Errorf("invalid music identity")
			}
			for i, c := range q.ID {
				if i == 8 || i == 13 || i == 18 || i == 23 {
					if c != '-' {
						return fmt.Errorf("invalid music identity")
					}
				} else if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
					return fmt.Errorf("invalid music identity")
				}
			}
		} else {
			n, e := strconv.ParseInt(q.ID, 10, 64)
			if e != nil || n <= 0 || strconv.FormatInt(n, 10) != q.ID {
				return fmt.Errorf("invalid numeric identity")
			}
		}
	}
	if q.Operation == "search" && strings.TrimSpace(q.Q) != "" && q.ID == "" {
		return nil
	}
	if q.Operation == "detail" && q.ID != "" && q.Q == "" {
		return nil
	}
	if q.Operation == "season" && q.Kind == "show" && q.Provider == "tmdb" && q.ID != "" && q.Q == "" {
		return nil
	}
	return fmt.Errorf("unsupported operation")
}

type Result struct {
	Data       json.RawMessage `json:"data"`
	Complete   bool            `json:"complete"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

type Entry struct {
	Key       string
	Result    Result
	FetchedAt time.Time
	ExpiresAt time.Time
}

// ErrRegistryFull: every stored self-registered credential is protected
// (recently used, or not the caller's network), so none can be evicted.
var ErrRegistryFull = errors.New("registry full")

type FetchError struct {
	Code       string
	Status     int
	RetryAfter time.Duration
}

func (e *FetchError) Error() string { return e.Code }
