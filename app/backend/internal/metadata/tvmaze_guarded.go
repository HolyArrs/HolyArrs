package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

// GuardedTVMaze asks TVmaze directly (Your own keys) and reads every answer
// with the metadata server's own decoder and checks, so a show looks the
// same either way: the same text limits, picture and id checks, and the
// same episode checks.
type GuardedTVMaze struct{ *TVMaze }

// SearchShows finds shows by name on TVmaze; a result that fails the
// server's checks is left out.
func (g GuardedTVMaze) SearchShows(ctx context.Context, query string) ([]Show, error) {
	b, err := g.get(ctx, "/search/shows", url.Values{"q": {query}})
	if errors.Is(err, ErrShowNotFound) {
		return []Show{}, nil
	}
	if err != nil {
		return nil, err
	}
	var res []struct {
		Show json.RawMessage `json:"show"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("TVMaze: odd answer: %w", err)
	}
	out := make([]Show, 0, len(res))
	for _, r := range res {
		item, ok := mazeItem(r.Show)
		if !ok {
			continue
		}
		if s, _, err := serviceTVMazeShowItem(item); err == nil {
			out = append(out, s)
		}
	}
	return out, nil
}

// Show takes "tvmaze:<id>", or "tvdb:<id>" or "imdb:<tt id>" looked up on
// TVmaze, and returns the show with its episodes and specials.
func (g GuardedTVMaze) Show(ctx context.Context, id string) (Show, error) {
	kind, v, ok := splitID(id)
	if !ok {
		return Show{}, ErrShowNotFound
	}
	var b []byte
	var err error
	switch kind {
	case "tvmaze":
		if !serviceID("tvmaze", v) {
			return Show{}, ErrShowNotFound
		}
		b, err = g.get(ctx, "/shows/"+v, nil)
	case "tvdb":
		b, err = g.get(ctx, "/lookup/shows", url.Values{"thetvdb": {v}})
	case "imdb":
		b, err = g.get(ctx, "/lookup/shows", url.Values{"imdb": {v}})
	default:
		return Show{}, ErrShowNotFound
	}
	if err != nil {
		return Show{}, err
	}
	item, ok := mazeItem(b)
	if !ok {
		return Show{}, ErrServiceInvalid
	}
	eps, err := g.get(ctx, "/shows/"+item.SourceID+"/episodes", url.Values{"specials": {"1"}})
	if err != nil {
		return Show{}, err
	}
	var shape map[string]json.RawMessage
	if json.Unmarshal(item.SourceData, &shape) != nil {
		return Show{}, ErrServiceInvalid
	}
	shape["_embedded"], _ = json.Marshal(map[string]json.RawMessage{"episodes": eps})
	if item.SourceData, err = json.Marshal(shape); err != nil {
		return Show{}, ErrServiceInvalid
	}
	return serviceTVMazeDetail(item)
}

// mazeItem wraps one TVmaze show answer the way the server sends it.
func mazeItem(raw []byte) (serviceItem, bool) {
	var id struct {
		ID int64 `json:"id"`
	}
	if json.Unmarshal(raw, &id) != nil || id.ID <= 0 {
		return serviceItem{}, false
	}
	return serviceItem{Source: "tvmaze", Kind: "show", SourceID: strconv.FormatInt(id.ID, 10), SourceData: raw}, true
}

// ModeShows asks the metadata server, or the source directly, as the mode
// says; off answers ErrSourceOff.
type ModeShows struct {
	Modes  *Modes
	Server ShowProvider
	Direct ShowProvider
}

func (p ModeShows) pick() (ShowProvider, error) {
	switch p.Modes.Current() {
	case ModeServer:
		return p.Server, nil
	case ModeOwn:
		if p.Direct != nil {
			return p.Direct, nil
		}
	}
	return nil, ErrSourceOff
}

// SearchShows finds shows with the source the mode picks.
func (p ModeShows) SearchShows(ctx context.Context, query string) ([]Show, error) {
	src, err := p.pick()
	if err != nil {
		return nil, err
	}
	return src.SearchShows(ctx, query)
}

// Show looks one show up with the source the mode picks.
func (p ModeShows) Show(ctx context.Context, id string) (Show, error) {
	src, err := p.pick()
	if err != nil {
		return Show{}, err
	}
	return src.Show(ctx, id)
}
