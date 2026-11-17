package arrapi

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// neverAddDefaultSize is the page size when none is sent, as in the old apps.
const neverAddDefaultSize = 10

// reNameYear finds a year written after a name, as in "Name (2001)".
var reNameYear = regexp.MustCompile(`\((\d{4})\)\s*$`)

// neverAddPage pages this side's part of the Never-add list as the old
// apps' exclusion list: shows with their TVDB id and title, movies with
// their TMDB id, title and year. Pulsarr reads every page before each sync
// and stops the sync when it cannot. The list is sorted by name unless the
// key is id. An empty list is one empty page.
func (h *handler) neverAddPage(w http.ResponseWriter, r *http.Request) {
	list, err := database.Select[mediafiles.NeverAdd](r.Context(), h.Store.Read(), "never_add", "kind = ? ORDER BY id", h.folderKind())
	if err != nil {
		internalError(w, err)
		return
	}
	key := query(r, "sortKey")
	if key == "" {
		key = "title"
		if h.side == "radarr" {
			key = "movieTitle"
		}
	}
	desc := strings.EqualFold(query(r, "sortDirection"), "descending")
	word := "ascending"
	if desc {
		word = "descending"
	}
	byID := strings.EqualFold(key, "id")
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if desc {
			a, b = b, a
		}
		if !byID {
			if x, y := strings.ToLower(a.Name), strings.ToLower(b.Name); x != y {
				return x < y
			}
		}
		return a.ID < b.ID
	})
	size := neverAddDefaultSize
	if n, err := strconv.Atoi(query(r, "pageSize")); err == nil && n > 0 {
		size = min(n, 10000)
	}
	page := 1
	if n, err := strconv.Atoi(query(r, "page")); err == nil && n > 0 {
		page = n
	}
	records := []map[string]any{}
	if start := (page - 1) * size; start < len(list) {
		for _, n := range list[start:min(start+size, len(list))] {
			if h.side == "sonarr" {
				records = append(records, map[string]any{"id": n.ID, "tvdbId": sourceID(n.SourceIDs, "tvdb"), "title": n.Name})
				continue
			}
			year := 0
			if m := reNameYear.FindStringSubmatch(n.Name); m != nil {
				year, _ = strconv.Atoi(m[1])
			}
			records = append(records, map[string]any{"id": n.ID, "tmdbId": sourceID(n.SourceIDs, "tmdb"), "movieTitle": n.Name, "movieYear": year})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"page": page, "pageSize": size, "sortKey": key, "sortDirection": word,
		"totalRecords": len(list), "records": records})
}
