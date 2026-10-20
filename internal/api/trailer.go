package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// Trailers in HolyArrs' own player: the title's YouTube trailer, resolved
// by yt-dlp to a direct H.264 MP4 address on YouTube's video servers. Only
// the resolved address is kept (in memory, until shortly before it
// expires); no video passes through or is stored by HolyArrs.

// EnvYTDLP names the yt-dlp program to use; empty looks for yt-dlp in PATH.
const EnvYTDLP = "HOLYARRS_YTDLP"

const (
	trailerTimeout   = 8 * time.Second
	trailerPerMinute = 12               // resolves a person may start in a minute (cached answers are free)
	trailerKeep      = 4 * time.Hour    // when the address names no expiry
	trailerEarly     = 10 * time.Minute // dropped this long before it expires
	trailerMiss      = 10 * time.Minute // a title with no direct stream is not tried again before this
)

// trailerFormat asks for a single H.264 MP4 of at most 720p: muxed first
// (picture and sound), else picture only.
const trailerFormat = "best[ext=mp4][vcodec^=avc1][height<=720]/bestvideo[ext=mp4][vcodec^=avc1][height<=720]"

// ytdlp is how yt-dlp is started; tests swap it for a fake.
var ytdlp = func() (bin string, pre []string, ok bool) {
	if p := strings.TrimSpace(os.Getenv(EnvYTDLP)); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil, true
		}
		return "", nil, false
	}
	p, err := exec.LookPath("yt-dlp")
	return p, nil, err == nil
}

// TrailerStream is the answer of GET /trailer/{kind}/{tmdb_id}.
type TrailerStream struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
	Width     int       `json:"width,omitempty"`
	Height    int       `json:"height,omitempty"`
	// Key is the YouTube video, for the embed fallback.
	Key string `json:"key"`
}

type trailerEntry struct {
	s    TrailerStream
	miss time.Time // set: no direct stream, until this time
}

type trailerState struct {
	mu     sync.Mutex
	cache  map[string]trailerEntry
	budget map[int64][]time.Time
}

func (t *trailerState) get(key string, now time.Time) (trailerEntry, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.cache[key]
	if !ok {
		return e, false
	}
	if (!e.miss.IsZero() && now.After(e.miss)) || (e.miss.IsZero() && now.After(e.s.ExpiresAt.Add(-trailerEarly))) {
		delete(t.cache, key)
		return e, false
	}
	return e, true
}

func (t *trailerState) put(key string, e trailerEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cache == nil {
		t.cache = map[string]trailerEntry{}
	}
	if len(t.cache) > 2000 {
		clear(t.cache)
	}
	t.cache[key] = e
}

// allow spends one of the person's resolves for this minute.
func (t *trailerState) allow(user int64, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.budget == nil {
		t.budget = map[int64][]time.Time{}
	}
	kept := t.budget[user][:0]
	for _, at := range t.budget[user] {
		if now.Sub(at) < time.Minute {
			kept = append(kept, at)
		}
	}
	if len(kept) >= trailerPerMinute {
		t.budget[user] = kept
		return false
	}
	t.budget[user] = append(kept, now)
	return true
}

// trailerLanguage is the first language of Accept-Language ("de" from "de-DE,de;q=0.9").
func trailerLanguage(r *http.Request) string {
	if l := strings.TrimSpace(r.URL.Query().Get("language")); len(l) == 2 {
		return strings.ToLower(l)
	}
	first, _, _ := strings.Cut(r.Header.Get("Accept-Language"), ",")
	first, _, _ = strings.Cut(strings.TrimSpace(first), ";")
	first, _, _ = strings.Cut(first, "-")
	if len(first) == 2 {
		return strings.ToLower(first)
	}
	return "en"
}

