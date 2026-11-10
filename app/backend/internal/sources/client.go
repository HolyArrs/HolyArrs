package sources

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// maxRetryAfter caps a Source's Retry-After, in seconds (one day).
const maxRetryAfter = 86400

const (
	// Error kinds. They match the reasons a Source can be paused for.
	KindDown         = "down"
	KindRateLimited  = "rate_limited"
	KindLoginRefused = "login_refused"
	KindBroken       = "broken_definition"
	KindBlocked      = "blocked"
)

// Error is a failed request to a Source. It never holds the URL, so the
// Source's key cannot leak into logs.
type Error struct {
	Kind string
	// Detail names the kind of block when Kind is blocked.
	Detail     string
	RetryAfter time.Duration
	Msg        string
	cause      error
}

// Error describes the failure without the URL.
func (e *Error) Error() string { return "source " + e.Kind + ": " + e.Msg }

// Unwrap returns the lower level error, if any.
func (e *Error) Unwrap() error { return e.cause }

// Code is the failure kind in the form the jobs queue records.
func (e *Error) Code() string { return "source_" + e.Kind }

// Config says how to reach one Source and how hard it may be used.
type Config struct {
	URL       string
	APIKey    string
	Protocol  string
	GapMS     int
	PerWindow int
	Window    time.Duration
	Timeout   time.Duration
	HTTP      *http.Client
	// AllowAddr is asked about every address a request would connect to. Nil
	// means public addresses only, or any but metadata ones when AllowLocal is set.
	AllowAddr func(net.IP) bool
	// AllowLocal lets this Source reach the local network.
	AllowLocal bool
	// Format is torznab, newznab or rss. An rss Source is read as it is: its URL
	// plus APIKey (the URL's query) is the whole feed.
	Format string
	// Proxy carries every request when set. Its address goes through the same
	// network policy as the Source unless ProxyAllowLocal is set.
	Proxy           *url.URL
	ProxyAllowLocal bool
	// Observe is told about every call that reached the network.
	Observe func(op string, took time.Duration, err error)
	// resolve looks names up; nil means the system resolver. Tests set it.
	resolve func(ctx context.Context, host string) ([]net.IP, error)
	// PathKeys are passkeys the URL holds in its path. They are never logged.
	PathKeys []string
}

// Client is a Torznab/Newznab client for one Source. It is safe for
// concurrent use, and requests are spaced by the Source's limits.
type Client struct {
	cfg  Config
	http *http.Client

	mu   sync.Mutex
	last time.Time
	hits []time.Time
	caps *Caps
}

// New makes a Client from cfg, filling in the defaults.
func New(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Window <= 0 {
		cfg.Window = time.Hour
	}
	if cfg.AllowAddr == nil {
		cfg.AllowAddr = outbound.PublicOnly
		if cfg.AllowLocal {
			cfg.AllowAddr = outbound.NotMetadata
		}
	}
	var h http.Client
	if cfg.HTTP != nil {
		h = *cfg.HTTP
	} else {
		h = http.Client{Timeout: cfg.Timeout, Transport: transport(cfg)}
	}
	if h.CheckRedirect == nil {
		h.CheckRedirect = sameHostRedirect
	}
	return &Client{cfg: cfg, http: &h}
}

// scrub takes the key and the proxy password out of a message, in every
// spelling a URL may give them.
func (c *Client) scrub(msg string) string {
	hide := func(v string) {
		if v != "" {
			msg = strings.ReplaceAll(msg, v, "[hidden]")
			msg = strings.ReplaceAll(msg, url.QueryEscape(v), "[hidden]")
			msg = strings.ReplaceAll(msg, url.PathEscape(v), "[hidden]")
			_, user, _ := strings.Cut(url.UserPassword("", v).String(), ":")
			msg = strings.ReplaceAll(msg, user, "[hidden]")
		}
	}
	hide(c.cfg.APIKey)
	for _, k := range c.cfg.PathKeys {
		hide(k)
	}
	if c.cfg.Proxy != nil {
		pass, _ := c.cfg.Proxy.User.Password()
		hide(pass)
	}
	return msg
}

// Backoff is how long a Source stays paused after n failures in a row: one
// minute, doubling, at most six hours.
func Backoff(n int) time.Duration {
	d := time.Minute
	for i := 1; i < n && d < 6*time.Hour; i++ {
		d *= 2
	}
	return min(d, 6*time.Hour)
}

