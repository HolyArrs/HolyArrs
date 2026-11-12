package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// DefaultArtworkCap is the artwork cache size cap (decision 0002, Images;
// 5 GB since 24 October 2026 so every music page's covers fit ahead).
const DefaultArtworkCap = 5 << 30

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
	// Fallback names a second address for a picture whose own host could
	// not be reached (TMDB down: the metadata server passes the picture
	// through); nil or "" has none.
	Fallback func(url string) string
	mu       sync.Mutex
	// checking holds the addresses being re-checked in the background.
	checking map[string]bool
	// fetching holds the downloads under way by cache name: a second ask for
	// the same picture waits for the first instead of downloading it again
	// (two writers renaming onto one file fail on Windows, and the page got
	// a broken picture).
	fetching map[string]chan struct{}
	// size is the cache's bytes, counted by the first trim and kept up by
	// each download (listing a large cache takes seconds, so it is listed
	// only when the count goes over the cap); trims counts the listings.
	trimMu  sync.Mutex
	setCap  atomic.Int64 // SetCap's cap, over Cap when set
	size    int64
	counted bool
	trims   int
}

// TVMazeImages is TVmaze's picture host (posters for shows whose details
// come from TVmaze); matched as an exact https prefix.
const TVMazeImages = "https://static.tvmaze.com/"

// PictureSources is the app's ArtworkCache.Allowed list for the metadata
// server at server: TMDB, the server's own pictures, its older picture
// address and TVmaze.
func PictureSources(server string) []string {
	return []string{metadata.ImageBase + "/", server + "/v1/img/", metadata.OldPictures(server), TVMazeImages}
}

// ErrArtworkURL means a picture URL is not from an allowed place.
var ErrArtworkURL = errors.New("picture URL is not from an allowed host")

// serverPictureKeep is how long a picture at a stable address is used before it is checked again.
const serverPictureKeep = 24 * time.Hour

// TMDBKeep is the longest a copy of a TMDB picture is kept (TMDB's rule:
// six months); an older copy is fetched again, or dropped when unused.
const TMDBKeep = 182 * 24 * time.Hour

// tmdbIdle is how long an old TMDB copy may go unshown before the sweep
// drops it instead of fetching it again.
const tmdbIdle = 30 * 24 * time.Hour

// IsTMDBPicture says url is a picture on TMDB's own picture host.
func IsTMDBPicture(url string) bool { return strings.HasPrefix(url, metadata.ImageBase+"/") }

// TMDBPassThrough maps a TMDB picture to the metadata server's pass-through
// address ({server}/v1/img/tmdb/{size}/{file}), asked only when TMDB's own
// network fails; the server stores nothing.
func TMDBPassThrough(server string) func(string) string {
	server = strings.TrimRight(server, "/")
	return func(u string) string {
		rest, ok := strings.CutPrefix(u, metadata.ImageBase+"/")
		size, file, ok2 := strings.Cut(rest, "/")
		if !ok || !ok2 || server == "" {
			return ""
		}
		if _, ok := TMDBArtworkURL(metadata.ImageBase, size, file); !ok {
			return ""
		}
		return server + "/v1/img/tmdb/" + size + "/" + file
	}
}

// ArtworkName is the cache file name for a picture URL.
func ArtworkName(url string) string {
	h := sha256.Sum256([]byte(url))
	path, _, _ := strings.Cut(url, "?")
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".png" && ext != ".webp" {
		ext = ".jpg"
	}
	return hex.EncodeToString(h[:16]) + ext
}

// tmdbSizes are the picture sizes TMDB serves; asking for one of them means
// the stored file is used as it comes, with no resizing here.
var tmdbSizes = map[string]bool{"w92": true, "w154": true, "w185": true, "w300": true, "w342": true, "w500": true, "w780": true, "w1280": true, "h632": true, "original": true}

