// Package sites talks to subtitle sites. Each kind of site has a small adapter
// behind one interface. A shared HTTP client enforces each site's limits and
// the app's network rule, and failures map to the pause each one earns.
package sites

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// Query is what a search asks a site for.
type Query struct {
	Kind        string // movie or episode
	Title       string
	Year        int
	Season      int
	Episode     int
	IMDb        string
	TMDB        string
	TVDB        string
	ReleaseName string
	// Hash is the video's 64-bit file hash in OpenSubtitles style as 16 hex
	// digits. It is empty when the file could not be read.
	Hash string
	// Size is the video file's size in bytes, sent with the hash. 0 when unknown.
	Size      int64
	Languages []string // two-letter codes
}

// Candidate is one subtitle a site offered.
type Candidate struct {
	Site            string `json:"site"`
	Key             string `json:"key"` // the site's own id
	Language        string `json:"language"`
	HearingImpaired bool   `json:"hearing_impaired"`
	Forced          bool   `json:"forced"`
	// ReleaseNames are the release names the site keeps for the subtitle in a
	// separate field. Only the lab adapter sends our release name in the query,
	// so it drops a name equal to ours on subtitles the site marks as built from
	// the query. An echo must not score as an exact release match.
	ReleaseNames []string `json:"release_names,omitempty"`
	Title        string   `json:"title,omitempty"`
	Year         int      `json:"year,omitempty"`
	Season       int      `json:"season,omitempty"`
	Episode      int      `json:"episode,omitempty"`
	MatchedByID  bool     `json:"matched_by_id,omitempty"` // the site found it by the title's own id
	Uploader     string   `json:"uploader,omitempty"`
	Trusted      bool     `json:"trusted,omitempty"`
	FPS          float64  `json:"fps,omitempty"`
	Corrected    bool     `json:"corrected,omitempty"`
	Downloads    int      `json:"downloads,omitempty"`
	Format       string   `json:"format,omitempty"`
	// MachineMade says the site flags the file as made by a machine:
	// AITranslated or MachineTranslated. Empty means made by a person.
	MachineMade string `json:"machine_made,omitempty"`
	Link        string `json:"-"` // where Download fetches it; never shown
}

// Site is one subtitle site.
type Site interface {
	Search(ctx context.Context, q Query) ([]Candidate, error)
	Download(ctx context.Context, c Candidate) ([]byte, error)
}

// Kind describes one kind of site the app can use.
type Kind struct {
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	DefaultURL   string `json:"default_url,omitempty"`
	Movies       bool   `json:"movies"`
	Shows        bool   `json:"shows"`
	NeedsAccount bool   `json:"needs_account"`
	// Account names the details a site that needs an account asks for:
	// username, password and api_key.
	Account []string `json:"account,omitempty"`
	Note    string   `json:"note,omitempty"`
	// DefaultOff means a new site of this kind starts off unless the person turns
	// it on in the same step.
	DefaultOff bool `json:"default_off,omitempty"`
	// Later means the kind is listed so people know it is planned, but it is not
	// built and cannot be added yet.
	Later bool `json:"later,omitempty"`
}

