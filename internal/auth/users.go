package auth

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

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/ratings"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Invites.
const (
	InviteDays    = 7  // default life of an invite link
	maxInviteDays = 30 // longest an invite may last
	invitePath    = "/invite/"
)

// Sign-in settings live in the this_app group with keys starting sign_in_.
const (
	settingsGroup = "this_app"
	signInPrefix  = "sign_in_"
)

// signInSetting reads one sign-in setting; "" when unset.
func (a *Auth) signInSetting(ctx context.Context, key string) string {
	var v string
	_ = a.s.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = ? AND key = ?", settingsGroup, signInPrefix+key).Scan(&v)
	return v
}

func putSetting(tx *sql.Tx, grp, key, value string, by domain.Actor, now time.Time) error {
	actor, _ := json.Marshal(by)
	t := store.FormatTime(now)
	_, err := tx.Exec(`INSERT INTO settings(grp, key, value, created_at, created_by, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
		grp, key, value, t, string(actor), t, string(actor))
	return err
}

// UserView is a user as the Users screen shows them.
type UserView struct {
	ID        int64         `json:"id"`
	Name      string        `json:"name"`
	Role      string        `json:"role"`
	State     string        `json:"state"`
	Perms     []Perm        `json:"perms"`
	TwoFactor bool          `json:"two_factor"`
	Password  bool          `json:"password"` // signs in with a HolyArrs password
	Links     []LinkView    `json:"links"`
	SSOLinks  []SSOLinkView `json:"sso_links"` // single sign-on and trusted proxy identities
	CreatedAt string        `json:"created_at"`
	Avatar    *string       `json:"avatar"` // the picture's address, or null
	LastSeen  *string       `json:"last_seen_at,omitempty"`
	// MaxRating is a Kid's age-rating limit ("" for no limit).
	MaxRating     string `json:"max_rating"`
	RatingCountry string `json:"rating_country"`
}

// countryOr reads an empty rating country as US.
func countryOr(c string) string {
	if c = ratings.Norm(c); c == "" {
		return "US"
	}
	return c
}

// kidLimit checks an age-rating limit asked for a person of role and
// returns it as stored, or an error in plain words for a 400. A limit is
// refused while sign-in is skipped on the local network (skipLocal).
func kidLimit(role, max, country string, perms []Perm, skipLocal bool) (string, string, error) {
	max, country = ratings.Norm(max), countryOr(country)
	if !ratings.Valid(max) {
		return "", "", fmt.Errorf("unknown age rating %q: pick one of %s, or leave it empty for no limit", max, ratings.Choices())
	}
	if !ratings.ValidCountry(country) {
		return "", "", errors.New("Only US age ratings are known for now.")
	}
	if max != "" && role != RoleMember {
		return "", "", errors.New("Only members can have an age-rating limit.")
	}
	if max != "" && skipLocal {
		return "", "", errors.New(errSkipLocalKid)
	}
	if max != "" {
		// A Kid may only ask: any other permission would open routes
		// (live events, downloads, history, jobs) the limit does not cover.
		for _, p := range perms {
			if p != PermAsk && p != PermAutoApprove {
				return "", "", errors.New("A Kid account can only ask for titles.")
			}
		}
	}
	return max, country, nil
}

// SkipLocal says sign-in is skipped on the local network.
func (a *Auth) SkipLocal() bool { return a.skipLocal.Load() }

// OutsideJoinOpen says new people can join by signing in with Plex,
// Jellyfin, Emby or single sign-on.
func (a *Auth) OutsideJoinOpen(ctx context.Context) bool {
	for _, p := range []string{ProviderPlex, ProviderJellyfin, ProviderEmby} {
		if v := a.way(ctx, p); v.On && v.Join != JoinNobody {
			return true
		}
	}
	sso := a.ssoNow()
	return (sso.oidc != nil && sso.oidc.opt.AllowJoin) || (sso.forward != nil && sso.forward.opt.AllowJoin)
}

const errSkipLocalKid = "Turn off skipping sign-in on the local network first: with it on, anyone at home signs in as the owner."

// LinkView is an outside account linked to a user.
type LinkView struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
}

func (a *Auth) userRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/users", a.listUsers)
	m.HandleFunc("PUT /api/v1/users/{id}", a.editUser)
	m.HandleFunc("POST /api/v1/users/{id}/two-factor/reset", a.resetUserTwoFactor)
	m.HandleFunc("POST /api/v1/users/{id}/password", a.setUserPassword)
	m.HandleFunc("GET /api/v1/users/invites", a.listInvites)
	m.HandleFunc("POST /api/v1/users/invites", a.makeInvite)
	m.HandleFunc("DELETE /api/v1/users/invites/{id}", a.deleteInvite)
	m.HandleFunc("GET /api/v1/auth/invites/{token}", a.lookInvite)
	m.HandleFunc("POST /api/v1/auth/invites/{token}", a.acceptInvite)
	m.HandleFunc("POST /api/v1/auth/password", a.changePassword)
}

func serverError(w http.ResponseWriter, what string, err error) {
	log.Printf("auth: %s: %v", what, err)
	fail(w, http.StatusInternalServerError, "something went wrong; see the log")
}

func (a *Auth) listUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := a.s.Read().QueryContext(r.Context(), `SELECT u.id, u.name, u.role, u.state, u.perms, u.totp_secret_id IS NOT NULL,
		u.password_hash != '', u.created_at, (SELECT max(last_seen_at) FROM sessions WHERE user_id = u.id), u.max_rating, u.rating_country,
		(SELECT etag FROM user_avatars WHERE user_id = u.id)
		FROM users u ORDER BY CASE u.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, u.name`)
	if err != nil {
		serverError(w, "users", err)
		return
	}
	out := []UserView{}
	byID := map[int64]int{}
	for rows.Next() {
		var u UserView
		var perms string
		var seen, tag sql.NullString
		if err := rows.Scan(&u.ID, &u.Name, &u.Role, &u.State, &perms, &u.TwoFactor, &u.Password, &u.CreatedAt, &seen, &u.MaxRating, &u.RatingCountry, &tag); err != nil {
			rows.Close()
			serverError(w, "users", err)
			return
		}
		u.Perms = Who{Role: u.Role, Perms: parsePerms(perms)}.Effective()
		u.RatingCountry = countryOr(u.RatingCountry)
		u.Links = []LinkView{}
		u.SSOLinks = []SSOLinkView{}
		u.Avatar = avatarURL(u.ID, tag)
		if seen.Valid {
			u.LastSeen = &seen.String
		}
		byID[u.ID] = len(out)
		out = append(out, u)
	}
	rows.Close()
	links, err := a.s.Read().QueryContext(r.Context(), "SELECT user_id, provider, external_name FROM user_links ORDER BY provider")
	if err != nil {
		serverError(w, "users", err)
		return
	}
	defer links.Close()
	for links.Next() {
		var id int64
		var l LinkView
		if links.Scan(&id, &l.Provider, &l.Name) == nil {
			if i, ok := byID[id]; ok {
				out[i].Links = append(out[i].Links, l)
			}
		}
	}
	sso, err := a.s.Read().QueryContext(r.Context(), "SELECT user_id, kind, issuer, display_name, created_at FROM sso_links ORDER BY kind, issuer")
	if err != nil {
		serverError(w, "users", err)
		return
	}
	defer sso.Close()
	for sso.Next() {
		var id int64
		var l SSOLinkView
		var name sql.NullString
		if sso.Scan(&id, &l.Kind, &l.Issuer, &name, &l.CreatedAt) == nil {
			l.Name = name.String
			if i, ok := byID[id]; ok {
				out[i].SSOLinks = append(out[i].SSOLinks, l)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out, "perms": AllPerms})
}

// mayGrant checks the permissions an owner or admin hands out: an admin may
// only give what they hold, and only to members.
func mayGrant(by Who, role string, perms []Perm) error {
	if by.Role == RoleOwner {
		return nil
	}
	if role != RoleMember {
		return errors.New("only the owner can make admins")
	}
	for _, p := range perms {
		if !by.Has(p) {
			return fmt.Errorf("you cannot give a permission you do not have (%s)", p)
		}
	}
	return nil
}

func readPerms(raw []string) ([]Perm, error) {
	out := []Perm{}
	for _, s := range raw {
		p := Perm(s)
		if !validPerm(p) {
			return nil, fmt.Errorf("unknown permission %q", s)
		}
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out, nil
}

// mayMakeAdmin: an admin signs in to a session holding every scope, the
// control scope included, so an API key may invite or promote an admin
// only when it holds the control scope itself. The admin scope does not
// imply control. It writes 403 and returns false otherwise.
func mayMakeAdmin(w http.ResponseWriter, by Who) bool {
	if by.Via == "api_key" && !ScopesCover(by.Scopes, ScopeControl) {
		fail(w, http.StatusForbidden, "making an admin with an API key needs the control scope")
		return false
	}
	return true
}

func (a *Auth) editUser(w http.ResponseWriter, r *http.Request) {
	if !RequireStrong(w, r) {
		return
	}
	by, _ := From(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, "no such user")
		return
	}
	var body struct {
		Role          *string   `json:"role"`
		Perms         *[]string `json:"perms"`
		State         *string   `json:"state"`
		MaxRating     *string   `json:"max_rating"`
		RatingCountry *string   `json:"rating_country"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, `send {"role": "...", "perms": [...], "state": "active|disabled"}`)
		return
	}
	var role, state, permsRaw, maxRating, country string
	err = a.s.Read().QueryRowContext(r.Context(), "SELECT role, state, perms, max_rating, rating_country FROM users WHERE id = ?", id).
		Scan(&role, &state, &permsRaw, &maxRating, &country)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, http.StatusNotFound, "no such user")
		return
	}
	if err != nil {
		serverError(w, "edit user", err)
		return
	}
	if role == RoleOwner {
		fail(w, http.StatusForbidden, "the owner cannot be changed here; use the reset-owner command to change the owner")
		return
	}
	oldRating, oldCountry := maxRating, country
	if by.Role != RoleOwner && role != RoleMember {
		fail(w, http.StatusForbidden, "only the owner can change an admin")
		return
	}
	if id == by.UserID {
		fail(w, http.StatusForbidden, "you cannot change your own role or permissions")
		return
	}
	newRole, newState, perms := role, state, parsePerms(permsRaw)
	if body.Role != nil {
		if *body.Role != RoleAdmin && *body.Role != RoleMember {
			fail(w, http.StatusBadRequest, "role must be admin or member")
			return
		}
		if *body.Role != role {
			perms = DefaultPerms(*body.Role)
		}
		newRole = *body.Role
	}
	if body.Perms != nil {
		if perms, err = readPerms(*body.Perms); err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if body.State != nil {
		if *body.State != "active" && *body.State != "disabled" {
			fail(w, http.StatusBadRequest, "state must be active or disabled")
			return
		}
		newState = *body.State
	}
	if body.MaxRating != nil {
		maxRating = *body.MaxRating
	} else if newRole != RoleMember {
		maxRating = "" // making someone an admin clears their limit
	}
	if body.RatingCountry != nil {
		country = *body.RatingCountry
	}
	if maxRating, country, err = kidLimit(newRole, maxRating, country, perms, a.skipLocal.Load()); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if newRole == RoleAdmin && role != RoleAdmin && !mayMakeAdmin(w, by) {
		return
	}
	if err := mayGrant(by, newRole, perms); err != nil {
		fail(w, http.StatusForbidden, err.Error())
		return
	}
	actor, _ := json.Marshal(Actor(r.Context()))
	err = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE users SET role = ?, perms = ?, state = ?, max_rating = ?, rating_country = ?, updated_at = ?, updated_by = ? WHERE id = ?",
			newRole, permsJSON(perms), newState, maxRating, country, store.FormatTime(a.now()), string(actor), id); err != nil {
			return err
		}
		if newState != "active" {
			// A disabled person is signed out everywhere and their keys stop.
			if _, err := tx.Exec("DELETE FROM sessions WHERE user_id = ?", id); err != nil {
				return err
			}
			_, err := tx.Exec("DELETE FROM api_keys WHERE owner_user_id = ?", id)
			return err
		}
		return nil
	})
	if err != nil {
		serverError(w, "edit user", err)
		return
	}
	log.Printf("auth: user %d changed by user %d: role %s, state %s, age-rating limit %q", id, by.UserID, newRole, newState, maxRating)
	var what []string
	if newRole != role {
		what = append(what, "role "+role+" to "+newRole)
	}
	if !samePerms(perms, parsePerms(permsRaw)) {
		what = append(what, "permissions")
	}
	if newState != state {
		if newState == "active" {
			what = append(what, "turned back on")
		} else {
			what = append(what, "turned off, signed out and API keys removed")
		}
	}
	if maxRating != oldRating || country != oldCountry {
		what = append(what, "age-rating limit")
	}
	if len(what) > 0 {
		a.accountEvent(r, id, "edited", byName(r)+" changed "+a.userName(r.Context(), id)+": "+strings.Join(what, ", "),
			map[string]any{"role": newRole, "state": newState})
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "role": newRole, "state": newState,
		"perms": Who{Role: newRole, Perms: perms}.Effective(), "max_rating": maxRating, "rating_country": country})
}

