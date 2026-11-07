package importer

import (
	"context"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

// upgradeFacts adds the old and new quality ("1080p WEB-DL") and release
// names to a file_replaced Event's data, for those the files kept.
func (im *Importer) upgradeFacts(ctx context.Context, data map[string]any, oldID, newID int64) {
	for prefix, id := range map[string]int64{"old_": oldID, "new_": newID} {
		var f domain.File
		if store.Get(ctx, im.Store.Read(), "files", id, &f) != nil {
			continue
		}
		if q := qualityText(f.Quality); q != "" {
			data[prefix+"quality"] = q
		}
		if f.ReleaseName != nil && *f.ReleaseName != "" {
			data[prefix+"release"] = *f.ReleaseName
		}
	}
}

// qualityText is a short quality label: resolution, source and HDR.
func qualityText(q domain.Quality) string {
	var parts []string
	if q.Resolution > 0 {
		parts = append(parts, strconv.Itoa(q.Resolution)+"p")
	}
	if q.Source != "" {
		parts = append(parts, strings.ToUpper(strings.ReplaceAll(q.Source, "_", "-"))) // web_dl: WEB-DL
	}
	for _, h := range q.HDR {
		parts = append(parts, strings.ToUpper(h))
	}
	return strings.Join(parts, " ")
}
