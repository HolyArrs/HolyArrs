package downloads

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Usenet queue order and priority for one client item. Top stays on the
// shared Controller route.
func (a *Web) usenetQueueRoutes(m *http.ServeMux) {
	move := func(where string) http.HandlerFunc {
		return a.itemAct(true, func(r *http.Request, e Engine, id string) error {
			err := asUsenetQueue(e, func(q UsenetQueue) error { return q.MoveItem(r.Context(), id, where) })
			if err == nil {
				a.itemChanged(r, id, "move_"+where, "moved "+where)
			}
			return err
		})
	}
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/up", move(MoveUp))
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/down", move(MoveDown))
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/bottom", move(MoveBottom))
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/priority", a.itemPriority)
}

func asUsenetQueue(e Engine, f func(UsenetQueue) error) error {
	q, ok := e.(UsenetQueue)
	if !ok {
		return ErrNotSupported
	}
	return f(q)
}

const priorityHelp = `body is {"priority": "low", "normal", "high" or "force"}`

// itemPriority sets one Usenet job's priority.
func (a *Web) itemPriority(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Priority string `json:"priority"`
	}
	if !webserver.Decode(w, r, &in, priorityHelp) {
		return
	}
	switch in.Priority {
	case PriorityLow, PriorityNormal, PriorityHigh, PriorityForce:
	default:
		webserver.Fail(w, http.StatusBadRequest, priorityHelp)
		return
	}
	a.itemAct(true, func(r *http.Request, e Engine, id string) error {
		err := asUsenetQueue(e, func(q UsenetQueue) error { return q.SetPriority(r.Context(), id, in.Priority) })
		if err == nil {
			a.itemChanged(r, id, "priority", "set the priority to "+in.Priority+" for")
		}
		return err
	})(w, r)
}
