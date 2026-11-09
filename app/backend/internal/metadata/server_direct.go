package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// These read, from the metadata server, what the app once asked Deezer,
// Wikipedia and MusicBrainz for itself. Installs never call outside
// sources: the server asks once, keeps the answer and refreshes it on a
// schedule, so its cost never grows with installs.

// MusicDeezerArtist reads the server's Deezer match for an artist: its
// albums with explicit marks, fans and top songs (GET
// /v1/music/deezer/artist/{mbid}). A zero ID means no match.
func (s *Service) MusicDeezerArtist(ctx context.Context, mbid string) (DeezerProfile, error) {
	if !ValidMBID(mbid) {
		return DeezerProfile{}, ErrServiceInvalid
	}
	var out struct {
		Artist struct {
			ID      int64         `json:"id"`
			Picture string        `json:"picture"`
			Fans    int64         `json:"fans"`
			Top     []DeezerTrack `json:"top"`
			Albums  []DeezerAlbum `json:"albums"`
		} `json:"artist"`
	}
	err := s.musicGet(ctx, "/v1/music/deezer/artist/"+strings.ToLower(mbid), url.Values{}, &out)
	if errors.Is(err, ErrNotFound) {
		return DeezerProfile{}, nil
	}
	if err != nil {
		return DeezerProfile{}, err
	}
	a := out.Artist
	return DeezerProfile{ID: a.ID, Picture: a.Picture, Fans: a.Fans, Top: a.Top, Albums: a.Albums}, nil
}

// MusicDeezerAlbum reads one Deezer album's label and per-track explicit
// marks from the server (GET /v1/music/deezer/album/{id}).
func (s *Service) MusicDeezerAlbum(ctx context.Context, id int64) (DeezerAlbumInfo, error) {
	info := DeezerAlbumInfo{Tracks: map[string]bool{}}
	if id <= 0 {
		return info, nil
	}
	var out struct {
		Album struct {
			Label    string `json:"label"`
			Explicit bool   `json:"explicit"`
			Tracks   []struct {
				Title    string `json:"title"`
				Explicit bool   `json:"explicit"`
			} `json:"tracks"`
		} `json:"album"`
	}
	err := s.musicGet(ctx, "/v1/music/deezer/album/"+strconv.FormatInt(id, 10), url.Values{}, &out)
	if errors.Is(err, ErrNotFound) {
		return info, nil
	}
	if err != nil {
		return info, err
	}
	info.Label, info.Explicit = out.Album.Label, out.Album.Explicit
	for _, t := range out.Album.Tracks {
		info.Tracks[SongKey(t.Title)] = t.Explicit
	}
	return info, nil
}

// MusicArtistBio reads an artist's Wikipedia summary from the server
// (GET /v1/wikidata/artist/{mbid}: the article its Wikidata item links);
// nil when there is none.
func (s *Service) MusicArtistBio(ctx context.Context, mbid string) (*WikiSummary, error) {
	if !ValidMBID(mbid) {
		return nil, ErrServiceInvalid
	}
	e, err := s.wikiEntity(ctx, "/v1/wikidata/artist/"+strings.ToLower(mbid))
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return e.Summary, nil
}

// MusicReleases reads the release groups first out from..to (YYYY-MM-DD)
// by the given artists (at most 90), of the given primary types, from the
// server's new-music calendar and stored discographies (GET
// /v1/music/releases). No search is made anywhere.
func (s *Service) MusicReleases(ctx context.Context, from, to string, types, artists []string) ([]NewRelease, error) {
	q := url.Values{"from": {from}, "to": {to}}
	var ids []string
	for _, a := range artists {
		if ValidMBID(a) && len(ids) < 90 {
			ids = append(ids, strings.ToLower(a))
		}
	}
	if len(ids) > 0 {
		q.Set("artists", strings.Join(ids, ","))
	}
	if len(types) > 0 {
		q.Set("types", strings.ToLower(strings.Join(types, ",")))
	}
	var out struct {
		Items []struct {
			MBID           string   `json:"mbid"`
			ArtistMBID     string   `json:"artist_mbid"`
			Title          string   `json:"title"`
			Artist         string   `json:"artist_name"`
			ReleaseDate    string   `json:"release_date"`
			Type           string   `json:"type"`
			SecondaryTypes []string `json:"secondary_types"`
			Tags           []string `json:"tags"`
		} `json:"items"`
	}
	if err := s.musicGet(ctx, "/v1/music/releases", q, &out); err != nil {
		return nil, err
	}
	list := []NewRelease{}
	for _, it := range out.Items {
		day := it.ReleaseDate
		if len(day) > 10 {
			day = day[:10]
		}
		if !ValidMBID(it.MBID) || it.Title == "" || len(day) != 10 || !ValidMBID(it.ArtistMBID) {
			continue
		}
		r := NewRelease{MBID: strings.ToLower(it.MBID), Name: it.Title, PrimaryType: strings.ToLower(it.Type), ReleaseDate: day,
			Artist: it.Artist, ArtistMBID: strings.ToLower(it.ArtistMBID), Tags: it.Tags}
		for _, st := range it.SecondaryTypes {
			r.SecondaryTypes = append(r.SecondaryTypes, strings.ToLower(st))
		}
		list = append(list, r)
	}
	return list, nil
}

