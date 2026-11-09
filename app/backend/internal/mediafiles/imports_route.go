package mediafiles

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// ListImports serves GET /api/v1/imports: the newest Imports (?limit=).
func ListImports(store *database.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		is, err := database.Select[Import](r.Context(), store.Read(), "imports", "1=1 ORDER BY id DESC LIMIT ?", webserver.QueryLimit(r, 20))
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		if is == nil {
			is = []Import{}
		}
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"imports": is})
	}
}
