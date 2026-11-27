package plugins

import (
	"log"
	"strings"
)

// stopProc ends id's process and its API token at the same moment, so a
// blocked or stopped plugin can't call HolyArrs any more.
func (e *Engine) stopProc(id string) error {
	e.mu.Lock()
	p := e.procs[id]
	delete(e.procs, id)
	e.revoke(id)
	e.mu.Unlock()
	e.dropFeed(id, nil) // its event stream ends with it
	if p == nil {
		return nil
	}
	return p.Stop()
}

// revoke ends the token of id's last start, if any.
func (e *Engine) revoke(id string) {
	if e.Revoke != nil {
		e.Revoke(id)
	}
}

// Locked says whether plugins here run with both the file and network
// lock. Without one, a start gets an API token only when the owner allowed
// going without it (mayToken).
func (e *Engine) Locked() bool {
	return e.Runner != nil && !e.NoFileLock && !e.NoNetLock
}

// mayToken says whether a start here may get an API token: each lock this
// system lacks must be one the owner allowed going without on the Plugins
// page. The token adds nothing a plugin without the file lock could
// not reach anyway, and without it the allowed start could not use its
// approved scopes at all.
func (e *Engine) mayToken() bool {
	if e.Runner == nil {
		return false
	}
	if e.Locked() {
		return true
	}
	if e.init() != nil {
		return false
	}
	st, err := e.store.Load()
	if err != nil || st == nil {
		return false
	}
	return (!e.NoFileLock || st.AllowNoFileLock) && (!e.NoNetLock || st.AllowNoNetLock)
}

// tokenEnv is the API token env of one start of id: only for a plugin with
// approved scopes that may get one (mayToken). A target already in env is kept.
func (e *Engine) tokenEnv(id string, env []string) []string {
	scopes := e.GrantedScopes(id)
	if e.Token == nil || len(scopes) == 0 {
		return nil
	}
	if !e.mayToken() {
		log.Printf("plugins: %s starts without an API token: this system lacks a plugin lock the owner has not allowed going without", id)
		return nil
	}
	var out []string
	for _, kv := range e.Token(id, scopes) {
		if strings.HasPrefix(kv, "HOLYARRS_PLUGIN_TARGET=") && selfAddr(env) != "" {
			continue
		}
		out = append(out, kv)
	}
	return out
}