// TMDBArtworkURL is the TMDB picture at base (metadata.ImageBase) for a
// size and file name taken from /api/v1/artwork/tmdb/{size}/{file}; ok is
// false for an unknown size or a name that is not a plain picture file.
func TMDBArtworkURL(base, size, file string) (string, bool) {
	if !tmdbSizes[size] || len(file) < 5 || len(file) > 80 {
		return "", false
	}
	stem, ext, ok := strings.Cut(file, ".")
	if !ok || (ext != "jpg" && ext != "png" && ext != "webp") || stem == "" {
		return "", false
	}
	for _, r := range stem {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return "", false
		}
	}
	return strings.TrimSuffix(base, "/") + "/" + size + "/" + file, true
}

// Fetch stores the picture at url (when not already there) and returns its
// cache name.
func (c *ArtworkCache) Fetch(ctx context.Context, url string) (string, error) {
	return c.FetchUnless(ctx, url, nil)
}

// ErrArtworkRejected means the answer was not a real picture: reject said so,
// or (for an artist's photo) it is only a stand-in (see StandInReason).
var ErrArtworkRejected = errors.New("picture: rejected")

// FetchUnless is Fetch, but nothing is stored when reject (given the
// response headers) says the answer is only a stand-in. The metadata
// server's own stand-in (X-Metadata-Placeholder) is always refused.
func (c *ArtworkCache) FetchUnless(ctx context.Context, url string, reject func(http.Header) bool) (string, error) {
	return c.fetch(ctx, url, reject, true)
}

// ArtworkStatusError is a picture address's non-200 answer.
type ArtworkStatusError struct{ Code int }

func (e *ArtworkStatusError) Error() string { return fmt.Sprintf("picture: answered %d", e.Code) }

// Temporary reports an answer that says nothing about the picture (rate
// limit, server trouble): ask again later, never treat it as missing.
func (e *ArtworkStatusError) Temporary() bool { return e.Code == 429 || e.Code >= 500 }

// ErrArtworkMoved means FetchStored's address answered with a redirect.
var ErrArtworkMoved = errors.New("picture: moved")

// FetchStored is FetchUnless without following a redirect: only a picture
// the address answers itself is kept (the metadata server redirects an
// album cover it has not stored yet; the caller then asks its other sources).
func (c *ArtworkCache) FetchStored(ctx context.Context, url string, reject func(http.Header) bool) (string, error) {
	return c.fetch(ctx, url, reject, false)
}

func (c *ArtworkCache) fetch(ctx context.Context, url string, reject func(http.Header) bool, follow bool) (string, error) {
	if !c.allowed(url) {
		return "", ErrArtworkURL
	}
	name := ArtworkName(url)
	path := filepath.Join(c.Dir, name)
	// A stored picture answers at once; one whose address may later show
	// another picture is re-checked in the background.
	if _, err := os.Stat(path); err == nil {
		c.Recheck(url, reject)
		return name, nil
	}
	c.mu.Lock()
	if c.fetching == nil {
		c.fetching = map[string]chan struct{}{}
	}
	if ch, busy := c.fetching[name]; busy {
		c.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		// The first download is done: its picture answers, or (it failed)
		// this ask tries on its own.
		return c.fetch(ctx, url, reject, follow)
	}
	ch := make(chan struct{})
	c.fetching[name] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.fetching, name)
		c.mu.Unlock()
		close(ch)
	}()
	if _, err := os.Stat(path); err == nil {
		return name, nil
	}
	if err := os.MkdirAll(c.Dir, 0o750); err != nil {
		return "", err
	}
	if _, err := c.download(ctx, url, reject, "", follow); err != nil {
		return "", err
	}
	return name, nil
}

// StableArtworkURL says the picture at url may change while its address
// stays the same (the metadata server's unversioned picture addresses).
// TMDB names and versioned (?v=) addresses change with the picture.
func StableArtworkURL(url string) bool {
	u, err := neturl.Parse(url)
	if err != nil {
		return false
	}
	switch p := u.Path; {
	case strings.Contains(p, "/v1/img/"):
		return u.Query().Get("v") == ""
	case strings.Contains(p, "/v1/music/cover/"), strings.Contains(p, "/release-group/"), strings.Contains(p, "/release/"):
		return true
	}
	return false
}

