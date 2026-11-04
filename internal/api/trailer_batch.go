package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/parts"
	"github.com/CoderLuii/holyarrs/internal/urlbase"
)

// trailerBatchMax is how many titles one batch may name.
const trailerBatchMax = 40

// trailerBatchView is the answer of GET /trailer/batch: the streams the
// trailer pass kept (memory or SQLite), by "kind:tmdb_id", and the artists
// whose videos are still being looked up ahead of time (ask again). A title
// is never pending: its trailer is looked up on the pass's schedule only.
type trailerBatchView struct {
	Streams map[string]TrailerStream `json:"streams"`
	// Videos are artists' music videos ("artist:mbid") already looked up,
	// as the artist page's ?part=video answers them.
	Videos  map[string]ArtistVideoView `json:"videos"`
	Pending []string                   `json:"pending"`
}

// trailerBatch answers the cards on screen at once with the streams the
// trailer pass kept; a title without one is left out (the card plays the
// embed) and no lookup starts: a title nothing was kept for is noted, so
// the next pass looks it up (rows outside the stored Explore pools, such as
// a genre's rows or New on your services, get covered this way). Artists'
// videos not yet known are looked up ahead of time (prefetchArtistVideos).
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
	out := trailerBatchView{Streams: map[string]TrailerStream{}, Videos: map[string]ArtistVideoView{}, Pending: []string{}}
	var artists []string
	var unknown [][2]string
	seen := map[string]bool{}
	for _, id := range ids {
		kind, tmdb, ok := strings.Cut(id, ":")
		if kind == "artist" {
			mbid := strings.ToLower(tmdb)
			if !metadata.ValidMBID(mbid) {
				fail(w, http.StatusBadRequest, "ids must name 1 to "+strconv.Itoa(trailerBatchMax)+" titles as kind:tmdb_id or artist:mbid")
				return
			}
			if seen[id] || !a.artistVideosOn() {
				continue
			}
			seen[id] = true
			switch e, kept := a.keptVideos(r.Context(), mbid); {
			case kept && len(e.list) == 0:
				// No videos (for now): nothing to wait for.
			default:
				if v, s, ready := a.readyVideo(r.Context(), e.list); ready {
					if view, ok := a.videoView(r, v, s); ok {
						out.Videos[id] = view
						continue
					}
				}
				out.Pending = append(out.Pending, id)
				artists = append(artists, mbid)
			}
			continue
		}
		n, err := strconv.ParseInt(tmdb, 10, 64)
		if !ok || (kind != "movie" && kind != "show") || err != nil || n <= 0 {
			fail(w, http.StatusBadRequest, "ids must name 1 to "+strconv.Itoa(trailerBatchMax)+" titles as kind:tmdb_id")
			return
		}
		if seen[id] || len(a.kindsOn(kind)) == 0 {
			continue
		}
		seen[id] = true
		e, known := a.keptTrailer(r.Context(), kind+"|"+tmdb+"|"+lang, now)
		if !known {
			unknown = append(unknown, [2]string{kind, tmdb})
		}
		if known && e.miss.IsZero() {
			s := e.s
			if s.Type == "hls" {
				if rid := a.trailerRelay.register(a.person(r).UserID, s.URL, s.ExpiresAt); rid != "" {
					s.ProxyURL = urlbase.From(r.Context()) + "/api/v1/trailer/stream/" + rid + "/manifest.m3u8"
				}
			}
			out.Streams[id] = s
		}
	}
	a.noteTrailerTitles(lang, unknown)
	a.prefetchArtistVideos(r, artists)
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, out)
}

// artistVideosOn says whether artist music videos can play here: music on,
// a music source and a metadata server that lists videos.
func (a *API) artistVideosOn() bool {
	p, _ := parts.ForTitleKind("artist")
	return a.partIsOn(p) && a.s.Library != nil && a.s.Library.Artists != nil && a.s.ArtistVideos != nil
}
