package api

import (
	"net/http"
	"sort"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// TagUse is one tag in use and how many titles, Downloaders and Sources have it.
type TagUse struct {
	Label       string `json:"label"`
	Titles      int    `json:"titles"`
	Downloaders int    `json:"downloaders"`
	Sources     int    `json:"sources"`
}

// listTags answers GET /api/v1/tags: every tag in use (#108), for the tag
// fields' suggestions and to show where a tag is used.
func (a *API) listTags(w http.ResponseWriter, r *http.Request) {
	uses := map[string]*TagUse{}
	for _, q := range []struct {
		table string
		count func(*TagUse)
	}{
		{"titles", func(u *TagUse) { u.Titles++ }},
		{"downloaders", func(u *TagUse) { u.Downloaders++ }},
	} {
		// A row saved without tags may hold JSON null rather than a list.
		where := " WHERE json_type(x.tags) = 'array'"
		if q.table == "titles" {
			where += " AND x.state != 'removed'"
		}
		if q.table == "downloaders" {
			where += " AND x.removed = 0"
		}
		rows, err := a.s.Store.Read().QueryContext(r.Context(), "SELECT j.value FROM "+q.table+" x, json_each(x.tags) j"+where)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		for rows.Next() {
			var label string
			if err := rows.Scan(&label); err != nil {
				_ = rows.Close()
				webserver.Internal(w, "api", err)
				return
			}
			if uses[label] == nil {
				uses[label] = &TagUse{Label: label}
			}
			q.count(uses[label])
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
	}
	onSources, err := sources.TagUses(r.Context(), a.s.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	for label, n := range onSources {
		if uses[label] == nil {
			uses[label] = &TagUse{Label: label}
		}
		uses[label].Sources += n
	}
	out := make([]TagUse, 0, len(uses))
	for _, u := range uses {
		out = append(out, *u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"tags": out})
}