// StableFile says the stored picture at path came from a stable address
// (it has a check file), so its bytes may be replaced in place.
func StableFile(path string) bool {
	_, err := os.Stat(filepath.Join(filepath.Dir(path), checkName(filepath.Base(path))))
	return err == nil
}

// checkName is the hidden file beside a picture that keeps its ETag; its
// time is when the picture was last checked.
func checkName(name string) string { return ".chk-" + name }

// tmdbName is the hidden file beside a TMDB picture that keeps its address;
// its time is when the copy was fetched.
func tmdbName(name string) string { return ".tmdb-" + name }

// Recheck asks the source again, in the background and at most daily, for a
// stored picture whose address is stable: the ETag is sent, so an unchanged
// picture costs a 304 and no download. A TMDB copy older than TMDBKeep (or
// of unknown age) is fetched again. Pages never wait on it.
func (c *ArtworkCache) Recheck(url string, reject func(http.Header) bool) {
	if IsTMDBPicture(url) && c.allowed(url) {
		c.refreshTMDB(url, false)
		return
	}
	if !StableArtworkURL(url) || !c.allowed(url) {
		return
	}
	chk := filepath.Join(c.Dir, checkName(ArtworkName(url)))
	if st, err := os.Stat(chk); err == nil && time.Since(st.ModTime()) < serverPictureKeep {
		return
	}
	c.mu.Lock()
	if c.checking == nil {
		c.checking = map[string]bool{}
	}
	if c.checking[url] {
		c.mu.Unlock()
		return
	}
	c.checking[url] = true
	c.mu.Unlock()
	go func() {
		defer func() {
			c.mu.Lock()
			delete(c.checking, url)
			c.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(metadata.Ahead(context.Background()), 30*time.Second)
		defer cancel()
		etag, _ := os.ReadFile(chk)
		if _, err := c.download(ctx, url, reject, string(etag), true); err != nil {
			// Try again in an hour; the stored picture keeps answering.
			at := time.Now().Add(time.Hour - serverPictureKeep)
			if os.Chtimes(chk, at, at) != nil {
				_ = os.WriteFile(chk, etag, 0o640)
				_ = os.Chtimes(chk, at, at)
			}
		}
	}()
}

// download fetches url into the cache (with If-None-Match when etag is
// set) and records its ETag and check time; a 304 only marks the check.
// Without follow a redirect answers ErrArtworkMoved. A picture whose host
// cannot be reached (no answer, or server trouble) comes through Fallback.
func (c *ArtworkCache) download(ctx context.Context, url string, reject func(http.Header) bool, etag string, follow bool) (int, error) {
	resp, err := c.get(ctx, url, etag, follow)
	down := err != nil && !errors.Is(err, ErrArtworkMoved) && !errors.Is(err, ErrArtworkURL) && ctx.Err() == nil
	if fb := c.fallback(url); fb != "" && (down || err == nil && resp.StatusCode >= 500) {
		if resp != nil {
			_ = resp.Body.Close()
		}
		resp, err = c.get(ctx, fb, "", follow)
	}
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	name := ArtworkName(url)
	path := filepath.Join(c.Dir, name)
	chk := filepath.Join(c.Dir, checkName(name))
	if resp.StatusCode == http.StatusNotModified && etag != "" {
		now := time.Now()
		if os.Chtimes(chk, now, now) != nil {
			_ = os.WriteFile(chk, []byte(etag), 0o640)
		}
		return resp.StatusCode, nil
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, &ArtworkStatusError{Code: resp.StatusCode}
	}
	if resp.Header.Get("X-Metadata-Placeholder") != "" || (reject != nil && reject(resp.Header)) {
		return resp.StatusCode, ErrArtworkRejected
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "image/") {
		return resp.StatusCode, fmt.Errorf("picture: not an image (%s)", ct)
	}
	tmp, err := os.CreateTemp(c.Dir, ".part-*")
	if err != nil {
		return resp.StatusCode, err
	}
	n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxPicture+1))
	_ = tmp.Close()
	if err == nil && n > maxPicture {
		err = errors.New("picture: too large")
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return resp.StatusCode, err
	}
	// An artist's photo that is only a stand-in (a known one, one flat colour,
	// a grey person outline) is never kept.
	if isArtistPhotoURL(url) {
		if b, rerr := os.ReadFile(tmp.Name()); rerr == nil {
			if why := StandInReason(b); why != "" {
				_ = os.Remove(tmp.Name())
				return resp.StatusCode, fmt.Errorf("%w: %s", ErrArtworkRejected, why)
			}
		}
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		// A new picture another download stored first (it may be open
		// for a Reader, so it cannot be replaced on Windows) is the same
		// picture: that is a success.
		if _, serr := os.Stat(path); serr == nil && etag == "" {
			return resp.StatusCode, nil
		}
		return resp.StatusCode, err
	}
	if StableArtworkURL(url) {
		_ = os.WriteFile(chk, []byte(resp.Header.Get("ETag")), 0o640)
	}
	if IsTMDBPicture(url) {
		_ = os.WriteFile(filepath.Join(c.Dir, tmdbName(name)), []byte(url), 0o640)
	}
	c.added(n)
	return resp.StatusCode, nil
}

