package sources

import (
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// chooseRelease sends a Release a person picked. Body (optional):
// {"force": bool, "slot_id": int}.
func (h *Handlers) chooseRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Force  bool  `json:"force"`
		SlotID int64 `json:"slot_id"`
	}
	if r.ContentLength != 0 && !webserver.Decode(w, r, &in, `body is {"force": bool, "slot_id": int}, both optional`) {
		return
	}
	ch, err := h.search().Choose(r.Context(), id, in.SlotID, in.Force, actorOf(r))
	if err != nil {
		SearchError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, ch)
}

// turnDownRelease puts a Release on the Turned down list. Body (optional):
// {"reason": text}.
func (h *Handlers) turnDownRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Reason string `json:"reason"`
		SlotID int64  `json:"slot_id"`
	}
	if r.ContentLength != 0 && !webserver.Decode(w, r, &in, `body is {"reason": text, "slot_id": int}, both optional`) {
		return
	}
	if len(in.Reason) > 500 {
		webserver.Fail(w, http.StatusBadRequest, "reason is at most 500 characters")
		return
	}
	if err := h.search().TurnDownFor(r.Context(), id, in.SlotID, strings.TrimSpace(in.Reason), actorOf(r)); err != nil {
		SearchError(w, err)
		return
	}
	var rel domain.Release
	if err := database.Get(r.Context(), h.Store.Read(), "releases", id, &rel); err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	v := []ReleaseView{releaseView(rel)}
	h.named(r.Context(), v)
	webserver.WriteJSON(w, http.StatusOK, v[0])
}

func (h *Handlers) listState(state string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n, off := queryPage(r, 50)
		list, total, err := h.search().List(r.Context(), state, n, off)
		if err != nil {
			webserver.Internal(w, "sources", err)
			return
		}
		out := make([]ReleaseView, 0, len(list))
		for _, x := range list {
			out = append(out, releaseView(x.Release))
		}
		h.named(r.Context(), out)
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"releases": out, "total": total, "limit": n, "offset": off})
	}
}

func (h *Handlers) removeFrom(state string, one bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var id int64
		if one {
			var ok bool
			if id, ok = pathID(w, r); !ok {
				return
			}
		}
		var n int
		var err error
		if state == "held" {
			n, err = h.search().RemoveHeld(r.Context(), id, actorOf(r))
		} else {
			n, err = h.search().RemoveTurnedDown(r.Context(), id, actorOf(r))
		}
		if err != nil {
			SearchError(w, err)
			return
		}
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"removed": n})
	}
}