func (a *Auth) resetUserTwoFactor(w http.ResponseWriter, r *http.Request) {
	if !RequireStrong(w, r) {
		return
	}
	by, _ := From(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, "no such user")
		return
	}
	var role string
	if err := a.s.Read().QueryRowContext(r.Context(), "SELECT role FROM users WHERE id = ?", id).Scan(&role); err != nil {
		fail(w, http.StatusNotFound, "no such user")
		return
	}
	if role == RoleOwner || (by.Role != RoleOwner && role != RoleMember) {
		fail(w, http.StatusForbidden, "you may reset two-factor sign-in only for people below you; the owner uses the reset-owner command")
		return
	}
	err = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		if err := clearTwoFactor(tx, id); err != nil {
			return err
		}
		_, err := tx.Exec("DELETE FROM sessions WHERE user_id = ?", id)
		return err
	})
	if err != nil {
		serverError(w, "reset two-factor", err)
		return
	}
	log.Printf("auth: two-factor sign-in for user %d reset by user %d", id, by.UserID)
	a.accountEvent(r, id, "two_factor_reset", byName(r)+" reset two-factor sign-in for "+a.userName(r.Context(), id), nil)
	w.WriteHeader(http.StatusNoContent)
}

// setUserPassword lets the owner or an admin give someone below them a new
// password when they forgot it. The person is signed out everywhere and their
// API keys stop, the same as a turned-off account.
func (a *Auth) setUserPassword(w http.ResponseWriter, r *http.Request) {
	if !RequireStrong(w, r) {
		return
	}
	by, _ := From(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, "no such user")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, `send {"password": "..."}`)
		return
	}
	var role, permsRaw string
	if err := a.s.Read().QueryRowContext(r.Context(), "SELECT role, perms FROM users WHERE id = ?", id).Scan(&role, &permsRaw); err != nil {
		fail(w, http.StatusNotFound, "no such user")
		return
	}
	// Signing in as them must not give an admin more than they already have.
	if err := mayGrant(by, role, parsePerms(permsRaw)); err != nil && role == RoleMember {
		fail(w, http.StatusForbidden, "they may do things you may not, so only the owner can set their password")
		return
	}
	if id == by.UserID {
		fail(w, http.StatusForbidden, "change your own password from your account")
		return
	}
	if role == RoleOwner || (by.Role != RoleOwner && role != RoleMember) {
		fail(w, http.StatusForbidden, "you may set a password only for people below you; the owner uses the reset-owner command")
		return
	}
	if msg := passwordProblem(body.Password, MinPasswordLength(r.Context(), a.s.Read())); msg != "" {
		fail(w, http.StatusBadRequest, msg)
		return
	}
	h, err := HashPassword(body.Password)
	if err != nil {
		serverError(w, "set password", err)
		return
	}
	err = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?", h, store.FormatTime(a.now()), id); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM sessions WHERE user_id = ?", id); err != nil {
			return err
		}
		if err := endOutsideLinks(tx, id); err != nil {
			return err
		}
		_, err := tx.Exec("DELETE FROM api_keys WHERE owner_user_id = ?", id)
		return err
	})
	if err != nil {
		serverError(w, "set password", err)
		return
	}
	log.Printf("auth: password for user %d set by user %d", id, by.UserID)
	a.accountEvent(r, id, "password_set", byName(r)+" set a new password for "+a.userName(r.Context(), id)+"; signed out, API keys and outside sign-in links removed", nil)
	w.WriteHeader(http.StatusNoContent)
}