// serviceArtistRels is what the server's stored MusicBrainz artist answer
// says of its genres and links (url-rels and artist-rels are included).
type serviceArtistRels struct {
	Genres []struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	} `json:"genres"`
	Relations []struct {
		Type   string `json:"type"`
		Artist struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"artist"`
		URL struct {
			Resource string `json:"resource"`
		} `json:"url"`
	} `json:"relations"`
}

// artistRels reads the server's stored MusicBrainz artist answer (the same
// one the artist page uses, kept at the edge).
func (m *serviceMusic) artistRels(ctx context.Context, mbid string) (serviceArtistRels, error) {
	var a serviceArtistRels
	if !ValidMBID(mbid) {
		return a, ErrArtistNotFound
	}
	ctx, cancel := serviceContext(ctx)
	defer cancel()
	item, err := m.service.detail(ctx, "musicbrainz", "artist", strings.ToLower(mbid))
	if err != nil && !(errors.Is(err, ErrServicePending) && item.Source != "") {
		return a, serviceMusicError(err)
	}
	if json.Unmarshal(item.SourceData, &a) != nil {
		return a, ErrServiceInvalid
	}
	return a, nil
}

// ArtistIDs reads an artist's Deezer and Wikidata ids from the links on the
// server's stored MusicBrainz answer.
func (m *serviceMusic) ArtistIDs(ctx context.Context, mbid string) (ArtistIDs, error) {
	a, err := m.artistRels(ctx, mbid)
	if err != nil {
		return ArtistIDs{}, err
	}
	links := make([]string, 0, len(a.Relations))
	for _, r := range a.Relations {
		if r.URL.Resource != "" {
			links = append(links, r.URL.Resource)
		}
	}
	return parseArtistURLs(links), nil
}

// ArtistGenres reads an artist's genres from the server's stored answer.
func (m *serviceMusic) ArtistGenres(ctx context.Context, mbid string) ([]string, error) {
	g, _, err := m.ArtistGenresLinks(ctx, mbid)
	return g, err
}

// ArtistGenresLinks reads an artist's top genres and the artists it worked
// with (members, bands, collaborations) from the server's stored answer.
func (m *serviceMusic) ArtistGenresLinks(ctx context.Context, mbid string) ([]string, []ArtistLink, error) {
	a, err := m.artistRels(ctx, mbid)
	if err != nil {
		return nil, nil, err
	}
	sort.SliceStable(a.Genres, func(i, j int) bool { return a.Genres[i].Count > a.Genres[j].Count })
	out := []string{}
	for _, g := range a.Genres {
		if n := strings.ToLower(strings.TrimSpace(g.Name)); n != "" && g.Count > 0 && len(out) < 5 {
			out = append(out, n)
		}
	}
	links := []ArtistLink{}
	seen := map[string]bool{strings.ToLower(mbid): true}
	for _, r := range a.Relations {
		id := strings.ToLower(r.Artist.ID)
		switch r.Type {
		case "member of band", "collaboration", "is person", "supporting musician", "subgroup", "founder", "artistic director":
		default:
			continue
		}
		if ValidMBID(id) && r.Artist.Name != "" && !seen[id] {
			seen[id] = true
			links = append(links, ArtistLink{MBID: id, Name: r.Artist.Name, Type: r.Type})
		}
	}
	return out, links, nil
}

// SoundtrackFor finds a title's soundtrack albums through the server's
// stored MusicBrainz search (the server asks once for every install).
func (m *serviceMusic) SoundtrackFor(ctx context.Context, title string, year int) ([]Album, error) {
	q := strings.TrimSpace(title)
	if q == "" {
		return []Album{}, nil
	}
	ctx, cancel := serviceContext(ctx)
	defer cancel()
	query := `releasegroup:"` + luceneSpecial.ReplaceAllString(q, `\$1`) + `" AND secondarytype:soundtrack`
	res, err := m.service.search(ctx, "musicbrainz", "release-group", query, 1, 5)
	if errors.Is(err, ErrNotFound) {
		return []Album{}, nil
	}
	if err != nil {
		return nil, serviceMusicError(err)
	}
	out := []Album{}
	low := strings.ToLower(q)
	for _, item := range res.Items {
		var g struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Score *int   `json:"score"`
			First string `json:"first-release-date"`
		}
		if item.decode(&g) != nil || !ValidMBID(g.ID) || (g.Score != nil && *g.Score < 80) || !strings.Contains(strings.ToLower(g.Title), low) {
			continue
		}
		if year > 0 && len(g.First) >= 4 {
			if y, err := strconv.Atoi(g.First[:4]); err != nil || y < year-1 || y > year+1 {
				continue
			}
		}
		out = append(out, Album{MBID: strings.ToLower(g.ID), Name: g.Title, PrimaryType: "album", SecondaryTypes: []string{"soundtrack"}, ReleaseDate: g.First})
	}
	return out, nil
}

// ReleasesBetween reads new releases by the given artists from the server
// (its calendar and stored discographies): no MusicBrainz search.
func (m *serviceMusic) ReleasesBetween(ctx context.Context, from, to string, types, artists []string, limit int) ([]NewRelease, error) {
	list, err := m.service.MusicReleases(ctx, from, to, types, artists)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}
