package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	htmlesc "html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// FlareSolverr (github.com/FlareSolverr/FlareSolverr) is a small service
// that opens a page in a real browser, waits out the site's bot check and
// hands back the page and the cookies it got. Prowlarr and Jackett use it the
// same way. HolyArrs asks it only when a GET meets a bot check, then reuses
// its cookies until the site checks again.

// ErrFlareSolverr is a FlareSolverr address that cannot be used.
var ErrFlareSolverr = errors.New("the FlareSolverr address must look like http://host:8191")

// ErrFlareAgain is a FlareSolverr address moved to another host: it would be handed the Source's address (with any key in it) and the proxy password.
var ErrFlareAgain = errors.New("the FlareSolverr address changed to another host; type the Source's key and proxy password again so they are never sent to a new host by itself")

// flareWait is how long FlareSolverr may take over one page; its browser
// can need most of a minute for a check.
const flareWait = 60 * time.Second

// clearance is the cookie and browser User-Agent that passed a site's check.
type clearance struct {
	host   string
	cookie string
	agent  string
}

func (c *Client) clearanceFor(host string) *clearance {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pass != nil && c.pass.host == host {
		return c.pass
	}
	return nil
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

// canSolve says whether a blocked call goes to FlareSolverr. Only a plain
// GET is handed over: FlareSolverr cannot send a JSON body or our headers.
// Nor can it open a plugin's address, which is no page on the web.
func (c *Client) canSolve(block, method string, head http.Header) bool {
	return !IsPluginSource(c.cfg.URL) && block == BlockBotCheck && c.cfg.FlareSolverr != nil && method == http.MethodGet && len(head) == 0
}

// forget drops cookies the site no longer takes.
func (c *Client) forget(p *clearance) {
	if p == nil {
		return
	}
	c.mu.Lock()
	if c.pass == p {
		c.pass = nil
	}
	c.mu.Unlock()
}

// ParseFlareSolverr checks a FlareSolverr address: http or https, a host, no
// user, path or query. Empty is none.
func ParseFlareSolverr(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/" && u.Path != "/v1") || u.RawQuery != "" || u.Fragment != "" {
		return "", ErrFlareSolverr
	}
	return u.Scheme + "://" + u.Host, nil
}

type flareProxy struct {
	URL      string `json:"url"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type flareAsk struct {
	Cmd        string      `json:"cmd"`
	URL        string      `json:"url"`
	MaxTimeout int         `json:"maxTimeout"`
	Proxy      *flareProxy `json:"proxy,omitempty"`
}

type flareAnswer struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	Solution struct {
		URL      string `json:"url"`
		Status   int    `json:"status"`
		Response string `json:"response"`
		Cookies  []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"cookies"`
		UserAgent string `json:"userAgent"`
	} `json:"solution"`
}

