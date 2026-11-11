package accounts

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Each person's Plex token: kept sealed in the vault on their Plex
// link, from a Plex sign-in or a link made in My account, so Requests can read
// their Plex watchlist. It never leaves the server. Whatever removes a link or
// keeps a newer token removes the old vault row in the same write.

const plexTokenKind = "plex_token"

// PlexClientID is this HolyArrs's id toward Plex, the one its tokens belong to.
func (a *Auth) PlexClientID(ctx context.Context) string { return a.clientID(ctx) }

// PlexToken is the Plex token kept for userID, or "" when none is kept, the
// person has no Plex link or is turned off. The owner's token from the
// Settings link (kept for importing people) counts for the owner.
func (a *Auth) PlexToken(ctx context.Context, userID int64) (string, error) {
	if a.vault == nil {
		return "", nil
	}
	ref, role, state, err := plexLinkRow(ctx, a.s.Read(), userID)
	if errors.Is(err, sql.ErrNoRows) || err == nil && state != "active" {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if ref.Valid {
		return a.vault.Get(ctx, ref.Int64)
	}
	if role == webserver.RoleOwner {
		return a.vault.Setting(ctx, settingsGroup, signInPrefix+keyPlexOwner)
	}
	return "", nil
}

// PlexOwnerLink is the owner's Plex link from Settings, which Requests reads
// Plex friends' watchlists with: its token ("" when none is kept) and the
// server people must share ("" for any).
func (a *Auth) PlexOwnerLink(ctx context.Context) (token, server string) {
	if a.vault == nil {
		return "", ""
	}
	tok, err := a.vault.Setting(ctx, settingsGroup, signInPrefix+keyPlexOwner)
	if err != nil || tok == "" {
		return "", ""
	}
	return tok, a.way(ctx, ProviderPlex).ServerID
}

func (a *Auth) plexMineRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/auth/plex/my-link", a.myPlexLink)
	m.HandleFunc("POST /api/v1/auth/plex/my-link/start", a.myPlexLinkStart)
	m.HandleFunc("POST /api/v1/auth/plex/my-link/finish", a.myPlexLinkFinish)
	m.HandleFunc("DELETE /api/v1/auth/plex/my-link", a.myPlexUnlink)
}

// keepPlexToken puts a newer token on an existing Plex link after a sign-in.
// It never blocks the sign-in: a failure is logged and the old token stays.
func (a *Auth) keepPlexToken(ctx context.Context, userID int64, extID, tok string) {
	if a.vault == nil || tok == "" {
		return
	}
	ref, err := plexTokenRef(ctx, a.s.Read(), extID, userID)
	if err != nil {
		return
	}
	if ref.Valid {
		if cur, err := a.vault.Get(ctx, ref.Int64); err == nil && cur == tok {
			return
		}
	}
	if err := a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		ids, err := plexTokenIDsOfLink(ctx, tx, extID, userID)
		if err != nil {
			return err
		}
		if err := a.vault.DeleteTx(ctx, tx, ids...); err != nil {
			return err
		}
		id, err := a.vault.PutTx(tx, plexTokenKind, tok, domain.System)
		if err != nil {
			return err
		}
		n, err := setPlexTokenRef(ctx, tx, id, extID, userID)
		if err != nil {
			return err
		}
		if n == 0 {
			return sql.ErrNoRows // the link went meanwhile: keep nothing
		}
		return nil
	}); err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("auth: keeping the Plex sign-in of user %d: %v", userID, err)
	}
}

