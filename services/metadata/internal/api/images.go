package api

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/gen2brain/webp"

	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

// ImageStore is the picture store (internal/imgstore).
type ImageStore interface {
	Served(ctx context.Context, kind, id, want string) (path string, rec imgstore.Record, final, ok bool)
}

// Images serves /v1/img/{kind}/{id}/{size}.{webp|jpg} (logos .png; no
// extension chooses by Accept, with Vary: Accept): a stored picture from
// disk. WebP is what is stored; a .jpg ask (older app builds) is made from
// it on the fly and never kept. Sizes: covers and photos 250 (served the
// 500), 500 and 1200; wide backgrounds 960 and 1920 (500 and 1200 served
// those); logos 500 and 1200 (one file). With ?v= matching the stored
// version the answer is immutable for a year.
//
// A picture not stored yet is fetched at P0 for someone waiting (an
// install's fair share) and served when it arrives within 2.5 s; otherwise
// an album answers a 60 s redirect to its Cover Art Archive front, an
// artist a neutral placeholder never kept, a wide background or logo a 404
// kept nowhere.
type Images struct {
	Store ImageStore
	// Want queues a picture job (P1). WantAhead queues one for a background
	// ask (X-Fetch: ahead: an install's preload or warm, nobody waiting) at
	// P2 or lower; nil queues nothing for those.
	Want      func(kind, mbid string)
	WantAhead func(kind, mbid string)
	// WantNow queues the job for an ask someone waits on (an open page)
	// at P0, under the asking install's fair share; nil uses Want.
	WantNow func(ctx context.Context, kind, mbid string)
	// WantRow queues a picture a row's batch ask found missing, at prefetch
	// priority (P2) under the install in ctx.
	WantRow func(ctx context.Context, kind, mbid string)
	// WantBig queues the big size of a cover or photo asked before it was
	// made (the card size stands in meanwhile).
	WantBig func(kind, mbid string)
	// Now fetches a missing picture right away (P0) and reports whether it
	// is stored; nil or false falls through to the queued answer.
	Now func(ctx context.Context, kind, mbid string) bool
	// CAA is the Cover Art Archive base (default https://coverartarchive.org);
	// Redirect reports whether redirecting there is on (kill switch).
	CAA      string
	Redirect func() bool
}

var placeholder = sync.OnceValue(func() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 250, 250))
	for i := range img.Pix {
		img.Pix[i] = 0x2a
		if i%4 == 3 {
			img.Pix[i] = 0xff
		}
	}
	var b bytes.Buffer
	_ = jpeg.Encode(&b, img, &jpeg.Options{Quality: 60})
	return b.Bytes()
})

// imageSizes are the sizes asked per kind.
var imageSizes = map[string]map[int]bool{
	"artist": {250: true, 500: true, 1200: true}, "album": {250: true, 500: true, 1200: true},
	"wide": {500: true, 960: true, 1200: true, 1920: true}, "logo": {500: true, 1200: true},
	"title-logo": {500: true, 1200: true}, "title-banner": {1000: true}, "title-wide": {1600: true},
	"person": {250: true, 500: true, 1200: true},
}

