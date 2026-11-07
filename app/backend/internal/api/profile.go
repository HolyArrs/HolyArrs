package api

import (
	"database/sql"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"
)

// My profile: the display name each person sets for themselves. The sign-in
// name (username) and the role are not editable here; an owner changes a
// role on Users and roles, and the username stays what people sign in with.
func (a *API) profileRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/my/profile", a.getProfile)
	m.HandleFunc("PUT /api/v1/my/profile", a.putProfile)
}

type myProfile struct {
	DisplayName string `json:"display_name"`
	Username    string `json:"username"`
}

const displayNameMax = 64

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

func (a *API) getProfile(w http.ResponseWriter, r *http.Request) {
	who := a.person(r)
	p := myProfile{Username: who.Name}
	err := a.s.Store.Read().QueryRowContext(r.Context(), "SELECT display_name FROM user_profiles WHERE user_id = ?", who.UserID).Scan(&p.DisplayName)
	if err != nil && err != sql.ErrNoRows {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *API) putProfile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DisplayName string `json:"display_name"`
	}
	if !decode(w, r, &in, `send {"display_name": "..."}`) {
		return
	}
	name, ok := cleanDisplayName(in.DisplayName)
	if !ok {
		fail(w, http.StatusBadRequest, "pick a display name of up to 64 characters")
		return
	}
	me := a.person(r).UserID
	err := a.s.Store.Write(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO user_profiles (user_id, display_name) VALUES (?, ?)
			ON CONFLICT (user_id) DO UPDATE SET display_name = excluded.display_name`, me, name)
		return err
	})
	if err != nil {
		internal(w, err)
		return
	}
	a.getProfile(w, r)
}
