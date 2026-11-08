package music

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Picture records: the metadata server's version, stored size and main
// colour of each music picture it keeps. Every music answer carries the
// records of the pictures it shows ("pictures", by "album:<mbid>",
// "artist:<mbid>", "wide:<mbid>", "logo:<mbid>"), so a card draws its
// box and colour at first paint and asks the versioned address, which the
// browser and this install keep a year. A row's records come in one batch
// call, never one call per card.

const (
	// picKeep is how long an item whose main picture (cover or photo) is
	// stored keeps its records; picShort for one still missing it (the
	// server fetches missing pictures as they are asked for).
	picKeep  = 6 * time.Hour
	picShort = 2 * time.Minute
	// picMax caps the remembered items (the list starts over past it).
	picMax = 60000
	// picAnswerMax caps the items one answer lists records for.
	picAnswerMax = 400
)

// picWait is how long an answer waits for records not known yet; what comes
// later is there for the next answer (tests change it).
var picWait = 400 * time.Millisecond

type PicEntry struct {
	Artist metadata.ArtistPictures
	Album  metadata.AlbumPictures
	at     time.Time
}

func (e PicEntry) fresh(kind string, now time.Time) bool {
	keep := picShort
	if kind == "album" && e.Album.Cover != nil || kind == "artist" && e.Artist.Photo != nil {
		keep = picKeep
	}
	return now.Sub(e.at) < keep
}

type picRecords struct {
	mu sync.Mutex
	m  map[string]PicEntry
	// busy are the items being asked now: each channel closes when its
	// batch has answered.
	busy map[string]chan struct{}
	// aheadBusy marks the busy items asked by a background warm: those
	// may wait for the gate's background budget, so a person's answer
	// asks them again itself instead of waiting.
	aheadBusy map[string]bool
}

// PictureRecords answers the records of these artists and albums, asking
// the server for the ones not known (in batches of up to
// metadata.MusicPicturesMax, all at once) and waiting up to wait for them.
func (h *Handlers) PictureRecords(ctx context.Context, artists, albums []string, wait time.Duration) map[string]PicEntry {
	c := &h.pics
	now := time.Now()
	type ask struct {
		kind, id string
		known    bool
	}
	person := !metadata.IsAhead(ctx)
	var need []ask
	var waits []chan struct{}
	seen := map[string]bool{}
	// Kept packages' records are read before the lock: an album's comes from
	// the database (not kept in memory), and a page asks hundreds at once.
	lists := []struct {
		kind string
		ids  []string
	}{{"artist", artists}, {"album", albums}}
	kept := map[string]PicEntry{}
	for _, l := range lists {
		for _, id := range l.ids {
			id = strings.ToLower(id)
			if _, done := kept[l.kind+"|"+id]; done || !metadata.ValidMBID(id) {
				continue
			}
			if pe, ok := h.packagePictures(l.kind, id); ok {
				kept[l.kind+"|"+id] = pe
			}
		}
	}
	c.mu.Lock()
	if c.m == nil || len(c.m) > picMax {
		c.m = map[string]PicEntry{}
	}
	if c.busy == nil || c.aheadBusy == nil {
		c.busy, c.aheadBusy = map[string]chan struct{}{}, map[string]bool{}
	}
	for _, l := range lists {
		for _, id := range l.ids {
			id = strings.ToLower(id)
			k := l.kind + "|" + id
			if seen[k] || !metadata.ValidMBID(id) {
				continue
			}
			seen[k] = true
			if pe, ok := kept[k]; ok {
				// A kept package's records answer without asking the server.
				c.m[k] = pe
				continue
			}
			e, known := c.m[k]
			if known && e.fresh(l.kind, now) {
				continue
			}
			// A kept record past its time is served as it is and asked again
			// without waiting: only records not known at all hold an answer.
			if ch := c.busy[k]; ch != nil && !(person && c.aheadBusy[k]) {
				if !known {
					waits = append(waits, ch)
				}
				continue
			}
			need = append(need, ask{l.kind, id, known})
		}
	}
	if h.MusicPictures == nil {
		need = nil
	}
	for len(need) > 0 {
		n := min(len(need), metadata.MusicPicturesMax)
		batch := need[:n]
		need = need[n:]
		done := make(chan struct{})
		var ar, al []string
		hold := false
		for _, x := range batch {
			hold = hold || !x.known
			c.busy[x.kind+"|"+x.id] = done
			c.aheadBusy[x.kind+"|"+x.id] = !person
			if x.kind == "artist" {
				ar = append(ar, x.id)
			} else {
				al = append(al, x.id)
			}
		}
		if hold {
			waits = append(waits, done)
		}
		go h.askPictureRecords(context.WithoutCancel(ctx), ar, al, done)
	}
	c.mu.Unlock()
	if len(waits) > 0 && wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
	wait:
		for _, ch := range waits {
			select {
			case <-ch:
			case <-t.C:
				break wait
			case <-ctx.Done():
				break wait
			}
		}
	}
	out := map[string]PicEntry{}
	c.mu.Lock()
	for k := range seen {
		if e, ok := c.m[k]; ok {
			out[k] = e
		}
	}
	c.mu.Unlock()
	return out
}