func (s *Server) serveImage(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/img/"), "/")
	if parts[0] == "tmdb" {
		s.serveTMDBImage(w, r, parts)
		return
	}
	q := r.URL.Query()
	ok := len(parts) == 3 && imageSizes[parts[0]] != nil && imgstore.ValidID(parts[0], parts[1])
	size, format := 0, ""
	if ok {
		name := parts[2]
		switch {
		case parts[0] == "logo" || parts[0] == "title-logo":
			// Logos are PNG only (they keep transparency).
			if !strings.HasSuffix(name, ".png") {
				ok = false
			}
			name, format = strings.TrimSuffix(name, ".png"), "png"
		case strings.HasSuffix(name, ".jpg"):
			name, format = strings.TrimSuffix(name, ".jpg"), "jpeg"
		case strings.HasSuffix(name, ".webp"):
			name, format = strings.TrimSuffix(name, ".webp"), "webp"
		}
		size, _ = strconv.Atoi(name)
		ok = ok && imageSizes[parts[0]][size] && strconv.Itoa(size) == name
	}
	negotiated := format == ""
	if negotiated {
		// No extension: the format follows Accept. The free edge keeps one
		// copy per URL whatever Vary says, so this answer is browser-only.
		format = "jpeg"
		if strings.Contains(r.Header.Get("Accept"), "image/webp") {
			format = "webp"
		}
	}
	for k, v := range q {
		ok = ok && k == "v" && len(v) == 1 && len(v[0]) <= 16
	}
	if !ok {
		problem(w, 400, "invalid_query", 0)
		return
	}
	im := s.opts.Images
	if im == nil || im.Store == nil {
		problem(w, 404, "not_found", 0)
		return
	}
	kind, mbid, want := parts[0], parts[1], strconv.Itoa(size)
	h := w.Header()
	// A background ask never becomes urgent work: no inline fetch, no P1.
	ahead := r.Header.Get("X-Fetch") == "ahead"
	who, _ := r.Context().Value(callerKey{}).(caller)
	queue := im.Want
	if ahead {
		queue = im.WantAhead
	} else if im.WantNow != nil {
		ictx := fetchq.WithInstall(context.WithoutCancel(r.Context()), who.who)
		queue = func(kind, mbid string) { im.WantNow(ictx, kind, mbid) }
	}
	path, rec, final, found := im.Store.Served(r.Context(), kind, mbid, want)
	if !found && s.anonSlimAbsent(r, slimKindOf(kind), mbid) {
		// Random ids from a caller without a credential never start
		// MusicBrainz or picture work (as on the MusicBrainz route).
		problem(w, 404, "not_found", 0)
		return
	}
	if !found && !ahead && im.Now != nil {
		if im.Now(fetchq.WithInstall(r.Context(), who.who), kind, mbid) {
			path, rec, final, found = im.Store.Served(r.Context(), kind, mbid, want)
		}
	}
	if found {
		if !final && im.WantBig != nil {
			im.WantBig(kind, mbid)
		}
		if s.writePicture(w, r, path, rec, want, format, final, negotiated) {
			return
		}
	}
	if queue != nil {
		queue(kind, mbid)
	}
	if kind != "album" && kind != "artist" {
		// Not stored yet: a 404 kept nowhere, so the stored one shows soon.
		h.Set("Cache-Control", "no-store")
		problem(w, 404, "not_found", 0)
		return
	}
	if kind == "album" && (im.Redirect == nil || im.Redirect()) {
		base := im.CAA
		if base == "" {
			base = "https://coverartarchive.org"
		}
		h.Set("Cache-Control", "public, max-age=60, s-maxage=60")
		h.Set("Location", base+"/release-group/"+mbid+"/front-"+strconv.Itoa(min(size, 1200)))
		w.WriteHeader(302)
		return
	}
	h.Set("Content-Type", "image/jpeg")
	// The real photo is queued; never keep the stand-in.
	h.Set("Cache-Control", "no-store")
	h.Set("X-Metadata-Placeholder", "true")
	w.WriteHeader(200)
	_, _ = w.Write(placeholder())
}

// writePicture answers a stored picture's file in the format asked; false
// when the file could not be read (the caller answers as if not stored).
func (s *Server) writePicture(w http.ResponseWriter, r *http.Request, path string, rec imgstore.Record, want, format string, final, negotiated bool) bool {
	h := w.Header()
	var body []byte
	var err error
	if format == "jpeg" {
		body, err = jpegOf(r.Context(), path)
	} else {
		body, err = os.ReadFile(path)
	}
	if err != nil {
		return false
	}
	etag := `"` + rec.Version + "-" + want + "-" + format + `"`
	h.Set("ETag", etag)
	if !final {
		// A smaller stand-in while the big size is made.
		h.Set(partialHeader, "true")
	}
	switch {
	case negotiated:
		// Chosen by Accept: browser-only, whatever else holds (the free
		// edge keeps one copy per address whatever Vary says).
		h.Set("Vary", "Accept")
		h.Set("Cache-Control", "private, max-age=86400")
		if !final {
			h.Set("Cache-Control", "private, max-age=60")
		}
	case !final:
		h.Set("Cache-Control", "public, max-age=60, s-maxage=60")
	case r.URL.Query().Get("v") == rec.Version:
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	default:
		// Without the version the address may later show a changed
		// picture: re-checked within the hour with the ETag.
		h.Set("Cache-Control", "public, max-age=600, s-maxage=3600")
	}
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(304)
		return true
	}
	// Stored pictures are WebP, PNG or a JPEG made from them, never SVG.
	h.Set("Content-Type", "image/"+format)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(200)
	_, _ = w.Write(body)
	return true
}

// jpegOf makes a JPEG from a stored WebP for an ask that names .jpg (older
// app builds); it takes a picture-work slot and is never stored.
func jpegOf(ctx context.Context, path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	release, err := imgstore.Slot(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	img, err := webp.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return imgstore.EncodeJPEG(img)
}

// slimKindOf is the slim copy kind behind a picture kind ("" when the
// picture is not keyed by a MusicBrainz id).
func slimKindOf(kind string) string {
	switch kind {
	case "album":
		return "release-group"
	case "artist", "wide", "logo":
		return "artist"
	}
	return ""
}

// anonSlimAbsent reports a request without a credential for an id the
// loaded slim copy does not hold.
func (s *Server) anonSlimAbsent(r *http.Request, kind, mbid string) bool {
	return kind != "" && Anonymous(r.Context()) && s.SlimAbsent(r.Context(), kind, mbid)
}