// fallback is the second address for url, when it is allowed.
func (c *ArtworkCache) fallback(url string) string {
	if c.Fallback == nil {
		return ""
	}
	if fb := c.Fallback(url); fb != "" && c.allowed(fb) {
		return fb
	}
	return ""
}

// get asks url once. Every redirect must stay on an allowed place, and no
// Referer is ever sent (TMDB's rule; Go adds one on a redirect).
func (c *ArtworkCache) get(ctx context.Context, url, etag string, follow bool) (*http.Response, error) {
	base := c.HTTP
	if base == nil {
		base = ArtworkClient()
	}
	cl := *base
	cl.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !follow {
			return ErrArtworkMoved
		}
		if len(via) >= 5 || !c.allowed(req.URL.String()) {
			return ErrArtworkURL
		}
		req.Header.Del("Referer")
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if metadata.IsAhead(ctx) {
		// A preload or warm: nobody waits, so the metadata server never
		// treats it as urgent work.
		req.Header.Set("X-Fetch", "ahead")
	}
	resp, err := cl.Do(req)
	if err != nil {
		if errors.Is(err, ErrArtworkMoved) || errors.Is(err, ErrArtworkURL) {
			return nil, err
		}
		return nil, fmt.Errorf("picture: %w", err)
	}
	return resp, nil
}

// refreshTMDB fetches a TMDB copy again when it is older than TMDBKeep or
// its age is unknown (stored before ages were kept): in the background for
// a page, at once for the sweep, which drops a copy unshown for tmdbIdle
// instead. A picture TMDB no longer has is dropped; any other failure keeps
// the copy until the next try.
func (c *ArtworkCache) refreshTMDB(url string, sweep bool) {
	name := ArtworkName(url)
	pic := filepath.Join(c.Dir, name)
	mark := filepath.Join(c.Dir, tmdbName(name))
	if st, err := os.Stat(mark); err == nil && time.Since(st.ModTime()) < TMDBKeep {
		return
	}
	if sweep {
		if st, err := os.Stat(pic); err != nil || time.Since(st.ModTime()) > tmdbIdle {
			_ = os.Remove(pic)
			_ = os.Remove(mark)
			return
		}
	}
	c.mu.Lock()
	if c.checking == nil {
		c.checking = map[string]bool{}
	}
	if c.checking[url] {
		c.mu.Unlock()
		return
	}
	c.checking[url] = true
	c.mu.Unlock()
	run := func() {
		defer func() {
			c.mu.Lock()
			delete(c.checking, url)
			c.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(metadata.Ahead(context.Background()), 30*time.Second)
		defer cancel()
		_, err := c.download(ctx, url, nil, "", true)
		var se *ArtworkStatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			_ = os.Remove(pic)
			_ = os.Remove(mark)
		}
	}
	if sweep {
		run()
		return
	}
	go run()
}

