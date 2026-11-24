package health

import (
	"context"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// sameDownloaders warns about each Downloader that reaches the same app as
// an older one (same kind, host and port; a path such as /sabnzbd does not
// count): each job then shows twice and imports can trip over each other.
// Only the newer twin gets a line; with no twins there is none. Twins that
// each see only their own categories, with no category in common, are kept
// apart already and get no line.
func (e *Engine) sameDownloaders(ctx context.Context) ([]Result, error) {
	ds, err := database.Select[domain.Downloader](ctx, e.Store.Read(), "downloaders", "removed = 0 ORDER BY id")
	if err != nil {
		return nil, err
	}
	var out []Result
	for i, d := range ds {
		for _, older := range ds[:i] {
			if domain.SameClient(older, d) && !fenced(older, d) {
				out = append(out, Result{Key: "same_downloader:" + strconv.FormatInt(d.ID, 10), Kind: "same_downloader", Subject: d.Name, State: Warning,
					Message: d.Name + " reaches the same app as " + older.Name + ", so its downloads can show twice. Remove one in Settings > Downloaders"})
				break
			}
		}
	}
	return out, nil
}

// fenced reports whether two Downloaders on the same app each show only
// their own categories and share none of them (after-import ones included).
func fenced(a, b domain.Downloader) bool {
	if !a.OwnOnly || !b.OwnOnly {
		return false
	}
	for _, x := range a.Categories() {
		for _, y := range b.Categories() {
			if strings.EqualFold(strings.TrimSpace(x), strings.TrimSpace(y)) {
				return false
			}
		}
	}
	return true
}
