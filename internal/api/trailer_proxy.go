package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Trailer pass-through: for a browser without native HLS (Chrome, Firefox,
// Edge) that wants sound, or a viewer whose direct playback is refused
// (YouTube ties its addresses to the IP that resolved them), HolyArrs
// relays the HLS manifest and its segments. Only addresses HolyArrs
// resolved itself are relayed, under opaque ids kept here; every address
// must be on YouTube's video servers. Bytes are streamed as they arrive
// and never stored.

const (
	proxyStreams   = 4                // streams a person may have going at once (hero, hover cards just left)
	proxyRequests  = 6                // relayed requests a person may have in flight
	proxyIdle      = 15 * time.Second // a stream with no request this long no longer counts
	proxyTimeout   = 60 * time.Second // one relayed request, start to end
	proxyMaxBody   = 64 << 20         // one relayed answer
	proxyMaxList   = 2 << 20          // one playlist
	proxyMaxURLs   = 5000             // addresses one stream may hold
	proxyMaxStream = 1 << 30          // bytes one stream may relay in all
	proxyMaxActive = 12               // streams the whole install may have going at once
	proxyLife      = 20 * time.Minute // how long one stream may relay, from its first request
)

// trailerProxyClient fetches from YouTube; tests point it at a fake.
var trailerProxyClient = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !googlevideo(req.URL.String()) {
			return errors.New("redirect refused")
		}
		return nil
	},
}

type proxyStream struct {
	user     int64
	manifest string
	expires  time.Time
	urls     []string
	index    map[string]int
	last     time.Time
	sent     int64
	started  time.Time
}

type trailerProxy struct {
	mu       sync.Mutex
	streams  map[string]*proxyStream
	byKey    map[string]string
	inFlight map[int64]int
}

