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
	if !ok {
		out["setup_ways"] = setupWays
	}
	writeJSON(w, http.StatusOK, out)
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
	if msg := nameProblem(c.Name); msg != "" {
		fail(w, http.StatusBadRequest, msg)
		return
	}
	if msg := passwordProblem(c.Password, MinPasswordLength(r.Context(), a.s.Read())); msg != "" {
		fail(w, http.StatusBadRequest, msg)
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
		if !homeName(h) || len(a.proxies) > 0 || forwarded(r) {
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
		n, err := countOwners(r.Context(), tx)
		if err != nil {
			return err
		}
		if n > 0 {
			return errOwnerExists
		}
		id, err = insertUserTx(tx, &domain.User{Name: c.Name, Role: "owner", PasswordHash: h, State: "active", Perms: "[]"}, domain.System)
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
