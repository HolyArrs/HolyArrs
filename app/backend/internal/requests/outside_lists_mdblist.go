package requests

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// ErrMDBListKey means the saved MDBList key could not be read.
var ErrMDBListKey = errors.New("the MDBList key could not be read; save it again in Outside lists settings")

// mdblistPage is how many items one MDBList API read asks for. Free keys
// allow 1000 requests a day, so a sync should take few reads.
const mdblistPage = 1000

// mdblistItem is one list item. The API names the TVDB id tvdb_id; the
// public link names it tvdbid.
type mdblistItem struct {
	ID        int64  `json:"id"` // TMDB id
	IMDB      string `json:"imdb_id"`
	TVDB      int64  `json:"tvdb_id"`
	TVDBSite  int64  `json:"tvdbid"`
	MediaType string `json:"mediatype"`
	Title     string `json:"title"`
}

// entry maps an item to the ids a list gives; TVDB ids are kept for shows only.
func (it mdblistItem) entry(titleKind string) entry {
	e := entry{imdb: it.IMDB, name: it.Title}
	if it.ID > 0 {
		e.tmdb = strconv.FormatInt(it.ID, 10)
	}
	if tvdb := max(it.TVDB, it.TVDBSite); titleKind == "show" && tvdb > 0 {
		e.tvdb = strconv.FormatInt(tvdb, 10)
	}
	return e
}

// fetchMDBList reads an MDBList list (ref user/slug): through the API when
// the owner saved an MDBList key, by its public link otherwise. A failing
// API read is returned as is, never retried on the public link, so a bad
// key shows up on the list instead of being hidden. Nothing goes out while
// lookups are off.
func (s *OutsideLists) fetchMDBList(ctx context.Context, ref, titleKind string) ([]entry, error) {
	if s.TMDBRoute != nil {
		if _, err := s.TMDBRoute(ctx); errors.Is(err, metadata.ErrSourceOff) {
			return nil, ErrListsOff
		}
	}
	key := ""
	if s.MDBListKey != nil {
		k, err := s.MDBListKey(ctx)
		if err != nil {
			return nil, ErrMDBListKey // never a quiet switch to the public link
		}
		key = strings.TrimSpace(k)
	}
	user, slug, _ := strings.Cut(ref, "/")
	path := "/lists/" + url.PathEscape(user) + "/" + url.PathEscape(slug)
	if key == "" {
		return s.fetchMDBListSite(ctx, path, titleKind)
	}
	return s.fetchMDBListAPI(ctx, path, key, titleKind)
}

// fetchMDBListSite reads the list's public JSON link in one read; it holds
// both movies and shows, so only those of the list's kind are kept.
func (s *OutsideLists) fetchMDBListSite(ctx context.Context, path, titleKind string) ([]entry, error) {
	base := strings.TrimRight(s.MDBListSite, "/")
	if base == "" {
		base = "https://mdblist.com"
	}
	var items []mdblistItem
	if _, err := s.get(ctx, "MDBList", base, base+path+"/json", nil, &items); err != nil {
		return nil, err
	}
	out := []entry{}
	for _, it := range items {
		if it.MediaType == titleKind {
			out = append(out, it.entry(titleKind))
		}
	}
	return out, nil
}

// fetchMDBListAPI pages the API's list items, up to maxPages reads.
func (s *OutsideLists) fetchMDBListAPI(ctx context.Context, path, key, titleKind string) ([]entry, error) {
	base := strings.TrimRight(s.MDBListBase, "/")
	if base == "" {
		base = "https://api.mdblist.com"
	}
	out := []entry{}
	for page, offset := 0, 0; page < maxPages; page++ {
		var body struct {
			Movies     []mdblistItem `json:"movies"`
			Shows      []mdblistItem `json:"shows"`
			Pagination struct {
				HasMore bool `json:"has_more"`
			} `json:"pagination"`
		}
		q := url.Values{"apikey": {key}, "limit": {strconv.Itoa(mdblistPage)}, "offset": {strconv.Itoa(offset)}}
		if _, err := s.getWith(ctx, sameHost(s.client()), "MDBList", base, base+path+"/items?"+q.Encode(), nil, &body, key); err != nil {
			return nil, err
		}
		items := body.Movies
		if titleKind == "show" {
			items = body.Shows
		}
		for _, it := range items {
			out = append(out, it.entry(titleKind))
		}
		// limit and offset count movies and shows together (checked on a mixed
		// list: each page held limit items across both, total was combined).
		offset += len(body.Movies) + len(body.Shows)
		if !body.Pagination.HasMore || len(body.Movies)+len(body.Shows) == 0 {
			break
		}
	}
	return out, nil
}

// sameHost is c refusing redirects to another host: the MDBList key rides
// in the query, which a followed redirect would carry along.
func sameHost(c *http.Client) *http.Client {
	cp := *c
	cp.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || req.URL.Host != via[0].URL.Host {
			return http.ErrUseLastResponse
		}
		return nil
	}
	return &cp
}
