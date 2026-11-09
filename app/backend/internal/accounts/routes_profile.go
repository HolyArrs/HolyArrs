package accounts

import (
	"database/sql"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

const displayNameMax = 64

type myProfile struct {
	DisplayName string `json:"display_name"`
	Username    string `json:"username"`
}

// My profile: the display name each person sets for themselves. The sign-in
// name (username) and the role are not editable here; an owner changes a
// role on Users and roles, and the username stays what people sign in with.
func (a *Auth) profileRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/my/profile", a.getProfile)
	m.HandleFunc("PUT /api/v1/my/profile", a.putProfile)
}

// cleanDisplayName trims the name and refuses control characters and names
// over 64 characters. An empty name means "show my username".
func cleanDisplayName(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > displayNameMax {
		return "", false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return s, true
}

func (a *Auth) getProfile(w http.ResponseWriter, r *http.Request) {
	who, ok := webserver.From(r.Context())
	if !ok {
		fail(w, http.StatusUnauthorized, "sign in first")
		return
	}
	p := myProfile{Username: who.Name}
	var err error
	p.DisplayName, err = displayName(r.Context(), a.s.Read(), who.UserID)
	if err != nil && err != sql.ErrNoRows {
		webserver.Internal(w, "accounts", err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *Auth) putProfile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DisplayName string `json:"display_name"`
	}
	if !webserver.Decode(w, r, &in, `send {"display_name": "..."}`) {
		return
	}
	name, ok := cleanDisplayName(in.DisplayName)
	if !ok {
		fail(w, http.StatusBadRequest, "pick a display name of up to 64 characters")
		return
	}
	me, _ := webserver.From(r.Context())
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		return putDisplayName(r.Context(), tx, me.UserID, name)
	})
	if err != nil {
		webserver.Internal(w, "accounts", err)
		return
	}
	a.getProfile(w, r)
}
