package api

import (
	"net/http"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/parts"
)

type libraryMissingView struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Year       *int   `json:"year"`
	StateLabel string `json:"state_label"`
}

// librarySummary reads only the count and eight rows displayed on Home.
func (a *API) librarySummary(w http.ResponseWriter, r *http.Request) {
	var kinds []string
	var args []any
	for _, kind := range []string{"movie", "show", "artist"} {
		part, _ := parts.ForTitleKind(kind)
		if a.partIsOn(part) || (a.s.Parts != nil && a.s.Parts.SafeMode) {
			kinds = append(kinds, "?")
			args = append(args, kind)
		}
	}
	where := "t.state != 'removed' AND t.kind IN (" + strings.Join(kinds, ",") + ")"
	if len(kinds) == 0 {
		where = "0"
	}
	ctx, db := r.Context(), a.s.Store.Read()
	var total int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM titles t WHERE "+where, args...).Scan(&total); err != nil {
		internal(w, err)
		return
	}
	rows, err := db.QueryContext(ctx, `SELECT t.id, t.name, t.year
		FROM titles t WHERE `+where+` AND t.kind = 'movie' AND t.id IN
		(SELECT i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.state IN ('wanted', 'searching'))
		ORDER BY t.sort_name, t.id LIMIT 8`, args...)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	missing := []libraryMissingView{}
	for rows.Next() {
		var title libraryMissingView
		if err := rows.Scan(&title.ID, &title.Name, &title.Year); err != nil {
			internal(w, err)
			return
		}
		missing = append(missing, title)
	}
	if err := rows.Err(); err != nil {
		internal(w, err)
		return
	}
	if err := rows.Close(); err != nil {
		internal(w, err)
		return
	}
	for i := range missing {
		var state domain.SlotState
		if err := db.QueryRowContext(ctx, `SELECT s.state FROM slots s JOIN items i ON i.id = s.item_id
			WHERE i.title_id = ? ORDER BY s.id LIMIT 1`, missing[i].ID).Scan(&state); err != nil {
			internal(w, err)
			return
		}
		missing[i].StateLabel = StateLabels[state]
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": total, "missing": missing})
}
