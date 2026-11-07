package api

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloader"
)

// Usenet queue order and priority for one client item. Top stays on the
// shared Controller route.
func (a *API) usenetQueueRoutes(m *http.ServeMux) {
	move := func(where string) http.HandlerFunc {
		return a.itemAct(true, func(r *http.Request, e downloader.Engine, id string) error {
			err := asUsenetQueue(e, func(q downloader.UsenetQueue) error { return q.MoveItem(r.Context(), id, where) })
			if err == nil {
				a.itemChanged(r, id, "move_"+where, "moved "+where)
			}
			return err
		})
	}
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/up", move(downloader.MoveUp))
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/down", move(downloader.MoveDown))
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/bottom", move(downloader.MoveBottom))
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/priority", a.itemPriority)
}

func asUsenetQueue(e downloader.Engine, f func(downloader.UsenetQueue) error) error {
	q, ok := e.(downloader.UsenetQueue)
	if !ok {
		return downloader.ErrNotSupported
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
	case downloader.PriorityLow, downloader.PriorityNormal, downloader.PriorityHigh, downloader.PriorityForce:
	default:
		fail(w, http.StatusBadRequest, priorityHelp)
		return
	}
	a.itemAct(true, func(r *http.Request, e downloader.Engine, id string) error {
		err := asUsenetQueue(e, func(q downloader.UsenetQueue) error { return q.SetPriority(r.Context(), id, in.Priority) })
		if err == nil {
			a.itemChanged(r, id, "priority", "set the priority to "+in.Priority+" for")
		}
		return err
	})(w, r)
}
