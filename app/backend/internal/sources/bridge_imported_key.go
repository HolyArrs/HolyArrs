package sources

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// prowlarrFeed is a Prowlarr per-site feed path (/5/api).
var prowlarrFeed = regexp.MustCompile(`^/\d+/api/?$`)

// importedKey fills an empty Prowlarr key with the one the Sources an import
// brought already use for that same Prowlarr (its per-site feeds), so setup
// links it without asking for the key again. The key never leaves the
// server. It always lets the request go on; an empty key is refused later.
func (h *Handlers) importedKey(r *http.Request, in *bridgeBody) bool {
	// Only the owner, signed in in the browser: a key is never handed to a
	// lesser role or an API key.
	c, ok := webserver.From(r.Context())
	if in.Key != "" || in.Kind != BridgeKindProwlarr || h.Vault == nil || !ok || !ownerInBrowser(c) {
		return true
	}
	want, err := url.Parse(in.URL)
	if err != nil || want.Host == "" {
		return true
	}
	srcs, err := database.Select[domain.Source](r.Context(), h.Store.Read(), "sources", "1=1")
	if err != nil {
		return true
	}
	for _, s := range srcs {
		u, err := url.Parse(s.URL)
		// The same Prowlarr exactly: scheme, host and port (a key kept for
		// https is never sent over http, or to another port).
		if err != nil || s.SecretRef == nil || !prowlarrFeed.MatchString(u.Path) || !sameProwlarr(u, want) {
			continue
		}
		if key, err := h.Vault.Get(r.Context(), *s.SecretRef); err == nil && key != "" {
			in.Key = key
			return true
		}
	}
	return true
}

// sameProwlarr says the stored feed u is on exactly the Prowlarr want names:
// same scheme, host and port, and u is one of its per-site feeds.
func sameProwlarr(u, want *url.URL) bool {
	return prowlarrFeed.MatchString(u.Path) && u.Scheme == want.Scheme && strings.EqualFold(u.Hostname(), want.Hostname()) && u.Port() == want.Port()
}

// ownerInBrowser says the caller is the owner, signed in in a browser.
func ownerInBrowser(c webserver.Caller) bool {
	return c.Role == webserver.RoleOwner && c.Via == "session"
}
