package api

import (
	"github.com/HolyArrs/holyarrs/app/backend/internal/feature"
	"context"
	"database/sql"
	"net/http"
	"slices"
	"time"
	_ "time/tzdata" // viewers' time zones, on hosts without a zone database

	"github.com/HolyArrs/holyarrs/app/backend/internal/explore"
)

// exploreMoment is the viewer's local time: the tz query value or the
// X-Timezone header (an IANA zone such as Europe/Paris), else the
// server's own zone.
func exploreMoment(r *http.Request, now time.Time) (explore.Moment, string) {
	tz := r.URL.Query().Get("tz")
	if tz == "" {
		tz = r.Header.Get("X-Timezone")
	}
	if tz != "" && len(tz) <= 64 {
		if loc, err := time.LoadLocation(tz); err == nil {
			return explore.Moment{Local: now.In(loc)}, loc.String()
		}
	}
	return explore.Moment{Local: now.Local()}, ""
}

// titleExplore gives the page's rows their titles from the member's
// title memory, and remembers the templates it showed today.
func (a *API) titleExplore(ctx context.Context, member int64, pg explore.Page, m explore.Moment) (explore.Page, error) {
	day := m.Local.Format("2006-01-02")
	from := m.Local.AddDate(0, 0, -(explore.NoRepeatDays - 1)).Format("2006-01-02")
	c := explore.TitleContext{Member: member, Kid: pg.Kid, Moment: m, Day: day, Taste: pg.Taste,
		Recent: map[string]string{}, Today: map[string]string{}}
	rows, err := a.s.Store.Read().QueryContext(ctx, "SELECT row_id, template_id, day FROM explore_titles WHERE member_id = ? AND day >= ?", member, from)
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
	out, shown := explore.TitlePage(pg, c)
	var fresh []explore.Shown
	for _, s := range shown {
		if c.Today[s.RowID] != s.TemplateID {
			fresh = append(fresh, s)
		}
	}
	if len(fresh) > 0 {
		err = a.s.Store.Write(ctx, func(tx *sql.Tx) error {
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

// exploreTitleCount adds to the per-layer measuring counts: smart-titled
// rows against plain ones. Fixed head rows are left out.
func (a *API) exploreTitleCount(ctx context.Context, member int64, field string, layers []string) {
	var keep []string
	for _, l := range layers {
		if validTitleLayer(l) {
			keep = append(keep, l)
		}
	}
	if len(keep) == 0 || !feature.On(feature.RowTitleStats) {
		return
	}
	day := explore.Day(a.exploreNow())
	_ = a.s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, l := range keep {
			if _, err := tx.Exec(`INSERT INTO explore_title_counts (member_id, layer, day, `+field+`) VALUES (?, ?, ?, 1)
				ON CONFLICT (member_id, layer, day) DO UPDATE SET `+field+` = `+field+` + 1`, member, l, day); err != nil {
				return err
			}
		}
		return nil
	})
}

func validTitleLayer(l string) bool {
	return l == explore.LayerPlain || slices.Contains(explore.Layers, l)
}

func rowLayers(rows []exploreRowView) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.TitleLayer)
	}
	return out
}

// exploreTitleStats answers GET /api/v1/explore/title-stats (owner only):
// for the last 30 days, rows shown, cards opened and asks started, by
// title layer, so smart titles can be compared with plain ones. Counts
// stay on this server and never change what Explore shows.
func (a *API) exploreTitleStats(w http.ResponseWriter, r *http.Request) {
	from := explore.Day(a.exploreNow().AddDate(0, 0, -30))
	rows, err := a.s.Store.Read().QueryContext(r.Context(), `SELECT layer, SUM(shown), SUM(opens), SUM(asks) FROM explore_title_counts
		WHERE day >= ? GROUP BY layer ORDER BY layer`, from)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	type stat struct {
		Layer string `json:"layer"`
		Shown int    `json:"shown"`
		Opens int    `json:"opens"`
		Asks  int    `json:"asks"`
	}
	out := []stat{}
	for rows.Next() {
		var s stat
		if err := rows.Scan(&s.Layer, &s.Shown, &s.Opens, &s.Asks); err != nil {
			internal(w, err)
			return
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": 30, "templates": len(explore.Templates), "layers": out})
}
