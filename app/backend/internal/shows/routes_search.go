package shows

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// Show search (stage 3.2): an episode, a season or the whole show, by hand.

// SearchHandlers takes a show's part of POST /titles/{id}/search (served by
// sources, which hands show Titles to Search).
type SearchHandlers struct {
	Store *database.Store
	Queue *jobs.Queue
}

// Search queues the searches a person asked for on a show page: one
// episode Slot (slot_id), an episode's Slots (item_id), a season
// (season) or, with none of them, every season that has an episode
// kept up to date and looking. version_id narrows to one Version.
// choose false only lists the Releases, and needs an episode or a season.
func (h *SearchHandlers) Search(w http.ResponseWriter, r *http.Request, t mediafiles.Title, in sources.TitleSearchIn) {
	ctx := r.Context()
	db := h.Store.Read()
	listOnly := in.Choose != nil && !*in.Choose
	if listOnly && in.SlotID == nil && in.ItemID == nil && in.Season == nil {
		webserver.Fail(w, http.StatusBadRequest, "say which episode (slot_id or item_id) or season to list Releases for")
		return
	}
	vs, err := mediafiles.VersionsOfTitle(ctx, db, t.ID)
	if err != nil {
		webserver.Internal(w, "shows", err)
		return
	}
	if in.VersionID != nil {
		vs = slices.DeleteFunc(vs, func(v mediafiles.Version) bool { return v.ID != *in.VersionID })
		if len(vs) == 0 {
			webserver.Fail(w, http.StatusNotFound, "no such Version on this show")
			return
		}
	}
	by := actor(r)
	ids := []int64{}
	episode := func(slotID int64) error {
		params := map[string]any{"by_hand": true}
		key := "hand_search_slot:" + strconv.FormatInt(slotID, 10)
		if listOnly {
			params["choose"] = false
			key = "show_slot:" + strconv.FormatInt(slotID, 10)
		}
		id, _, err := h.Queue.Enqueue(ctx, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: slotID}, params, key, by)
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
		id, _, err := h.Queue.Enqueue(ctx, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: partID}, params, key, by)
		if err == nil {
			ids = append(ids, id)
		}
		return err
	}
	verOK := func(id int64) bool {
		return slices.ContainsFunc(vs, func(v mediafiles.Version) bool { return v.ID == id })
	}
	switch {
	case in.SlotID != nil:
		var sl mediafiles.Slot
		if err := mediafiles.LoadSlot(ctx, db, *in.SlotID, &sl); err != nil || !mediafiles.ItemOfTitle(ctx, db, sl.ItemID, t.ID) || !verOK(sl.VersionID) {
			webserver.Fail(w, http.StatusNotFound, "no such Slot on this show")
			return
		}
		if err := episode(sl.ID); err != nil {
			webserver.Internal(w, "shows", err)
			return
		}
	case in.ItemID != nil:
		if !mediafiles.ItemOfTitle(ctx, db, *in.ItemID, t.ID) {
			webserver.Fail(w, http.StatusNotFound, "no such episode on this show")
			return
		}
		slots, err := mediafiles.SlotsOfItem(ctx, db, *in.ItemID)
		if err != nil {
			webserver.Internal(w, "shows", err)
			return
		}
		for _, sl := range slots {
			if !verOK(sl.VersionID) {
				continue
			}
			if err := episode(sl.ID); err != nil {
				webserver.Internal(w, "shows", err)
				return
			}
		}
	case in.Season != nil:
		partID, err := mediafiles.SeasonPartID(ctx, db, t.ID, *in.Season)
		if err != nil {
			webserver.Fail(w, http.StatusNotFound, "no such season")
			return
		}
		for _, v := range vs {
			if err := season(partID, v.ID, false); err != nil {
				webserver.Internal(w, "shows", err)
				return
			}
		}
	default:
		// The whole show: every season with an episode kept up to date
		// that is looking for a copy, in a show and Version kept up to date.
		list, err := ShowSeasonsToSearch(ctx, db, t.ID)
		if err != nil {
			webserver.Internal(w, "shows", err)
			return
		}
		for _, x := range list {
			if !verOK(x.VersionID) {
				continue
			}
			if err := season(x.PartID, x.VersionID, true); err != nil {
				webserver.Internal(w, "shows", err)
				return
			}
		}
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_ids": ids})
}
