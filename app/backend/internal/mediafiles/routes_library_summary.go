package mediafiles

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
)

type libraryMissingView struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Year       *int   `json:"year"`
	StateLabel string `json:"state_label"`
	// Image is the title's first poster, so Home's Missing row shows real covers.
	Image string `json:"image,omitempty"`
}

// recentMax is how many newest titles the summary carries.
const recentMax = 20

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
	if kid, kidArgs := kidSQL(a.kidMax(r)); kid != "" {
		// A Kid counts and sees only the titles their limit allows.
		where += " AND " + kid
		args = append(args, kidArgs...)
	}
	ctx, db := r.Context(), a.s.Store.Read()
	var total int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM titles t WHERE "+where, args...).Scan(&total); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	rows, err := db.QueryContext(ctx, `SELECT t.id, t.name, t.year, t.artwork
		FROM titles t WHERE `+where+` AND t.kind = 'movie' AND t.id IN
		(SELECT i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.state IN ('wanted', 'searching'))
		ORDER BY t.sort_name, t.id LIMIT 8`, args...)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	defer rows.Close()
	missing := []libraryMissingView{}
	for rows.Next() {
		var title libraryMissingView
		var art sql.NullString
		if err := rows.Scan(&title.ID, &title.Name, &title.Year, &art); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		var posters []string
		if art.Valid && json.Unmarshal([]byte(art.String), &posters) == nil {
			if links := artworkLinks(posters); len(links) > 0 {
				title.Image = links[0]
			}
		}
		missing = append(missing, title)
	}
	if err := rows.Err(); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if err := rows.Close(); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	for i := range missing {
		var state mediafiles.SlotState
		if err := db.QueryRowContext(ctx, `SELECT s.state FROM slots s JOIN items i ON i.id = s.item_id
			WHERE i.title_id = ? ORDER BY s.id LIMIT 1`, missing[i].ID).Scan(&state); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		missing[i].StateLabel = StateLabels[state]
	}
	// Recent: the newest titles by date added, for Home's Recently added.
	recentWhere := strings.ReplaceAll(where, "t.", "") + " ORDER BY created_at DESC, id DESC LIMIT ?"
	newest, err := database.Select[mediafiles.Title](ctx, db, "titles", recentWhere, append(args, recentMax)...)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	recent := []TitleView{}
	for _, t := range newest {
		v, err := a.titleView(ctx, t)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		recent = append(recent, v)
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"total": total, "missing": missing, "recent": recent, "card_art": a.libraryCardArt(r, newest)})
}
