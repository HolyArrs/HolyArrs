package api

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
)

// Usenet queue order and priority for one client item. Top stays on the
// shared Controller route.
func (a *API) usenetQueueRoutes(m *http.ServeMux) {
	move := func(where string) http.HandlerFunc {
		return a.itemAct(true, func(r *http.Request, e downloads.Engine, id string) error {
			err := asUsenetQueue(e, func(q downloads.UsenetQueue) error { return q.MoveItem(r.Context(), id, where) })
			if err == nil {
				a.itemChanged(r, id, "move_"+where, "moved "+where)
			}
			return err
		})
	}
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/up", move(downloads.MoveUp))
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/down", move(downloads.MoveDown))
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/bottom", move(downloads.MoveBottom))
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/priority", a.itemPriority)
}

func asUsenetQueue(e downloads.Engine, f func(downloads.UsenetQueue) error) error {
	q, ok := e.(downloads.UsenetQueue)
	if !ok {
		return downloads.ErrNotSupported
	}
	return f(q)
}

const priorityHelp = `body is {"priority": "low", "normal", "high" or "force"}`

// itemPriority sets one Usenet job's priority.
func (a *API) itemPriority(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Priority string `json:"priority"`
	}
	if !decode(w, r, &in, priorityHelp) {
		return
	}
	switch in.Priority {
	case downloads.PriorityLow, downloads.PriorityNormal, downloads.PriorityHigh, downloads.PriorityForce:
	default:
		fail(w, http.StatusBadRequest, priorityHelp)
		return
	}
	a.itemAct(true, func(r *http.Request, e downloads.Engine, id string) error {
		err := asUsenetQueue(e, func(q downloads.UsenetQueue) error { return q.SetPriority(r.Context(), id, in.Priority) })
		if err == nil {
			a.itemChanged(r, id, "priority", "set the priority to "+in.Priority+" for")
		}
		return err
	})(w, r)
}