// SweepTMDB walks the TMDB copies once: each older than TMDBKeep is fetched
// again, or dropped when nobody has shown it for a month. It runs at start
// and then daily (KeepTMDBFresh); nothing waits on it.
func (c *ArtworkCache) SweepTMDB() {
	es, err := os.ReadDir(c.Dir)
	if err != nil {
		return
	}
	for _, e := range es {
		name, ok := strings.CutPrefix(e.Name(), ".tmdb-")
		if !ok {
			continue
		}
		in, err := e.Info()
		if err != nil || time.Since(in.ModTime()) < TMDBKeep {
			continue
		}
		mark := filepath.Join(c.Dir, e.Name())
		b, _ := os.ReadFile(mark)
		if url := string(b); IsTMDBPicture(url) && ArtworkName(url) == name {
			c.refreshTMDB(url, true)
			continue
		}
		_ = os.Remove(filepath.Join(c.Dir, name))
		_ = os.Remove(mark)
	}
}

// KeepTMDBFresh runs SweepTMDB now and then once a day until ctx ends.
func (c *ArtworkCache) KeepTMDBFresh(ctx context.Context) {
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		c.SweepTMDB()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// added counts a stored picture's bytes; the cache is listed and trimmed
// only before its size is known and when the count goes over the cap.
func (c *ArtworkCache) added(n int64) {
	c.mu.Lock()
	c.size += n
	over := !c.counted || c.size > c.limit()
	c.mu.Unlock()
	if over {
		c.trim()
	}
}

// SetCap changes the size cap while running (Settings); the next download
// over it trims.
func (c *ArtworkCache) SetCap(n int64) {
	c.setCap.Store(n)
	c.mu.Lock()
	c.counted = false // count again, so a smaller cap trims at once
	c.mu.Unlock()
}

func (c *ArtworkCache) limit() int64 {
	if n := c.setCap.Load(); n > 0 {
		return n
	}
	if c.Cap <= 0 {
		return DefaultArtworkCap
	}
	return c.Cap
}

func (c *ArtworkCache) trimCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.trims
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

// trim counts the cache and, over its cap, drops the least recently used
// pictures down to 90% of it (so the next downloads need no listing).
func (c *ArtworkCache) trim() {
	c.trimMu.Lock()
	defer c.trimMu.Unlock()
	limit := c.limit()
	c.mu.Lock()
	done := c.counted && c.size <= limit // another trim just ran
	c.mu.Unlock()
	if done {
		return
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
	if total > limit {
		sort.Slice(fs, func(i, j int) bool { return fs[i].at.Before(fs[j].at) })
		for _, x := range fs {
			if total <= limit-limit/10 {
				break
			}
			if os.Remove(x.path) == nil {
				total -= x.size
				base := filepath.Base(x.path)
				_ = os.Remove(filepath.Join(c.Dir, checkName(base)))
				_ = os.Remove(filepath.Join(c.Dir, tmdbName(base)))
			}
		}
	}
	c.mu.Lock()
	c.size, c.counted = total, true
	c.trims++
	c.mu.Unlock()
}

// ArtworkClient is the client pictures are fetched with: public addresses
// only, no proxy, HTTP/2 with connections kept between pictures (a row's
// posters share one connection to TMDB's picture host).
func ArtworkClient() *http.Client {
	d := outbound.PolicyDialer(outbound.PublicOnly)
	return &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: nil, DialContext: d.DialContext, ForceAttemptHTTP2: true,
		MaxIdleConns: 32, MaxIdleConnsPerHost: 8, IdleConnTimeout: 90 * time.Second,
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
