package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// FlareSolverr (github.com/FlareSolverr/FlareSolverr) opens a page in a real
// browser, waits out the site's bot check and hands back the page with the
// cookies it got. Its API: POST <address>/v1 with
// {"cmd": "request.get", "url": ..., "maxTimeout": ms, "proxy": {...}};
// the answer has "status" ("ok" or "error"), "message" and "solution"
// (url, status, response, cookies, userAgent).

const (
	// maxTimeout is how long FlareSolverr's browser may work on one page.
	// HolyArrs waits 75 seconds for the answer, from when the job was queued.
	maxTimeout = 60 * time.Second
	// callWait is how long the plugin waits for FlareSolverr: its own limit
	// plus a little for starting the browser and sending the page.
	callWait = maxTimeout + 8*time.Second
	// bodyMax is the most HolyArrs takes as the page (16 MB).
	bodyMax = 16 << 20
	// readMax is the most read of FlareSolverr's answer: the page, escaped
	// in JSON, plus the cookies and headers around it.
	readMax = 40 << 20
	// cookiesMax is the most cookies HolyArrs keeps of one answer.
	cookiesMax = 50

	downFirst = 10 * time.Second
	downMost  = 2 * time.Minute
)

var (
	errNoAddress  = errors.New("the FlareSolverr address is not set; set it in the plugin's settings")
	errBadAddress = errors.New("the FlareSolverr address must look like http://flaresolverr:8191")
	errDown       = errors.New("FlareSolverr did not answer; check its address, that it runs, and that its host and port are added under Also allow your own server")
	errSlow       = errors.New("FlareSolverr took too long to answer")
	errNotFS      = errors.New("the FlareSolverr address answered with something that is not FlareSolverr; check the address")
	errStill      = errors.New("FlareSolverr's browser was still on the bot check page")
	errTooLarge   = errors.New("the page is larger than 16 MB")
)

// parseAddress checks the FlareSolverr address: http or https, a host, no
// user, query or path other than /v1. A bare "flaresolverr:8191" means http.
// Empty is none.
func parseAddress(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/" && u.Path != "/v1") || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", errBadAddress
	}
	return u.Scheme + "://" + u.Host, nil
}

