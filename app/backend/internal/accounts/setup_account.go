package accounts

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// setupOwnerOpen says whether the request is the owner's browser session
// while first-run setup is still open, with the setup's proof. It answers
// 403 itself when not.
func (a *Auth) setupOwnerOpen(w http.ResponseWriter, r *http.Request) (webserver.Caller, bool) {
	who, ok := webserver.From(r.Context())
	if !ok || who.Via != "session" || who.Role != webserver.RoleOwner || !RecentLocal(r) ||
		database.SettingOr(r.Context(), a.s.Read(), settingsGroup, settingFirstRunDone, "") == "1" {
		fail(w, http.StatusForbidden, "only the owner does this, during first-run setup")
		return webserver.Caller{}, false
	}
	return who, true
}

// setupLink links a Jellyfin or Emby admin account to the owner during
// first-run setup ("Link your media servers"): an extra way to sign in, and
// the API key that sign-in makes is kept for the Media server step.
func (a *Auth) setupLink(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if kind != ProviderJellyfin && kind != ProviderEmby {
		fail(w, http.StatusNotFound, "no such sign-in way")
		return
	}
	who, ok := a.setupOwnerOpen(w, r)
	if !ok {
		return
	}
	addr, user, ok := a.setupServerSignIn(w, r, kind)
	if !ok {
		return
	}
	ctx := r.Context()
	err := a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		return a.linkSetupServerTx(ctx, tx, who.UserID, kind, addr, user)
	})
	if a.setupFailed(w, err, "this "+kindWord[kind]+" account") {
		return
	}
	a.accountEvent(r, who.UserID, "sign_in_linked", byName(r)+" linked the "+kind+" account "+outsideName(user.Name), map[string]any{"provider": kind})
	writeJSON(w, http.StatusOK, map[string]string{"provider": kind, "name": user.Name, "url": addr})
}

// setupAccount lets the owner change the username, display name and,
// when sent, the password made on the account step, while first-run setup
// is still open (Back from the first wizard step).
func (a *Auth) setupAccount(w http.ResponseWriter, r *http.Request) {
	who, ok := a.setupOwnerOpen(w, r)
	if !ok {
		return
	}
	var in struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if !webserver.Decode(w, r, &in, `send {"username": "...", "display_name": "...", "password": "..."}`) {
		return
	}
	name := strings.TrimSpace(in.Username)
	if msg := typedNameProblem(name); msg != "" {
		fail(w, http.StatusBadRequest, msg)
		return
	}
	display, ok := cleanDisplayName(in.DisplayName)
	if !ok {
		fail(w, http.StatusBadRequest, "pick a display name of up to 64 characters")
		return
	}
	hash := ""
	if in.Password != "" {
		if msg := passwordProblem(in.Password, MinPasswordLength(r.Context(), a.s.Read())); msg != "" {
			fail(w, http.StatusBadRequest, msg)
			return
		}
		h, err := hashPassword(in.Password, a.hash)
		if err != nil {
			webserver.Internal(w, "accounts", err)
			return
		}
		hash = h
	}
	ctx := r.Context()
	errTaken := errors.New("taken")
	err := a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		taken, err := userNameTakenByOther(ctx, tx, name, who.UserID)
		if err != nil {
			return err
		}
		if taken {
			return errTaken
		}
		now := database.FormatTime(a.now())
		if err := setUserName(ctx, tx, name, now, who.UserID); err != nil {
			return err
		}
		if hash != "" {
			if err := setPasswordHash(ctx, tx, hash, now, who.UserID); err != nil {
				return err
			}
			// A new password ends every other sign-in of the owner, as a normal
			// password change does; this browser stays signed in.
			keep := ""
			if ck, err := sessionCookie(r); err == nil {
				keep = sum(ck.Value)
			}
			if err := deleteSessionsExceptHash(ctx, tx, who.UserID, keep); err != nil {
				return err
			}
		}
		return putDisplayName(ctx, tx, who.UserID, display)
	})
	if errors.Is(err, errTaken) {
		fail(w, http.StatusConflict, "that username is taken")
		return
	}
	if err != nil {
		webserver.Internal(w, "accounts", err)
		return
	}
	a.accountEvent(r, who.UserID, "setup_account_changed", name+" changed their account during setup", nil)
	writeJSON(w, http.StatusOK, map[string]string{"username": name, "display_name": display})
}
