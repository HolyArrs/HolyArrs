package musicdata

import (
	"context"
	"net/url"
	"strconv"

	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
)

// similarMax is how many similar artists Apple Music is asked for.
const similarMax = 10

// AppleSimilar answers the artists Apple Music's artist view lists as
// similar to the seed, best first (nil when Apple has none). The caller
// maps them to MusicBrainz ids.
func (s *Service) AppleSimilar(ctx context.Context, seed string) ([]musictrend.NamedArtist, error) {
	st, id, err := s.o.Sources.AppleArtist(ctx, seed)
	if err != nil || id == "" {
		return nil, err
	}
	var d struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		} `json:"data"`
	}
	path := "/v1/catalog/" + url.PathEscape(st) + "/artists/" + url.PathEscape(id) + "/view/similar-artists"
	if _, err := s.o.Sources.AppleJSON(ctx, path, url.Values{"limit": {strconv.Itoa(similarMax)}}, &d); err != nil {
		return nil, err
	}
	var out []musictrend.NamedArtist
	for _, x := range d.Data {
		if x.Attributes.Name != "" && len(out) < similarMax {
			out = append(out, musictrend.NamedArtist{ID: x.ID, Name: x.Attributes.Name})
		}
	}
	return out, nil
}
