package accounts

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// deletePerson removes a person: they are signed out, their keys, links and
// two-factor go, and their row is renamed so what they asked for stays.
// Nobody deletes the owner or themself; only the owner deletes an admin.
func (a *Auth) deletePerson(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	by, _ := webserver.From(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, "no such user")
		return
	}
	role, err := personToDelete(r.Context(), a.s.Read(), id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, http.StatusNotFound, "no such user")
		return
	}
	if err != nil {
		serverError(w, "delete user", err)
		return
	}
	switch {
	case role == webserver.RoleOwner:
		fail(w, http.StatusForbidden, "the owner cannot be deleted")
		return
	case id == by.UserID:
		fail(w, http.StatusForbidden, "you cannot delete yourself")
		return
	case role != webserver.RoleMember && by.Role != webserver.RoleOwner:
		fail(w, http.StatusForbidden, "only the owner can delete an admin")
		return
	}
	name := a.userName(r.Context(), id)
	actor, _ := json.Marshal(webserver.Actor(r.Context()))
	now := database.FormatTime(a.now())
	err = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		if err := clearTwoFactor(r.Context(), tx, a.vault, id); err != nil {
			return err
		}
		if err := endOutsideLinks(r.Context(), tx, a.vault, id); err != nil {
			return err
		}
		if err := deleteSessionsOf(r.Context(), tx, id); err != nil {
			return err
		}
		if err := deleteKeysOf(r.Context(), tx, id); err != nil {
			return err
		}
		return removePersonRow(r.Context(), tx, id, now, string(actor))
	})
	if err != nil {
		serverError(w, "delete user", err)
		return
	}
	log.Printf("auth: user %d deleted by user %d", id, by.UserID)
	a.accountEvent(r, id, "deleted", byName(r)+" deleted "+name, nil)
	w.WriteHeader(http.StatusNoContent)
}
