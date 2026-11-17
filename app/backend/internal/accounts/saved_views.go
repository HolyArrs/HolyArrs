package accounts

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Saved views: the view each person last picked on a page, kept on their
// account so a phone, another browser or the installed app opens on the same
// view. Only the names below are kept. An unknown name or a value that is too
// long is dropped quietly, never an error; the page checks each value again
// when it reads it. When one person picks on two devices, the last pick wins.

// viewValueMax is the longest value kept, in bytes of compact JSON.
const viewValueMax = 256

// viewNames are the picks a page may save: the Library view per kind, the
// Import a library view, and the Calendar's view and follows.
var viewNames = map[string]bool{
	"library.view.all":    true,
	"library.view.movie":  true,
	"library.view.show":   true,
	"library.view.artist": true,
	"library.import.view": true,
	"calendar.view":       true,
	"calendar.follows":    true,
}

func (a *Auth) savedViewRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/my/views", a.getViews)
	m.HandleFunc("PUT /api/v1/my/views", a.putViews)
}

// viewsOf is a person's saved picks; empty, never nil, when there are none
// or they cannot be read, so the session answer never fails over them.
func (a *Auth) viewsOf(ctx context.Context, userID int64) map[string]json.RawMessage {
	v, err := savedViews(ctx, a.s.Read(), userID)
	if err != nil || v == nil {
		return map[string]json.RawMessage{}
	}
	return v
}

// cleanViewValue compacts a value and refuses one over viewValueMax.
func cleanViewValue(raw json.RawMessage) (string, bool) {
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil || b.Len() == 0 || b.Len() > viewValueMax {
		return "", false
	}
	return b.String(), true
}

func (a *Auth) getViews(w http.ResponseWriter, r *http.Request) {
	who, ok := webserver.From(r.Context())
	if !ok {
		fail(w, http.StatusUnauthorized, "sign in first")
		return
	}
	v, err := savedViews(r.Context(), a.s.Read(), who.UserID)
	if err != nil {
		webserver.Internal(w, "accounts", err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// putViews saves the picks in the body; names left out keep their value and
// null forgets a pick.
func (a *Auth) putViews(w http.ResponseWriter, r *http.Request) {
	var in map[string]json.RawMessage
	if !webserver.Decode(w, r, &in, `send {"library.view.movie": "posters"}`) {
		return
	}
	me, _ := webserver.From(r.Context())
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		for name, raw := range in {
			if !viewNames[name] {
				continue
			}
			value, ok := cleanViewValue(raw)
			if !ok {
				continue
			}
			var err error
			if value == "null" {
				err = forgetSavedView(r.Context(), tx, me.UserID, name)
			} else {
				err = putSavedView(r.Context(), tx, me.UserID, name, value)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		webserver.Internal(w, "accounts", err)
		return
	}
	a.getViews(w, r)
}
