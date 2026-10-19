package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/source"
)

// DefaultArtworkCap is the artwork cache size cap (decision 0002, Images).
const DefaultArtworkCap = 2 << 30

// maxPicture is the largest picture accepted.
const maxPicture = 10 << 20

// ArtworkCache keeps posters on disk under a size cap, dropping the least
// recently used first. It is rebuildable and not part of backups.
type ArtworkCache struct {
	Dir string
	// Cap is the size cap in bytes (DefaultArtworkCap when 0).
	Cap int64
	// Allowed lists URL prefixes pictures may come from (the TMDB image
	// host); anything else is refused.
	Allowed []string
	// HTTP is the client to fetch with (tests); nil means ArtworkClient.
	// Redirects are checked against Allowed either way.
	HTTP *http.Client
	mu   sync.Mutex
}

// ErrArtworkURL means a picture URL is not from an allowed place.
var ErrArtworkURL = errors.New("picture URL is not from an allowed host")

// ArtworkName is the cache file name for a picture URL.
func ArtworkName(url string) string {
	h := sha256.Sum256([]byte(url))
	ext := strings.ToLower(filepath.Ext(url))
	if ext != ".png" && ext != ".webp" {
		ext = ".jpg"
	}
	return hex.EncodeToString(h[:16]) + ext
}

// Fetch stores the picture at url (when not already there) and returns its
// cache name.
func (c *ArtworkCache) Fetch(ctx context.Context, url string) (string, error) {
	if !c.allowed(url) {
		return "", ErrArtworkURL
	}
	name := ArtworkName(url)
	path := filepath.Join(c.Dir, name)
	if _, err := os.Stat(path); err == nil {
		return name, nil
	}
	if err := os.MkdirAll(c.Dir, 0o750); err != nil {
		return "", err
	}
	base := c.HTTP
	if base == nil {
		base = ArtworkClient()
	}
	// Every redirect must stay on an allowed place too.
	cl := *base
	cl.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !c.allowed(req.URL.String()) {
			return ErrArtworkURL
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := cl.Do(req)
	if err != nil {
		return "", fmt.Errorf("picture: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("picture: answered %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "image/") {
		return "", fmt.Errorf("picture: not an image (%s)", ct)
	}
	tmp, err := os.CreateTemp(c.Dir, ".part-*")
	if err != nil {
		return "", err
	}
	n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxPicture+1))
	_ = tmp.Close()
	if err == nil && n > maxPicture {
		err = errors.New("picture: too large")
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	c.trim()
	return name, nil
}

// Path returns the file for a cache name and marks it used; ok is false
// when the name is not a cache file.
func (c *ArtworkCache) Path(name string) (string, bool) {
	if name != filepath.Base(name) || strings.HasPrefix(name, ".") || len(name) < 36 {
		return "", false
	}
	p := filepath.Join(c.Dir, name)
	if _, err := os.Stat(p); err != nil {
		return "", false
	}
	now := time.Now()
	_ = os.Chtimes(p, now, now)
	return p, true
}

// trim drops the least recently used pictures until the cache fits its cap.
func (c *ArtworkCache) trim() {
	c.mu.Lock()
	defer c.mu.Unlock()
	limit := c.Cap
	if limit <= 0 {
		limit = DefaultArtworkCap
	}
	es, err := os.ReadDir(c.Dir)
	if err != nil {
		return
	}
	type f struct {
		path string
		size int64
		at   time.Time
	}
	var fs []f
	var total int64
	for _, e := range es {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if in, err := e.Info(); err == nil {
			fs = append(fs, f{filepath.Join(c.Dir, e.Name()), in.Size(), in.ModTime()})
			total += in.Size()
		}
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].at.Before(fs[j].at) })
	for _, x := range fs {
		if total <= limit {
			break
		}
		if os.Remove(x.path) == nil {
			total -= x.size
		}
	}
}

// ArtworkClient is the client pictures are fetched with: public addresses
// only, no proxy.
func ArtworkClient() *http.Client {
	d := source.PolicyDialer(source.PublicOnly)
	return &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: nil, DialContext: d.DialContext,
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 20 * time.Second}}
}

// allowed says url is from an allowed place.
func (c *ArtworkCache) allowed(url string) bool {
	if strings.Contains(url, "..") {
		return false
	}
	for _, p := range c.Allowed {
		if strings.HasPrefix(url, p) || wildHost(url, p) {
			return true
		}
	}
	return false
}

// wildHost matches an Allowed entry "https://*.example.org/" against any
// https URL on a subdomain of example.org (archive.org serves covers from
// many numbered hosts).
func wildHost(raw, p string) bool {
	rest, ok := strings.CutPrefix(p, "https://*.")
	if !ok {
		return false
	}
	u, err := neturl.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	return strings.HasSuffix(u.Hostname(), "."+strings.TrimSuffix(rest, "/"))
}
