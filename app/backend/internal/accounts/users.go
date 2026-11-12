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

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Sign-in settings live in the this_app group with keys starting sign_in_.
const (
	settingsGroup = "this_app"
	signInPrefix  = "sign_in_"
)

const errSkipLocalKid = "Turn off skipping sign-in on the local network first: with it on, anyone at home signs in as the owner."

// UserView is a user as the Users screen shows them.
type UserView struct {
	ID        int64            `json:"id"`
	Name      string           `json:"name"`
	Role      string           `json:"role"`
	State     string           `json:"state"`
	Perms     []webserver.Perm `json:"perms"`
	TwoFactor bool             `json:"two_factor"`
	Password  bool             `json:"password"` // signs in with a HolyArrs password
	Links     []LinkView       `json:"links"`
	SSOLinks  []SSOLinkView    `json:"sso_links"` // single sign-on and trusted proxy identities
	CreatedAt string           `json:"created_at"`
	Avatar    *string          `json:"avatar"` // the picture's address, or null
	LastSeen  *string          `json:"last_seen_at,omitempty"`
	// MaxRating is a Kid's age-rating limit ("" for no limit).
	MaxRating     string `json:"max_rating"`
	RatingCountry string `json:"rating_country"`
}

// LinkView is an outside account linked to a user.
type LinkView struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
}

// SkipLocal says sign-in is skipped on the local network for someone who is
// not a Kid, so a Kid account at home would mean nothing.
func (a *Auth) SkipLocal() bool { return a.skipBlocksKids(context.Background()) }

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

// signInSetting reads one sign-in setting; "" when unset.
func (a *Auth) signInSetting(ctx context.Context, key string) string {
	return database.SettingOr(ctx, a.s.Read(), settingsGroup, signInPrefix+key, "")
}

// countryOr reads an empty rating country as US.
func countryOr(c string) string {
	if c = metadata.NormRating(c); c == "" {
		return "US"
	}
	return c
}

// kidLimit checks an age-rating limit asked for a person of role and
// returns it as stored, or an error in plain words for a 400. A limit is
// refused while sign-in is skipped on the local network for someone who is
// not a Kid (skipLocal).
func kidLimit(role, max, country string, perms []webserver.Perm, skipLocal bool) (string, string, error) {
	max, country = metadata.NormRating(max), countryOr(country)
	if !metadata.ValidRating(max) {
		return "", "", fmt.Errorf("unknown age rating %q: pick one of %s, or leave it empty for no limit", max, metadata.RatingChoices())
	}
	if !metadata.ValidRatingCountry(country) {
		return "", "", errors.New("Only US age ratings are known for now.")
	}
	if max != "" && role != webserver.RoleMember {
		return "", "", errors.New("Only members can have an age-rating limit.")
	}
	if max != "" && skipLocal {
		return "", "", errors.New(errSkipLocalKid)
	}
	if max != "" {
		// A Kid may only ask: any other permission would open routes
		// (live events, downloads, history, jobs) the limit does not cover.
		for _, p := range perms {
			if p != webserver.PermAsk && p != webserver.PermAutoApprove && p != webserver.PermAutoApproveFourK {
				return "", "", errors.New("A Kid account can only ask for titles.")
			}
		}
	}
	return max, country, nil
}

func serverError(w http.ResponseWriter, what string, err error) {
	log.Printf("auth: %s: %v", what, err)
	fail(w, http.StatusInternalServerError, "something went wrong; see the log")
}

func (a *Auth) listUsers(w http.ResponseWriter, r *http.Request) {
	out, err := listUserViews(r.Context(), a.s.Read())
	if err != nil {
		serverError(w, "users", err)
		return
	}
	byID := map[int64]int{}
	for i, u := range out {
		byID[u.ID] = i
	}
	if err := eachUserLink(r.Context(), a.s.Read(), func(id int64, l LinkView) {
		if i, ok := byID[id]; ok {
			out[i].Links = append(out[i].Links, l)
		}
	}); err != nil {
		serverError(w, "users", err)
		return
	}
	if err := eachSSOLink(r.Context(), a.s.Read(), func(id int64, l SSOLinkView) {
		if i, ok := byID[id]; ok {
			out[i].SSOLinks = append(out[i].SSOLinks, l)
		}
	}); err != nil {
		serverError(w, "users", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out, "perms": webserver.AllPerms})
}

// mayGrant checks the permissions an owner or admin hands out: an admin may
// only give what they hold, and only to members.
func mayGrant(by webserver.Caller, role string, perms []webserver.Perm) error {
	if by.Role == webserver.RoleOwner {
		return nil
	}
	if role != webserver.RoleMember {
		return errors.New("only the owner can make admins")
	}
	for _, p := range perms {
		if !by.Has(p) {
			return fmt.Errorf("you cannot give a permission you do not have (%s)", p)
		}
	}
	return nil
}

