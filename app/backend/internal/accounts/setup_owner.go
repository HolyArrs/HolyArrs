package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

var errOwnerExists = errors.New("owner exists")

var errNoLocalSignIn = errors.New("signs in only through an outside account")

type creds struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	Code     string `json:"code,omitempty"` // ignored; first run needs no code
	// First run only: the owner's display name, saved with the account.
	DisplayName string `json:"display_name,omitempty"`
}

func (a *Auth) owner(ctx context.Context) (int64, string, bool) {
	id, name, err := ownerIdentity(ctx, a.s.Read())
	return id, name, err == nil
}

func (a *Auth) state(w http.ResponseWriter, r *http.Request) {
	_, _, ok := a.owner(r.Context())
	out := map[string]any{"needs_setup": !ok, "ways": a.ways(r.Context()), "safe_mode": a.safeMode,
		"min_password_length": MinPasswordLength(r.Context(), a.s.Read())}
	if a.configUnmapped {
		out["config_unmapped"] = true
	}
	// Opened by a name saves are refused on: say so on arrival, not on the
	// first save.
	if !a.hostAllowed(r) {
		out["host_refused"] = echoHost(r.Host)
	}
	if ok && (a.resetLinkOn(r.Context()) || a.envReset) {
		out["owner_reset"] = true
	}
	if a.envReset {
		out["owner_reset_env"] = true
	}
	// The owner exists but has not finished first-run setup: sign-in stays the setup step.
	if ok && database.SettingOr(r.Context(), a.s.Read(), settingsGroup, settingFirstRunDone, "") != "1" {
		out["setup_open"] = true
	}
	if !ok {
		out["setup_ways"] = setupWaysNow()
	}
	writeJSON(w, http.StatusOK, out)
}

// insertFirstOwner makes the owner, or returns errOwnerExists when there is
// one. The count runs in the same write, so two first runs cannot both win.
func insertFirstOwner(ctx context.Context, tx *sql.Tx, name, hash string) (int64, error) {
	n, err := countOwners(ctx, tx)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		return 0, errOwnerExists
	}
	return insertUserTx(tx, &domain.User{Name: name, Role: "owner", PasswordHash: hash, State: "active", Perms: "[]"}, domain.System)
}

func readCreds(w http.ResponseWriter, r *http.Request) (creds, bool) {
	var c creds
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&c); err != nil {
		fail(w, http.StatusBadRequest, `send {"name": "...", "password": "..."}`)
		return c, false
	}
	c.Name = strings.TrimSpace(c.Name)
	return c, true
}

func (a *Auth) setup(w http.ResponseWriter, r *http.Request) {
	c, ok := readCreds(w, r)
	if !ok {
		return
	}
	if msg := typedNameProblem(c.Name); msg != "" {
		fail(w, http.StatusBadRequest, msg)
		return
	}
	if msg := passwordProblem(c.Password, MinPasswordLength(r.Context(), a.s.Read())); msg != "" {
		fail(w, http.StatusBadRequest, msg)
		return
	}
	display, ok := cleanDisplayName(c.DisplayName)
	if !ok {
		fail(w, http.StatusBadRequest, "pick a display name of up to 64 characters")
		return
	}
	if _, _, exists := a.owner(r.Context()); exists {
		fail(w, http.StatusConflict, "the owner is already set up; sign in instead")
		return
	}
	// No setup code: whoever opens a fresh HolyArrs first makes the owner;
	// `holyarrs reset-owner` takes it back.
	//
	// A home name (tower.local, nas) is accepted here only with no proxy in
	// between; it is then saved.
	newHost := ""
	if !a.hostAllowed(r) {
		h := normHost(r.Host)
		if !homeName(h) || a.anyProxy() || forwarded(r) {
			a.logRefusedHost(r.Host)
			fail(w, http.StatusForbidden, hostRefusedMsg(r.Host))
			return
		}
		newHost = h
	}
	h, err := hashPassword(c.Password, a.hash)
	if err != nil {
		log.Printf("auth: %v", err)
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	var id int64
	var saved []string
	err = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		var err error
		id, err = insertFirstOwner(r.Context(), tx, c.Name, h)
		if err == nil && display != "" {
			err = putDisplayName(r.Context(), tx, id, display)
		}
		if err != nil || newHost == "" {
			return err
		}
		saved = append(slices.Clone(a.savedHostList()), newHost)
		v, _ := json.Marshal(saved)
		return database.PutSetting(tx, settingsGroup, signInPrefix+keyAllowedHosts, string(v), domain.System, a.now())
	})
	if errors.Is(err, errOwnerExists) {
		fail(w, http.StatusConflict, "the owner is already set up; sign in instead")
		return
	}
	if err != nil {
		log.Printf("auth: setup: %v", err)
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	if newHost != "" {
		a.savedHosts.Store(&saved)
		log.Printf("auth: first-run setup on %s; the name is saved under Allowed host names", newHost)
	}
	log.Printf("auth: first-run setup made the owner %q from %s", c.Name, r.RemoteAddr)
	a.startSetupSession(w, r, id, c.Name, http.StatusCreated, AuthMethodLocalSetup)
}