// Kinds lists the kinds of site in the order screens show them.
var Kinds = []Kind{
	{Kind: "opensubtitles", Name: "OpenSubtitles.com", DefaultURL: "https://api.opensubtitles.com/api/v1", Movies: true, Shows: true, NeedsAccount: true,
		Account: []string{"api_key", "username", "password"},
		Note:    "Needs your account: an API key from the site's consumer page, plus your username and password (downloads need a sign-in). Free accounts get a small number of downloads a day."},
	{Kind: "subdl", Name: "subdl", DefaultURL: "https://api.subdl.com/api/v1", Movies: true, Shows: true, NeedsAccount: true, Account: []string{"api_key"},
		Note: "Needs your account: an API key from your subdl account page. Its download count resets at midnight UTC."},
	{Kind: "podnapisi", Name: "Podnapisi", DefaultURL: "https://www.podnapisi.net", Movies: true, Shows: true, DefaultOff: true,
		Note: "No account. Starts turned off: the site name does not resolve right now."},
	{Kind: "gestdown", Name: "Gestdown", DefaultURL: "https://api.gestdown.info", Shows: true,
		Note: "Shows only (Addic7ed subtitles through Gestdown). No account. Answers checked against recorded replies only."},
	{Kind: "addic7ed", Name: "Addic7ed", DefaultURL: "https://www.addic7ed.com", Shows: true, NeedsAccount: true, Account: []string{"username", "password"},
		Note: "Needs your account: your Addic7ed user name and password; HolyArrs signs in itself. Shows only. Used gently (one call every 3 seconds at most, a daily cap); an anti-bot check makes it rest. Checked against fake pages only."},
	{Kind: "yifysubtitles", Name: "YIFY Subtitles", DefaultURL: "https://yifysubtitles.ch", Movies: true,
		Note: "Movies only, found by their IMDb id. No account. Checked against fake pages only."},
	{Kind: "tvsubtitles", Name: "TVsubtitles", DefaultURL: "http://www.tvsubtitles.net", Shows: true,
		Note: "Shows only. No account. The site has no https, so calls to it are not encrypted. Checked against fake pages only."},
	{Kind: "bsplayer", Name: "BSPlayer", DefaultURL: "http://s1.api.bsplayer-subtitles.com", Movies: true, Shows: true, DefaultOff: true,
		Note: "Movies and shows, matched by the video file's hash, so it needs the file on disk. No account. Starts turned off: the site is often down. Checked against fake replies only."},
	{Kind: "fake", Name: "Lab subtitle site", Movies: true, Shows: true, Note: "The lab's fake site, for tests."},
	{Kind: "fake_account", Name: "Lab subtitle site with an account", Movies: true, Shows: true, NeedsAccount: true, Account: []string{"api_key"},
		Note: "The lab's fake site asking for a key, for tests of sites that need your account."},
}

// KindOf looks up a kind by name.
func KindOf(kind string) (Kind, bool) {
	for _, k := range Kinds {
		if k.Kind == kind {
			return k, true
		}
	}
	return Kind{}, false
}

// Config says how to reach one site and how hard it may be used.
type Config struct {
	Name       string
	URL        string
	Key        string // API key for kinds that need one; never logged
	Username   string // account name for kinds that sign in; never logged
	Password   string // never logged
	GapMS      int    // shortest time between two calls, 1000 by default
	PerWindow  int    // most calls per Window, 60 by default; 0 keeps the default
	Window     time.Duration
	Timeout    time.Duration // 20s by default
	AllowLocal bool          // may reach the local network, never cloud metadata
	// HTTP replaces the client in tests. The network rule is then up to the test.
	HTTP *http.Client
}

// New builds the site for a kind.
func New(kind string, cfg Config) (Site, error) {
	c, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	switch kind {
	case "gestdown":
		return &gestdown{c: c}, nil
	case "podnapisi":
		return &podnapisi{c: c}, nil
	case "fake", "fake_account":
		return &fake{c: c}, nil
	case "opensubtitles":
		return newOpenSubtitles(c), nil
	case "addic7ed":
		return newAddic7ed(c), nil
	case "yifysubtitles":
		return newYify(c), nil
	case "tvsubtitles":
		return newTVSubs(c), nil
	case "bsplayer":
		return newBSPlayer(c), nil
	case "subdl":
		c.keyHeader = "" // subdl takes the key in the query
		return &subdl{c: c}, nil
	}
	return nil, fmt.Errorf("unknown kind of subtitle site %q", kind)
}

// Error kinds. They double as the sites' pause reasons.
const (
	KindDown          = "down"
	KindRateLimited   = "rate_limited"
	KindDownloadLimit = "download_limit"
	KindLoginRefused  = "login_refused"
	KindBlocked       = "blocked"
	KindUnreadable    = "unreadable"
)

// Error is a failed call to a site. It never holds the URL or a key, so it is
// safe to log and show.
type Error struct {
	Kind       string
	RetryAfter time.Duration
	Msg        string
}

func (e *Error) Error() string { return "subtitle site " + e.Kind + ": " + e.Msg }

// Code lets the jobs queue record the kind.
func (e *Error) Code() string { return "subtitle_site_" + e.Kind }