// askPictureRecords is one batch call; a failed call keeps nothing, so the
// next answer asks again.
func (h *Handlers) askPictureRecords(ctx context.Context, artists, albums []string, done chan struct{}) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	got, err := h.MusicPictures(ctx, artists, albums)
	c := &h.pics
	now := time.Now()
	c.mu.Lock()
	for _, id := range artists {
		if err == nil {
			c.m["artist|"+id] = PicEntry{Artist: got.Artists[id], at: now}
		}
		if c.busy["artist|"+id] == done {
			delete(c.busy, "artist|"+id)
			delete(c.aheadBusy, "artist|"+id)
		}
	}
	for _, id := range albums {
		if err == nil {
			c.m["album|"+id] = PicEntry{Album: got.Albums[id], at: now}
		}
		if c.busy["album|"+id] == done {
			delete(c.busy, "album|"+id)
			delete(c.aheadBusy, "album|"+id)
		}
	}
	c.mu.Unlock()
	close(done)
}

// pictureRecord is the known record of one picture (kind album, artist,
// wide or logo), or nil; it never asks the server.
func (h *Handlers) pictureRecord(kind, mbid string) *metadata.PictureRecord {
	owner := "artist"
	if kind == "album" {
		owner = "album"
	}
	c := &h.pics
	c.mu.Lock()
	e, ok := c.m[owner+"|"+strings.ToLower(mbid)]
	c.mu.Unlock()
	if !ok {
		if e, ok = h.packagePictures(owner, mbid); !ok {
			return nil
		}
	}
	switch kind {
	case "album":
		return e.Album.Cover
	case "artist":
		return e.Artist.Photo
	case "wide":
		return e.Artist.Wide
	case "logo":
		return e.Artist.Logo
	}
	return nil
}

// PictureVersion is the stored version of a picture ("" when not known).
func (h *Handlers) PictureVersion(kind, mbid string) string {
	if r := h.pictureRecord(kind, mbid); r != nil {
		return r.V
	}
	return ""
}

// WarmPictureRecords asks the records of a page's items ahead (one batch per
// hundred) without waiting, so the pictures warmed next are the versioned
// ones the page asks for (the warm waits for them: AwaitPictureRecords).
// The items come as kind ("album", "artist", "wide", "logo") and mbid.
func (h *Handlers) WarmPictureRecords(kinds, mbids []string) {
	var ar, al []string
	for i, kind := range kinds {
		switch kind {
		case "album":
			al = append(al, mbids[i])
		case "artist", "wide", "logo":
			ar = append(ar, mbids[i])
		}
	}
	if len(ar)+len(al) > 0 {
		h.PictureRecords(metadata.Ahead(context.Background()), ar, al, 0)
	}
}

// AwaitPictureRecords waits up to wait for an item's records that are being
// asked now; it never asks the server itself (one call per row, never one
// per card).
func (h *Handlers) AwaitPictureRecords(ctx context.Context, kind, mbid string, wait time.Duration) {
	owner := "artist"
	if kind == "album" {
		owner = "album"
	}
	c := &h.pics
	c.mu.Lock()
	ch := c.busy[owner+"|"+strings.ToLower(mbid)]
	c.mu.Unlock()
	if ch == nil {
		return
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ch:
	case <-t.C:
	case <-ctx.Done():
	}
}