func (a *API) trailerStream(w http.ResponseWriter, r *http.Request) {
	kind, id := r.PathValue("kind"), r.PathValue("tmdb_id")
	if kind != "movie" && kind != "show" {
		fail(w, http.StatusBadRequest, "kind must be movie or show")
		return
	}
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		fail(w, http.StatusBadRequest, "tmdb_id must be a TMDB number")
		return
	}
	if len(a.kindsOn(kind)) == 0 {
		fail(w, http.StatusConflict, "that part is off")
		return
	}
	// Kid accounts never autoplay trailers.
	if a.kidMax(r) != "" {
		fail(w, http.StatusForbidden, "trailers do not play for Kid accounts")
		return
	}
	if !a.uiSettings(r.Context()).AutoplayTrailers {
		fail(w, http.StatusNotFound, "trailers are turned off")
		return
	}
	lang := trailerLanguage(r)
	ckey := kind + "|" + id + "|" + lang
	now := time.Now()
	if e, ok := a.trailers.get(ckey, now); ok {
		if !e.miss.IsZero() {
			fail(w, http.StatusNotFound, "no direct stream")
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, e.s)
		return
	}
	bin, pre, found := ytdlp()
	if !found {
		fail(w, http.StatusNotFound, "no direct stream")
		return
	}
	if !a.trailers.allow(a.person(r).UserID, now) {
		w.Header().Set("Retry-After", "60")
		fail(w, http.StatusTooManyRequests, "too many trailers; try again in a minute")
		return
	}
	d, ok := metadata.DiscovererFor(r.Context(), a.s.Metadata)
	if !ok {
		fail(w, http.StatusNotFound, "no direct stream")
		return
	}
	det, err := a.trailerDetails(r.Context(), d, kind, id)
	if err != nil {
		discoverFail(w, err)
		return
	}
	v, ok := metadata.PickTrailer(det.Videos, lang)
	if !ok {
		a.trailers.put(ckey, trailerEntry{miss: now.Add(trailerMiss)})
		fail(w, http.StatusNotFound, "no trailer")
		return
	}
	s, err := resolveTrailer(r.Context(), bin, pre, v.Key, now)
	if err != nil {
		a.trailers.put(ckey, trailerEntry{miss: now.Add(trailerMiss)})
		fail(w, http.StatusNotFound, "no direct stream")
		return
	}
	a.trailers.put(ckey, trailerEntry{s: s})
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, s)
}

// trailerDetails reads the title's details through the details cache.
func (a *API) trailerDetails(ctx context.Context, d metadata.Discoverer, kind, id string) (metadata.Details, error) {
	key := "details|" + kind + "|" + id
	if e, ok := a.discover.get(key); ok && e.details != nil && time.Since(e.at) < discoverTTL["details"] {
		return *e.details, nil
	}
	det, err := d.Details(ctx, kind, id)
	if err != nil {
		return det, err
	}
	a.discover.put(key, discoverEntry{at: time.Now(), details: &det})
	return det, nil
}

var errNoStream = errors.New("no direct stream")

// resolveTrailer runs yt-dlp (at most trailerTimeout) for one YouTube key
// and returns the chosen format's direct address.
func resolveTrailer(ctx context.Context, bin string, pre []string, key string, now time.Time) (TrailerStream, error) {
	ctx, cancel := context.WithTimeout(ctx, trailerTimeout)
	defer cancel()
	args := append(append([]string{}, pre...), "-f", trailerFormat, "-j", "--no-playlist", "--no-warnings", "--no-cache-dir", "--", "https://www.youtube.com/watch?v="+key)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil || len(out) > 8<<20 {
		return TrailerStream{}, errNoStream
	}
	var j struct {
		URL      string `json:"url"`
		Ext      string `json:"ext"`
		VCodec   string `json:"vcodec"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
		Protocol string `json:"protocol"`
	}
	if json.Unmarshal(out, &j) != nil {
		return TrailerStream{}, errNoStream
	}
	u, err := url.Parse(j.URL)
	if err != nil || u.Scheme != "https" || !strings.HasSuffix(u.Hostname(), ".googlevideo.com") || j.Ext != "mp4" || !strings.HasPrefix(j.VCodec, "avc1") || (j.Protocol != "" && j.Protocol != "https") || j.Height > 720 {
		return TrailerStream{}, errNoStream
	}
	exp := now.Add(trailerKeep)
	if n, err := strconv.ParseInt(u.Query().Get("expire"), 10, 64); err == nil && n > now.Unix() {
		exp = time.Unix(n, 0).UTC()
	}
	return TrailerStream{URL: j.URL, ExpiresAt: exp, Width: j.Width, Height: j.Height, Key: key}, nil
}
