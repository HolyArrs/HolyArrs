package mediaservers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// watchLinks answers where a Library title opens on the media servers:
// {"links": [{server, kind, url}]}. It names servers and their addresses,
// so it needs manage_library (see accounts.RoutePerms).
func (h *Handlers) watchLinks(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("title"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusBadRequest, "send title, a Library title id")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	links, err := h.Linker.WatchLinks(ctx, id)
	if errors.Is(err, database.ErrNotFound) {
		webserver.Fail(w, http.StatusNotFound, "no such title")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"links": links})
}
