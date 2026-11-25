package mediafiles

import (
	"context"
	"net/http"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// HoldingHandlers list the deleted files still in the holding folders and
// put them back. The library service comes in as functions.
type HoldingHandlers struct {
	// HeldFiles lists the files waiting in the holding folders (titleID
	// 0: of every title); HeldCount only counts them, from the database.
	HeldFiles func(ctx context.Context, titleID int64) (any, error)
	HeldCount func(ctx context.Context, titleID int64) (int, error)
	// RestoreHeld moves files back; it returns the per-file results and
	// the ids of deleted titles that came back.
	// how is "", "swap" or "both" for a place that is taken.
	RestoreHeld func(ctx context.Context, ids []int64, how string, by database.Actor) (any, []int64, error)
	// QueueRescan queues a Library rescan and returns its job id.
	QueueRescan func(ctx context.Context, by database.Actor) (int64, error)
	Actor       func(r *http.Request) database.Actor
}

// Routes registers the holding routes.
func (h *HoldingHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/holding", h.listHolding)
	m.HandleFunc("POST /api/v1/holding/restore", h.restoreHolding)
}

func (h *HoldingHandlers) listHolding(w http.ResponseWriter, r *http.Request) {
	titleID, _ := strconv.ParseInt(r.URL.Query().Get("title_id"), 10, 64)
	if r.URL.Query().Get("count") == "1" {
		n, err := h.HeldCount(r.Context(), titleID)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"count": n})
		return
	}
	files, err := h.HeldFiles(r.Context(), titleID)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"files": files})
}

// restoreHolding moves held files back; a deleted title comes back with
// its file and a rescan is queued so the Library sees it again.
func (h *HoldingHandlers) restoreHolding(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int64 `json:"ids"`
		How string  `json:"how"`
	}
	if !webserver.Decode(w, r, &body, `send {"ids": [<file id>, ...], "how": "" | "swap" | "both"}`) {
		return
	}
	if len(body.IDs) == 0 || len(body.IDs) > 5000 {
		webserver.Fail(w, http.StatusBadRequest, "send 1 to 5000 ids")
		return
	}
	res, back, err := h.RestoreHeld(r.Context(), body.IDs, body.How, h.Actor(r))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	out := map[string]any{"results": res, "titles_back": back}
	if len(back) > 0 {
		if id, err := h.QueueRescan(r.Context(), h.Actor(r)); err == nil {
			out["rescan_job_id"] = id
		}
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}
