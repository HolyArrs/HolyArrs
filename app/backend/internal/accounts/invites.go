package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Invites.
const (
	InviteDays    = 7  // default life of an invite link
	maxInviteDays = 30 // longest an invite may last
	invitePath    = "/invite/"
)

var errInviteGone = errors.New("invite gone")

var errNameTaken = errors.New("name taken")

// InviteView is an invite without its link.
type InviteView struct {
	ID        string           `json:"id"`
	Role      string           `json:"role"`
	Perms     []webserver.Perm `json:"perms"`
	Note      string           `json:"note"`
	CreatedBy string           `json:"created_by"`
	CreatedAt string           `json:"created_at"`
	ExpiresAt string           `json:"expires_at"`
	UsedAt    *string          `json:"used_at,omitempty"`
	UsedBy    *string          `json:"used_by,omitempty"`
	// MaxRating is the age-rating limit the new person gets ("" for none).
	MaxRating     string `json:"max_rating"`
	RatingCountry string `json:"rating_country"`
}

func (a *Auth) listInvites(w http.ResponseWriter, r *http.Request) {
	out, err := listInviteViews(r.Context(), a.s.Read())
	if err != nil {
		serverError(w, "invites", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"invites": out})
}

func (a *Auth) makeInvite(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	by, _ := webserver.From(r.Context())
	var body struct {
		Role          string    `json:"role"`
		Perms         *[]string `json:"perms"`
		Note          string    `json:"note"`
		Days          int       `json:"days"`
		MaxRating     string    `json:"max_rating"`
		RatingCountry string    `json:"rating_country"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, `send {"role": "member", "perms": [...], "note": "...", "days": 7}`)
		return
	}
	if body.Role == "" {
		body.Role = webserver.RoleMember
	}
	if body.Role != webserver.RoleAdmin && body.Role != webserver.RoleMember {
		fail(w, http.StatusBadRequest, "role must be admin or member")
		return
	}
	perms := webserver.DefaultPerms(body.Role)
	if body.Perms != nil {
		var err error
		if perms, err = readPerms(*body.Perms); err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	maxRating, country, err := kidLimit(body.Role, body.MaxRating, body.RatingCountry, perms, a.skipLocal.Load())
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Role == webserver.RoleAdmin && !mayMakeAdmin(w, by) {
		return
	}
	if err := mayGrant(by, body.Role, perms); err != nil {
		fail(w, http.StatusForbidden, err.Error())
		return
	}
	if body.Days == 0 {
		body.Days = InviteDays
	}
	if body.Days < 1 || body.Days > maxInviteDays {
		fail(w, http.StatusBadRequest, fmt.Sprintf("an invite lasts 1 to %d days", maxInviteDays))
		return
	}
	body.Note = truncate(strings.TrimSpace(body.Note), 100)
	tok := token(32)
	now := a.now()
	expires := now.Add(time.Duration(body.Days) * 24 * time.Hour)
	err = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		return insertInvite(r.Context(), tx, sum(tok), database.FormatTime(now), by.UserID, body.Role, permsJSON(perms), body.Note, database.FormatTime(expires), maxRating, country)
	})
	if err != nil {
		serverError(w, "invite", err)
		return
	}
	log.Printf("auth: user %d made a %s invite", by.UserID, body.Role)
	a.accountEvent(r, by.UserID, "invite_made", byName(r)+" made an invite for a new "+body.Role, map[string]any{"role": body.Role, "days": body.Days})
	writeJSON(w, http.StatusCreated, map[string]any{"id": sum(tok), "path": invitePath + tok, "role": body.Role, "perms": perms,
		"expires_at": database.FormatTime(expires), "note": "This link is shown only now and works once.",
		"max_rating": maxRating, "rating_country": country})
}

func (a *Auth) deleteInvite(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	by, _ := webserver.From(r.Context())
	var n int64
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		// Admins may take back only their own invites.
		var err error
		n, err = deleteOpenInvite(r.Context(), tx, r.PathValue("id"), by.Role == webserver.RoleOwner, by.UserID)
		return err
	})
	if err != nil {
		serverError(w, "invite", err)
		return
	}
	if n == 0 {
		fail(w, http.StatusNotFound, "no such open invite")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// inviteGate applies the sign-in back-off to invite tokens, so links cannot
// be guessed. It returns false after writing 429.
func (a *Auth) inviteGate(w http.ResponseWriter, r *http.Request) (string, bool) {
	key := a.clientKey(r)
	a.mu.Lock()
	wait := a.waitFor(key, freeTries)
	a.mu.Unlock()
	if wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		fail(w, http.StatusTooManyRequests, "too many tries; wait a little and try again")
		return key, false
	}
	return key, true
}

func (a *Auth) openInvite(ctx context.Context, q database.Querier, tok string) (role, perms, note, maxRating, country string, ok bool) {
	// The person who made the invite must still be an active owner or admin.
	role, perms, note, expires, maxRating, country, err := openInviteRow(ctx, q, sum(tok))
	if err != nil || !a.now().Before(parseTime(expires)) {
		return "", "", "", "", "", false
	}
	return role, perms, note, maxRating, countryOr(country), true
}

func (a *Auth) lookInvite(w http.ResponseWriter, r *http.Request) {
	key, ok := a.inviteGate(w, r)
	if !ok {
		return
	}
	role, _, note, maxRating, country, ok := a.openInvite(r.Context(), a.s.Read(), r.PathValue("token"))
	if !ok {
		a.mu.Lock()
		a.noteFail(key)
		a.mu.Unlock()
		fail(w, http.StatusNotFound, "this invite link is used up, ended or wrong; ask for a new one")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"role": role, "note": note, "max_rating": maxRating, "rating_country": country})
}

// inviteUser reads an open invite inside a write and returns the new user it
// makes (no name or password yet), or errInviteGone.
func (a *Auth) inviteUser(tx *sql.Tx, tok string) (domain.User, error) {
	role, perms, expires, maxRating, country, byRole, byPerms, err := inviteMakerRow(context.Background(), tx, sum(tok))
	if err != nil || !a.now().Before(parseTime(expires)) {
		return domain.User{}, errInviteGone
	}
	// The link gives only what its maker still holds today.
	perms = permsJSON(slices.DeleteFunc(parsePerms(perms), func(p webserver.Perm) bool {
		return !(webserver.Caller{Role: byRole, Perms: parsePerms(byPerms)}).Has(p)
	}))
	return domain.User{Role: role, State: "active", Perms: perms, MaxRating: maxRating, RatingCountry: countryOr(country)}, nil
}

// useInvite marks an invite used by the new user id, inside the same write.
func (a *Auth) useInvite(tx *sql.Tx, tok string, id int64) error {
	n, err := markInviteUsed(context.Background(), tx, database.FormatTime(a.now()), id, sum(tok))
	if err != nil {
		return err
	}
	if n != 1 {
		return errInviteGone
	}
	return nil
}

func (a *Auth) acceptInvite(w http.ResponseWriter, r *http.Request) {
	key, ok := a.inviteGate(w, r)
	if !ok {
		return
	}
	c, ok := readCreds(w, r)
	if !ok {
		return
	}
	if msg := nameProblem(c.Name); msg != "" {
		fail(w, http.StatusBadRequest, msg)
		return
	}
	if msg := passwordProblem(c.Password, MinPasswordLength(r.Context(), a.s.Read())); msg != "" {
		fail(w, http.StatusBadRequest, msg)
		return
	}
	tok := r.PathValue("token")
	if _, _, _, _, _, ok := a.openInvite(r.Context(), a.s.Read(), tok); !ok {
		a.mu.Lock()
		a.noteFail(key)
		a.mu.Unlock()
		fail(w, http.StatusNotFound, "this invite link is used up, ended or wrong; ask for a new one")
		return
	}
	h, err := hashPassword(c.Password, a.hash)
	if err != nil {
		serverError(w, "invite", err)
		return
	}
	var id int64
	err = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		u, err := a.inviteUser(tx, tok)
		if err != nil {
			return err
		}
		n, err := countUsersNamed(r.Context(), tx, c.Name)
		if err != nil {
			return err
		}
		if n > 0 {
			return errNameTaken
		}
		u.Name, u.PasswordHash = c.Name, h
		if id, err = insertUserTx(tx, &u, domain.System); err != nil {
			return err
		}
		return a.useInvite(tx, tok, id)
	})
	switch {
	case errors.Is(err, errInviteGone):
		fail(w, http.StatusNotFound, "this invite link is used up, ended or wrong; ask for a new one")
		return
	case errors.Is(err, errNameTaken):
		fail(w, http.StatusConflict, "that name is taken; pick another")
		return
	case err != nil:
		serverError(w, "invite", err)
		return
	}
	log.Printf("auth: invite used by new user %q (%d) from %s", c.Name, id, r.RemoteAddr)
	a.startSession(w, r, id, c.Name, http.StatusCreated)
}
