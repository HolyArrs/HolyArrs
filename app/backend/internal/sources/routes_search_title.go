package sources

import (
	"context"
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

// RetryImport asks again for the import of the finished downloads the given
// Slots wait on and returns the Slots it did that for (see
// downloads.Manager.RetryImportFirst).
type RetryImport func(ctx context.Context, slotIDs []int64, by domain.Actor) map[int64]bool

// KindSearch answers a title search for one kind of Title (a show or an
// artist), which its own module serves.
type KindSearch func(w http.ResponseWriter, r *http.Request, t mediafiles.Title, in TitleSearchIn)

// TitleSearchHandlers serves POST /api/v1/titles/{id}/search.
type TitleSearchHandlers struct {
	Store *database.Store
	Queue *jobs.Queue
	// Show and Artist take the search of a show or an artist.
	Show, Artist KindSearch
	// RetryImport, when set, goes first: a movie whose download is on disk but
	// failed to import is imported again instead of searched.
	RetryImport RetryImport
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
	// A search no Source can take would end as an empty success; say so now.
	if ok, err := handSearchable(r, h.Store); err != nil {
		webserver.Internal(w, "sources", err)
		return
	} else if !ok {
		webserver.Fail(w, http.StatusConflict, "No Source to search. Add one or turn one on in Settings > Sources.")
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
	var retried map[int64]bool
	if h.RetryImport != nil && params["choose"] != false {
		asked := []int64{}
		for _, s := range slots {
			if in.SlotID == nil || s.ID == *in.SlotID {
				asked = append(asked, s.ID)
			}
		}
		retried = h.RetryImport(r.Context(), asked, actorOf(r))
	}
	ids := []int64{}
	for _, s := range slots {
		if in.SlotID != nil && s.ID != *in.SlotID || retried[s.ID] {
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
	if in.SlotID != nil && len(ids) == 0 && !retried[*in.SlotID] {
		webserver.Fail(w, http.StatusNotFound, "no such Slot on this Title")
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_ids": ids, "imports_again": len(retried)})
}

// handSearchable reports whether any Source is on and can search (not RSS
// only). Paused Sources count: the search says why they were skipped. Kept
// loose on purpose: finer filters (kind, tags, use) stay with the search.
func handSearchable(r *http.Request, st *database.Store) (bool, error) {
	srcs, err := database.Select[domain.Source](r.Context(), st.Read(), "sources", "state IN ('enabled', 'paused')")
	if err != nil {
		return false, err
	}
	for _, s := range srcs {
		if s.Format != "rss" {
			return true, nil
		}
	}
	return false, nil
}