// solve fetches raw through FlareSolverr. The Source's own address rule
// still applies: its name is looked up and checked here first, so
// FlareSolverr is never sent somewhere the Source itself may not go. The
// Source's proxy is handed to FlareSolverr, so the page comes from the same
// address as every other call.
func (c *Client) solve(ctx context.Context, raw string) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil, &Error{Kind: KindBroken, Msg: "bad Source address"}
	}
	if _, err := checkName(ctx, c.cfg, u.Hostname()); err != nil {
		return nil, &Error{Kind: KindBlocked, Msg: c.scrub(err.Error())}
	}
	ask := flareAsk{Cmd: "request.get", URL: raw, MaxTimeout: int(flareWait / time.Millisecond)}
	if p := c.cfg.Proxy; p != nil {
		pass, _ := p.User.Password()
		ask.Proxy = &flareProxy{URL: p.Scheme + "://" + p.Host, Username: p.User.Username(), Password: pass}
	}
	payload, _ := json.Marshal(ask)
	rctx, cancel := context.WithTimeout(ctx, flareWait+15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodPost, strings.TrimRight(c.cfg.FlareSolverr.String(), "/")+"/v1", bytes.NewReader(payload))
	if err != nil {
		return nil, &Error{Kind: KindBroken, Msg: "bad FlareSolverr address"}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := flareClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		// No detail from the dial: FlareSolverr may sit on the local network,
		// and the reason would tell which hosts and ports there answer.
		return nil, &Error{Kind: KindDown, Msg: "FlareSolverr did not answer; check its address and that it runs", cause: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, &Error{Kind: KindDown, Msg: "reading FlareSolverr's answer failed"}
	}
	var a flareAnswer
	if json.Unmarshal(body, &a) != nil {
		return nil, &Error{Kind: KindBroken, Msg: "FlareSolverr's address answered with something that is not FlareSolverr; check the address"}
	}
	if a.Status != "ok" {
		msg := strings.TrimSpace(a.Message)
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, &Error{Kind: KindBlocked, Detail: BlockBotCheck, Msg: "FlareSolverr could not pass the site's bot check: " + c.scrub(msg)}
	}
	s := a.Solution
	// FlareSolverr's browser follows redirects and looks names up itself, so
	// the page it ended on is checked too: another site, or a name that now
	// leads somewhere the Source may not go, is refused and nothing is kept.
	// A name that changes its answer between the two lookups (DNS rebinding
	// at FlareSolverr's side) cannot be caught from here; run FlareSolverr
	// away from the home network, such as inside a VPN container.
	if s.URL != "" {
		end, err := url.Parse(s.URL)
		if err != nil || !sameSite(end.Hostname(), u.Hostname()) {
			return nil, &Error{Kind: KindBlocked, Msg: "FlareSolverr ended on another site than the Source's; refused"}
		}
		if _, err := checkName(ctx, c.cfg, end.Hostname()); err != nil {
			return nil, &Error{Kind: KindBlocked, Msg: c.scrub(err.Error())}
		}
	}
	switch {
	case s.Status == http.StatusTooManyRequests:
		return nil, &Error{Kind: KindRateLimited, Msg: "the Source asked us to slow down", RetryAfter: Backoff(1)}
	case s.Status >= 500:
		return nil, &Error{Kind: KindDown, Msg: fmt.Sprintf("%d %s (through FlareSolverr)", s.Status, http.StatusText(s.Status))}
	case s.Status != 0 && s.Status != http.StatusOK:
		return nil, &Error{Kind: KindBroken, Msg: fmt.Sprintf("%d %s (through FlareSolverr)", s.Status, http.StatusText(s.Status))}
	}
	if len(s.Cookies) > 0 && s.UserAgent != "" {
		parts := make([]string, 0, len(s.Cookies))
		for _, k := range s.Cookies {
			if k.Name != "" && !strings.ContainsAny(k.Name+k.Value, ";\r\n") {
				parts = append(parts, k.Name+"="+k.Value)
			}
		}
		if len(parts) > 0 {
			c.mu.Lock()
			c.pass = &clearance{host: u.Host, cookie: strings.Join(parts, "; "), agent: s.UserAgent}
			c.mu.Unlock()
		}
	}
	return unwrapPre([]byte(s.Response)), nil
}

// sameSite says whether two host names are one site, give or take "www.".
func sameSite(a, b string) bool {
	norm := func(h string) string { return strings.TrimPrefix(strings.ToLower(strings.TrimSuffix(h, ".")), "www.") }
	return a != "" && norm(a) == norm(b)
}

// flareClient reaches FlareSolverr straight, never through the app proxy:
// it is a service the person runs next to HolyArrs, usually on the local
// network. Cloud metadata addresses stay refused.
var flareClient = &http.Client{Transport: &http.Transport{DialContext: outbound.DirectDialer(outbound.NotMetadata).DialContext,
	ForceAttemptHTTP2: true, MaxIdleConns: 4, IdleConnTimeout: 90 * time.Second, ResponseHeaderTimeout: flareWait + 10*time.Second},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

var prePage = regexp.MustCompile(`(?is)^\s*<html[^>]*>\s*<head>.*?</head>\s*<body>\s*<pre[^>]*>(.*)</pre>`)

// unwrapPre takes a JSON or text answer out of the page a browser wraps it
// in, so a JSON site reads the same through FlareSolverr.
func unwrapPre(b []byte) []byte {
	if m := prePage.FindSubmatch(b); m != nil {
		inner := strings.TrimSpace(htmlesc.UnescapeString(string(m[1])))
		if strings.HasPrefix(inner, "{") || strings.HasPrefix(inner, "[") {
			return []byte(inner)
		}
	}
	return b
}
