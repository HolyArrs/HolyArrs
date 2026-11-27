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
// apart already and get no line. Fenced twins that still share a category
// get a line naming it, since changing it clears the line and removing a
// Downloader would break tag routing.
func (e *Engine) sameDownloaders(ctx context.Context) ([]Result, error) {
	ds, err := database.Select[domain.Downloader](ctx, e.Store.Read(), "downloaders", "removed = 0 ORDER BY id")
	if err != nil {
		return nil, err
	}
	var out []Result
	for i, d := range ds {
		for _, older := range ds[:i] {
			if !domain.SameClient(older, d) {
				continue
			}
			msg := d.Name + " reaches the same app as " + older.Name + ", so its downloads can show twice. Remove one in Settings > Downloaders"
			if older.OwnOnly && d.OwnOnly {
				c := sharedCategory(older, d)
				if c == "" {
					continue
				}
				msg = d.Name + " and " + older.Name + " both use the category \"" + c + "\", so downloads in it can show twice. Give " + d.Name + " its own category in Settings > Downloaders"
			}
			out = append(out, Result{Key: "same_downloader:" + strconv.FormatInt(d.ID, 10), Kind: "same_downloader", Subject: d.Name, State: Warning, Message: msg})
			break
		}
	}
	return out, nil
}

// sharedCategory is the first category two Downloaders both use (after-import
// ones included), or "" when they share none.
func sharedCategory(a, b domain.Downloader) string {
	for _, x := range a.Categories() {
		for _, y := range b.Categories() {
			if strings.EqualFold(strings.TrimSpace(x), strings.TrimSpace(y)) {
				return strings.TrimSpace(x)
			}
		}
	}
	return ""
}
