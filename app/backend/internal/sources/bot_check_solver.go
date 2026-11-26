package sources

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	htmlesc "html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A bot-check solver is a plugin (hook "botcheck", scope sources.botcheck),
// such as one that asks FlareSolverr or Byparr. HolyArrs runs no browser
// itself: when a plain GET meets a bot check, it hands that one page to the
// solver and keeps the cookie and User-Agent that got past it, for that
// host, until the site checks again. All the rules sit here, not in the
// plugin: the Source's address rule before and after, the same site only,
// and the key only for the solver it was typed for.

// BotCheckAsk is the page a solver opens: a GET of URL, with the Source's
// proxy (password included) so the page comes from the Source's address.
type BotCheckAsk struct {
	URL   string
	Proxy *url.URL
}

// BotCheckCookie is one cookie the solver's browser got.
type BotCheckCookie struct{ Name, Value string }

// BotCheckAnswer is where the solver's browser ended and what it got there.
type BotCheckAnswer struct {
	URL       string
	Status    int
	Body      string
	Cookies   []BotCheckCookie
	UserAgent string
}

// BotCheckSolver is the solver serving now. Pin names it and where it can
// reach; a Source with a key is only handed to the solver whose pin it was
// saved with.
type BotCheckSolver struct {
	Pin   string
	Solve func(ctx context.Context, ask BotCheckAsk) (BotCheckAnswer, error)
}

// BotCheck returns the solver serving now, or nil; the app sets it. Nil, or
// a nil answer, leaves a bot check Blocked as before.
var BotCheck func() *BotCheckSolver

// BotCheckPin makes a solver's pin from its plugin id and the hosts it may
// reach.
func BotCheckPin(plugin string, hosts []string) string {
	h := sha256.Sum256([]byte(plugin + "\x00" + strings.Join(hosts, "\x00")))
	return plugin + ":" + hex.EncodeToString(h[:8])
}

// extraBotCheckPin keeps, in a Source's extra, the pin of the solver its key
// and proxy password were typed for.
const extraBotCheckPin = "botcheck_pin"

// ErrBotCheckAgain is a Source with a key whose solver changed since the key was typed.
var ErrBotCheckAgain = errors.New("the bot-check plugin changed since this Source's key was typed; type the key and proxy password again so they are never handed to a new solver by themselves")

// pinBotCheck records the serving solver on src when every secret it holds
// (key, passkey, proxy password) was typed in this save; otherwise the
// stored pin stays. A solver is handed those, so it is recorded only then.
func pinBotCheck(src *domain.Source, typed bool) {
	if !typed {
		return
	}
	if src.Extra == nil {
		src.Extra = map[string]string{}
	}
	delete(src.Extra, extraBotCheckPin)
	if BotCheck == nil {
		return
	}
	if s := BotCheck(); s != nil && s.Pin != "" {
		src.Extra[extraBotCheckPin] = s.Pin
	}
}

// clearance is the cookie and User-Agent that passed one host's check.
type clearance struct {
	cookie string
	agent  string
}

const maxClearances = 16

func (c *Client) clearanceFor(host string) *clearance {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pass[host]
}

// useClearance puts the cookie and User-Agent that passed this host's check
// on req, and returns them so a new check can drop them.
func (c *Client) useClearance(req *http.Request) *clearance {
	p := c.clearanceFor(req.URL.Host)
	if p == nil {
		return nil
	}
	// The check's cookie only holds with the browser's own User-Agent.
	req.Header.Set("User-Agent", p.agent)
	if old := req.Header.Get("Cookie"); old != "" {
		req.Header.Set("Cookie", old+"; "+p.cookie)
	} else {
		req.Header.Set("Cookie", p.cookie)
	}
	return p
}

// forget drops a host's cookie once the site no longer takes it.
func (c *Client) forget(host string, p *clearance) {
	if p == nil {
		return
	}
	c.mu.Lock()
	if c.pass[host] == p {
		delete(c.pass, host)
	}
	c.mu.Unlock()
}

func (c *Client) keep(host string, p *clearance) {
	c.mu.Lock()
	if c.pass == nil || len(c.pass) >= maxClearances {
		c.pass = map[string]*clearance{}
	}
	c.pass[host] = p
	c.mu.Unlock()
}

// solver says whether a blocked call goes to a solver, and which. Only a
// plain GET is handed over: a solver cannot send a body or our headers.
// A plugin Source's address is no page on the web, so it never is.
func (c *Client) solver(block, method string, head http.Header) *BotCheckSolver {
	if block != BlockBotCheck || method != http.MethodGet || len(head) > 0 || IsPluginSource(c.cfg.URL) || BotCheck == nil {
		return nil
	}
	return BotCheck()
}

// holdsSecrets says whether handing the page over hands over something
// typed for this Source: a key, a passkey or a proxy password.
func (c *Client) holdsSecrets() bool {
	if c.cfg.APIKey != "" || len(c.cfg.PathKeys) > 0 {
		return true
	}
	if c.cfg.Proxy != nil {
		if pass, _ := c.cfg.Proxy.User.Password(); pass != "" {
			return true
		}
	}
	return false
}

