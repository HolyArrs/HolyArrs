package metadata

import (
	"context"
	"net/http"
	"strings"
)

// TMDBRoute is where one TMDB request goes: the address, the credential and
// the client. Off means lookups are off: nothing is sent, and answers kept
// for Base still answer.
type TMDBRoute struct {
	Base   string
	Access func(ctx context.Context) (string, error)
	HTTP   *http.Client
	Off    bool
}

func (r TMDBRoute) base() string {
	if r.Base == "" {
		return "https://api.themoviedb.org/3"
	}
	return strings.TrimRight(r.Base, "/")
}

// route is Route's answer, or the fixed BaseURL, Access and HTTP.
func (t *TMDB) route(ctx context.Context) TMDBRoute {
	if t.Route != nil {
		return t.Route(ctx)
	}
	return TMDBRoute{Base: t.base(), Access: t.Access, HTTP: t.HTTP}
}

// keptOnly answers from a kept copy of any age while lookups are off; saved
// copies are never thrown away by the mode.
func (t *TMDB) keptOnly(ctx context.Context, u string, disk bool) ([]byte, error) {
	if c, ok := t.kept(ctx, u, disk); ok && !c.miss && len(c.body) > 0 {
		return c.body, nil
	}
	return nil, ErrSourceOff
}
