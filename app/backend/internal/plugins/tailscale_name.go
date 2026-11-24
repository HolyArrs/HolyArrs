package plugins

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// nameFile, in the plugin's data folder, holds the device name the owner
// picked. The Tailscale plugin reads it at start; Tailscale turns it into
// the first part of the address (<name>.<tailnet>.ts.net).
const nameFile = "hostname"

var deviceNameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ErrBadName is a device name Tailscale would not take.
var ErrBadName = errors.New("use 1 to 63 lowercase letters, numbers or dashes, not starting or ending with a dash")

// SetDeviceName saves name for id and restarts it when running, so the new
// address takes effect.
func (e *Engine) SetDeviceName(id, name string) error {
	if err := e.init(); err != nil {
		return err
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if !deviceNameRE.MatchString(name) {
		return ErrBadName
	}
	e.opMu.Lock()
	defer e.opMu.Unlock()
	if _, err := e.installed(id); err != nil {
		return err
	}
	dir := e.store.DataDir(id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(dir, nameFile+".tmp")
	if err := os.WriteFile(tmp, []byte(name+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(dir, nameFile)); err != nil {
		return err
	}
	e.mu.Lock()
	_, running := e.procs[id]
	e.mu.Unlock()
	if !running {
		return nil
	}
	if err := e.stopProc(id); err != nil {
		return err
	}
	return e.start(id)
}