// solve fetches raw through s. The Source's own address rule holds: the
// page must be on the Source's site, its name is looked up and checked here
// first, and the page the solver ended on is checked the same way.
func (c *Client) solve(ctx context.Context, s *BotCheckSolver, raw string) ([]byte, error) {
	if c.holdsSecrets() && s.Pin != c.cfg.BotCheckPin {
		return nil, &Error{Kind: KindBlocked, Detail: BlockBotCheck, Msg: ErrBotCheckAgain.Error()}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.Scheme != "http" && u.Scheme != "https" {
		return nil, &Error{Kind: KindBroken, Msg: "bad Source address"}
	}
	if !sameSite(u.Hostname(), siteHost(c.cfg.URL)) {
		return nil, &Error{Kind: KindBlocked, Detail: BlockBotCheck, Msg: "the page is on another site than the Source's; not handed to the bot-check plugin"}
	}
	if _, err := checkName(ctx, c.cfg, u.Hostname()); err != nil {
		return nil, &Error{Kind: KindBlocked, Msg: c.scrub(err.Error())}
	}
	a, err := s.Solve(ctx, BotCheckAsk{URL: raw, Proxy: c.cfg.Proxy})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &Error{Kind: KindBlocked, Detail: BlockBotCheck, Msg: "the bot-check plugin could not pass the site's bot check: " + c.scrub(clipText(err.Error(), 200)), cause: err}
	}
	// The solver's browser follows redirects and looks names up itself, so
	// where it ended is checked too: another site, or a name that now leads
	// somewhere the Source may not go, is refused and nothing is kept.
	// A name that changes its answer between the two lookups (DNS rebinding
	// at the solver's side) cannot be caught from here.
	if a.URL != "" {
		end, err := url.Parse(a.URL)
		if err != nil || end.Scheme != "http" && end.Scheme != "https" || !sameSite(end.Hostname(), u.Hostname()) {
			return nil, &Error{Kind: KindBlocked, Msg: "the bot-check plugin ended on another site than the Source's; refused"}
		}
		if _, err := checkName(ctx, c.cfg, end.Hostname()); err != nil {
			return nil, &Error{Kind: KindBlocked, Msg: c.scrub(err.Error())}
		}
	}
	switch {
	case a.Status == http.StatusTooManyRequests:
		return nil, &Error{Kind: KindRateLimited, Msg: "the Source asked us to slow down", RetryAfter: Backoff(1)}
	case a.Status >= 500:
		return nil, &Error{Kind: KindDown, Msg: fmt.Sprintf("%d %s (through the bot-check plugin)", a.Status, http.StatusText(a.Status))}
	case a.Status != 0 && a.Status != http.StatusOK:
		return nil, &Error{Kind: KindBroken, Msg: fmt.Sprintf("%d %s (through the bot-check plugin)", a.Status, http.StatusText(a.Status))}
	}
	if p := cookieLine(a.Cookies); p != "" && a.UserAgent != "" {
		c.keep(u.Host, &clearance{cookie: p, agent: a.UserAgent})
	}
	body := unwrapPre([]byte(a.Body))
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

var cookieName = regexp.MustCompile(`^[!#$%&'*+\-.^_` + "`" + `|~0-9A-Za-z]{1,128}$`)

// cookieLine joins the cookies a solver got into one Cookie header, leaving
// out any that could break the header.
func cookieLine(cs []BotCheckCookie) string {
	parts := make([]string, 0, len(cs))
	n := 0
	for _, k := range cs {
		if !cookieName.MatchString(k.Name) || strings.ContainsAny(k.Value, ";,\r\n\x00 \"\\") || len(k.Value) > 4096 {
			continue
		}
		if n += len(k.Name) + len(k.Value); n > 8192 {
			break
		}
		parts = append(parts, k.Name+"="+k.Value)
	}
	return strings.Join(parts, "; ")
}

func siteHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// sameSite says whether two host names are one site, give or take "www.".
func sameSite(a, b string) bool {
	norm := func(h string) string { return strings.TrimPrefix(strings.ToLower(strings.TrimSuffix(h, ".")), "www.") }
	return a != "" && norm(a) == norm(b)
}

func clipText(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

var (
	prePage = regexp.MustCompile(`(?is)^\s*(?:<!doctype[^>]*>\s*)?<html[^>]*>\s*(?:<head>.*?</head>\s*)?<body[^>]*>\s*<pre[^>]*>(.*)</pre>`)
	// xmlViewer is where Chrome keeps the source of an XML page it shows as a tree.
	xmlViewer = regexp.MustCompile(`(?is)<div id="webkit-xml-viewer-source-xml"[^>]*>\s*<([a-z][\w:.-]*)`)
)

// unwrapPre takes a feed back out of the page a browser wraps it in, so a
// JSON site and a Torznab or Newznab feed read the same through a solver:
// JSON or text shown in a <pre>, or XML shown by Chrome's XML viewer. Any
// other page comes back as it is.
func unwrapPre(b []byte) []byte {
	if m := prePage.FindSubmatch(b); m != nil {
		inner := strings.TrimSpace(htmlesc.UnescapeString(string(m[1])))
		switch {
		case strings.HasPrefix(inner, "{") || strings.HasPrefix(inner, "["):
			return []byte(inner)
		case strings.HasPrefix(inner, "<") && wellFormedXML([]byte(inner)):
			return []byte(inner)
		}
	}
	if loc := xmlViewer.FindSubmatchIndex(b); loc != nil {
		root := string(b[loc[2]:loc[3]])
		start := loc[2] - 1
		if end := bytes.LastIndex(b, []byte("</"+root+">")); end > start {
			inner := b[start : end+len(root)+3]
			if wellFormedXML(inner) {
				return append([]byte(xml.Header), inner...)
			}
		}
	}
	return b
}

// wellFormedXML says whether b reads as one XML document.
func wellFormedXML(b []byte) bool {
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = true
	roots := 0
	depth := 0
	for {
		t, err := d.Token()
		if err == io.EOF {
			return roots == 1 && depth == 0
		}
		if err != nil {
			return false
		}
		switch t.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
}

// newSourceExtra is a new Source's extra, with the serving solver pinned:
// everything a new Source holds was typed just now.
func newSourceExtra() map[string]string {
	src := domain.Source{Extra: map[string]string{}}
	pinBotCheck(&src, true)
	return src.Extra
}
