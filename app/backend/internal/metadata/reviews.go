package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Review is one written review or comment on a title, as plain text.
type Review struct {
	Author string `json:"author"`
	Date   string `json:"date,omitempty"` // YYYY-MM-DD
	// Rating is the author's own score out of 10; 0 when they gave none.
	Rating  float64 `json:"rating,omitempty"`
	Text    string  `json:"text"`
	Likes   int     `json:"likes,omitempty"`
	Spoiler bool    `json:"spoiler,omitempty"`
}

// ReviewPage is one page of a source's reviews with its score.
type ReviewPage struct {
	Score float64 `json:"score"` // out of 10
	Votes int     `json:"votes"`
	// Distribution counts the votes per point, 1 to 10, when the source
	// shares it.
	Distribution []int `json:"distribution,omitempty"`
	// Watching is how many people the source sees watching right now.
	Watching *int     `json:"watching,omitempty"`
	Reviews  []Review `json:"reviews"`
	Page     int      `json:"page"`
	Pages    int      `json:"pages"`
}

// Review orders.
const (
	ReviewsNewest  = "newest"
	ReviewsHelpful = "helpful"
)

// ReviewLister reads a title's written reviews.
type ReviewLister interface {
	Reviews(ctx context.Context, kind, tmdbID string, page int, order string) (ReviewPage, error)
}

// ReviewListerFor returns the reviews surface selected for ctx.
func ReviewListerFor(ctx context.Context, provider Provider) (ReviewLister, bool) {
	if sw, ok := provider.(*Switch); ok {
		provider = sw.pick(ctx)
	}
	c, ok := provider.(ReviewLister)
	return c, ok
}

var (
	tagRe    = regexp.MustCompile(`<[^>]*>`)
	spacesRe = regexp.MustCompile(`\n{3,}`)
)

// plainText turns third-party review text into plain text: tags dropped,
// control characters removed, long blank runs shortened, capped in size.
func plainText(s string) string {
	s = tagRe.ReplaceAllString(s, "")
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, strings.ReplaceAll(s, "\r\n", "\n"))
	s = strings.TrimSpace(spacesRe.ReplaceAllString(s, "\n\n"))
	if r := []rune(s); len(r) > 20000 {
		s = string(r[:20000]) + "…"
	}
	return s
}

func dayOf(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ""
}

// Reviews reads TMDB's written reviews. TMDB lists them oldest first and
// ranks nothing, so both orders read from the last page backwards.
func (t *TMDB) Reviews(ctx context.Context, kind, tmdbID string, page int, order string) (ReviewPage, error) {
	n, err := strconv.ParseInt(tmdbID, 10, 64)
	if err != nil || n <= 0 || (kind != "movie" && kind != "show") {
		return ReviewPage{}, ErrNotFound
	}
	if page < 1 {
		page = 1
	}
	path := "/" + tvPath(kind) + "/" + strconv.FormatInt(n, 10) + "/reviews"
	type tmdbReviews struct {
		Page    int `json:"page"`
		Pages   int `json:"total_pages"`
		Results []struct {
			Author  string `json:"author"`
			Details struct {
				Name     string   `json:"name"`
				Username string   `json:"username"`
				Rating   *float64 `json:"rating"`
			} `json:"author_details"`
			Content string `json:"content"`
			Created string `json:"created_at"`
		} `json:"results"`
	}
	read := func(p int) (tmdbReviews, error) {
		var res tmdbReviews
		b, err := t.get(ctx, path, url.Values{"page": {strconv.Itoa(p)}})
		if err != nil {
			return res, err
		}
		if err := json.Unmarshal(b, &res); err != nil {
			return res, fmt.Errorf("TMDB: odd answer: %w", err)
		}
		return res, nil
	}
	first, err := read(1)
	if err != nil {
		return ReviewPage{}, err
	}
	pages := first.Pages
	if pages < 1 {
		pages = 1
	}
	out := ReviewPage{Page: page, Pages: pages, Reviews: []Review{}}
	if page > pages {
		return out, nil
	}
	res := first
	if want := pages - page + 1; want != 1 {
		if res, err = read(want); err != nil {
			return ReviewPage{}, err
		}
	}
	for _, r := range res.Results {
		name := r.Details.Name
		if name == "" {
			name = r.Author
		}
		if name == "" {
			name = r.Details.Username
		}
		rv := Review{Author: plainText(name), Date: dayOf(r.Created), Text: plainText(r.Content)}
		if r.Details.Rating != nil && *r.Details.Rating > 0 && *r.Details.Rating <= 10 {
			rv.Rating = *r.Details.Rating
		}
		if rv.Text != "" {
			out.Reviews = append(out.Reviews, rv)
		}
	}
	sort.SliceStable(out.Reviews, func(i, j int) bool { return out.Reviews[i].Date > out.Reviews[j].Date })
	return out, nil
}

// Trakt reads ratings, watchers and comments from Trakt. Its API needs a
// client id and, since July 2026, a VIP account's access token.
type Trakt struct {
	Base     string // default https://api.trakt.tv
	ClientID string
	Token    string
	HTTP     *http.Client
}

// ErrTraktLocked means Trakt refused the key or token (no VIP).
var ErrTraktLocked = errors.New("trakt refused the key")

