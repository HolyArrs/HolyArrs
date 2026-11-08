package backups

import (
	"context"
	"encoding/base64"
	"log"
	"net/http"
	"strconv"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// KeyKitPath (POST) downloads the key kit: the master key as a key file with
// notes on how to put it back.
const KeyKitPath = "/api/v1/system/key-kit"

// settingKeyKit keeps the fingerprint of the last key kit downloaded, so
// the Backups card stops asking once a matching copy is saved.
const settingKeyKit = "key_kit_print"

// KeyKit is the Backups card's key state.
type KeyKit struct {
	Needed      bool   `json:"needed"`
	Fingerprint string `json:"fingerprint"`
}

// KeyKitPrint returns the fingerprint of the last key kit downloaded ("" for none).
func (m *Manager) KeyKitPrint(ctx context.Context) string {
	if m.Store == nil {
		return ""
	}
	v, _ := m.setting(ctx, settingKeyKit)
	return v
}

// KeyPrint is the current key's fingerprint, or "" when there is no key or
// it is a named file (HOLYARRS_MASTER_KEY_FILE) the person looks after.
func (m *Manager) KeyPrint() string {
	if len(m.Key) == 0 || m.KeyFromEnv {
		return ""
	}
	return secrets.Fingerprint(m.Key)
}

func (m *Manager) keyKitState(ctx context.Context) KeyKit {
	st := KeyKit{}
	if len(m.Key) > 0 {
		st.Fingerprint = secrets.Fingerprint(m.Key)
	}
	if p := m.KeyPrint(); p != "" {
		st.Needed = m.KeyKitPrint(ctx) != p
	}
	return st
}

// keyKitFile is the master key with '#' notes: secrets.LoadKey reads it
// as a key file, so restoring is renaming it to holyarrs.key.
func keyKitFile(key []byte) []byte {
	fp := secrets.Fingerprint(key)
	return []byte("# HolyArrs master key kit\n" +
		"# fingerprint " + fp + "\n" +
		"# This file is the key that opens the saved passwords and API keys in your\n" +
		"# HolyArrs database. Backups do not hold it. Keep it apart from your backups.\n" +
		"# To put it back: rename this file to " + secrets.KeyFile + " and place it in the\n" +
		"# config folder (/config), or name it in " + secrets.EnvKeyFile + ".\n" +
		base64.StdEncoding.EncodeToString(key) + "\n")
}

func (m *Manager) keyKitRoute(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/system/key-kit", func(w http.ResponseWriter, r *http.Request) {
		// A browser session only: never an API key or a trusted-network skip.
		if who, ok := webserver.From(r.Context()); ok && who.Via != "session" {
			webserver.Fail(w, http.StatusForbidden, "sign in with your password in the browser to download the key kit")
			return
		}
		if len(m.Key) == 0 {
			webserver.Fail(w, http.StatusNotFound, "no master key is loaded")
			return
		}
		fp := secrets.Fingerprint(m.Key)
		body := keyKitFile(m.Key)
		if m.Store != nil {
			if err := m.setSetting(r.Context(), settingKeyKit, fp, webserver.Actor(r.Context())); err != nil {
				log.Printf("backup: saving the key kit state: %v", err)
			}
			who := "someone"
			if u, ok := webserver.From(r.Context()); ok && u.Name != "" {
				who = u.Name
			}
			_ = m.Store.AddEvent(context.WithoutCancel(r.Context()), &domain.Event{Actor: webserver.Actor(r.Context()), Kind: "backup_used",
				Summary: "Master key kit (fingerprint " + fp + ") downloaded by " + who,
				Data:    map[string]any{"action": "key_kit", "fingerprint": fp}})
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Disposition", `attachment; filename="holyarrs-key-kit-`+fp+`.key"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	})
}
