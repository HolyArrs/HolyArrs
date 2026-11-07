package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type serviceMovies struct {
	service *Service
	direct  Provider
}

// Movies uses the optional service for TMDB searches and TMDB identities.
// Other identity kinds retain the direct provider path.
func (s *Service) Movies(direct Provider) Provider {
	return &serviceMovies{service: s, direct: direct}
}

func (p *serviceMovies) Search(ctx context.Context, query string, year int) ([]Movie, error) {
	ctx, cancel := serviceContext(ctx)
	defer cancel()
	if year != 0 {
		if p.direct != nil {
			return p.direct.Search(ctx, query, year)
		}
		return nil, serviceIncomplete("movie year search needs the direct provider")
	}

	res, err := p.service.search(ctx, "tmdb", "movie", query, 1, 20)
	if err == nil {
		out := make([]Movie, 0, len(res.Items))
		for _, item := range res.Items {
			m, decodeErr := serviceMovie(item, false)
			if decodeErr != nil {
				err = decodeErr
				break
			}
			out = append(out, m)
		}
		if err == nil {
			return out, nil
		}
	}
	if p.direct != nil && serviceFallback(ctx, err) {
		return p.direct.Search(ctx, query, year)
	}
	return nil, err
}

func (p *serviceMovies) Movie(ctx context.Context, id string) (Movie, error) {
	ctx, cancel := serviceContext(ctx)
	defer cancel()
	kind, value, ok := splitID(id)
	if !ok {
		return Movie{}, ErrNotFound
	}
	if kind != "tmdb" {
		if p.direct != nil {
			return p.direct.Movie(ctx, id)
		}
		return Movie{}, ErrNotFound
	}
	if !serviceID("tmdb", value) {
		return Movie{}, ErrNotFound
	}

	item, err := p.service.detail(ctx, "tmdb", "movie", value)
	if err == nil {
		var out Movie
		out, err = serviceMovie(item, true)
		if err == nil {
			return out, nil
		}
	}
	if p.direct != nil && serviceFallback(ctx, err) {
		return p.direct.Movie(ctx, id)
	}
	return Movie{}, err
}

// Changed stays on the direct provider because the standalone service does
// not expose TMDB's changed-title feed.
func (p *serviceMovies) Changed(ctx context.Context, since time.Time) ([]string, error) {
	ctx, cancel := serviceContext(ctx)
	defer cancel()
	if direct, ok := p.direct.(ChangeLister); ok {
		return direct.Changed(ctx, since)
	}
	return nil, ErrNoChangeList
}

func serviceMovie(item serviceItem, detail bool) (Movie, error) {
	if item.Source != "tmdb" || item.Kind != "movie" || !serviceID("tmdb", item.SourceID) {
		return Movie{}, ErrServiceInvalid
	}
	var raw tmdbMovie
	if err := item.decode(&raw); err != nil || raw.ID <= 0 || strconv.FormatInt(raw.ID, 10) != item.SourceID {
		return Movie{}, ErrServiceInvalid
	}
	var err error
	if raw.Title, err = serviceTitleText(raw.Title, false); err != nil || raw.Title == "" {
		return Movie{}, ErrServiceInvalid
	}
	if raw.Overview, err = serviceTitleText(raw.Overview, true); err != nil {
		return Movie{}, ErrServiceInvalid
	}
	if raw.OriginalLanguage, err = serviceTitleText(raw.OriginalLanguage, false); err != nil {
		return Movie{}, ErrServiceInvalid
	}
	if raw.PosterPath != "" && !servicePosterPath(raw.PosterPath) {
		return Movie{}, ErrServiceInvalid
	}
	if raw.IMDbID != "" && !serviceExternalID(raw.IMDbID) {
		return Movie{}, ErrServiceInvalid
	}
	if raw.Runtime < 0 || raw.Runtime > 24*60 {
		return Movie{}, ErrServiceInvalid
	}
	collectionKnown := false
	if detail {
		var fields map[string]json.RawMessage
		if json.Unmarshal(item.SourceData, &fields) != nil {
			return Movie{}, ErrServiceInvalid
		}
		_, collectionKnown = fields["belongs_to_collection"]
	}
	if raw.BelongsToCollection != nil {
		if raw.BelongsToCollection.ID <= 0 {
			return Movie{}, ErrServiceInvalid
		}
		name, err := serviceTitleText(raw.BelongsToCollection.Name, false)
		if err != nil || name == "" {
			return Movie{}, ErrServiceInvalid
		}
		raw.BelongsToCollection.Name = name
	}
	return raw.movieWithCollection(ImageBase, collectionKnown), nil
}

func serviceTitleText(value string, markup bool) (string, error) {
	if !utf8.ValidString(value) || len(value) > 64<<10 {
		return "", ErrServiceInvalid
	}
	if markup {
		value = plain(value)
	} else {
		value = strings.TrimSpace(value)
	}
	for _, r := range value {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return "", ErrServiceInvalid
		}
	}
	return value, nil
}

func servicePosterPath(value string) bool {
	return strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") && len(value) <= 2048 &&
		!strings.ContainsAny(value, "\\?#\r\n\t") && !strings.Contains(value, "..")
}

func serviceExternalID(value string) bool {
	return value != "" && len(value) <= 80 && !strings.ContainsAny(value, "/\\?#% \t\r\n")
}

func serviceIncomplete(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrServiceIncomplete, fmt.Sprintf(format, args...))
}