// linkPlexTx links Plex account extID to userID with its token, replacing the
// person's earlier Plex link. errTaken: the account belongs to someone else.
// moved says an earlier, different Plex account was replaced: the devices it
// signed in are signed out here, and the caller drops its two-factor steps.
func (a *Auth) linkPlexTx(tx *sql.Tx, userID int64, extID, name, tok string, by domain.Actor) (moved bool, err error) {
	ctx := context.Background()
	other, err := plexLinkOwner(ctx, tx, extID)
	if err == nil && other != userID {
		return false, errTaken
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	before, err := plexExternalID(ctx, tx, userID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if moved = before != "" && before != extID; moved {
		if _, err := deleteSessionsByMethod(ctx, tx, userID, ProviderPlex); err != nil {
			return false, err
		}
	}
	ids, err := plexTokenIDsOfUser(ctx, tx, userID)
	if err != nil {
		return false, err
	}
	if err := a.vault.DeleteTx(ctx, tx, ids...); err != nil {
		return false, err
	}
	if _, err := deletePlexLink(ctx, tx, userID); err != nil {
		return false, err
	}
	var ref any
	if a.vault != nil && tok != "" {
		id, err := a.vault.PutTx(tx, plexTokenKind, tok, by)
		if err != nil {
			return false, err
		}
		ref = id
	}
	err = insertPlexLink(ctx, tx, userID, extID, truncate(name, 200), database.FormatTime(a.now()), ref)
	return moved, err
}

// myPlexLink says whether my Plex account is linked, as whom, and whether
// its sign-in is kept (so my watchlist can be read).
func (a *Auth) myPlexLink(w http.ResponseWriter, r *http.Request) {
	who, ok := webserver.From(r.Context())
	if !ok || who.UserID == 0 {
		fail(w, http.StatusForbidden, "sign in first")
		return
	}
	name, ref, err := plexLinkName(r.Context(), a.s.Read(), who.UserID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		serverError(w, "plex link", err)
		return
	}
	kept := ref.Valid
	if err == nil && !kept && who.Role == webserver.RoleOwner && a.vault != nil {
		v, _ := a.vault.Setting(r.Context(), settingsGroup, signInPrefix+keyPlexOwner)
		kept = v != ""
	}
	writeJSON(w, http.StatusOK, map[string]any{"linked": err == nil, "name": name, "kept": kept})
}

func (a *Auth) myPlexLinkStart(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSSOLink(w, r); !ok {
		return
	}
	if a.vault == nil {
		fail(w, http.StatusServiceUnavailable, "the secret store is not open")
		return
	}
	a.startPin(w, r, pinMine)
}

func (a *Auth) myPlexLinkFinish(w http.ResponseWriter, r *http.Request) {
	who, ok := a.requireSSOLink(w, r)
	if !ok {
		return
	}
	if a.vault == nil {
		fail(w, http.StatusServiceUnavailable, "the secret store is not open")
		return
	}
	ctx := r.Context()
	tok, ok := a.checkPin(w, r, pinMine)
	if !ok {
		return
	}
	hdr := a.plexHeaders(ctx)
	hdr["X-Plex-Token"] = tok
	var user struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	}
	code, err := a.fetch(ctx, http.MethodGet, a.plexURL+"/api/v2/user", hdr, nil, &user)
	if err != nil || code/100 != 2 || user.ID == 0 {
		log.Printf("auth: plex my-link user: status %d: %v", code, err)
		fail(w, http.StatusBadGateway, "Plex did not say who you are; try again")
		return
	}
	// The Plex calls above took a while: inside the write, the session must
	// still be there and fresh, so a password change or reset meanwhile wins.
	cookie, _ := r.Cookie(CookieName)
	proof := ssoLinkProof{family: a.currentFamily(r), userID: who.UserID}
	if cookie != nil && cookie.Value != "" {
		proof.hash = sum(cookie.Value)
	}
	me := webserver.Actor(ctx)
	var moved bool
	err = a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		if proof.hash == "" || proof.family == "" {
			return errNotAllowed
		}
		if err := a.linkSessionFreshTx(tx, proof); err != nil {
			return err
		}
		var err error
		moved, err = a.linkPlexTx(tx, who.UserID, strconv.FormatInt(user.ID, 10), user.Username, tok, me)
		return err
	})
	switch {
	case errors.Is(err, errTaken):
		fail(w, http.StatusConflict, "this Plex account is already linked to another HolyArrs user")
		return
	case errors.Is(err, errNotAllowed):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "confirm your local password again before linking", "code": "reauth"})
		return
	case err != nil:
		serverError(w, "plex link", err)
		return
	}
	if moved {
		a.dropTickets(func(t *ticket) bool { return t.userID == who.UserID && t.method == ProviderPlex })
	}
	if who.Role == webserver.RoleOwner {
		// The owner's import of people uses the same Plex account.
		if err := a.vault.SetSetting(ctx, settingsGroup, signInPrefix+keyPlexOwner, tok, me); err != nil {
			serverError(w, "plex link", err)
			return
		}
	}
	log.Printf("auth: user %d linked a Plex account from My account", who.UserID)
	a.accountEvent(r, who.UserID, "sign_in_linked", byName(r)+" linked the Plex account "+outsideName(user.Username), map[string]any{"provider": "plex"})
	writeJSON(w, http.StatusOK, map[string]any{"linked": true, "name": user.Username, "kept": true})
}

// myPlexUnlink removes my Plex link and its kept token and ends the sessions
// Plex signed in; refused when it is my only way to sign in.
func (a *Auth) myPlexUnlink(w http.ResponseWriter, r *http.Request) {
	who, ok := webserver.From(r.Context())
	if !ok || who.UserID == 0 || !RecentLocal(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "confirm your local password again before using this route", "code": "reauth"})
		return
	}
	ctx := r.Context()
	var ended int64
	err := a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		hash, err := passwordHashOf(ctx, tx, who.UserID)
		if err != nil {
			return err
		}
		others, err := otherWaysIn(ctx, tx, who.UserID)
		if err != nil {
			return err
		}
		if hash == "" && others == 0 {
			return errNoWayIn
		}
		ids, err := plexTokenIDsOfUser(ctx, tx, who.UserID)
		if err != nil {
			return err
		}
		if err := a.vault.DeleteTx(ctx, tx, ids...); err != nil {
			return err
		}
		n, err := deletePlexLink(ctx, tx, who.UserID)
		if err != nil {
			return err
		}
		if n == 0 {
			return sql.ErrNoRows
		}
		ended, err = deleteSessionsByMethod(ctx, tx, who.UserID, ProviderPlex)
		return err
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		fail(w, http.StatusNotFound, "no Plex account is linked")
		return
	case errors.Is(err, errNoWayIn):
		fail(w, http.StatusConflict, "this is your only way to sign in; set a HolyArrs password first")
		return
	case err != nil:
		serverError(w, "plex unlink", err)
		return
	}
	a.dropTickets(func(t *ticket) bool { return t.userID == who.UserID && t.method == ProviderPlex })
	if who.Role == webserver.RoleOwner && a.vault != nil {
		if err := a.vault.SetSetting(ctx, settingsGroup, signInPrefix+keyPlexOwner, "", webserver.Actor(ctx)); err != nil {
			serverError(w, "plex unlink", err)
			return
		}
	}
	a.accountEvent(r, who.UserID, "sign_in_unlinked", byName(r)+" unlinked their Plex account", map[string]any{"provider": "plex", "sessions_ended": ended})
	w.WriteHeader(http.StatusNoContent)
}
