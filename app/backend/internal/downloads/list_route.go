package downloads

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// ListDownloads serves GET /api/v1/downloads: the newest Downloads (?limit=).
// A removed download is left out, as on the Downloads screen; History keeps
// its Events and its row.
func ListDownloads(store *database.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ds, err := database.Select[domain.Download](r.Context(), store.Read(), "downloads", "state <> 'cleaned' ORDER BY id DESC LIMIT ?", limit(r, 50))
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		if ds == nil {
			ds = []domain.Download{}
		}
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"downloads": ds})
	}
}
