package sources

import (
	"net/http"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// TitleSearchIn is the body of POST /titles/{id}/search.
type TitleSearchIn struct {
	Choose    *bool  `json:"choose"`
	SlotID    *int64 `json:"slot_id"`
	ItemID    *int64 `json:"item_id"`
	Season    *int   `json:"season"`
	VersionID *int64 `json:"version_id"`
	AlbumID   *int64 `json:"album_id"` // artists: one album
}

// KindSearch answers a title search for one kind of Title (a show or an
// artist), which its own module serves.
type KindSearch func(w http.ResponseWriter, r *http.Request, t mediafiles.Title, in TitleSearchIn)

// TitleSearchHandlers serves POST /api/v1/titles/{id}/search.
type TitleSearchHandlers struct {
	Store *database.Store
	Queue *jobs.Queue
	// Show and Artist take the search of a show or an artist.
	Show, Artist KindSearch
}

// Routes registers the title search route.
func (h *TitleSearchHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/titles/{id}/search", h.searchTitle)
}

// searchTitle queues a search_slot Job for each Slot of the Title that is
// looking (or the one Slot asked for). Body (optional): {"choose": false}
// only judges and stores the Releases for a person to choose from;
// "slot_id" searches one Slot. A search from here is a single-Item hand
// search: the protocol wait is skipped, and the Release gate too, with a
// warning in each verdict.
func (h *TitleSearchHandlers) searchTitle(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var t mediafiles.Title
	if err == nil {
		err = mediafiles.LoadTitle(r.Context(), h.Store.Read(), id, &t)
	}
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, "no such Title")
		return
	}
	var in TitleSearchIn
	if r.ContentLength != 0 && !webserver.Decode(w, r, &in, `body is {"choose": bool, "slot_id": int}, both optional; for a show also "item_id", "season" and "version_id"; for an artist "album_id", "item_id" and "version_id"`) {
		return
	}
	if t.Kind == "show" {
		h.Show(w, r, t, in)
		return
	}
	if t.Kind == "artist" {
		h.Artist(w, r, t, in)
		return
	}
	if t.Kind != "movie" {
		webserver.Fail(w, http.StatusBadRequest, "this kind of title can't be searched")
		return
	}
	if in.ItemID != nil || in.Season != nil || in.VersionID != nil || in.AlbumID != nil {
		webserver.Fail(w, http.StatusBadRequest, "item_id, season, album_id and version_id are for shows and artists")
		return
	}
	slots, err := mediafiles.SlotsOfTitle(r.Context(), h.Store.Read(), t.ID)
	if err != nil {
		webserver.Internal(w, "sources", err)
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
		key := "hand_search_slot:" + strconv.FormatInt(s.ID, 10)
		if params["choose"] == false {
			key = "show_slot:" + strconv.FormatInt(s.ID, 10)
		}
		id, _, err := h.Queue.Enqueue(r.Context(), jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: s.ID}, params, key, actorOf(r))
		if err != nil {
			webserver.Internal(w, "sources", err)
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
