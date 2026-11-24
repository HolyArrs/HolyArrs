package plugins

import (
	"errors"
	"fmt"
	"log"
)

// SetAllowNoFileLock records the owner's install-wide choice to let signed
// plugins start without the file lock. Turning it on starts the plugins that
// were turned off for the missing lock; turning it off stops the running ones
// when this system has no lock. Memory, process and CPU limits stay on.
func (e *Engine) SetAllowNoFileLock(on bool) error {
	if err := e.init(); err != nil {
		return err
	}
	e.opMu.Lock()
	defer e.opMu.Unlock()
	if err := e.store.SetAllowNoFileLock(on); err != nil {
		return err
	}
	log.Printf("plugins: owner set signed plugins to run without the file lock: %v (%s)", on, e.FileLockReason)
	if !e.NoFileLock || e.Runner == nil {
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
		switch {
		case on && !running && (p.Enabled || p.DisabledReason == ErrNoLandlock.Error()):
			if err := e.store.SetEnabled(id, true, ""); err != nil {
				errs = append(errs, err)
				continue
			}
			if err := e.start(id); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", id, err))
			}
		case !on && running:
			if err := e.stopProc(id); err != nil {
				errs = append(errs, err)
				continue
			}
			errs = append(errs, e.store.SetEnabled(id, false, ErrNoLandlock.Error()))
		}
	}
	return errors.Join(errs...)
}
