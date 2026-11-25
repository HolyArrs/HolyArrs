package plugins

import (
	"errors"
	"fmt"
	"log"
)

// SetAllowNoNetLock records the owner's install-wide choice to let plugins
// that list hosts start when this system cannot keep them to those hosts.
// Turning it on starts the plugins turned off for the missing lock; turning
// it off stops the running ones that list hosts. UDP and raw sockets stay
// blocked and the proxy settings stay on either way.
func (e *Engine) SetAllowNoNetLock(on bool) error {
	if err := e.init(); err != nil {
		return err
	}
	e.opMu.Lock()
	defer e.opMu.Unlock()
	if err := e.store.SetAllowNoNetLock(on); err != nil {
		return err
	}
	log.Printf("plugins: owner set plugins with hosts to run without the network lock: %v (%s)", on, e.NetLockReason)
	if !e.NoNetLock || e.Runner == nil {
		return nil
	}
	st, err := e.store.Load()
	if err != nil {
		return err
	}
	var errs []error
	for id, p := range st.Plugins {
		e.mu.Lock()
		_, running := e.procs[id]
		e.mu.Unlock()
		m := e.readManifest(id)
		hasHosts := m != nil && len(m.Access.Hosts) > 0 || p.Approval != nil && len(p.Approval.OwnerHosts) > 0
		switch {
		case on && !running && (p.Enabled || p.DisabledReason == ErrNoNetLock.Error()):
			if err := e.store.SetEnabled(id, true, ""); err != nil {
				errs = append(errs, err)
				continue
			}
			if err := e.start(id); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", id, err))
			}
		case !on && running && hasHosts:
			if err := e.stopProc(id); err != nil {
				errs = append(errs, err)
				continue
			}
			errs = append(errs, e.store.SetEnabled(id, false, ErrNoNetLock.Error()))
		}
	}
	return errors.Join(errs...)
}

// showNetLock fills the overview's network lock fields.
func (e *Engine) showNetLock(ov *Overview, st *State) {
	ov.NoNetLock = e.Runner != nil && e.NoNetLock
	if ov.NoNetLock {
		ov.NetLockReason = e.NetLockReason
		if ov.NetLockReason == "" {
			ov.NetLockReason = FileLockOldKernel
		}
	}
	ov.AllowNoNetLock, ov.NoNetLockAsked = st.AllowNoNetLock, st.NoNetLockAsked
}
