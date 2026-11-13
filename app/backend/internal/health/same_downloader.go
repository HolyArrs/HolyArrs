package health

import (
	"context"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// sameDownloaders warns about each Downloader that reaches the same app as
// an older one (same kind, host and port; a path such as /sabnzbd does not
// count): each job then shows twice and imports can trip over each other.
// Only the newer twin gets a line; with no twins there is none.
func (e *Engine) sameDownloaders(ctx context.Context) ([]Result, error) {
	ds, err := database.Select[domain.Downloader](ctx, e.Store.Read(), "downloaders", "removed = 0 ORDER BY id")
	if err != nil {
		return nil, err
	}
	var out []Result
	for i, d := range ds {
		for _, older := range ds[:i] {
			if domain.SameClient(older, d) {
				out = append(out, Result{Key: "same_downloader:" + strconv.FormatInt(d.ID, 10), Kind: "same_downloader", Subject: d.Name, State: Warning,
					Message: d.Name + " reaches the same app as " + older.Name + ", so its downloads can show twice. Remove one in Settings > Downloaders"})
				break
			}
		}
	}
	return out, nil
}
