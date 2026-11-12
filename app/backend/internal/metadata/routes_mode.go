package metadata

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// ModeHandlers serves Settings > Metadata: where details come from and the
// owner's own keys.
type ModeHandlers struct {
	Modes *Modes
	Store *database.Store
	Vault *secrets.Vault
	// EnvTMDB says a TMDB key comes from the Docker settings.
	EnvTMDB bool
	// Test checks one own key (KeyTMDB, KeyTVDB with its PIN, KeyMDBList,
	// KeyFanart); nil answers that the key cannot be tested.
	Test func(ctx context.Context, key, value, pin string) error
}

// ModeAnswer is the answer of GET /settings/metadata.
type ModeAnswer struct {
	Mode Mode `json:"mode"`
	// Locked: the Docker settings set the mode; LockedNote says so.
	Locked     bool   `json:"locked"`
	LockedNote string `json:"locked_note,omitempty"`
	// DockerValue is a Docker value that was not understood (lookups are
	// then off).
	DockerValue string `json:"docker_value,omitempty"`
	// Keys says which own keys are saved (never the keys themselves).
	Keys        map[string]bool `json:"keys"`
	TMDBFromEnv bool            `json:"tmdb_key_from_docker"`
	Sources     []SourceState   `json:"sources"`
	Hosts       []string        `json:"hosts"`
	// Note is the calm line for pages while some rows are off.
	Note string `json:"note,omitempty"`
	// OwnKeysNote says up front what Your own keys leaves out.
	OwnKeysNote string `json:"own_keys_note"`
}

// OffNote is the calm line shown where rows are off on this install.
const OffNote = "Some rows are off on this install."

// WriteSourceOff answers a lookup that is off on this install: 200 with
// source_off and the calm note, never a 503 (nothing is down).
func WriteSourceOff(w http.ResponseWriter) {
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"source_off": true, "note": OffNote})
}

const ownKeysNote = "Your own keys asks each source yourself. Music charts, new music rows, streaming Top 10s, Wikipedia facts and anime maps come only from the HolyArrs server, so they are off."

// Routes mounts the Settings > Metadata routes.
func (h *ModeHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/settings/metadata", h.get)
	m.HandleFunc("PUT /api/v1/settings/metadata", h.put)
	m.HandleFunc("POST /api/v1/settings/metadata/test", h.test)
}

func (h *ModeHandlers) has(ctx context.Context) func(string) bool {
	saved := map[string]bool{}
	for _, k := range OwnKeyNames {
		if h.Vault != nil {
			if v, err := h.Vault.Setting(ctx, KeyGroup, k); err == nil && strings.TrimSpace(v) != "" {
				saved[k] = true
			}
		}
	}
	if h.EnvTMDB {
		saved[KeyTMDB] = true
	}
	return func(k string) bool { return saved[k] }
}

// Answer is the current Settings > Metadata answer.
func (h *ModeHandlers) Answer(ctx context.Context) ModeAnswer {
	has := h.has(ctx)
	mode := h.Modes.Current()
	a := ModeAnswer{Mode: mode, Locked: h.Modes.Locked(), DockerValue: h.Modes.BadValue(), Keys: map[string]bool{},
		TMDBFromEnv: h.EnvTMDB, Sources: SourceStates(mode, has), Hosts: ModeHosts(mode, has), OwnKeysNote: ownKeysNote}
	if a.Locked {
		a.LockedNote = "Set by your Docker settings"
	}
	for _, k := range OwnKeyNames {
		a.Keys[k] = has(k)
	}
	// TheTVDB is an extra the owner adds, not a row that is off.
	for i, s := range a.Sources {
		if s.State == "off" || s.State == "needs_key" && !SourceTable[i].Everywhere {
			a.Note = OffNote
			break
		}
	}
	return a
}

func (h *ModeHandlers) get(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, h.Answer(r.Context()))
}