// wait spaces requests by the gap and refuses once the window is full.
func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	now := time.Now()
	if c.cfg.PerWindow > 0 {
		keep := c.hits[:0]
		for _, h := range c.hits {
			if now.Sub(h) < c.cfg.Window {
				keep = append(keep, h)
			}
		}
		c.hits = keep
		if len(c.hits) >= c.cfg.PerWindow {
			retry := c.cfg.Window - now.Sub(c.hits[0])
			c.mu.Unlock()
			return &Error{Kind: KindRateLimited, RetryAfter: retry, Msg: fmt.Sprintf("%d requests in the window already", c.cfg.PerWindow)}
		}
	}
	at := now
	if next := c.last.Add(time.Duration(c.cfg.GapMS) * time.Millisecond); next.After(now) {
		at = next
	}
	c.last = at
	if c.cfg.PerWindow > 0 {
		c.hits = append(c.hits, at)
	}
	c.mu.Unlock()
	if d := time.Until(at); d > 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-t.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (c *Client) get(ctx context.Context, op string, q url.Values) ([]byte, error) {
	return c.getWithObserver(ctx, op, q, c.cfg.Observe)
}

func (c *Client) getWithObserver(ctx context.Context, op string, q url.Values, observer func(string, time.Duration, error)) ([]byte, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	raw := feedAddress(c.cfg.URL)
	if c.cfg.Format == "rss" {
		raw = c.cfg.URL
		if c.cfg.APIKey != "" {
			raw += "?" + c.cfg.APIKey
		}
	} else {
		if c.cfg.APIKey != "" {
			q.Set("apikey", c.cfg.APIKey)
		}
		raw += "?" + q.Encode()
	}
	start := time.Now()
	body, err := c.fetch(ctx, raw)
	if observer != nil && ctx.Err() == nil {
		observer(op, time.Since(start), err)
	}
	return body, err
}

func (c *Client) fetch(ctx context.Context, raw string) ([]byte, error) {
	identity := []string{c.cfg.URL, c.cfg.APIKey}
	if c.cfg.Proxy != nil {
		identity = append(identity, c.cfg.Proxy.String())
	}
	op, err := outbound.Begin(ctx, outbound.Target{Kind: "source", Identity: identity})
	if err != nil {
		if errors.Is(err, outbound.ErrSuppressed) {
			return nil, &Error{Kind: KindDown, Msg: err.Error(), cause: err}
		}
		return nil, err
	}
	outcome := outbound.Canceled
	defer func() { op.Finish(outcome) }()
	rctx, cancel := op.AttemptContext(ctx, c.cfg.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, raw, nil)
	if err != nil {
		outcome = outbound.TerminalFailure
		return nil, &Error{Kind: KindBroken, Msg: "bad Source address"}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		outcome = outbound.HTTPOutcome(0, err, ctx)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		if errors.Is(err, outbound.ErrAddressRefused) {
			msg := err.Error()
			if msg == outbound.ErrAddressRefused.Error() {
				msg += ": the address is on the local network, which this Source may not reach"
			}
			return nil, &Error{Kind: KindBlocked, Msg: c.scrub(msg)}
		}
		return nil, &Error{Kind: KindDown, Msg: c.scrub(err.Error())}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if closeErr := resp.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		outcome = outbound.HTTPOutcome(resp.StatusCode, err, ctx)
		return nil, &Error{Kind: KindDown, Msg: "reading the answer failed"}
	}
	outcome = outbound.HTTPOutcome(resp.StatusCode, nil, ctx)
	if b := botCheck(resp, body); b != "" {
		return nil, &Error{Kind: KindBlocked, Detail: b, Msg: resp.Status + ": " + blockText[b]}
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		e := &Error{Kind: KindRateLimited, Msg: "the Source asked us to slow down", RetryAfter: Backoff(1)}
		if s, err := strconv.ParseInt(strings.TrimSpace(resp.Header.Get("Retry-After")), 10, 64); err == nil && s > 0 {
			e.RetryAfter = time.Duration(min(s, maxRetryAfter)) * time.Second
		}
		return nil, e
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, &Error{Kind: KindLoginRefused, Msg: resp.Status}
	case resp.StatusCode >= 500:
		return nil, &Error{Kind: KindDown, Msg: resp.Status}
	case resp.StatusCode != http.StatusOK:
		return nil, &Error{Kind: KindBroken, Msg: resp.Status}
	}
	if err := feedError(body); err != nil {
		var se *Error
		if errors.As(err, &se) {
			clean := *se
			clean.Msg = c.scrub(clean.Msg)
			return nil, &clean
		}
		return nil, err
	}
	return body, nil
}

