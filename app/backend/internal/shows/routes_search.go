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
	// RetryImport, when set, goes first: an episode whose download (a season
	// pack, say) is on disk but failed to import is imported again, and its
	// season is not searched episode by episode.
	RetryImport sources.RetryImport
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
	retried, retriedParts, err := h.importFirst(r, t.ID, listOnly, in, verOK)
	if err != nil {
		webserver.Internal(w, "shows", err)
		return
	}
	switch {
	case in.SlotID != nil:
		var sl mediafiles.Slot
		if err := mediafiles.LoadSlot(ctx, db, *in.SlotID, &sl); err != nil || !mediafiles.ItemOfTitle(ctx, db, sl.ItemID, t.ID) || !verOK(sl.VersionID) {
			webserver.Fail(w, http.StatusNotFound, "no such Slot on this show")
			return
		}
		if retried[sl.ID] {
			break
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
			if !verOK(sl.VersionID) || retried[sl.ID] {
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
			if retriedParts[partID] {
				break
			}
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
			if !verOK(x.VersionID) || retriedParts[x.PartID] {
				continue
			}
			if err := season(x.PartID, x.VersionID, true); err != nil {
				webserver.Internal(w, "shows", err)
				return
			}
		}
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_ids": ids, "imports_again": len(retried)})
}

// importFirst asks again for the import of the downloads the asked
// episodes wait on (see RetryImport) and returns those episodes' Slots and
// seasons. A list-only search changes nothing.
func (h *SearchHandlers) importFirst(r *http.Request, titleID int64, listOnly bool, in sources.TitleSearchIn, verOK func(int64) bool) (map[int64]bool, map[int64]bool, error) {
	if h.RetryImport == nil || listOnly {
		return nil, nil, nil
	}
	ctx, db := r.Context(), h.Store.Read()
	slots, err := mediafiles.SlotsOfTitle(ctx, db, titleID)
	if err != nil {
		return nil, nil, err
	}
	var season int64
	if in.Season != nil {
		if season, err = mediafiles.SeasonPartID(ctx, db, titleID, *in.Season); err != nil {
			return nil, nil, nil
		}
	}
	partOf := map[int64]int64{}
	asked := []int64{}
	for _, sl := range slots {
		if sl.State != mediafiles.SlotNeedsADecision || sl.ActiveDownloadID == nil || !verOK(sl.VersionID) ||
			in.SlotID != nil && sl.ID != *in.SlotID || in.ItemID != nil && sl.ItemID != *in.ItemID {
			continue
		}
		var it mediafiles.Item
		if err := mediafiles.LoadItem(ctx, db, sl.ItemID, &it); err != nil || it.PartID == nil || in.Season != nil && *it.PartID != season {
			continue
		}
		partOf[sl.ID] = *it.PartID
		asked = append(asked, sl.ID)
	}
	if len(asked) == 0 {
		return nil, nil, nil
	}
	retried := h.RetryImport(ctx, asked, actor(r))
	parts := map[int64]bool{}
	for sid := range retried {
		parts[partOf[sid]] = true
	}
	return retried, parts, nil
}
