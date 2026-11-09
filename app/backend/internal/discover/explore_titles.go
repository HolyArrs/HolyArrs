package discover

import (
	"context"
	"database/sql"
	"net/http"
	"slices"
	"time"
	_ "time/tzdata" // viewers' time zones, on hosts without a zone database

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// ExploreMoment is the viewer's local time: the tz query value or the
// X-Timezone header (an IANA zone such as Europe/Paris), else the
// server's own zone.
func ExploreMoment(r *http.Request, now time.Time) (engine.Moment, string) {
	tz := r.URL.Query().Get("tz")
	if tz == "" {
		tz = r.Header.Get("X-Timezone")
	}
	if tz != "" && len(tz) <= 64 {
		if loc, err := time.LoadLocation(tz); err == nil {
			return engine.Moment{Local: now.In(loc)}, loc.String()
		}
	}
	return engine.Moment{Local: now.Local()}, ""
}

// TitleExplore gives the page's rows their titles from the member's
// title memory, and remembers the templates it showed today.
func TitleExplore(ctx context.Context, store *database.Store, member int64, pg engine.Page, m engine.Moment) (engine.Page, error) {
	day := m.Local.Format("2006-01-02")
	from := m.Local.AddDate(0, 0, -(engine.NoRepeatDays - 1)).Format("2006-01-02")
	c := engine.TitleContext{Member: member, Kid: pg.Kid, Moment: m, Day: day, Taste: pg.Taste,
		Recent: map[string]string{}, Today: map[string]string{}}
	rows, err := store.Read().QueryContext(ctx, "SELECT row_id, template_id, day FROM explore_titles WHERE member_id = ? AND day >= ?", member, from)
	if err != nil {
		return pg, err
	}
	for rows.Next() {
		var row, id, d string
		if err := rows.Scan(&row, &id, &d); err != nil {
			rows.Close()
			return pg, err
		}
		if d > c.Recent[id] {
			c.Recent[id] = d
		}
		if d == day {
			c.Today[row] = id
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return pg, err
	}
	out, shown := engine.TitlePage(pg, c)
	var fresh []engine.Shown
	for _, s := range shown {
		if c.Today[s.RowID] != s.TemplateID {
			fresh = append(fresh, s)
		}
	}
	if len(fresh) > 0 {
		err = store.Write(ctx, func(tx *sql.Tx) error {
			for _, s := range fresh {
				if _, err := tx.Exec(`INSERT INTO explore_titles (member_id, day, row_id, template_id) VALUES (?, ?, ?, ?)
					ON CONFLICT (member_id, day, row_id) DO UPDATE SET template_id = excluded.template_id`, member, day, s.RowID, s.TemplateID); err != nil {
					return err
				}
			}
			return nil
		})
	}
	return out, err
}

// ExploreTitleCount adds to the per-layer measuring counts: smart-titled
// rows against plain ones. Fixed head rows are left out.
func ExploreTitleCount(ctx context.Context, store *database.Store, now time.Time, member int64, field string, layers []string) {
	var keep []string
	for _, l := range layers {
		if ValidTitleLayer(l) {
			keep = append(keep, l)
		}
	}
	if len(keep) == 0 || !startup.FeatureOn(startup.RowTitleStats) {
		return
	}
	day := engine.Day(now)
	_ = store.Write(ctx, func(tx *sql.Tx) error {
		return accounts.AddLayerCounts(ctx, tx, member, day, field, keep)
	})
}

// ValidTitleLayer reports whether l is a layer the title counts keep.
func ValidTitleLayer(l string) bool {
	return l == engine.LayerPlain || slices.Contains(engine.Layers, l)
}

// ExploreTitleStats answers GET /api/v1/explore/title-stats (owner only):
// for the last 30 days, rows shown, cards opened and asks started, by
// title layer, so smart titles can be compared with plain ones. Counts
// stay on this server and never change what Explore shows.
func ExploreTitleStats(w http.ResponseWriter, r *http.Request, store *database.Store, now time.Time) {
	from := engine.Day(now.AddDate(0, 0, -30))
	got, err := accounts.LayerTotals(r.Context(), store.Read(), from)
	if err != nil {
		webserver.Internal(w, "discover", err)
		return
	}
	type stat struct {
		Layer string `json:"layer"`
		Shown int    `json:"shown"`
		Opens int    `json:"opens"`
		Asks  int    `json:"asks"`
	}
	out := []stat{}
	for _, t := range got {
		out = append(out, stat{Layer: t.Layer, Shown: t.Shown, Opens: t.Opens, Asks: t.Asks})
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"days": 30, "templates": len(engine.Templates), "layers": out})
}