// feedError reads a Newznab error answer, an error root with a code and a
// description.
func feedError(body []byte) error {
	var e struct {
		XMLName     xml.Name
		Code        int    `xml:"code,attr"`
		Description string `xml:"description,attr"`
	}
	if xml.Unmarshal(body, &e) != nil || e.XMLName.Local != "error" {
		return nil
	}
	kind := KindBroken
	switch {
	case e.Code >= 100 && e.Code < 200:
		kind = KindLoginRefused
	case e.Code == 429 || e.Code == 500 || e.Code == 501:
		kind = KindRateLimited
	case e.Code >= 900:
		kind = KindDown
	}
	return &Error{Kind: kind, Msg: fmt.Sprintf("code %d: %s", e.Code, e.Description), RetryAfter: Backoff(1)}
}

// Caps is what a Source says it can do.
type Caps struct {
	Search      bool
	MovieSearch bool
	TVSearch    bool
	MusicSearch bool
	Params      map[string][]string
	Categories  []int
	Max         int
}

// Supports says whether a search type takes a parameter.
func (c Caps) Supports(kind, param string) bool {
	for _, p := range c.Params[kind] {
		if p == param {
			return true
		}
	}
	return false
}

type capsSearch struct {
	Available string `xml:"available,attr"`
	Params    string `xml:"supportedParams,attr"`
}

func (s capsSearch) on() bool { return s.Available == "yes" }

