package api

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/source"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

// Show search (stage 3.2): an episode, a season or the whole show, by hand.

// showSearchIn is the body of POST /titles/{id}/search for a show.
type showSearchIn struct {
	Choose    *bool  `json:"choose"`
	SlotID    *int64 `json:"slot_id"`
	ItemID    *int64 `json:"item_id"`
	Season    *int   `json:"season"`
	VersionID *int64 `json:"version_id"`
	AlbumID   *int64 `json:"album_id"` // artists: one album
}

// searchShow queues the searches a person asked for on a show page: one
// episode Slot (slot_id), an episode's Slots (item_id), a season
// (season) or, with none of them, every season that has an episode
// kept up to date and looking. version_id narrows to one Version.
// choose false only lists the Releases, and needs an episode or a season.
func (a *API) searchShow(w http.ResponseWriter, r *http.Request, t domain.Title, in showSearchIn) {
	ctx := r.Context()
	db := a.s.Store.Read()
	listOnly := in.Choose != nil && !*in.Choose
	if listOnly && in.SlotID == nil && in.ItemID == nil && in.Season == nil {
		fail(w, http.StatusBadRequest, "say which episode (slot_id or item_id) or season to list Releases for")
		return
	}
	vs, err := store.Select[domain.Version](ctx, db, "versions", "title_id = ? ORDER BY is_main DESC, id", t.ID)
	if err != nil {
		internal(w, err)
		return
	}
	if in.VersionID != nil {
		vs = slices.DeleteFunc(vs, func(v domain.Version) bool { return v.ID != *in.VersionID })
		if len(vs) == 0 {
			fail(w, http.StatusNotFound, "no such Version on this show")
			return
		}
	}
	by := actor(r)
	ids := []int64{}
	episode := func(slotID int64) error {
		params := map[string]any{"by_hand": true}
		key := "hand_search_slot:" + itoa(slotID)
		if listOnly {
			params["choose"] = false
			key = "show_slot:" + itoa(slotID)
		}
		id, _, err := a.s.Queue.Enqueue(ctx, source.KindSearchSlot, &domain.Subject{Type: "slot", ID: slotID}, params, key, by)
		if err == nil {
			ids = append(ids, id)
		}
		return err
	}
	season := func(partID, versionID int64, keptOnly bool) error {
		params := map[string]any{"by_hand": true, "version_id": versionID}
		key := fmt.Sprintf("hand_search_season:%d:%d", partID, versionID)
		if keptOnly {
			params["kept_only"] = true
		}
		if listOnly {
			params["choose"] = false
			key = fmt.Sprintf("show_season:%d:%d", partID, versionID)
		}
		id, _, err := a.s.Queue.Enqueue(ctx, source.KindSearchSeason, &domain.Subject{Type: "part", ID: partID}, params, key, by)
		if err == nil {
			ids = append(ids, id)
		}
		return err
	}
	verOK := func(id int64) bool {
		return slices.ContainsFunc(vs, func(v domain.Version) bool { return v.ID == id })
	}
	switch {
	case in.SlotID != nil:
		var sl domain.Slot
		if err := store.Get(ctx, db, "slots", *in.SlotID, &sl); err != nil || !a.slotOfTitle(ctx, sl.ItemID, t.ID) || !verOK(sl.VersionID) {
			fail(w, http.StatusNotFound, "no such Slot on this show")
			return
		}
		if err := episode(sl.ID); err != nil {
			internal(w, err)
			return
		}
	case in.ItemID != nil:
		if !a.slotOfTitle(ctx, *in.ItemID, t.ID) {
			fail(w, http.StatusNotFound, "no such episode on this show")
			return
		}
		slots, err := store.Select[domain.Slot](ctx, db, "slots", "item_id = ? ORDER BY id", *in.ItemID)
		if err != nil {
			internal(w, err)
			return
		}
		for _, sl := range slots {
			if !verOK(sl.VersionID) {
				continue
			}
			if err := episode(sl.ID); err != nil {
				internal(w, err)
				return
			}
		}
	case in.Season != nil:
		var partID int64
		if err := db.QueryRowContext(ctx, "SELECT id FROM parts WHERE title_id = ? AND kind = 'season' AND number = ?", t.ID, *in.Season).Scan(&partID); err != nil {
			fail(w, http.StatusNotFound, "no such season")
			return
		}
		for _, v := range vs {
			if err := season(partID, v.ID, false); err != nil {
				internal(w, err)
				return
			}
		}
	default:
		// The whole show: every season with an episode kept up to date
		// that is looking for a copy, in a show and Version kept up to date.
		list, err := library.ShowSeasonsToSearch(ctx, db, t.ID)
		if err != nil {
			internal(w, err)
			return
		}
		for _, x := range list {
			if !verOK(x.VersionID) {
				continue
			}
			if err := season(x.PartID, x.VersionID, true); err != nil {
				internal(w, err)
				return
			}
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_ids": ids})
}

func (a *API) slotOfTitle(ctx context.Context, itemID, titleID int64) bool {
	var n int
	_ = a.s.Store.Read().QueryRowContext(ctx, "SELECT count(*) FROM items WHERE id = ? AND title_id = ?", itemID, titleID).Scan(&n)
	return n == 1
}

// episodeLabels names the episodes of Slots: "S01E02", "S01E01, S01E02",
// or "S01 (10 episodes)" for a long run of one season. "" for movies.
func (a *API) episodeLabels(ctx context.Context, slotIDs []int64) string {
	if len(slotIDs) == 0 {
		return ""
	}
	db := a.s.Store.Read()
	var labels []string
	seasons := map[int]int{}
	for _, id := range slotIDs {
		var it domain.Item
		var itemID int64
		if db.QueryRowContext(ctx, "SELECT item_id FROM slots WHERE id = ?", id).Scan(&itemID) != nil {
			continue
		}
		if store.Get(ctx, db, "items", itemID, &it) != nil || it.Kind != "episode" {
			continue
		}
		l := source.EpisodeLabel(it)
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

// seasonLabel names a season Job's subject: the show and "S01".
func (a *API) seasonLabel(ctx context.Context, partID int64) (int64, string, string) {
	var titleID int64
	var name string
	var number int
	_ = a.s.Store.Read().QueryRowContext(ctx, `SELECT t.id, t.name, COALESCE(p.number, 0) FROM parts p JOIN titles t ON t.id = p.title_id WHERE p.id = ?`,
		partID).Scan(&titleID, &name, &number)
	return titleID, name, fmt.Sprintf("S%02d", number)
}
