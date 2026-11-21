package metadata

import (
	"context"
	"net/http"
	"net/url"
)

// TraktRatings reads a title's Trakt rating with the Trakt client id alone;
// no sign-in is needed.
type TraktRatings struct {
	// Key reads the Trakt client id; "" turns the ratings off.
	Key  func(ctx context.Context) (string, error)
	Base string // default https://api.trakt.tv
	HTTP *http.Client
}

// TraktScore is a title's Trakt rating out of 10 and its vote count.
type TraktScore struct {
	Rating float64
	Votes  int
}

// KeyNow is the Trakt client id; "" when none is saved.
func (r *TraktRatings) KeyNow(ctx context.Context) string {
	if r == nil || r.Key == nil {
		return ""
	}
	k, err := r.Key(ctx)
	if err != nil {
		return ""
	}
	return k
}

// Rating reads the Trakt rating of a movie or show (kind) by its IMDb id.
// ErrNotFound: Trakt does not know it. ErrTraktLocked: Trakt refused the
// client id. ErrRateLimited: Trakt asks to slow down.
func (r *TraktRatings) Rating(ctx context.Context, kind, imdb string) (TraktScore, error) {
	key := r.KeyNow(ctx)
	if key == "" {
		return TraktScore{}, ErrNoAccess
	}
	path := "/movies/"
	if kind == "show" {
		path = "/shows/"
	}
	var out struct {
		Rating float64 `json:"rating"`
		Votes  int     `json:"votes"`
	}
	k := &Trakt{Base: r.Base, ClientID: key, HTTP: r.HTTP}
	if _, err := k.get(ctx, path+url.PathEscape(imdb)+"/ratings", &out); err != nil {
		return TraktScore{}, err
	}
	if out.Rating < 0 || out.Rating > 10 || out.Votes < 0 {
		return TraktScore{}, ErrNotFound
	}
	return TraktScore{Rating: out.Rating, Votes: out.Votes}, nil
}