var musicArtPath = regexp.MustCompile(`/api/v1/music-trending/art/(album|artist|wide|logo)/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)

// answerPictures is the "pictures" map for every music picture an encoded
// answer shows (and the extra picture paths given).
func (h *Handlers) answerPictures(ctx context.Context, b []byte, extra ...string) map[string]*metadata.PictureRecord {
	type item struct{ kind, mbid string }
	var items []item
	seen := map[item]bool{}
	var artists, albums []string
	add := func(kind, mbid string) {
		it := item{kind, mbid}
		if seen[it] || len(items) >= picAnswerMax {
			return
		}
		seen[it] = true
		items = append(items, it)
		if kind == "album" {
			albums = append(albums, mbid)
		} else {
			artists = append(artists, mbid)
		}
	}
	for _, m := range musicArtPath.FindAllSubmatch(b, -1) {
		add(string(m[1]), string(m[2]))
	}
	for _, p := range extra {
		if m := musicArtPath.FindStringSubmatch(p); m != nil {
			add(m[1], m[2])
		}
	}
	if len(items) == 0 {
		return nil
	}
	h.PictureRecords(ctx, artists, albums, picWait)
	out := map[string]*metadata.PictureRecord{}
	for _, it := range items {
		kinds := []string{it.kind}
		if it.kind != "album" {
			// An artist's wide background and logo too: the page a card opens
			// (its hero) is preloaded at the versioned address.
			kinds = []string{"artist", "wide", "logo"}
		}
		for _, k := range kinds {
			if r := h.pictureRecord(k, it.mbid); r != nil {
				out[k+":"+it.mbid] = r
			}
		}
	}
	return out
}

// WithPictures adds the "pictures" map to an encoded JSON object.
func (h *Handlers) WithPictures(ctx context.Context, b []byte, extra ...string) []byte {
	pics := h.answerPictures(ctx, b, extra...)
	t := bytes.TrimLeft(b, " \t\r\n")
	if len(pics) == 0 || len(t) < 2 || t[0] != '{' {
		return b
	}
	m, err := json.Marshal(pics)
	if err != nil {
		return b
	}
	rest := bytes.TrimLeft(t[1:], " \t\r\n")
	out := make([]byte, 0, len(b)+len(m)+16)
	out = append(out, `{"pictures":`...)
	out = append(out, m...)
	if len(rest) > 0 && rest[0] != '}' {
		out = append(out, ',')
	}
	return append(out, rest...)
}

// WriteMusicJSON is writeJSON for answers that show music pictures: the
// answer carries their records.
func (h *Handlers) WriteMusicJSON(w http.ResponseWriter, r *http.Request, v any, extra ...string) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		webserver.Internal(w, "music", err)
		return
	}
	b := h.WithPictures(r.Context(), buf.Bytes(), extra...)
	// The answer's hash is its ETag: a revisit with the same answer is a
	// 304 without the body.
	sum := sha256.Sum256(b)
	tag := `"` + hex.EncodeToString(sum[:12]) + `"`
	w.Header().Set("ETag", tag)
	if etagMatch(r.Header.Get("If-None-Match"), tag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

// etagMatch says an If-None-Match header names tag (weak or strong, or *).
func etagMatch(h, tag string) bool {
	for _, t := range strings.Split(h, ",") {
		t = strings.TrimPrefix(strings.TrimSpace(t), "W/")
		if t == tag || t == "*" {
			return true
		}
	}
	return false
}

// packagePictures is the records a kept artist package has for an artist
// or album (kind "artist" or "album"), stamped now.
func (h *Handlers) packagePictures(kind, mbid string) (PicEntry, bool) {
	p := h.Packages
	if p == nil {
		return PicEntry{}, false
	}
	switch kind {
	case "artist":
		if pics, ok := p.ArtistPictures(mbid); ok {
			return PicEntry{Artist: pics, at: time.Now()}, true
		}
	case "album":
		if cover, ok := p.AlbumCover(mbid); ok {
			return PicEntry{Album: metadata.AlbumPictures{Cover: cover}, at: time.Now()}, true
		}
	}
	return PicEntry{}, false
}
