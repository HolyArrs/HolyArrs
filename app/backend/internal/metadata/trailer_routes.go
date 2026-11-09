package metadata

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// trailerBatchMax is how many titles one batch may name.
const trailerBatchMax = 40

// trailerBatchView is the answer of GET /trailer/batch: the streams the
// trailer pass kept (memory or SQLite), by "kind:tmdb_id", and the artists'
// ready music videos. Nothing is ever pending: everything is looked up on
// the pass's schedule only (Pending stays for older pages, always empty).
type trailerBatchView struct {
	Streams map[string]TrailerStream `json:"streams"`
	// Videos are artists' music videos ("artist:mbid") already looked up,
	// as the artist page's ?part=video answers them.
	Videos  map[string]any `json:"videos"`
	Pending []string       `json:"pending"`
}

// Routes registers the trailer routes.
func (t *Trailers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/trailer/batch", t.trailerBatch)
	m.HandleFunc("GET /api/v1/trailer/{kind}/{tmdb_id}", t.trailerStream)
	m.HandleFunc("GET /api/v1/trailer/stream/{id}/manifest.m3u8", t.trailerManifest)
	m.HandleFunc("GET /api/v1/trailer/stream/{id}/seg/{n}", t.trailerSegment)
}

func (t *Trailers) trailerStream(w http.ResponseWriter, r *http.Request) {
	kind, id := r.PathValue("kind"), r.PathValue("tmdb_id")
	if kind != "movie" && kind != "show" {
		webserver.Fail(w, http.StatusBadRequest, "kind must be movie or show")
		return
	}
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		webserver.Fail(w, http.StatusBadRequest, "tmdb_id must be a TMDB number")
		return
	}
	if len(t.KindsOn(kind)) == 0 {
		webserver.Fail(w, http.StatusConflict, "that part is off")
		return
	}
	// Kid accounts never autoplay trailers.
	if t.Ages.Max(r) != "" {
		webserver.Fail(w, http.StatusForbidden, "trailers do not play for Kid accounts")
		return
	}
	if !t.Settings.AutoplayTrailers(r.Context()) {
		webserver.Fail(w, http.StatusNotFound, "trailers are turned off")
		return
	}
	// Only what the trailer pass kept (memory, then SQLite): a page open
	// never starts a lookup.
	lang := TrailerLanguage(r)
	e, ok := t.Kept(r.Context(), kind+"|"+id+"|"+lang, time.Now())
	w.Header().Set("Cache-Control", "private, no-store")
	switch {
	case !ok:
		// Not looked up yet: noted for the next trailer pass, and the key
		// learned in the background (no YouTube call); the kept key is for
		// YouTube's player meanwhile.
		t.Note(lang, [][2]string{{kind, id}})
		t.Wants.Warm(lang, [][2]string{{kind, id}})
		webserver.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "no direct stream", "key": t.KnownKey(r.Context(), kind, id, lang)})
	case e.None:
		webserver.Fail(w, http.StatusNotFound, "no trailer")
	case !e.Miss.IsZero():
		// A miss says why (the reader's reason) and which video the YouTube
		// player may still play ("" when every one tried will not play).
		body := map[string]string{"error": "no direct stream"}
		if e.Reason != "" {
			body["reason"], body["key"] = string(e.Reason), e.Key
		}
		webserver.WriteJSON(w, http.StatusNotFound, body)
	default:
		s := e.Stream
		if s.Type == "hls" {
			s.ProxyURL = t.RelayURL(r, s)
		}
		webserver.WriteJSON(w, http.StatusOK, s)
	}
}

// trailerBatch answers the cards on screen at once with the streams the
// trailer pass kept; a title without one is left out (the card plays the
// embed) and no lookup starts: a title nothing was kept for is noted, so
// the next pass looks it up (rows outside the stored Explore pools, such as
// a genre's rows or New on your services, get covered this way). Artists
// are noted the same way, so the pass makes their music videos ready.
func (t *Trailers) trailerBatch(w http.ResponseWriter, r *http.Request) {
	if t.Ages.Max(r) != "" {
		webserver.Fail(w, http.StatusForbidden, "trailers do not play for Kid accounts")
		return
	}
	if !t.Settings.AutoplayTrailers(r.Context()) {
		webserver.Fail(w, http.StatusNotFound, "trailers are turned off")
		return
	}
	var ids []string
	for _, raw := range strings.Split(r.URL.Query().Get("ids"), ",") {
		if raw = strings.TrimSpace(raw); raw != "" {
			ids = append(ids, raw)
		}
	}
	if len(ids) == 0 || len(ids) > trailerBatchMax {
		webserver.Fail(w, http.StatusBadRequest, "ids must name 1 to "+strconv.Itoa(trailerBatchMax)+" titles as kind:tmdb_id")
		return
	}
	lang := TrailerLanguage(r)
	now := time.Now()
	out := trailerBatchView{Streams: map[string]TrailerStream{}, Videos: map[string]any{}, Pending: []string{}}
	var artists []string
	var unknown [][2]string
	seen := map[string]bool{}
	for _, id := range ids {
		kind, tmdb, ok := strings.Cut(id, ":")
		if kind == "artist" {
			mbid := strings.ToLower(tmdb)
			if !ValidMBID(mbid) {
				webserver.Fail(w, http.StatusBadRequest, "ids must name 1 to "+strconv.Itoa(trailerBatchMax)+" titles as kind:tmdb_id or artist:mbid")
				return
			}
			if seen[id] || !t.Videos.On() {
				continue
			}
			seen[id] = true
			// Noted for the trailer pass, which makes one of the artist's
			// videos ready and keeps it ready; none is looked up here.
			artists = append(artists, mbid)
			if view, ok := t.Videos.Ready(r, mbid); ok {
				out.Videos[id] = view
			}
			continue
		}
		n, err := strconv.ParseInt(tmdb, 10, 64)
		if !ok || (kind != "movie" && kind != "show") || err != nil || n <= 0 {
			webserver.Fail(w, http.StatusBadRequest, "ids must name 1 to "+strconv.Itoa(trailerBatchMax)+" titles as kind:tmdb_id")
			return
		}
		if seen[id] || len(t.KindsOn(kind)) == 0 {
			continue
		}
		seen[id] = true
		e, known := t.Kept(r.Context(), kind+"|"+tmdb+"|"+lang, now)
		if !known {
			unknown = append(unknown, [2]string{kind, tmdb})
		}
		if known && e.Miss.IsZero() {
			s := e.Stream
			if s.Type == "hls" {
				s.ProxyURL = t.RelayURL(r, s)
			}
			out.Streams[id] = s
		}
	}
	t.Note(lang, unknown)
	t.Videos.Note(r.Context(), artists)
	w.Header().Set("Cache-Control", "private, no-store")
	webserver.WriteJSON(w, http.StatusOK, out)
}
