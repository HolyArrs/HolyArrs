package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// NewRelease is one release group out (or coming) in a date window, with
// its first credited artist.
type NewRelease struct {
	MBID           string   `json:"mbid"`
	Name           string   `json:"name"`
	PrimaryType    string   `json:"type"` // album, ep, single
	SecondaryTypes []string `json:"secondary_types,omitempty"`
	ReleaseDate    string   `json:"release_date"` // YYYY-MM-DD
	Artist         string   `json:"artist"`
	ArtistMBID     string   `json:"artist_mbid"`
	Tags           []string `json:"tags,omitempty"`
}

// ReleasesBetween searches MusicBrainz for release groups of the given
// primary types (album, ep, single) first out from..to (YYYY-MM-DD, both
// included), by the given artists when any are named. One paced request.
func (m *MusicBrainz) ReleasesBetween(ctx context.Context, from, to string, types, artists []string, limit int) ([]NewRelease, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	query := "firstreleasedate:[" + from + " TO " + to + "]"
	if len(types) > 0 {
		query += " AND primarytype:(" + strings.Join(types, " OR ") + ")"
	}
	var ids []string
	for _, a := range artists {
		if ValidMBID(a) {
			ids = append(ids, a)
		}
	}
	if len(ids) > 0 {
		query += " AND arid:(" + strings.Join(ids, " OR ") + ")"
	}
	b, err := m.get(ctx, "/ws/2/release-group", url.Values{"query": {query}, "limit": {fmt.Sprint(limit)}})
	if errors.Is(err, ErrArtistNotFound) {
		return []NewRelease{}, nil
	}
	if err != nil {
		return nil, err
	}
	return parseNewReleases(b)
}

func parseNewReleases(b []byte) ([]NewRelease, error) {
	var res struct {
		Groups []struct {
			ID        string   `json:"id"`
			Title     string   `json:"title"`
			Primary   string   `json:"primary-type"`
			Secondary []string `json:"secondary-types"`
			First     string   `json:"first-release-date"`
			Credit    []struct {
				Name   string `json:"name"`
				Artist struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"artist"`
			} `json:"artist-credit"`
			Tags []struct {
				Name  string `json:"name"`
				Count int    `json:"count"`
			} `json:"tags"`
		} `json:"release-groups"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("MusicBrainz: odd answer")
	}
	out := []NewRelease{}
	for _, g := range res.Groups {
		if !ValidMBID(g.ID) || g.Title == "" || len(g.First) != 10 || len(g.Credit) == 0 || !ValidMBID(g.Credit[0].Artist.ID) {
			continue
		}
		r := NewRelease{MBID: strings.ToLower(g.ID), Name: g.Title, PrimaryType: strings.ToLower(g.Primary), ReleaseDate: g.First,
			Artist: g.Credit[0].Artist.Name, ArtistMBID: strings.ToLower(g.Credit[0].Artist.ID)}
		for _, s := range g.Secondary {
			r.SecondaryTypes = append(r.SecondaryTypes, strings.ToLower(s))
		}
		for _, t := range g.Tags {
			if t.Count > 0 {
				r.Tags = append(r.Tags, strings.ToLower(t.Name))
			}
		}
		out = append(out, r)
	}
	return out, nil
}
