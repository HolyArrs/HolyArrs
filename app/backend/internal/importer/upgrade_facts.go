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
		if f.Size > 0 {
			data[prefix+"size"] = f.Size
		}
	}
}

// markUpgrade notes on an import event that it replaced files: upgraded,
// and with one replaced file its old and new quality, release and size.
func (im *Importer) markUpgrade(ctx context.Context, data map[string]any, pairs map[int64]int64) {
	if len(pairs) == 0 {
		return
	}
	data["upgraded"] = true
	if len(pairs) == 1 {
		for oldID, newID := range pairs {
			im.upgradeFacts(ctx, data, oldID, newID)
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

// markShowUpgrade marks a show import as an upgrade when every placed
// episode replaced a file.
func (im *Importer) markShowUpgrade(ctx context.Context, data map[string]any, newFiles map[int64]int64, placed int) {
	upgraded := map[int64]int64{}
	for oldID, newID := range newFiles {
		var f mediafiles.File
		if database.Get(ctx, im.Store.Read(), "files", oldID, &f) == nil && f.State == "replaced" {
			upgraded[oldID] = newID
		}
	}
	if len(upgraded) > 0 && len(upgraded) >= placed {
		im.markUpgrade(ctx, data, upgraded)
	}
}
