package importer

import (
	"context"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func (im *Importer) upgradeFacts(ctx context.Context, data map[string]any, oldID, newID int64) {
	for prefix, id := range map[string]int64{"old_": oldID, "new_": newID} {
		var f mediafiles.File
		if database.Get(ctx, im.Store.Read(), "files", id, &f) != nil {
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

func qualityText(q mediafiles.Quality) string {
	var parts []string
	if q.Resolution > 0 {
		parts = append(parts, strconv.Itoa(q.Resolution)+"p")
	}
	if q.Source != "" {
		parts = append(parts, strings.ToUpper(strings.ReplaceAll(q.Source, "_", "-")))
	}
	for _, h := range q.HDR {
		parts = append(parts, strings.ToUpper(h))
	}
	return strings.Join(parts, " ")
}
