package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// States written to status.json.
const (
	StateNotConnected = "not_connected"
	StateNeedsSignIn  = "waiting_for_sign_in"
	StateNeedsAdmin   = "waiting_for_admin_approval"
	StateConnected    = "connected"
	StateExpiring     = "login_expiring"
	StateExpired      = "login_expired"
	StateDeleted      = "device_deleted"
)

// expiringWithin is when a coming login expiry is reported.
const expiringWithin = 14 * 24 * time.Hour

// Status is what HolyArrs reads from status.json.
type Status struct {
	State       string    `json:"state"`
	LoginURL    string    `json:"login_url,omitempty"`
	DeviceName  string    `json:"device_name,omitempty"`
	TSNetName   string    `json:"tsnet_name,omitempty"`
	Version     string    `json:"tailscale_version,omitempty"`
	Updated     time.Time `json:"last_updated"`
	LoginExpiry time.Time `json:"login_expiry,omitzero"`
	Connection  string    `json:"connection,omitempty"` // direct, relayed or empty
}

// facts are the parts of Tailscale's own state that pick a State.
type facts struct {
	backend    string // ipnstate BackendState
	authURL    string
	keyExpiry  time.Time
	hadNodeKey bool // this device was signed in before
	keyExpired bool // Tailscale says the node key expired
}

// pickState turns Tailscale's state into one of ours.
func pickState(f facts, now time.Time) string {
	switch f.backend {
	case "Running":
		if !f.keyExpiry.IsZero() && f.keyExpiry.Sub(now) < expiringWithin {
			return StateExpiring
		}
		return StateConnected
	case "NeedsMachineAuth":
		return StateNeedsAdmin
	case "NeedsLogin":
		switch {
		case f.keyExpired:
			return StateExpired
		case f.hadNodeKey && f.authURL == "":
			// Signed in before, no expiry, yet asked to sign in again:
			// the device was removed in Tailscale's admin page.
			return StateDeleted
		case f.authURL != "":
			return StateNeedsSignIn
		}
	}
	return StateNotConnected
}

// writeStatus saves s as status.json in dir: written to a temp file and
// renamed, so HolyArrs never reads half a file. The login link is the only
// sensitive field; the folder is the plugin's own.
func writeStatus(dir string, s Status) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".status-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, "status.json"))
}
