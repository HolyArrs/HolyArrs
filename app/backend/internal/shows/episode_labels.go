package shows

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// SlotOfTitle reports whether Item itemID is one of Title titleID.
func (h *Handlers) SlotOfTitle(ctx context.Context, itemID, titleID int64) bool {
	var n int
	_ = h.Store.Read().QueryRowContext(ctx, "SELECT count(*) FROM items WHERE id = ? AND title_id = ?", itemID, titleID).Scan(&n)
	return n == 1
}

// EpisodeLabels names the episodes of Slots: "S01E02", "S01E01, S01E02",
// or "S01 (10 episodes)" for a long run of one season. "" for movies.
func (h *Handlers) EpisodeLabels(ctx context.Context, slotIDs []int64) string {
	if len(slotIDs) == 0 {
		return ""
	}
	db := h.Store.Read()
	var labels []string
	seasons := map[int]int{}
	for _, id := range slotIDs {
		var it mediafiles.Item
		var itemID int64
		if db.QueryRowContext(ctx, "SELECT item_id FROM slots WHERE id = ?", id).Scan(&itemID) != nil {
			continue
		}
		if database.Get(ctx, db, "items", itemID, &it) != nil || it.Kind != "episode" {
			continue
		}
		l := sources.EpisodeLabel(it)
		if !slices.Contains(labels, l) {
			labels = append(labels, l)
			if it.Numbers.Season != nil {
				seasons[*it.Numbers.Season]++
			}
		}
	}
	if len(labels) > 3 {
		var parts []string
		keys := make([]int, 0, len(seasons))
		for k := range seasons {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("S%02d (%d episodes)", k, seasons[k]))
		}
		return strings.Join(parts, ", ")
	}
	return strings.Join(labels, ", ")
}

// SeasonLabel names a season Job's subject: the show and "S01".
func (h *Handlers) SeasonLabel(ctx context.Context, partID int64) (int64, string, string) {
	var titleID int64
	var name string
	var number int
	_ = h.Store.Read().QueryRowContext(ctx, `SELECT t.id, t.name, COALESCE(p.number, 0) FROM parts p JOIN titles t ON t.id = p.title_id WHERE p.id = ?`,
		partID).Scan(&titleID, &name, &number)
	return titleID, name, fmt.Sprintf("S%02d", number)
}
