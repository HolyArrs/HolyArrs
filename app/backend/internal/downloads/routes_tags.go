package downloads

import (
	"context"
	"database/sql"
	"net/http"
	"sort"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// TagUse is one tag in use and how many titles, Downloaders and Sources have it.
type TagUse struct {
	Label       string `json:"label"`
	Titles      int    `json:"titles"`
	Downloaders int    `json:"downloaders"`
	Sources     int    `json:"sources"`
}

// TagHandlers serves the tag list.
type TagHandlers struct {
	DB *sql.DB
}

// Routes registers GET /api/v1/tags.
func (h *TagHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/tags", h.listTags)
}

// listTags answers GET /api/v1/tags: every tag in use, for the tag
// fields' suggestions and to show where a tag is used.
func (h *TagHandlers) listTags(w http.ResponseWriter, r *http.Request) {
	uses := map[string]*TagUse{}
	for _, q := range []struct {
		read  func(context.Context, *sql.DB) (map[string]int, error)
		count func(*TagUse, int)
	}{
		{mediafiles.TitleTagUses, func(u *TagUse, n int) { u.Titles += n }},
		{DownloaderTagUses, func(u *TagUse, n int) { u.Downloaders += n }},
		{sources.TagUses, func(u *TagUse, n int) { u.Sources += n }},
	} {
		got, err := q.read(r.Context(), h.DB)
		if err != nil {
			webserver.Internal(w, "downloads", err)
			return
		}
		for label, n := range got {
			if uses[label] == nil {
				uses[label] = &TagUse{Label: label}
			}
			q.count(uses[label], n)
		}
	}
	out := make([]TagUse, 0, len(uses))
	for _, u := range uses {
		out = append(out, *u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"tags": out})
}