// InviteView is an invite without its link.
type InviteView struct {
	ID        string  `json:"id"`
	Role      string  `json:"role"`
	Perms     []Perm  `json:"perms"`
	Note      string  `json:"note"`
	CreatedBy string  `json:"created_by"`
	CreatedAt string  `json:"created_at"`
	ExpiresAt string  `json:"expires_at"`
	UsedAt    *string `json:"used_at,omitempty"`
	UsedBy    *string `json:"used_by,omitempty"`
	// MaxRating is the age-rating limit the new person gets ("" for none).
	MaxRating     string `json:"max_rating"`
	RatingCountry string `json:"rating_country"`
}

func (a *Auth) listInvites(w http.ResponseWriter, r *http.Request) {
	rows, err := a.s.Read().QueryContext(r.Context(), `SELECT i.hash, i.role, i.perms, i.note, c.name, i.created_at, i.expires_at, i.used_at, u.name,
		i.max_rating, i.rating_country
		FROM invites i JOIN users c ON c.id = i.created_by LEFT JOIN users u ON u.id = i.used_by ORDER BY i.created_at DESC LIMIT 200`)
	if err != nil {
		serverError(w, "invites", err)
		return
	}
	defer rows.Close()
	out := []InviteView{}
	for rows.Next() {
		var v InviteView
		var perms string
		var usedAt, usedBy sql.NullString
		if err := rows.Scan(&v.ID, &v.Role, &perms, &v.Note, &v.CreatedBy, &v.CreatedAt, &v.ExpiresAt, &usedAt, &usedBy, &v.MaxRating, &v.RatingCountry); err != nil {
			serverError(w, "invites", err)
			return
		}
		v.Perms = parsePerms(perms)
		v.RatingCountry = countryOr(v.RatingCountry)
		if usedAt.Valid {
			v.UsedAt = &usedAt.String
		}
		if usedBy.Valid {
			v.UsedBy = &usedBy.String
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"invites": out})
}

