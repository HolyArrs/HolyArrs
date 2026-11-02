package extras

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

// Fetcher is the server's paced provider client.
type Fetcher interface {
	Fetch(context.Context, model.Query) (model.Result, error)
}

// TMDBToTVDB returns a TVDB lookup over the server's TMDB client
// (/tv/{id}/external_ids). ErrNotFound when the show has no TVDB id.
func TMDBToTVDB(f Fetcher) func(context.Context, string) (string, error) {
	return func(ctx context.Context, tmdb string) (string, error) {
		path := "/tv/" + tmdb + "/external_ids"
		params, err := model.TMDBRead(path, url.Values{})
		if err != nil {
			return "", ErrNotFound
		}
		res, err := f.Fetch(ctx, model.Query{Provider: "tmdb", Kind: "raw", Operation: model.OperationTMDB, Path: path, Params: params, Page: 1, Limit: 20})
		if err != nil {
			return "", err
		}
		var ids struct {
			TVDB int64 `json:"tvdb_id"`
		}
		if json.Unmarshal(res.Data, &ids) != nil || ids.TVDB <= 0 {
			return "", ErrNotFound
		}
		return strconv.FormatInt(ids.TVDB, 10), nil
	}
}
