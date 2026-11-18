package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// signedOut is the disabled reason a sign-out leaves.
const signedOut = "signed out by the owner"

// signOutWithin caps a sign-out run; the plugin's own logout gives up sooner.
const signOutWithin = 30 * time.Second

// TailscaleStatus is the part of the plugin's status.json HolyArrs passes on.
type TailscaleStatus struct {
	State            string `json:"state"`
	DeviceName       string `json:"device_name,omitempty"`
	TsnetName        string `json:"tsnet_name,omitempty"`
	TailscaleVersion string `json:"tailscale_version,omitempty"`
	LastUpdated      string `json:"last_updated,omitempty"`
	LoginExpiry      string `json:"login_expiry,omitempty"`
	Connection       string `json:"connection,omitempty"`
	LoginURL         string `json:"login_url,omitempty"`
}

// statusMax caps how much of status.json is read.
const statusMax = 64 << 10

// Status returns the plugin's status.json as written by the plugin, or a
// not-connected answer when it has written none yet. Only a regular file is
// read, at most statusMax bytes, and only the known fields are kept.
func (e *Engine) Status(id string) (*TailscaleStatus, error) {
	if err := e.init(); err != nil {
		return nil, err
	}
	if _, err := e.installed(id); err != nil {
		return nil, err
	}
	path := filepath.Join(e.store.DataDir(id), "status.json")
	fi, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return &TailscaleStatus{State: "not_connected"}, nil
	}
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errors.New("plugin status: not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// The path may have been swapped for a link between Lstat and Open:
	// the opened file must be the same plain file Lstat saw.
	if ofi, err := f.Stat(); err != nil || !ofi.Mode().IsRegular() || !os.SameFile(fi, ofi) {
		return nil, errors.New("plugin status: not a regular file")
	}
	var out TailscaleStatus
	if err := json.NewDecoder(io.LimitReader(f, statusMax)).Decode(&out); err != nil {
		return nil, fmt.Errorf("plugin status: %w", err)
	}
	if out.State == "" {
		out.State = "not_connected"
	}
	return &out, nil
}

// SignOut stops id, runs it once with --sign-out through the same checked,
// sandboxed start, and leaves it stopped.
// removedRemotely is false when the plugin wiped its state but could not
// log the device out (it exits with code 3), so the owner removes it in
// Tailscale's admin page.
func (e *Engine) SignOut(ctx context.Context, id string) (removedRemotely bool, err error) {
	if err := e.init(); err != nil {
		return false, err
	}
	if e.Runner == nil {
		return false, ErrNotLinux
	}
	e.opMu.Lock()
	defer e.opMu.Unlock()
	p, err := e.installed(id)
	if err != nil {
		return false, err
	}
	if err := e.stopProc(id); err != nil {
		return false, fmt.Errorf("stopping plugin: %w", err)
	}
	// The run passes the same start checks, which refuse a disabled plugin:
	// an owner stop is lifted for the run, any other reason still refuses.
	if !p.Enabled && !ownerReason(p.DisabledReason) {
		return false, fmt.Errorf("%w: %s", ErrDisabled, p.DisabledReason)
	}
	if !p.Enabled {
		if err := e.store.SetEnabled(id, true, ""); err != nil {
			return false, err
		}
	}
	// Leave it off as signed out, unless something else (such as the
	// blocklist) turned it off meanwhile: that reason is kept.
	defer func() {
		if now, err := e.installed(id); err == nil && (now.Enabled || ownerReason(now.DisabledReason)) {
			_ = e.store.SetEnabled(id, false, signedOut)
		}
	}()
	e.mu.Lock()
	in, err := e.startInput()
	e.mu.Unlock()
	if err != nil {
		return false, err
	}
	var env []string
	if e.Env != nil {
		env = e.Env(id)
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), signOutWithin)
	defer cancel()
	proc, err := e.store.Start(ctx, e.Runner, id, in, []string{"--sign-out"}, env)
	if err != nil {
		return false, err
	}
	done := make(chan error, 1)
	go func() { done <- proc.Wait() }()
	select {
	case err := <-done:
		var exit interface{ ExitCode() int }
		if errors.As(err, &exit) && exit.ExitCode() == 3 {
			return false, nil
		}
		return err == nil, err
	case <-ctx.Done():
		_ = proc.Stop()
		return false, fmt.Errorf("sign-out did not finish in %s", signOutWithin)
	}
}

// ownerReason reports whether reason is one the owner set (or none), which a
// sign-out may lift and replace.
func ownerReason(reason string) bool {
	return reason == "" || reason == "stopped by the owner" || reason == signedOut
}
