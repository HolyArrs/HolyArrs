package lists

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// linkPage is the public page of a source artist, the form MusicBrainz
// keeps as an artist's link (allow-listed for the brand grep: an address,
// never shown or logged).
const linkPage = "https://open.spotify.com/artist/"

var errNoLink = errors.New("lists: no linked id")

// MBLink answers an artist's MusicBrainz id from its source link with one
// URL lookup (base is https://musicbrainz.org/ws/2; the client paces it
// with every other MusicBrainz call). Albums have no direct link to a
// release group, so they go by name.
func MBLink(c *http.Client, base, userAgent string) func(ctx context.Context, kind, ext string) (string, error) {
	return func(ctx context.Context, kind, ext string) (string, error) {
		if kind != "artist" || ext == "" || strings.ContainsAny(ext, "/?&#") {
			return "", errNoLink
		}
		q := url.Values{"resource": {linkPage + ext}, "inc": {"artist-rels"}, "fmt": {"json"}}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/url?"+q.Encode(), nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "application/json")
		resp, err := c.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", errNoLink
		}
		var body struct {
			Relations []struct {
				Target string `json:"target-type"`
				Artist struct {
					ID string `json:"id"`
				} `json:"artist"`
			} `json:"relations"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return "", err
		}
		id := ""
		for _, r := range body.Relations {
			if r.Target != "artist" || !mbidRe.MatchString(r.Artist.ID) {
				continue
			}
			if id != "" && id != r.Artist.ID {
				return "", errNoLink // one page linked to two artists: names decide
			}
			id = r.Artist.ID
		}
		if id == "" {
			return "", errNoLink
		}
		return id, nil
	}
}