const modeHelp = `send {"mode": "full" | "own" | "off", "keys": {"tmdb_token": "...", "tvdb_key": "...", "tvdb_pin": "...", "mdblist_key": "...", "fanart_key": "..."}}; an empty key removes it, a left-out key stays`

func (h *ModeHandlers) put(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode *string           `json:"mode"`
		Keys map[string]string `json:"keys"`
	}
	if !webserver.Decode(w, r, &in, modeHelp) {
		return
	}
	var mode Mode
	if in.Mode != nil {
		var ok bool
		if mode, ok = ParseMode(*in.Mode); !ok {
			webserver.Fail(w, http.StatusBadRequest, modeHelp)
			return
		}
		if h.Modes.Locked() && mode != h.Modes.Current() {
			webserver.Fail(w, http.StatusConflict, "Set by your Docker settings")
			return
		}
	}
	for k, v := range in.Keys {
		if !slices.Contains(OwnKeyNames, k) || len(v) > 4096 || strings.ContainsAny(v, "\r\n") {
			webserver.Fail(w, http.StatusBadRequest, modeHelp)
			return
		}
	}
	by := webserver.ActorOr(r, database.System)
	if h.Vault != nil {
		for k, v := range in.Keys {
			if v == secrets.Masked {
				continue // the page sent the mask back: unchanged
			}
			if err := h.Vault.SetSetting(r.Context(), KeyGroup, k, strings.TrimSpace(v), by); err != nil {
				webserver.Internal(w, "metadata keys", err)
				return
			}
		}
	}
	if in.Mode != nil && !h.Modes.Locked() {
		if err := SaveMode(r.Context(), h.Store, mode, by); err != nil {
			webserver.Internal(w, "metadata mode", err)
			return
		}
		// Applies at once: the next request already goes the new way.
		_ = h.Modes.Set(mode)
	}
	webserver.WriteJSON(w, http.StatusOK, h.Answer(r.Context()))
}

const testHelp = `send {"key": "tmdb_token" | "tvdb_key" | "mdblist_key" | "fanart_key", "value": "<key, or empty for the saved one>", "pin": "<TheTVDB PIN, optional>"}`

func (h *ModeHandlers) test(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key   string `json:"key"`
		Value string `json:"value"`
		PIN   string `json:"pin"`
	}
	if !webserver.Decode(w, r, &in, testHelp) {
		return
	}
	if !slices.Contains(OwnKeyNames, in.Key) || in.Key == KeyTVDBPIN {
		webserver.Fail(w, http.StatusBadRequest, testHelp)
		return
	}
	value, pin := strings.TrimSpace(in.Value), strings.TrimSpace(in.PIN)
	if h.Vault != nil {
		if value == "" || value == secrets.Masked {
			value, _ = h.Vault.Setting(r.Context(), KeyGroup, in.Key)
		}
		if in.Key == KeyTVDB && (pin == "" || pin == secrets.Masked) {
			pin, _ = h.Vault.Setting(r.Context(), KeyGroup, KeyTVDBPIN)
		}
	}
	answer := map[string]any{"ok": false}
	switch err := h.check(r.Context(), in.Key, value, pin); {
	case err == nil:
		answer["ok"], answer["message"] = true, "Key works"
	case errors.Is(err, errNoKey):
		answer["message"] = "No key saved yet"
	case errors.Is(err, ErrTVDBRefused):
		answer["message"] = "Key or PIN refused"
	case errors.Is(err, ErrNoAccess), errors.Is(err, ErrNoTVDBKey):
		answer["message"] = "Key refused"
	default:
		answer["message"] = "Could not reach the service; try again"
	}
	// A test answers 200 either way: the page shows the line it gets.
	webserver.WriteJSON(w, http.StatusOK, answer)
}

var errNoKey = errors.New("no key")

func (h *ModeHandlers) check(ctx context.Context, key, value, pin string) error {
	if value == "" {
		return errNoKey
	}
	if h.Test == nil {
		return errors.New("no tester")
	}
	return h.Test(ctx, key, value, pin)
}
