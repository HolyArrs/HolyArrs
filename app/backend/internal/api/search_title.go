package api

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// searchTitle queues a search_slot Job for each Slot of the Title that is
// looking (or the one Slot asked for). Body (optional): {"choose": false}
// only judges and stores the Releases for a person to choose from;
// "slot_id" searches one Slot. A search from here is a single-Item hand
// search: the protocol wait is skipped, and the Release gate too, with a
// warning in each verdict.
func (a *API) searchTitle(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	var in showSearchIn
	if r.ContentLength != 0 && !decode(w, r, &in, `body is {"choose": bool, "slot_id": int}, both optional; for a show also "item_id", "season" and "version_id"; for an artist "album_id", "item_id" and "version_id"`) {
		return
	}
	if t.Kind == "show" {
		a.searchShow(w, r, t, in)
		return
	}
	if t.Kind == "artist" {
		a.searchArtist(w, r, t, in)
		return
	}
	if in.ItemID != nil || in.Season != nil || in.VersionID != nil || in.AlbumID != nil {
		webserver.Fail(w, http.StatusBadRequest, "item_id, season, album_id and version_id are for shows and artists")
		return
	}
	slots, err := database.Select[mediafiles.Slot](r.Context(), a.s.Store.Read(), "slots",
		"item_id IN (SELECT id FROM items WHERE title_id = ?) ORDER BY id", t.ID)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	params := map[string]any{"by_hand": true}
	if in.Choose != nil && !*in.Choose {
		params["choose"] = false
	}
	ids := []int64{}
	for _, s := range slots {
		if in.SlotID != nil && s.ID != *in.SlotID {
			continue
		}
		// Its own key: a bulk search waiting for the same Slot must not
		// swallow a person's search (which skips the wait and the gate).
		key := "hand_search_slot:" + itoa(s.ID)
		if params["choose"] == false {
			key = "show_slot:" + itoa(s.ID)
		}
		id, _, err := a.s.Queue.Enqueue(r.Context(), jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: s.ID}, params, key, actor(r))
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		ids = append(ids, id)
	}
	if in.SlotID != nil && len(ids) == 0 {
		webserver.Fail(w, http.StatusNotFound, "no such Slot on this Title")
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_ids": ids})
}