func (k *Trakt) get(ctx context.Context, path string, into any) (http.Header, error) {
	base := strings.TrimRight(k.Base, "/")
	if base == "" {
		base = "https://api.trakt.tv"
	}
	op, err := outbound.Begin(ctx, outbound.Target{Kind: "metadata_trakt", Identity: []string{base, k.ClientID, k.Token}})
	if err != nil {
		return nil, err
	}
	outcome := outbound.Canceled
	defer func() { op.Finish(outcome) }()
	actx, cancel := op.AttemptContext(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(actx, http.MethodGet, base+path, nil)
	if err != nil {
		outcome = outbound.TerminalFailure
		return nil, err
	}
	req.Header.Set("trakt-api-version", "2")
	req.Header.Set("trakt-api-key", k.ClientID)
	req.Header.Set("Authorization", "Bearer "+k.Token)
	req.Header.Set("Content-Type", "application/json")
	cl := k.HTTP
	if cl == nil {
		cl = http.DefaultClient
	}
	resp, err := cl.Do(req)
	if err != nil {
		outcome = outbound.TransientFailure
		return nil, fmt.Errorf("Trakt did not answer")
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 426:
		outcome = outbound.TerminalFailure
		return nil, ErrTraktLocked
	case resp.StatusCode == 404:
		outcome = outbound.Success
		return nil, ErrNotFound
	case resp.StatusCode == 429:
		outcome = outbound.TransientFailure
		return nil, ErrRateLimited
	case resp.StatusCode != 200:
		outcome = outbound.TransientFailure
		return nil, fmt.Errorf("Trakt answered %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(into); err != nil {
		outcome = outbound.TransientFailure
		return nil, fmt.Errorf("Trakt: odd answer: %w", err)
	}
	outcome = outbound.Success
	return resp.Header, nil
}

// Reviews reads a title's Trakt rating, distribution, watchers and one
// page of comments (newest, or most liked for helpful).
func (k *Trakt) Reviews(ctx context.Context, kind, tmdbID string, page int, order string) (ReviewPage, error) {
	n, err := strconv.ParseInt(tmdbID, 10, 64)
	if err != nil || n <= 0 || (kind != "movie" && kind != "show") {
		return ReviewPage{}, ErrNotFound
	}
	if page < 1 {
		page = 1
	}
	typ, many := "movie", "movies"
	if kind == "show" {
		typ, many = "show", "shows"
	}
	type traktIDs struct {
		IDs struct {
			Trakt int64 `json:"trakt"`
		} `json:"ids"`
	}
	var found []map[string]json.RawMessage
	if _, err := k.get(ctx, "/search/tmdb/"+strconv.FormatInt(n, 10)+"?type="+typ, &found); err != nil {
		return ReviewPage{}, err
	}
	var id int64
	for _, f := range found {
		var m traktIDs
		if raw, ok := f[typ]; ok && json.Unmarshal(raw, &m) == nil && m.IDs.Trakt > 0 {
			id = m.IDs.Trakt
			break
		}
	}
	if id == 0 {
		return ReviewPage{}, ErrNotFound
	}
	base := "/" + many + "/" + strconv.FormatInt(id, 10)
	var rt struct {
		Rating       float64        `json:"rating"`
		Votes        int            `json:"votes"`
		Distribution map[string]int `json:"distribution"`
	}
	if _, err := k.get(ctx, base+"/ratings", &rt); err != nil {
		return ReviewPage{}, err
	}
	out := ReviewPage{Score: float64(int(rt.Rating*10+0.5)) / 10, Votes: rt.Votes, Page: page, Pages: 1, Reviews: []Review{}}
	if len(rt.Distribution) > 0 {
		out.Distribution = make([]int, 10)
		for i := range out.Distribution {
			out.Distribution[i] = rt.Distribution[strconv.Itoa(i+1)]
		}
	}
	var watching []json.RawMessage
	if _, err := k.get(ctx, base+"/watching", &watching); err == nil {
		w := len(watching)
		out.Watching = &w
	}
	sortBy := "newest"
	if order == ReviewsHelpful {
		sortBy = "likes"
	}
	var cs []struct {
		Comment string `json:"comment"`
		Spoiler bool   `json:"spoiler"`
		Created string `json:"created_at"`
		Likes   int    `json:"likes"`
		Rating  *int   `json:"user_rating"`
		User    struct {
			Username string `json:"username"`
			Name     string `json:"name"`
		} `json:"user"`
	}
	h, err := k.get(ctx, base+"/comments/"+sortBy+"?limit=10&page="+strconv.Itoa(page), &cs)
	if err != nil {
		return ReviewPage{}, err
	}
	if p, _ := strconv.Atoi(h.Get("X-Pagination-Page-Count")); p > 0 {
		out.Pages = p
	}
	for _, c := range cs {
		name := c.User.Name
		if name == "" {
			name = c.User.Username
		}
		rv := Review{Author: plainText(name), Date: dayOf(c.Created), Text: plainText(c.Comment), Likes: c.Likes, Spoiler: c.Spoiler}
		if c.Rating != nil && *c.Rating > 0 && *c.Rating <= 10 {
			rv.Rating = float64(*c.Rating)
		}
		if rv.Text != "" {
			out.Reviews = append(out.Reviews, rv)
		}
	}
	return out, nil
}
