package mediafiles

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"net/url"
)

// LibraryMissingView is one Missing row title on Home.
type LibraryMissingView struct {
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
func (h *Handlers) librarySummary(w http.ResponseWriter, r *http.Request) {
	var kinds []string
	var args []any
	for _, kind := range []string{"movie", "show", "artist"} {
		if h.KindShown == nil || h.KindShown(kind) {
			kinds = append(kinds, "?")
			args = append(args, kind)
		}
	}
	where := "t.state != 'removed' AND t.kind IN (" + strings.Join(kinds, ",") + ")"
	if len(kinds) == 0 {
		where = "0"
	}
	if kid, kidArgs := KidSQL(h.KidMax(r)); kid != "" {
		// A Kid counts and sees only the titles their limit allows.
		where += " AND " + kid
		args = append(args, kidArgs...)
	}
	ctx, db := r.Context(), h.Store.Read()
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
	missing := []LibraryMissingView{}
	for rows.Next() {
		var title LibraryMissingView
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
		var state SlotState
		if err := db.QueryRowContext(ctx, `SELECT s.state FROM slots s JOIN items i ON i.id = s.item_id
			WHERE i.title_id = ? ORDER BY s.id LIMIT 1`, missing[i].ID).Scan(&state); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		missing[i].StateLabel = StateLabels[state]
	}
	// Recent: the newest titles by date added, for Home's Recently added.
	recentWhere := strings.ReplaceAll(where, "t.", "") + " ORDER BY created_at DESC, id DESC LIMIT ?"
	newest, err := database.Select[Title](ctx, db, "titles", recentWhere, append(args, recentMax)...)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	recent := []any{}
	for _, t := range newest {
		v, err := h.TitleView(ctx, t)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		recent = append(recent, v)
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"total": total, "missing": missing, "recent": recent, "card_art": h.CardArt(r, newest)})
}

// StateLabels are the words people see for Slot states.
var StateLabels = map[SlotState]string{
	SlotNotOutYet: "Not out yet", SlotWanted: "Missing", SlotSearching: "Searching",
	SlotHeld: "Held", SlotChosen: "Chosen", SlotDownloading: "Downloading",
	SlotStalled: "Stalled", SlotFailed: "Failed", SlotImporting: "Importing",
	SlotNeedsADecision: "Needs a decision", SlotRejected: "Rejected", SlotInLibrary: "In library",
	SlotBetterWanted: "Better copy sought", SlotRemoved: "Removed",
}

// artworkLinks turns stored artwork ("poster:<cache name>") into links the
// web app can load (/api/v1/artwork/<cache name>); anything else is kept.
func artworkLinks(in []string) []string {
	out := make([]string, 0, len(in))
	for _, a := range in {
		if name, ok := strings.CutPrefix(a, "poster:"); ok && name != "" {
			a = "/api/v1/artwork/" + url.PathEscape(name)
		}
		out = append(out, a)
	}
	return out
}
