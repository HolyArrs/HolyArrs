package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/urlbase"
)

// trailerBatchMax is how many titles one batch may name.
const trailerBatchMax = 40

// trailerBatchView is the answer of GET /trailer/batch: the streams already
// known (memory or SQLite), by "kind:tmdb_id", and the titles still being
// looked up ahead of time (ask again, or ask the single route on hover).
type trailerBatchView struct {
	Streams map[string]TrailerStream `json:"streams"`
	Pending []string                 `json:"pending"`
}

// trailerBatch answers the cards on screen at once: known streams come back
// straight away; the rest are looked up ahead of time, one after another
// within the install's ahead-of-time budget (prefetchTrailers), so they are
// ready by the time someone hovers. It never spends a person's own resolves.
func (a *API) trailerBatch(w http.ResponseWriter, r *http.Request) {
	if a.kidMax(r) != "" {
		fail(w, http.StatusForbidden, "trailers do not play for Kid accounts")
		return
	}
	if !a.uiSettings(r.Context()).AutoplayTrailers {
		fail(w, http.StatusNotFound, "trailers are turned off")
		return
	}
	var ids []string
	for _, raw := range strings.Split(r.URL.Query().Get("ids"), ",") {
		if raw = strings.TrimSpace(raw); raw != "" {
			ids = append(ids, raw)
		}
	}
	if len(ids) == 0 || len(ids) > trailerBatchMax {
		fail(w, http.StatusBadRequest, "ids must name 1 to "+strconv.Itoa(trailerBatchMax)+" titles as kind:tmdb_id")
		return
	}
	lang := trailerLanguage(r)
	now := time.Now()
	out := trailerBatchView{Streams: map[string]TrailerStream{}, Pending: []string{}}
	var ahead [][2]string
	seen := map[string]bool{}
	for _, id := range ids {
		kind, tmdb, ok := strings.Cut(id, ":")
		n, err := strconv.ParseInt(tmdb, 10, 64)
		if !ok || (kind != "movie" && kind != "show") || err != nil || n <= 0 {
			fail(w, http.StatusBadRequest, "ids must name 1 to "+strconv.Itoa(trailerBatchMax)+" titles as kind:tmdb_id")
			return
		}
		if seen[id] || len(a.kindsOn(kind)) == 0 {
			continue
		}
		seen[id] = true
		ckey := kind + "|" + tmdb + "|" + lang
		e, known := a.trailers.get(ckey, now)
		if !known && a.trailerStored(r.Context(), ckey, now) {
			e, known = a.trailers.get(ckey, now)
		}
		switch {
		case known && e.miss.IsZero():
			s := e.s
			if s.Type == "hls" {
				if rid := a.trailerRelay.register(a.person(r).UserID, s.URL, s.ExpiresAt); rid != "" {
					s.ProxyURL = urlbase.From(r.Context()) + "/api/v1/trailer/stream/" + rid + "/manifest.m3u8"
				}
			}
			out.Streams[id] = s
		case known:
			// No direct stream for this title (for now): nothing to wait for.
		default:
			out.Pending = append(out.Pending, id)
			ahead = append(ahead, [2]string{kind, tmdb})
		}
	}
	a.prefetchTrailers(r, ahead)
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, out)
}
