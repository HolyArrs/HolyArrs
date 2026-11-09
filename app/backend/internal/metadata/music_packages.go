package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Artist packages: the metadata server builds one package per covered
// artist (header, every release group with its cover record, the picked
// albums' track lists, the artist's picture records). An install copies
// them all on a fixed schedule (the music package copy), so a music page
// answers from local disk and never waits on the server.

// PackageGroup is one release group in a package.
type PackageGroup struct {
	ID               string         `json:"id"`
	Title            string         `json:"title"`
	Kind             string         `json:"kind"`
	PrimaryType      string         `json:"primary_type,omitempty"`
	SecondaryTypes   []string       `json:"secondary_types"`
	FirstReleaseDate string         `json:"first_release_date,omitempty"`
	Disambiguation   string         `json:"disambiguation,omitempty"`
	Cover            *PictureRecord `json:"cover,omitempty"`
	// Tracklist: "stored" (in Albums), "pending" or "not_picked".
	Tracklist string `json:"tracklist"`
}

// MusicPackage is one artist's package.
type MusicPackage struct {
	SchemaVersion         int                       `json:"schema_version"`
	MBID                  string                    `json:"mbid"`
	Version               int64                     `json:"version"`
	Hash                  string                    `json:"hash"`
	BuiltAt               time.Time                 `json:"built_at"`
	Rank                  int                       `json:"rank"`
	Complete              bool                      `json:"complete"`
	Missing               []string                  `json:"missing"`
	Artist                json.RawMessage           `json:"artist"`
	Pictures              map[string]*PictureRecord `json:"pictures"`
	ReleaseGroupCount     int                       `json:"release_group_count"`
	ReleaseGroupsComplete bool                      `json:"release_groups_complete"`
	Kinds                 map[string]int            `json:"kinds"`
	ReleaseGroups         []PackageGroup            `json:"release_groups"`
	Albums                map[string]MusicAlbum     `json:"albums"`
}

// Valid says the package can be stored: schema 1, a MusicBrainz id, a
// version and sane release groups.
func (p *MusicPackage) Valid() bool {
	if p.SchemaVersion != 1 || !ValidMBID(p.MBID) || p.Version <= 0 || len(p.Artist) == 0 {
		return false
	}
	for _, g := range p.ReleaseGroups {
		if !ValidMBID(g.ID) {
			return false
		}
	}
	return true
}

// PackageArtist is the package's artist with every release group as an
// album; Partial when the server's list is not the whole discography.
func (p *MusicPackage) PackageArtist() (Artist, error) {
	var item serviceItem
	if json.Unmarshal(p.Artist, &item) != nil {
		return Artist{}, ErrServiceInvalid
	}
	a, err := parseServiceArtist(item, false)
	if err != nil {
		return Artist{}, err
	}
	a.Popularity = servicePopularity(item)
	a.Albums = make([]Album, 0, len(p.ReleaseGroups))
	for _, g := range p.ReleaseGroups {
		if strings.TrimSpace(g.Title) == "" {
			continue
		}
		sec := make([]string, 0, len(g.SecondaryTypes))
		for _, s := range g.SecondaryTypes {
			sec = append(sec, strings.ToLower(s))
		}
		a.Albums = append(a.Albums, Album{MBID: strings.ToLower(g.ID), Name: g.Title, PrimaryType: strings.ToLower(g.PrimaryType),
			SecondaryTypes: sec, ReleaseDate: g.FirstReleaseDate, Disambiguation: g.Disambiguation})
	}
	a.Partial = !p.ReleaseGroupsComplete
	return a, nil
}

// Pressings is the stored edition of one release group with its tracks;
// ErrListCut when the album has more editions than the stored one.
func (p *MusicPackage) Pressings(rg string) ([]Pressing, bool, error) {
	al, ok := p.Albums[strings.ToLower(rg)]
	if !ok {
		return nil, false, nil
	}
	ps, ok := al.pressings()
	if !ok {
		return nil, false, nil
	}
	if al.EditionCount > 1 {
		return ps, true, ErrListCut
	}
	return ps, true, nil
}

// PackagePage is one page of GET /v1/music/packages.
type PackagePage struct {
	SchemaVersion int               `json:"schema_version"`
	Since         int64             `json:"since"`
	Next          int64             `json:"next"`
	More          bool              `json:"more"`
	Latest        int64             `json:"latest"`
	Packages      []json.RawMessage `json:"packages"`
}

// packagePageMax bounds one page (the server keeps pages near 8 MB).
const packagePageMax = 64 << 20

// MusicPackages reads one page of packages changed after since. It sends
// no credential: the list is public, so an install's copy says nothing
// about the install.
func (s *Service) MusicPackages(ctx context.Context, since int64, limit int) (PackagePage, error) {
	var out PackagePage
	q := url.Values{"since": {strconv.FormatInt(since, 10)}, "limit": {strconv.Itoa(min(max(limit, 1), 200))}}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/v1/music/packages?"+q.Encode(), nil)
	req.Header.Set("Accept", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return out, ErrServiceUnavailable
	}
	defer resp.Body.Close()
	if err := packageStatus(resp.StatusCode); err != nil {
		return out, err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, packagePageMax+1))
	if err != nil {
		return out, ErrServiceUnavailable
	}
	if len(body) > packagePageMax || json.Unmarshal(body, &out) != nil || out.SchemaVersion != 1 || out.Next < since {
		return PackagePage{}, ErrServiceInvalid
	}
	return out, nil
}

func packageStatus(code int) error {
	switch {
	case code == http.StatusOK:
		return nil
	case code == http.StatusNotFound:
		return ErrNotFound
	case code == http.StatusTooManyRequests:
		return ErrServiceBusy
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return ErrServiceAccess
	}
	return ErrServiceUnavailable
}

// ErrPackageCovered says a package-want found the artist covered already.
var ErrPackageCovered = errors.New("artist package already covered")

// WantMusicPackage asks the server to build the artist's package (an id
// only). nil means queued; ErrPackageCovered that it exists already.
func (s *Service) WantMusicPackage(ctx context.Context, mbid string) error {
	if !ValidMBID(mbid) {
		return ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tok, err := s.access(ctx)
	if err != nil || tok == "" || strings.ContainsAny(tok, "\r\n") {
		return ErrServiceAccess
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/v1/music/package-want/"+strings.ToLower(mbid), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return ErrServiceUnavailable
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	_ = resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusAccepted:
		return nil
	case http.StatusOK:
		return ErrPackageCovered
	}
	return packageStatus(resp.StatusCode)
}

// LocalMusic answers artists and albums from the packages kept on this
// install's disk. ok false means not covered here.
type LocalMusic interface {
	LocalArtist(ctx context.Context, mbid string) (Artist, bool)
	LocalPressings(ctx context.Context, rg string) ([]Pressing, bool, error)
	LocalAlbumArtist(ctx context.Context, rg string) (string, bool)
	// Uncovered is told an artist a page asked for that no package covers.
	Uncovered(mbid string)
}

type localHolder struct{ l LocalMusic }

// SetLocalMusic makes the hosted music source answer from l first.
func (s *Service) SetLocalMusic(l LocalMusic) { s.local.Store(&localHolder{l}) }

func (s *Service) localMusic() LocalMusic {
	if h := s.local.Load(); h != nil {
		return h.l
	}
	return nil
}