// PauseFor returns how long a site rests after err, given n failures in a row
// counting this one, at time now.
func PauseFor(err error, n int, now time.Time) (string, time.Duration) {
	kind, retry := KindDown, time.Duration(0)
	var se *Error
	if errors.As(err, &se) {
		kind, retry = se.Kind, se.RetryAfter
	}
	switch kind {
	case KindRateLimited:
		if retry > 0 {
			return kind, min(retry, 24*time.Hour)
		}
		return kind, time.Hour
	case KindDownloadLimit:
		if retry > 0 { // the site said when its daily count resets
			return kind, min(retry, 24*time.Hour)
		}
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
		return kind, next.Sub(now.UTC())
	case KindUnreadable:
		return kind, 6 * time.Hour
	case KindLoginRefused, KindBlocked:
		return kind, 12 * time.Hour
	}
	d := 20 * time.Minute
	for i := 1; i < n && d < 6*time.Hour; i++ {
		d *= 2
	}
	return KindDown, min(d, 6*time.Hour)
}

// Caps on the size of an answer.
const (
	maxSearchBody   = 4 << 20
	maxDownloadBody = 8 << 20
)

type client struct {
	cfg  Config
	base *url.URL
	http *http.Client

	// keyHeader is the header that carries the key; empty means the adapter puts
	// it elsewhere. userAgent replaces "HolyArrs" for sites that want a named app.
	keyHeader string
	userAgent string
	// status, when set, reads a site's own answer to a failed call before the
	// shared rules, for example a quota answer.
	status func(resp *http.Response, body []byte) error

	mu   sync.Mutex
	last time.Time
	hits []time.Time
}

func newClient(cfg Config) (*client, error) {
	u, err := url.Parse(strings.TrimRight(cfg.URL, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, errors.New("a subtitle site needs an http or https address without a user name")
	}
	if cfg.GapMS <= 0 {
		cfg.GapMS = 1000
	}
	if cfg.PerWindow <= 0 {
		cfg.PerWindow = 60
	}
	if cfg.Window <= 0 {
		cfg.Window = time.Hour
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 20 * time.Second
	}
	h := cfg.HTTP
	if h == nil {
		allow := outbound.PublicOnly
		if cfg.AllowLocal {
			allow = outbound.NotMetadata
		}
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.Proxy = nil // the network rule must see the real address
		t.DialContext = outbound.PolicyDialer(allow).DialContext
		h = &http.Client{Timeout: cfg.Timeout, Transport: t}
	} else {
		cp := *h
		h = &cp
	}
	if h.CheckRedirect == nil {
		h.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Host != via[0].URL.Host || req.URL.Scheme != via[0].URL.Scheme {
				return errors.New("the site redirected to another host or scheme; refused")
			}
			return nil
		}
	}
	return &client{cfg: cfg, base: u, http: h, keyHeader: "X-Api-Key"}, nil
}

// wait spaces calls by the gap and refuses once the window is full.
func (c *client) wait(ctx context.Context) error {
	c.mu.Lock()
	now := time.Now()
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
		return &Error{Kind: KindRateLimited, RetryAfter: retry, Msg: fmt.Sprintf("%d calls in the window already", c.cfg.PerWindow)}
	}
	at := now
	if next := c.last.Add(time.Duration(c.cfg.GapMS) * time.Millisecond); next.After(now) {
		at = next
	}
	c.last = at
	c.hits = append(c.hits, at)
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

// ref resolves a path with its query against the site's address. The result
// must stay on the site's host.
func (c *client) ref(p string) (string, error) {
	u, err := c.base.Parse(p)
	if err != nil || u.Host != c.base.Host || u.Scheme != c.base.Scheme {
		return "", &Error{Kind: KindUnreadable, Msg: "the site pointed at another address"}
	}
	if !strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "http") {
		return "", &Error{Kind: KindUnreadable, Msg: "the site sent a link HolyArrs cannot follow"}
	}
	if strings.HasPrefix(p, "/") && c.base.Path != "" {
		u.Path = c.base.Path + u.Path
	}
	return u.String(), nil
}

// get fetches a path on the site. With missingOK set, a 404 returns nil and
// no error.
func (c *client) get(ctx context.Context, p string, accept string, limit int64, missingOK bool) ([]byte, error) {
	raw, err := c.ref(p)
	if err != nil {
		return nil, err
	}
	return c.do(ctx, http.MethodGet, raw, nil, nil, accept, limit, missingOK)
}

// fetch follows a download link a site handed out. A link on the site's own
// host is fetched like any path. A link on another host, where the site's
// download servers live, must use https and still passes the network rule.
func (c *client) fetch(ctx context.Context, link string, limit int64) ([]byte, error) {
	u, err := url.Parse(link)
	if err != nil || u.User != nil {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site sent a link HolyArrs cannot follow"}
	}
	if !u.IsAbs() || (u.Host == c.base.Host && u.Scheme == c.base.Scheme) {
		return c.get(ctx, link, "", limit, false)
	}
	if u.Scheme != "https" || u.Host == "" {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site sent a download link that is not https"}
	}
	return c.do(ctx, http.MethodGet, u.String(), nil, nil, "", limit, false)
}

