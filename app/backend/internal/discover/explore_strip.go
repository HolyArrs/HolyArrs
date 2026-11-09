package discover

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// StripRowsMax and StripRowLen bound the row titles a strip request may pass.
const (
	StripRowsMax = 40
	StripRowLen  = 80
)

// StripRows are the row titles passed with row=; false when there are
// more than StripRowsMax or one is longer than StripRowLen.
func StripRows(r *http.Request) ([]string, bool) {
	raw := r.URL.Query()["row"]
	if len(raw) > StripRowsMax {
		return nil, false
	}
	var out []string
	for _, v := range raw {
		if len(v) > StripRowLen {
			return nil, false
		}
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out, true
}

// StripRowsHash is a short hash of the rows, normalised and sorted.
func StripRowsHash(rows []string) string {
	n := make([]string, len(rows))
	for i, v := range rows {
		n[i] = strings.ToLower(v)
	}
	sort.Strings(n)
	sum := sha256.Sum256([]byte(strings.Join(n, "|")))
	return hex.EncodeToString(sum[:])[:16]
}

// PageGenreItems are the member's page titles of a genre.
func PageGenreItems(pg engine.Page, genre string) []engine.Item {
	var out []engine.Item
	seen := map[string]bool{}
	for _, row := range append(append([]engine.Row{}, pg.Head...), pg.Tail...) {
		for _, it := range row.Items {
			if seen[it.Key()] {
				continue
			}
			for _, g := range it.Genres {
				if g == genre {
					seen[it.Key()] = true
					out = append(out, it)
					break
				}
			}
		}
	}
	return out
}

// StripStats reads the member's strip counts of the last StripCountDays.
func StripStats(ctx context.Context, db *sql.DB, member int64, now time.Time) (map[string]engine.StripStats, map[string]bool, error) {
	got, err := accounts.CategoryCountsSince(ctx, db, member, engine.Day(now.AddDate(0, 0, -engine.StripCountDays)))
	if err != nil {
		return nil, nil, err
	}
	var days []engine.CategoryDay
	for _, c := range got {
		d := engine.CategoryDay{Category: c.Key, Shown: c.Shown, Opened: c.Opened}
		if d.Day, err = time.Parse("2006-01-02", c.Day); err == nil {
			days = append(days, d)
		}
	}
	stats, recent := engine.StripStatsFrom(days, now)
	return stats, recent, nil
}

// StripCategoryOK says id names a strip category: a Browse category or a
// genre tile's lower-case name.
func StripCategoryOK(id string) bool {
	if _, ok := engine.CategoryByID(id); ok {
		return true
	}
	if id == "" || len(id) > 32 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c == ' ' || c == '-' || c == '&') {
			return false
		}
	}
	return true
}