// register gives the person an opaque id for a manifest HolyArrs resolved
// (the same id again for the same manifest). "" when the address is not
// on YouTube's video servers.
func (p *trailerProxy) register(user int64, manifest string, expires time.Time) string {
	if !googlevideo(manifest) {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	if p.streams == nil {
		p.streams, p.byKey, p.inFlight = map[string]*proxyStream{}, map[string]string{}, map[int64]int{}
	}
	k := strconv.FormatInt(user, 10) + "|" + manifest
	if id, ok := p.byKey[k]; ok && p.streams[id] != nil {
		return id
	}
	for id, s := range p.streams {
		if now.After(s.expires) {
			delete(p.streams, id)
			delete(p.byKey, strconv.FormatInt(s.user, 10)+"|"+s.manifest)
		}
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	p.streams[id] = &proxyStream{user: user, manifest: manifest, expires: expires, index: map[string]int{}}
	p.byKey[k] = id
	return id
}

var errProxyBusy = errors.New("busy")

// begin finds the person's stream and takes one in-flight slot; done frees it.
func (p *trailerProxy) begin(user int64, id string, now time.Time) (*proxyStream, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.streams[id]
	if s == nil || s.user != user || now.After(s.expires) {
		return nil, errNoStream
	}
	if !s.started.IsZero() && now.Sub(s.started) > proxyLife {
		return nil, errProxyBusy
	}
	if now.Sub(s.last) >= proxyIdle {
		active, all := 0, 0
		for oid, o := range p.streams {
			if oid != id && now.Sub(o.last) < proxyIdle {
				all++
				if o.user == user {
					active++
				}
			}
		}
		if active >= proxyStreams || all >= proxyMaxActive {
			return nil, errProxyBusy
		}
	}
	if p.inFlight[user] >= proxyRequests || s.sent >= proxyMaxStream {
		return nil, errProxyBusy
	}
	p.inFlight[user]++
	s.last = now
	if s.started.IsZero() {
		s.started = now
	}
	return s, nil
}

func (p *trailerProxy) done(user int64, s *proxyStream, sent int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inFlight[user]--
	s.sent += sent
	s.last = time.Now()
}

// address is the stream's n-th address.
func (p *trailerProxy) address(s *proxyStream, n int) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if n < 0 || n >= len(s.urls) {
		return "", false
	}
	return s.urls[n], true
}

// add keeps an address for the stream and answers its number.
func (p *trailerProxy) add(s *proxyStream, abs string) (int, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if n, ok := s.index[abs]; ok {
		return n, true
	}
	if len(s.urls) >= proxyMaxURLs {
		return 0, false
	}
	s.urls = append(s.urls, abs)
	s.index[abs] = len(s.urls) - 1
	return len(s.urls) - 1, true
}

var m3u8URI = regexp.MustCompile(`URI="([^"]*)"`)

// rewriteM3U8 points every address in a playlist at the relay: each one is
// resolved against base, must be on YouTube's video servers, and becomes
// prefix+<number>. A foreign address refuses the whole playlist.
func rewriteM3U8(body []byte, base *url.URL, prefix string, add func(abs string) (int, bool)) ([]byte, error) {
	if !bytes.HasPrefix(bytes.TrimSpace(body), []byte("#EXTM3U")) {
		return nil, errors.New("not a playlist")
	}
	swap := func(ref string) (string, error) {
		u, err := base.Parse(strings.TrimSpace(ref))
		if err != nil || !googlevideo(u.String()) {
			return "", errors.New("foreign address")
		}
		n, ok := add(u.String())
		if !ok {
			return "", errors.New("too many addresses")
		}
		return prefix + strconv.Itoa(n), nil
	}
	var out bytes.Buffer
	var bad error
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimRight(line, "\r")
		t := strings.TrimSpace(line)
		switch {
		case t == "":
		case strings.HasPrefix(t, "#"):
			line = m3u8URI.ReplaceAllStringFunc(line, func(m string) string {
				s, err := swap(m3u8URI.FindStringSubmatch(m)[1])
				if err != nil {
					bad = err
					return m
				}
				return `URI="` + s + `"`
			})
		default:
			s, err := swap(t)
			if err != nil {
				bad = err
			}
			line = s
		}
		if bad != nil {
			return nil, bad
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

// proxyOpen checks the person and the stream, as both relay routes do.
func (a *API) proxyOpen(w http.ResponseWriter, r *http.Request) (*proxyStream, int64, bool) {
	if a.kidMax(r) != "" {
		fail(w, http.StatusForbidden, "trailers do not play for Kid accounts")
		return nil, 0, false
	}
	if len(a.kindsOn("")) == 0 {
		fail(w, http.StatusConflict, "that part is off")
		return nil, 0, false
	}
	user := a.person(r).UserID
	s, err := a.trailerRelay.begin(user, r.PathValue("id"), time.Now())
	switch {
	case errors.Is(err, errProxyBusy):
		w.Header().Set("Retry-After", "15")
		fail(w, http.StatusTooManyRequests, "too many trailer streams at once")
		return nil, 0, false
	case err != nil:
		fail(w, http.StatusNotFound, "no such trailer stream")
		return nil, 0, false
	}
	return s, user, true
}

func proxyFetch(ctx context.Context, raw string, rng string) (*http.Response, error) {
	if !googlevideo(raw) {
		return nil, errNoStream
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	if rng != "" {
		req.Header.Set("Range", rng)
	}
	res, err := trailerProxyClient.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusPartialContent {
		res.Body.Close()
		return nil, errors.New("upstream " + res.Status)
	}
	return res, nil
}

func proxyHeaders(w http.ResponseWriter, ctype string) {
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

const m3u8Type = "application/vnd.apple.mpegurl"

func isPlaylist(res *http.Response, raw string) bool {
	ct := strings.ToLower(res.Header.Get("Content-Type"))
	return strings.Contains(ct, "mpegurl") || strings.Contains(raw, "/hls_playlist/") || strings.Contains(raw, "/hls_variant/") || strings.HasSuffix(strings.SplitN(raw, "?", 2)[0], ".m3u8")
}

// relayPlaylist reads, rewrites and sends one playlist.
func (a *API) relayPlaylist(w http.ResponseWriter, s *proxyStream, res *http.Response, raw, prefix string) int64 {
	body, err := io.ReadAll(io.LimitReader(res.Body, proxyMaxList+1))
	if err != nil || len(body) > proxyMaxList {
		fail(w, http.StatusBadGateway, "trailer playlist unreadable")
		return 0
	}
	base, _ := url.Parse(raw)
	out, err := rewriteM3U8(body, base, prefix, func(abs string) (int, bool) { return a.trailerRelay.add(s, abs) })
	if err != nil {
		fail(w, http.StatusBadGateway, "trailer playlist refused")
		return 0
	}
	proxyHeaders(w, m3u8Type)
	w.Header().Set("Content-Length", strconv.Itoa(len(out)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
	return int64(len(body))
}

// trailerManifest answers GET /trailer/stream/{id}/manifest.m3u8.
func (a *API) trailerManifest(w http.ResponseWriter, r *http.Request) {
	s, user, ok := a.proxyOpen(w, r)
	if !ok {
		return
	}
	var sent int64
	defer func() { a.trailerRelay.done(user, s, sent) }()
	ctx, cancel := context.WithTimeout(r.Context(), proxyTimeout)
	defer cancel()
	res, err := proxyFetch(ctx, s.manifest, "")
	if err != nil {
		fail(w, http.StatusBadGateway, "YouTube refused the trailer")
		return
	}
	defer res.Body.Close()
	sent = a.relayPlaylist(w, s, res, s.manifest, "seg/")
}

// trailerSegment answers GET /trailer/stream/{id}/seg/{n}: a variant
// playlist (rewritten) or a segment (streamed as it arrives).
func (a *API) trailerSegment(w http.ResponseWriter, r *http.Request) {
	s, user, ok := a.proxyOpen(w, r)
	if !ok {
		return
	}
	var sent int64
	defer func() { a.trailerRelay.done(user, s, sent) }()
	n, err := strconv.Atoi(r.PathValue("n"))
	raw, found := a.trailerRelay.address(s, n)
	if err != nil || !found {
		fail(w, http.StatusNotFound, "no such trailer segment")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), proxyTimeout)
	defer cancel()
	res, err := proxyFetch(ctx, raw, r.Header.Get("Range"))
	if err != nil {
		fail(w, http.StatusBadGateway, "YouTube refused the trailer")
		return
	}
	defer res.Body.Close()
	if isPlaylist(res, raw) {
		sent = a.relayPlaylist(w, s, res, raw, "")
		return
	}
	ct := strings.ToLower(res.Header.Get("Content-Type"))
	if !strings.HasPrefix(ct, "video/") && !strings.HasPrefix(ct, "audio/") {
		ct = "application/octet-stream"
	}
	if res.ContentLength > proxyMaxBody {
		fail(w, http.StatusBadGateway, "trailer segment too large")
		return
	}
	proxyHeaders(w, ct)
	for _, h := range []string{"Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := res.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Now().Add(proxyTimeout))
	w.WriteHeader(res.StatusCode)
	buf := make([]byte, 32<<10)
	for sent < proxyMaxBody {
		k, err := res.Body.Read(buf)
		if k > 0 {
			if _, werr := w.Write(buf[:k]); werr != nil {
				return
			}
			sent += int64(k)
			_ = rc.Flush()
		}
		if err != nil {
			return
		}
	}
}