// do sends one call. A non-nil body goes as JSON and hdr adds headers.
func (c *client) do(ctx context.Context, method, raw string, body []byte, hdr map[string]string, accept string, limit int64, missingOK bool) ([]byte, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	op, err := outbound.Begin(ctx, outbound.Target{Kind: "subtitle_site", Identity: []string{c.cfg.URL, c.cfg.Key, c.cfg.Username, c.cfg.Password}})
	if err != nil {
		return nil, &Error{Kind: KindDown, Msg: err.Error()}
	}
	outcome := outbound.Canceled
	defer func() { op.Finish(outcome) }()
	rctx, cancel := op.AttemptContext(ctx, c.cfg.Timeout)
	defer cancel()
	var rd io.Reader
	if body != nil {
		rd = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(rctx, method, raw, rd)
	if err != nil {
		outcome = outbound.TerminalFailure
		return nil, &Error{Kind: KindUnreadable, Msg: "bad site address"}
	}
	req.Header.Set("User-Agent", c.agent())
	// The key and any sign-in header go only to the site's own host and scheme.
	// A download host elsewhere, such as a CDN, gets neither.
	onSite := req.URL.Host == c.base.Host && req.URL.Scheme == c.base.Scheme
	if !onSite {
		hdr = nil
	}
	if onSite && c.cfg.Key != "" && c.keyHeader != "" {
		req.Header.Set(c.keyHeader, c.cfg.Key)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		outcome = outbound.HTTPOutcome(0, err, ctx)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // drop the URL so it never leaks
		}
		if errors.Is(err, outbound.ErrAddressRefused) {
			return nil, &Error{Kind: KindBlocked, Msg: "the address is on the local network, which this site may not reach"}
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return nil, &Error{Kind: KindDown, Msg: "the site did not answer in time"}
		}
		return nil, &Error{Kind: KindDown, Msg: c.scrub(err.Error())}
	}
	out, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if closeErr := resp.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		outcome = outbound.HTTPOutcome(resp.StatusCode, err, ctx)
		return nil, &Error{Kind: KindDown, Msg: "reading the answer failed"}
	}
	outcome = outbound.HTTPOutcome(resp.StatusCode, nil, ctx)
	if int64(len(out)) > limit {
		outcome = outbound.TerminalFailure
		return nil, &Error{Kind: KindUnreadable, Msg: fmt.Sprintf("the answer is over %d bytes", limit)}
	}
	if c.status != nil {
		if e := c.status(resp, out); e != nil {
			if outcome != outbound.TransientFailure {
				outcome = outbound.TerminalFailure
			}
			return nil, e
		}
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		e := &Error{Kind: KindRateLimited, Msg: "the site asked HolyArrs to slow down"}
		if s, err := strconv.ParseInt(strings.TrimSpace(resp.Header.Get("Retry-After")), 10, 64); err == nil && s > 0 {
			e.RetryAfter = time.Duration(min(s, 86400)) * time.Second
		}
		return nil, e
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, &Error{Kind: KindLoginRefused, Msg: resp.Status}
	case resp.StatusCode == http.StatusNotFound && missingOK:
		return nil, nil
	case resp.StatusCode >= 500:
		return nil, &Error{Kind: KindDown, Msg: resp.Status}
	case resp.StatusCode != http.StatusOK:
		return nil, &Error{Kind: KindUnreadable, Msg: resp.Status}
	}
	return out, nil
}

func (c *client) agent() string {
	if c.userAgent != "" {
		return c.userAgent
	}
	return "HolyArrs"
}

func (c *client) scrub(msg string) string {
	for _, v := range []string{c.cfg.Key, c.cfg.Password, c.cfg.Username} {
		if len(v) >= 3 {
			msg = strings.ReplaceAll(msg, v, "[hidden]")
			msg = strings.ReplaceAll(msg, url.QueryEscape(v), "[hidden]")
		}
	}
	return msg
}

// unreadable wraps a decoding error.
func unreadable(what string, err error) error {
	return &Error{Kind: KindUnreadable, Msg: what + ": " + err.Error()}
}