// proxy is the Source's own proxy, as the hook hands it and as FlareSolverr
// takes it.
type proxy struct {
	URL      string `json:"url"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// ask is one page to open (the job's payload). URL can hold the Source's key.
type ask struct {
	URL   string `json:"url"`
	Proxy *proxy `json:"proxy,omitempty"`
}

type cookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// answer is what the hook takes back: where the browser ended and what it got.
type answer struct {
	URL       string   `json:"url"`
	Status    int      `json:"status"`
	Body      string   `json:"body"`
	Cookies   []cookie `json:"cookies,omitempty"`
	UserAgent string   `json:"user_agent,omitempty"`
}

type fsAsk struct {
	Cmd        string `json:"cmd"`
	URL        string `json:"url"`
	MaxTimeout int    `json:"maxTimeout"`
	Proxy      *proxy `json:"proxy,omitempty"`
}

type fsAnswer struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	Solution *struct {
		URL       string   `json:"url"`
		Status    int      `json:"status"`
		Response  string   `json:"response"`
		Cookies   []cookie `json:"cookies"`
		UserAgent string   `json:"userAgent"`
	} `json:"solution"`
}

// flareSolverr asks one FlareSolverr. When it does not answer at all, it is
// left alone for a while (10 seconds, doubling up to 2 minutes) and jobs in
// that time are answered at once, so a stopped FlareSolverr is not hammered.
type flareSolverr struct {
	base     string
	client   *http.Client
	callWait time.Duration
	now      func() time.Time

	mu        sync.Mutex
	downFor   time.Duration
	downUntil time.Time
}

func newFlareSolverr(base string) *flareSolverr {
	// FlareSolverr is reached through the plugin's proxy in HolyArrs, which
	// lets it through only to hosts the owner added.
	return &flareSolverr{base: base, callWait: callWait, now: time.Now,
		client: &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, MaxIdleConns: 4, IdleConnTimeout: 90 * time.Second},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (f *flareSolverr) isDown() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now().Before(f.downUntil)
}

func (f *flareSolverr) wentDown() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.downFor = min(max(2*f.downFor, downFirst), downMost)
	f.downUntil = f.now().Add(f.downFor)
}

func (f *flareSolverr) answered() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.downFor, f.downUntil = 0, time.Time{}
}

// open asks FlareSolverr for one page. Errors never hold the page's address.
func (f *flareSolverr) open(ctx context.Context, a ask) (answer, error) {
	if f.isDown() {
		return answer{}, errDown
	}
	payload, err := json.Marshal(fsAsk{Cmd: "request.get", URL: a.URL, MaxTimeout: int(maxTimeout / time.Millisecond), Proxy: a.Proxy})
	if err != nil {
		return answer{}, errNotFS
	}
	cctx, cancel := context.WithTimeout(ctx, f.callWait)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, f.base+"/v1", bytes.NewReader(payload))
	if err != nil {
		return answer{}, errBadAddress
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		switch {
		case ctx.Err() != nil:
			return answer{}, ctx.Err()
		case cctx.Err() != nil:
			// Busy on a hard check, not down: no back-off.
			return answer{}, errSlow
		}
		f.wentDown()
		return answer{}, errDown
	}
	defer resp.Body.Close()
	f.answered()
	body, err := io.ReadAll(io.LimitReader(resp.Body, readMax+1))
	switch {
	case cctx.Err() != nil && ctx.Err() == nil:
		return answer{}, errSlow
	case err != nil:
		return answer{}, errDown
	case len(body) > readMax:
		return answer{}, errTooLarge
	}
	var fa fsAnswer
	if json.Unmarshal(body, &fa) != nil || fa.Status == "" {
		return answer{}, errNotFS
	}
	if fa.Status != "ok" {
		// FlareSolverr's own reason, such as "Error solving the challenge.
		// Timeout after 60.0 seconds.", without any address in it.
		return answer{}, errors.New("FlareSolverr could not pass the bot check: " + clean(fa.Message, a.URL))
	}
	s := fa.Solution
	if s == nil {
		return answer{}, errNotFS
	}
	if len(s.Response) > bodyMax {
		return answer{}, errTooLarge
	}
	if stillBotCheck(s.Response) {
		return answer{}, errStill
	}
	out := answer{URL: s.URL, Status: s.Status, Body: s.Response, UserAgent: s.UserAgent}
	for _, c := range s.Cookies[:min(len(s.Cookies), cookiesMax)] {
		out.Cookies = append(out.Cookies, cookie{Name: c.Name, Value: c.Value})
	}
	return out, nil
}

// checkPage matches words that only a check page shows, in a lower-cased
// body: the same marks HolyArrs looks for, with the page title taken whole
// so a feed item called "Just a moment" is not one.
var checkPage = regexp.MustCompile(`<title>\s*(just a moment\.*|ddos-guard|please wait\.*)\s*</title>|/cdn-cgi/challenge-platform/h/|_cf_chl_opt|checking your browser|/\.well-known/ddos-guard/`)

// stillBotCheck says whether the page FlareSolverr ended on is still the
// site's check, so it never passed.
func stillBotCheck(body string) bool {
	return checkPage.MatchString(strings.ToLower(body))
}

var anyURL = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://\S+`)

// clean takes the page's address (and any other) out of a reason and clips
// it to 200 bytes, so neither the Source's key nor its site reaches HolyArrs's
// message or the log.
func clean(msg, page string) string {
	if page != "" {
		msg = strings.ReplaceAll(msg, page, "the page")
	}
	msg = strings.Join(strings.Fields(anyURL.ReplaceAllString(msg, "the page")), " ")
	if len(msg) > 200 {
		msg = msg[:200]
		for !utf8.ValidString(msg) {
			msg = msg[:len(msg)-1]
		}
	}
	return msg
}
