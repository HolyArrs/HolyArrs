package health

import (
	"context"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// saveFolders warns about each Usenet Downloader whose finished jobs HolyArrs
// can't import: it gives no file list, and with no save folder
// (HOLYARRS_SAVE_PATH) and no path pair HolyArrs never reads its folders.
// Torrent Downloaders give their file list, so they get no line.
func (e *Engine) saveFolders(ctx context.Context) ([]Result, error) {
	ds, err := database.Select[domain.Downloader](ctx, e.Store.Read(), "downloaders", "removed = 0 AND protocol = 'usenet' ORDER BY id")
	if err != nil {
		return nil, err
	}
	saved := strings.TrimSpace(e.SavePath) != ""
	out := make([]Result, 0, len(ds))
	for _, d := range ds {
		r := Result{Key: "save_folder:" + strconv.FormatInt(d.ID, 10), Kind: "save_folder", Subject: d.Name, State: OK}
		if !saved && !hasPair(d.PathMap) {
			r.State = Warning
			r.Message = d.Name + " has no save folder set, so HolyArrs can't import its finished downloads. Add a path pair in Settings > Downloaders, or set HOLYARRS_SAVE_PATH"
		}
		out = append(out, r)
	}
	return out, nil
}

func hasPair(pm []domain.PathMap) bool {
	for _, p := range pm {
		if strings.TrimSpace(p.Ours) != "" && strings.TrimSpace(p.Theirs) != "" {
			return true
		}
	}
	return false
}
