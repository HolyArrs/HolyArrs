package mediaservers

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func requirePlaybackOwner(w http.ResponseWriter, r *http.Request) bool {
	if c, ok := webserver.From(r.Context()); ok && c.Role == webserver.RoleOwner && c.Via == "session" {
		return true
	}
	webserver.Fail(w, http.StatusForbidden, "sign in as the owner to manage Plex playback")
	return false
}

func (h *Handlers) playbackServer(w http.ResponseWriter, r *http.Request) (domain.MediaServer, bool) {
	if !requirePlaybackOwner(w, r) {
		return domain.MediaServer{}, false
	}
	id, ok := idParam(w, r)
	if !ok {
		return domain.MediaServer{}, false
	}
	m, err := getServer(r.Context(), h.Store.Read(), id)
	if err != nil {
		writeError(w, err)
		return m, false
	}
	if m.Kind != "plex" {
		webserver.Fail(w, http.StatusNotFound, "no such media server")
		return m, false
	}
	return m, true
}

// playback returns the owner view of a Plex connection, with the latest
// active streams and a page of stopped plays matching the history filters.
func (h *Handlers) playback(w http.ResponseWriter, r *http.Request) {
	m, ok := h.playbackServer(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	offset, err := playbackOffset(query.Get("offset"))
	if err != nil {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	filter := playbackFilter{account: playbackText(query.Get("account"), 160), title: playbackText(query.Get("title"), 500), offset: offset}
	if from := query.Get("from"); from != "" {
		day, err := time.Parse(time.DateOnly, from)
		if err != nil {
			webserver.Fail(w, http.StatusBadRequest, "invalid playback history date range")
			return
		}
		filter.from = database.FormatTime(day)
	}
	if through := query.Get("through"); through != "" {
		day, err := time.Parse(time.DateOnly, through)
		if err != nil {
			webserver.Fail(w, http.StatusBadRequest, "invalid playback history date range")
			return
		}
		filter.until = database.FormatTime(day.AddDate(0, 0, 1))
		if strings.Compare(filter.from, filter.until) >= 0 {
			webserver.Fail(w, http.StatusBadRequest, "invalid playback history date range")
			return
		}
	}
	out, err := readPlayback(r.Context(), h.Store.Read(), m, filter, time.Now())
	if err != nil {
		internalError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

// clearPlayback removes recorded plays for the connection. An active
// stream can appear again on the next snapshot if monitoring is still on.
func (h *Handlers) clearPlayback(w http.ResponseWriter, r *http.Request) {
	m, ok := h.playbackServer(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if err := h.Store.Write(ctx, func(tx *sql.Tx) error {
		return resetPlayback(ctx, tx, m.ID, true)
	}); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
