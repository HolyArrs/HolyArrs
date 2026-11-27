package plugins

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// RefreshEvery is how soon the Refresh button may fetch the lists again;
// a press inside it answers with the lists already cached.
const RefreshEvery = 30 * time.Second

// Refresh fetches the signed plugin lists now, as the 6-hour check does,
// and says whether a newer plugin list or blocklist came.
func (e *Engine) Refresh(ctx context.Context) (changed bool, err error) {
	if err := e.init(); err != nil {
		return false, err
	}
	// The cached lists go in Dir, which a new install may not have yet.
	if err := os.MkdirAll(e.Dir, 0o700); err != nil {
		return false, err
	}
	before, err := e.store.Load()
	if err != nil {
		return false, err
	}
	if err := e.CheckBlocklist(ctx); err != nil {
		return false, err
	}
	after, err := e.store.Load()
	if err != nil {
		return false, err
	}
	return after.ListCounter > before.ListCounter || after.BlockCounter > before.BlockCounter, nil
}

// Refresh fetches the signed language list again: the pack sizes now, and
// installed packs the lists withdrew are dropped. Errors are only logged.
func (l *Languages) Refresh(ctx context.Context) {
	l.mu.Lock()
	l.sizesAt = time.Time{}
	l.mu.Unlock()
	l.LoadSizes(ctx)
	if err := l.Recheck(ctx); err != nil {
		log.Printf("languages: refresh: %v", err)
	}
}

// refresh is POST /api/v1/plugins/refresh: fetch the lists now and answer
// {"changed": bool, "plugins": <the list>}. A press within RefreshEvery of
// the last fetch fetches nothing and answers changed false. 502 when the
// plugin site could not be reached or its lists did not check out.
func (h *Handlers) refresh(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	h.refreshMu.Lock()
	fetch := h.refreshAt.IsZero() || time.Since(h.refreshAt) >= RefreshEvery
	if fetch {
		h.refreshAt = time.Now()
	}
	h.refreshMu.Unlock()
	changed := false
	if fetch {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		var err error
		if changed, err = h.Engine.Refresh(ctx); err != nil {
			log.Printf("plugins: refresh: %v", err)
			webserver.Fail(w, http.StatusBadGateway, "couldn't reach the plugin site")
			return
		}
		if h.Languages != nil {
			h.Languages.Refresh(ctx)
		}
	}
	ov, err := h.Engine.List(r.Context())
	if err != nil {
		webserver.Internal(w, "plugins", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"changed": changed, "plugins": ov})
}
