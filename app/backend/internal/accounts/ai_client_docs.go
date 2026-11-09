package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"syscall"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// Client ID metadata documents (MCP authorization spec, 2025-11-25 version): an AI app
// may use an https address as its client_id; the JSON there names the app
// and its return addresses. Claude, ChatGPT and VS Code use this by default.
// HolyArrs fetches it once an hour at most, from public addresses only,
// small, quick and without following redirects.

const (
	cimdFor     = time.Hour
	cimdMax     = 256
	cimdSize    = 5 << 10
	cimdTimeout = 5 * time.Second
)

// errPrivate refuses a document on this computer or a private network.
var errPrivate = errors.New("client metadata must come from a public address")

type aiCIMD struct {
	mu sync.Mutex
	m  map[string]cimdEntry
	// http is the fetcher; tests set it (and may reach this computer).
	http *http.Client
}

type cimdEntry struct {
	c     aiClient
	ok    bool
	until time.Time
}

func publicOnly(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || !outbound.PublicOnly(ip) {
		return errPrivate
	}
	return nil
}

func (c *aiCIMD) fetcher() *http.Client {
	if c.http != nil {
		return c.http
	}
	d := &net.Dialer{Timeout: cimdTimeout, Control: publicOnly}
	return &http.Client{
		Timeout:       cimdTimeout,
		Transport:     &http.Transport{DialContext: d.DialContext, Proxy: nil, TLSHandshakeTimeout: cimdTimeout},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// client reads (or recalls) the app named by an https client_id.
func (c *aiCIMD) client(ctx context.Context, id string) (aiClient, bool) {
	u, err := url.Parse(id)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || u.Path == "" || u.Path == "/" || len(id) > 512 {
		return aiClient{}, false
	}
	now := time.Now()
	c.mu.Lock()
	if e, ok := c.m[id]; ok && now.Before(e.until) {
		c.mu.Unlock()
		return e.c, e.ok
	}
	c.mu.Unlock()
	got, ok := c.fetch(ctx, id)
	c.mu.Lock()
	if c.m == nil || len(c.m) >= cimdMax {
		c.m = map[string]cimdEntry{}
	}
	c.m[id] = cimdEntry{c: got, ok: ok, until: now.Add(cimdFor)}
	c.mu.Unlock()
	return got, ok
}

func (c *aiCIMD) fetch(ctx context.Context, id string) (aiClient, bool) {
	ctx, cancel := context.WithTimeout(ctx, cimdTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, id, nil)
	if err != nil {
		return aiClient{}, false
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.fetcher().Do(req)
	if err != nil {
		return aiClient{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return aiClient{}, false
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, cimdSize+1))
	if err != nil || len(b) > cimdSize {
		return aiClient{}, false
	}
	var doc struct {
		ClientID   string   `json:"client_id"`
		Name       string   `json:"client_name"`
		Redirects  []string `json:"redirect_uris"`
		AuthMethod string   `json:"token_endpoint_auth_method"`
	}
	if json.Unmarshal(b, &doc) != nil || doc.ClientID != id || len(doc.Redirects) == 0 || len(doc.Redirects) > aiRedirectMax {
		return aiClient{}, false
	}
	// Public clients only: no secret is ever checked here.
	if doc.AuthMethod != "" && doc.AuthMethod != "none" {
		return aiClient{}, false
	}
	for _, r := range doc.Redirects {
		if !aiRedirectAllowed(r) {
			return aiClient{}, false
		}
	}
	return aiClient{Name: aiClientName(doc.Name), Redirects: doc.Redirects, ID: id}, true
}