func readPerms(raw []string) ([]webserver.Perm, error) {
	out := []webserver.Perm{}
	for _, s := range raw {
		p := webserver.Perm(s)
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
func mayMakeAdmin(w http.ResponseWriter, by webserver.Caller) bool {
	if by.Via == "api_key" && !webserver.ScopesCover(by.Scopes, webserver.ScopeControl) {
		fail(w, http.StatusForbidden, "making an admin with an API key needs the control scope")
		return false
	}
	return true
}

func (a *Auth) editUser(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	by, _ := webserver.From(r.Context())
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
	role, state, permsRaw, maxRating, country, err := userForEdit(r.Context(), a.s.Read(), id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, http.StatusNotFound, "no such user")
		return
	}
	if err != nil {
		serverError(w, "edit user", err)
		return
	}
	if role == webserver.RoleOwner {
		fail(w, http.StatusForbidden, "the owner cannot be changed here; use the reset-owner command to change the owner")
		return
	}
	oldRating, oldCountry := maxRating, country
	if by.Role != webserver.RoleOwner && role != webserver.RoleMember {
		fail(w, http.StatusForbidden, "only the owner can change an admin")
		return
	}
	if id == by.UserID {
		fail(w, http.StatusForbidden, "you cannot change your own role or permissions")
		return
	}
	newRole, newState, perms := role, state, parsePerms(permsRaw)
	if body.Role != nil {
		if *body.Role != webserver.RoleAdmin && *body.Role != webserver.RoleMember {
			fail(w, http.StatusBadRequest, "role must be admin or member")
			return
		}
		if *body.Role != role {
			perms = webserver.DefaultPerms(*body.Role)
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
	} else if newRole != webserver.RoleMember {
		maxRating = "" // making someone an admin clears their limit
	}
	if body.RatingCountry != nil {
		country = *body.RatingCountry
	}
	if maxRating, country, err = kidLimit(newRole, maxRating, country, perms, a.skipBlocksKids(r.Context())); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if newRole == webserver.RoleAdmin && role != webserver.RoleAdmin && !mayMakeAdmin(w, by) {
		return
	}
	if err := mayGrant(by, newRole, perms); err != nil {
		fail(w, http.StatusForbidden, err.Error())
		return
	}
	actor, _ := json.Marshal(webserver.Actor(r.Context()))
	err = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		if err := updateUserRow(r.Context(), tx, newRole, permsJSON(perms), newState, maxRating, country, database.FormatTime(a.now()), string(actor), id); err != nil {
			return err
		}
		if newState != "active" {
			// A disabled person is signed out everywhere and their keys stop.
			if err := deleteSessionsOf(r.Context(), tx, id); err != nil {
				return err
			}
			return deleteKeysOf(r.Context(), tx, id)
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
		"perms": webserver.Caller{Role: newRole, Perms: perms}.Effective(), "max_rating": maxRating, "rating_country": country})
}

func (a *Auth) resetUserTwoFactor(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	by, _ := webserver.From(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, "no such user")
		return
	}
	role, err := roleOfUser(r.Context(), a.s.Read(), id)
	if err != nil {
		fail(w, http.StatusNotFound, "no such user")
		return
	}
	if role == webserver.RoleOwner || (by.Role != webserver.RoleOwner && role != webserver.RoleMember) {
		fail(w, http.StatusForbidden, "you may reset two-factor sign-in only for people below you; the owner uses the reset-owner command")
		return
	}
	err = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		if err := clearTwoFactor(r.Context(), tx, a.vault, id); err != nil {
			return err
		}
		return deleteSessionsOf(r.Context(), tx, id)
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
	if !webserver.RequireStrong(w, r) {
		return
	}
	by, _ := webserver.From(r.Context())
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
	role, permsRaw, err := roleAndPerms(r.Context(), a.s.Read(), id)
	if err != nil {
		fail(w, http.StatusNotFound, "no such user")
		return
	}
	// Signing in as them must not give an admin more than they already have.
	if err := mayGrant(by, role, parsePerms(permsRaw)); err != nil && role == webserver.RoleMember {
		fail(w, http.StatusForbidden, "they may do things you may not, so only the owner can set their password")
		return
	}
	if id == by.UserID {
		fail(w, http.StatusForbidden, "change your own password from your account")
		return
	}
	if role == webserver.RoleOwner || (by.Role != webserver.RoleOwner && role != webserver.RoleMember) {
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
		if err := setPasswordHash(r.Context(), tx, h, database.FormatTime(a.now()), id); err != nil {
			return err
		}
		if err := deleteSessionsOf(r.Context(), tx, id); err != nil {
			return err
		}
		if err := endOutsideLinks(r.Context(), tx, a.vault, id); err != nil {
			return err
		}
		return deleteKeysOf(r.Context(), tx, id)
	})
	if err != nil {
		serverError(w, "set password", err)
		return
	}
	log.Printf("auth: password for user %d set by user %d", id, by.UserID)
	a.accountEvent(r, id, "password_set", byName(r)+" set a new password for "+a.userName(r.Context(), id)+"; signed out, API keys and outside sign-in links removed", nil)
	w.WriteHeader(http.StatusNoContent)
}

// changePassword sets a new password for the signed-in person after checking
// the current one; every other session of theirs ends.
func (a *Auth) changePassword(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	who, _ := webserver.From(r.Context())
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
	cur, err := passwordHashOf(r.Context(), a.s.Read(), who.UserID)
	if err != nil {
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
		if err := setPasswordHash(r.Context(), tx, h, database.FormatTime(a.now()), who.UserID); err != nil {
			return err
		}
		if err := deleteSessionsExceptHash(r.Context(), tx, who.UserID, keep); err != nil {
			return err
		}
		// A key or outside sign-in linked by someone who knew the old
		// password must stop too. A first password has no old one: the
		// media server sign-in they joined with stays.
		if !first {
			if err := endOutsideLinks(r.Context(), tx, a.vault, who.UserID); err != nil {
				return err
			}
		}
		return deleteKeysOf(r.Context(), tx, who.UserID)
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
func samePerms(x, y []webserver.Perm) bool {
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
