package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

const (
	// maxBatchIDs caps one row's ask.
	maxBatchIDs = 100
	// maxBatchQueued is how many missing pictures one ask may queue; the
	// rest come back missing and are fetched when their own cards ask.
	maxBatchQueued = 20
)

// serveMusicPictures answers GET /v1/music/pictures?artists=a,b&albums=c,d:
// one request for a whole row's picture records (version, size and main
// colour of each artist's photo, wide background and logo, each album's
// cover), so an install draws every card's box and colour at once and asks
// each picture by its versioned address. Missing pictures are queued at
// prefetch priority (P2) under the asking install's fair share, at most
// maxBatchQueued per ask, and only for an install (an ask without one gets
// the stored records alone); such an answer is kept a minute only.
func (s *Server) serveMusicPictures(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ids := map[string][]string{}
	seen := map[string]bool{}
	n := 0
	for k, v := range q {
		if (k != "artists" && k != "albums") || len(v) != 1 {
			problem(w, 400, "invalid_query", 0)
			return
		}
		for _, id := range strings.Split(v[0], ",") {
			id = strings.ToLower(strings.TrimSpace(id))
			if !mbidPattern.MatchString(id) {
				problem(w, 400, "invalid_query", 0)
				return
			}
			if n++; seen[k+id] {
				continue
			}
			seen[k+id] = true
			ids[k] = append(ids[k], id)
		}
	}
	if n == 0 || n > maxBatchIDs {
		problem(w, 400, "invalid_query", 0)
		return
	}
	if s.opts.Pictures == nil {
		problem(w, 404, "not_found", 0)
		return
	}
	var queue func(kind, mbid string)
	who, _ := r.Context().Value(callerKey{}).(caller)
	if im := s.opts.Images; im != nil && im.WantRow != nil && who.who != "" && who.who != "anon" {
		ictx := fetchq.WithInstall(context.WithoutCancel(r.Context()), who.who)
		queue = func(kind, mbid string) { im.WantRow(ictx, kind, mbid) }
	}
	out := map[string]map[string]map[string]imgstore.Info{"artists": {}, "albums": {}}
	partial, queued := false, 0
	for _, group := range []string{"artists", "albums"} {
		kind, main, slot := "artist", imgstore.KindArtist, "photo"
		if group == "albums" {
			kind, main, slot = "release-group", imgstore.KindAlbum, "cover"
		}
		for _, id := range ids[group] {
			p := s.opts.Pictures(r.Context(), kind, id)
			out[group][id] = p
			if _, ok := p[slot]; ok {
				continue
			}
			partial = true
			if queue != nil && queued < maxBatchQueued {
				queue(main, id)
				queued++
			}
		}
	}
	if partial {
		w.Header().Set(partialHeader, "true")
	}
	writeJSON(w, 200, out)
}
