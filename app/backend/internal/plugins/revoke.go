package plugins

import "strings"

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
// lock. A plugin started without either never gets an API token.
func (e *Engine) Locked() bool {
	return e.Runner != nil && !e.NoFileLock && !e.NoNetLock
}

// tokenEnv is the API token env of one start of id: only for a plugin with
// approved scopes that runs fully locked. A target already in env is kept.
func (e *Engine) tokenEnv(id string, env []string) []string {
	scopes := e.GrantedScopes(id)
	if e.Token == nil || len(scopes) == 0 || !e.Locked() {
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