func (s capsSearch) params() []string {
	var out []string
	for _, p := range strings.Split(s.Params, ",") {
		if p = strings.TrimSpace(strings.ToLower(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

type capsXML struct {
	Limits struct {
		Max int `xml:"max,attr"`
	} `xml:"limits"`
	Searching struct {
		Search capsSearch `xml:"search"`
		Movie  capsSearch `xml:"movie-search"`
		TV     capsSearch `xml:"tv-search"`
		Music  capsSearch `xml:"music-search"`
		Audio  capsSearch `xml:"audio-search"`
	} `xml:"searching"`
	Categories []struct {
		ID   int `xml:"id,attr"`
		Subs []struct {
			ID int `xml:"id,attr"`
		} `xml:"subcat"`
	} `xml:"categories>category"`
}

// Caps reads what the Source can do and remembers it.
func (c *Client) Caps(ctx context.Context) (Caps, error) {
	body, err := c.get(ctx, "caps", url.Values{"t": {"caps"}})
	if err != nil {
		return Caps{}, err
	}
	var x capsXML
	if err := xml.Unmarshal(body, &x); err != nil {
		return Caps{}, &Error{Kind: KindBroken, Msg: "caps are not valid XML: " + err.Error()}
	}
	caps := Caps{
		Search: x.Searching.Search.on(), MovieSearch: x.Searching.Movie.on(), TVSearch: x.Searching.TV.on(),
		MusicSearch: x.Searching.Music.on() || x.Searching.Audio.on(),
		Params: map[string][]string{"search": x.Searching.Search.params(), "movie": x.Searching.Movie.params(), "tv": x.Searching.TV.params(),
			"music": append(x.Searching.Music.params(), x.Searching.Audio.params()...)},
		Max: x.Limits.Max,
	}
	for _, cat := range x.Categories {
		caps.Categories = append(caps.Categories, cat.ID)
		for _, s := range cat.Subs {
			caps.Categories = append(caps.Categories, s.ID)
		}
	}
	c.mu.Lock()
	c.caps = &caps
	c.mu.Unlock()
	return caps, nil
}

// Query is one movie search, or a show or album search when TV or Music is set.
type Query struct {
	Title      string
	Year       int
	IMDb       string
	TMDb       int
	Categories []int
	// TV makes it a tv search. Season 0 with no Episode asks for every season.
	// Episode is a number, or MM/DD for a daily show (Season is then the year).
	// Words replaces the numbers for a Source that only has a text search.
	TV      bool
	Season  int
	Episode string
	Words   string
	TVDB    int
	TVMaze  int
	// Music makes it an album search by artist and album where the Source takes
	// them, otherwise a text search of both.
	Music  bool
	Artist string
	Album  string
}

// Item is one search result, before parsing and judging.
type Item struct {
	Title     string
	GUID      string
	Link      string
	Size      int64
	Protocol  string
	Seeders   *int
	Peers     *int
	Published time.Time
	InfoHash  string
	Files     int
}

type rssXML struct {
	Items []struct {
		Title     string `xml:"title"`
		GUID      string `xml:"guid"`
		Link      string `xml:"link"`
		Size      int64  `xml:"size"`
		PubDate   string `xml:"pubDate"`
		Enclosure struct {
			URL    string `xml:"url,attr"`
			Length int64  `xml:"length,attr"`
			Type   string `xml:"type,attr"`
		} `xml:"enclosure"`
		Attrs []struct {
			Name  string `xml:"name,attr"`
			Value string `xml:"value,attr"`
		} `xml:"attr"`
	} `xml:"channel>item"`
}

func textQuery(q Query) string {
	words := strings.FieldsFunc(q.Title, func(r rune) bool {
		return r != '\'' && !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if q.Year > 0 {
		words = append(words, strconv.Itoa(q.Year))
	}
	return strings.Join(words, " ")
}

// Search runs a movie search. It uses the ids the Source supports and falls
// back to a text search by title and year.
func (c *Client) Search(ctx context.Context, q Query) ([]Item, error) {
	c.mu.Lock()
	caps := c.caps
	c.mu.Unlock()
	if caps == nil {
		got, err := c.Caps(ctx)
		if err != nil {
			return nil, err
		}
		caps = &got
	}
	v := url.Values{}
	switch {
	case q.Music && caps.MusicSearch:
		v.Set("t", "music")
		named := false
		if caps.Supports("music", "artist") {
			v.Set("artist", q.Artist)
			named = true
		}
		if q.Album != "" && caps.Supports("music", "album") {
			v.Set("album", q.Album)
		} else if named && q.Album != "" {
			v.Set("q", textQuery(Query{Title: q.Album}))
		}
		if !named {
			v.Set("q", textQuery(Query{Title: q.Artist + " " + q.Album}))
		}
	case q.Music && caps.Search:
		v.Set("t", "search")
		v.Set("q", textQuery(Query{Title: q.Artist + " " + q.Album}))
	case q.Music:
		return nil, &Error{Kind: KindBroken, Msg: "the Source offers no music search"}
	case q.TV && caps.TVSearch:
		v.Set("t", "tvsearch")
		ids := false
		if q.TVDB > 0 && caps.Supports("tv", "tvdbid") {
			v.Set("tvdbid", strconv.Itoa(q.TVDB))
			ids = true
		}
		if q.TVMaze > 0 && caps.Supports("tv", "tvmazeid") {
			v.Set("tvmazeid", strconv.Itoa(q.TVMaze))
			ids = true
		}
		if !ids {
			v.Set("q", textQuery(Query{Title: q.Title}))
		}
		if q.Season > 0 || q.Episode != "" {
			v.Set("season", strconv.Itoa(q.Season))
		}
		if q.Episode != "" {
			v.Set("ep", q.Episode)
		}
	case q.TV && caps.Search:
		v.Set("t", "search")
		v.Set("q", strings.TrimSpace(textQuery(Query{Title: q.Title})+" "+q.Words))
	case q.TV:
		return nil, &Error{Kind: KindBroken, Msg: "the Source offers no show search"}
	case caps.MovieSearch:
		v.Set("t", "movie")
		ids := false
		if q.IMDb != "" && caps.Supports("movie", "imdbid") {
			v.Set("imdbid", strings.TrimPrefix(q.IMDb, "tt"))
			ids = true
		}
		if q.TMDb > 0 && caps.Supports("movie", "tmdbid") {
			v.Set("tmdbid", strconv.Itoa(q.TMDb))
			ids = true
		}
		if !ids {
			v.Set("q", textQuery(q))
		}
	case caps.Search:
		v.Set("t", "search")
		v.Set("q", textQuery(q))
	default:
		return nil, &Error{Kind: KindBroken, Msg: "the Source offers no search"}
	}
	if len(q.Categories) > 0 {
		cs := make([]string, len(q.Categories))
		for i, n := range q.Categories {
			cs[i] = strconv.Itoa(n)
		}
		v.Set("cat", strings.Join(cs, ","))
	}
	if caps.Max > 0 {
		v.Set("limit", strconv.Itoa(caps.Max))
	}
	body, err := c.get(ctx, "search", v)
	if err != nil {
		return nil, err
	}
	return parseItems(body, c.cfg.Protocol)
}

// Feed reads the newest releases: an rss Source's feed as it is, or a
// Torznab/Newznab search with no words.
func (c *Client) Feed(ctx context.Context, categories []int) ([]Item, error) {
	return c.feed(ctx, categories, c.cfg.Observe)
}

// feedProbe reads the feed under the client's request limits without
// recording call health. A schedule probe must not write Source counters; the
// feed job records the call when work or recovery is needed.
func (c *Client) feedProbe(ctx context.Context, categories []int, observer func(string, time.Duration, error)) ([]Item, error) {
	return c.feed(ctx, categories, observer)
}

func (c *Client) feed(ctx context.Context, categories []int, observer func(string, time.Duration, error)) ([]Item, error) {
	v := url.Values{}
	if c.cfg.Format != "rss" {
		v.Set("t", "search")
		if len(categories) > 0 {
			cs := make([]string, len(categories))
			for i, n := range categories {
				cs[i] = strconv.Itoa(n)
			}
			v.Set("cat", strings.Join(cs, ","))
		}
	}
	body, err := c.getWithObserver(ctx, "feed", v, observer)
	if err != nil {
		return nil, err
	}
	return parseItems(body, c.cfg.Protocol)
}

// Test checks that the Source answers: its caps for Torznab and Newznab, its
// feed for rss. It returns how many releases the feed holds, 0 for caps.
func (c *Client) Test(ctx context.Context) (int, error) {
	if c.cfg.Format == "rss" {
		items, err := c.Feed(ctx, nil)
		return len(items), err
	}
	_, err := c.Caps(ctx)
	return 0, err
}

func parseItems(body []byte, protocol string) ([]Item, error) {
	var x rssXML
	if err := xml.Unmarshal(body, &x); err != nil {
		return nil, &Error{Kind: KindBroken, Msg: "results are not valid XML: " + err.Error()}
	}
	out := make([]Item, 0, len(x.Items))
	for _, it := range x.Items {
		i := Item{Title: strings.TrimSpace(it.Title), GUID: it.GUID, Link: it.Enclosure.URL, Size: it.Enclosure.Length, Protocol: protocol}
		if i.Link == "" {
			i.Link = strings.TrimSpace(it.Link)
		}
		if i.Size == 0 {
			i.Size = it.Size
		}
		switch it.Enclosure.Type {
		case "application/x-nzb":
			i.Protocol = "usenet"
		case "application/x-bittorrent":
			i.Protocol = "torrent"
		}
		pd := strings.TrimSpace(it.PubDate)
		if t, err := time.Parse(time.RFC1123Z, pd); err == nil {
			i.Published = t.UTC()
		} else if t, err := time.Parse(time.RFC1123, pd); err == nil {
			i.Published = t.UTC()
		}
		for _, a := range it.Attrs {
			switch strings.ToLower(a.Name) {
			case "seeders":
				if n, err := strconv.Atoi(a.Value); err == nil {
					i.Seeders = &n
				}
			case "peers":
				if n, err := strconv.Atoi(a.Value); err == nil {
					i.Peers = &n
				}
			case "infohash":
				if h := strings.ToLower(strings.TrimSpace(a.Value)); releasename.ValidInfoHash(h) {
					i.InfoHash = h
				}
			case "files":
				if n, err := strconv.Atoi(a.Value); err == nil && n > 0 {
					i.Files = n
				}
			case "size":
				if i.Size == 0 {
					i.Size, _ = strconv.ParseInt(a.Value, 10, 64)
				}
			}
		}
		if i.Title == "" || i.Link == "" {
			continue
		}
		out = append(out, i)
	}
	return out, nil
}

// feedAddress is a Newznab or Torznab address ready to call. An address
// with no path (https://indexer.example) means its /api, and so do a
// Prowlarr site address (http://prowlarr:9696/5) and a Jackett one
// (.../results/torznab) saved without it by earlier versions.
func feedAddress(raw string) string {
	raw = strings.TrimRight(raw, "/")
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.RawQuery != "" {
		return raw
	}
	if u.Path == "" || prowlarrSitePath.MatchString(u.Path) || strings.HasSuffix(u.Path, "/results/torznab") {
		return raw + "/api"
	}
	return raw
}

var prowlarrSitePath = regexp.MustCompile(`^/[0-9]+$`)