func (a *Auth) makeInvite(w http.ResponseWriter, r *http.Request) {
	if !RequireStrong(w, r) {
		return
	}
	by, _ := From(r.Context())
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
		body.Role = RoleMember
	}
	if body.Role != RoleAdmin && body.Role != RoleMember {
		fail(w, http.StatusBadRequest, "role must be admin or member")
		return
	}
	perms := DefaultPerms(body.Role)
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
	if body.Role == RoleAdmin && !mayMakeAdmin(w, by) {
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
		_, err := tx.Exec(`INSERT INTO invites(hash, created_at, created_by, role, perms, note, expires_at, max_rating, rating_country)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			sum(tok), store.FormatTime(now), by.UserID, body.Role, permsJSON(perms), body.Note, store.FormatTime(expires), maxRating, country)
		return err
	})
	if err != nil {
		serverError(w, "invite", err)
		return
	}
	log.Printf("auth: user %d made a %s invite", by.UserID, body.Role)
	a.accountEvent(r, by.UserID, "invite_made", byName(r)+" made an invite for a new "+body.Role, map[string]any{"role": body.Role, "days": body.Days})
	writeJSON(w, http.StatusCreated, map[string]any{"id": sum(tok), "path": invitePath + tok, "role": body.Role, "perms": perms,
		"expires_at": store.FormatTime(expires), "note": "This link is shown only now and works once.",
		"max_rating": maxRating, "rating_country": country})
}

func (a *Auth) deleteInvite(w http.ResponseWriter, r *http.Request) {
	if !RequireStrong(w, r) {
		return
	}
	by, _ := From(r.Context())
	var n int64
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		// Admins may take back only their own invites.
		res, err := tx.Exec("DELETE FROM invites WHERE hash = ? AND used_at IS NULL AND (? OR created_by = ?)",
			r.PathValue("id"), by.Role == RoleOwner, by.UserID)
		if err == nil {
			n, err = res.RowsAffected()
		}
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

func (a *Auth) openInvite(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, tok string) (role, perms, note, maxRating, country string, ok bool) {
	var expires string
	// The person who made the invite must still be an active owner or admin.
	err := q.QueryRowContext(ctx, `SELECT i.role, i.perms, i.note, i.expires_at, i.max_rating, i.rating_country FROM invites i
		JOIN users c ON c.id = i.created_by
		WHERE i.hash = ? AND i.used_at IS NULL AND c.state = 'active' AND c.role IN ('owner', 'admin')`, sum(tok)).
		Scan(&role, &perms, &note, &expires, &maxRating, &country)
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

var errInviteGone = errors.New("invite gone")
var errNameTaken = errors.New("name taken")

func (a *Auth) acceptInvite(w http.ResponseWriter, r *http.Request) {
	key, ok := a.inviteGate(w, r)
	if !ok {
		return
	}
	c, ok := readCreds(w, r)
	if !ok {
		return
	}
	if c.Name == "" || len(c.Name) > 64 {
		fail(w, http.StatusBadRequest, "pick a name of 1 to 64 characters")
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
		var role, perms, expires, maxRating, country, byRole, byPerms string
		err := tx.QueryRow(`SELECT i.role, i.perms, i.expires_at, i.max_rating, i.rating_country, c.role, c.perms FROM invites i JOIN users c ON c.id = i.created_by
			WHERE i.hash = ? AND i.used_at IS NULL AND c.state = 'active' AND c.role IN ('owner', 'admin')`, sum(tok)).
			Scan(&role, &perms, &expires, &maxRating, &country, &byRole, &byPerms)
		if err != nil || !a.now().Before(parseTime(expires)) {
			return errInviteGone
		}
		// The link gives only what its maker still holds today.
		perms = permsJSON(slices.DeleteFunc(parsePerms(perms), func(p Perm) bool {
			return !(Who{Role: byRole, Perms: parsePerms(byPerms)}).Has(p)
		}))
		var n int
		if err := tx.QueryRow("SELECT count(*) FROM users WHERE name = ?", c.Name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return errNameTaken
		}
		id, err = store.Insert(tx, "users", &domain.User{Name: c.Name, Role: role, PasswordHash: h, State: "active", Perms: perms,
			MaxRating: maxRating, RatingCountry: countryOr(country)}, domain.System)
		if err != nil {
			return err
		}
		res, err := tx.Exec("UPDATE invites SET used_at = ?, used_by = ? WHERE hash = ? AND used_at IS NULL", store.FormatTime(a.now()), id, sum(tok))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return errInviteGone
		}
		return nil
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

// changePassword sets a new password for the signed-in person after checking
// the current one; every other session of theirs ends.
func (a *Auth) changePassword(w http.ResponseWriter, r *http.Request) {
	if !RequireStrong(w, r) {
		return
	}
	who, _ := From(r.Context())
	var body struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, `send {"current": "...", "new": "..."}`)
		return
	}
	if msg := passwordProblem(body.New, MinPasswordLength(r.Context(), a.s.Read())); msg != "" {
		fail(w, http.StatusBadRequest, msg)
		return
	}
	key := "pw:" + itoa(who.UserID)
	a.mu.Lock()
	wait := a.waitFor(key, freeTries)
	if wait == 0 {
		a.noteFail(key)
	}
	a.mu.Unlock()
	if wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		fail(w, http.StatusTooManyRequests, "too many wrong passwords; wait a little and try again")
		return
	}
	var cur string
	if err := a.s.Read().QueryRowContext(r.Context(), "SELECT password_hash FROM users WHERE id = ?", who.UserID).Scan(&cur); err != nil {
		serverError(w, "password", err)
		return
	}
	// People who only sign in through a media server have no password yet
	// and may set one. Changing one that exists needs a recent local proof,
	// on purpose also for API keys: a key never changes its user's password.
	first := cur == ""
	if !first && !RecentLocal(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "confirm your local password again before using this route", "code": "reauth"})
		return
	}
	if !first && !CheckPassword(cur, body.Current) {
		a.failedEvent(r, who.Name, "current password")

		fail(w, http.StatusUnauthorized, "the current password is wrong")
		return
	}
	a.mu.Lock()
	delete(a.fails, key)
	a.mu.Unlock()
	h, err := hashPassword(body.New, a.hash)
	if err != nil {
		serverError(w, "password", err)
		return
	}
	keep := ""
	if ck, err := r.Cookie(CookieName); err == nil {
		keep = sum(ck.Value)
	}
	err = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?", h, store.FormatTime(a.now()), who.UserID); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM sessions WHERE user_id = ? AND hash != ?", who.UserID, keep); err != nil {
			return err
		}
		// A key or outside sign-in linked by someone who knew the old
		// password must stop too. A first password has no old one: the
		// media server sign-in they joined with stays.
		if !first {
			if err := endOutsideLinks(tx, who.UserID); err != nil {
				return err
			}
		}
		_, err := tx.Exec("DELETE FROM api_keys WHERE owner_user_id = ?", who.UserID)
		return err
	})
	if err != nil {
		serverError(w, "password", err)
		return
	}
	if first {
		log.Printf("auth: user %d set a first password", who.UserID)
		a.accountEvent(r, who.UserID, "password_changed", who.Name+" set a first password; other sessions and API keys removed, media server sign-in kept", nil)
	} else {
		log.Printf("auth: user %d changed their password", who.UserID)
		a.accountEvent(r, who.UserID, "password_changed", who.Name+" changed their password; other sessions, API keys and outside sign-in links removed", nil)
	}

	w.WriteHeader(http.StatusNoContent)
}

// samePerms says two permission lists hold the same permissions, in any order.
func samePerms(x, y []Perm) bool {
	if len(x) != len(y) {
		return false
	}
	for _, p := range x {
		if !slices.Contains(y, p) {
			return false
		}
	}
	return true
}
