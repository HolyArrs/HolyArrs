package mediaservers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Handlers serves the media server link routes.
type Handlers struct {
	Store  *database.Store
	Vault  *secrets.Vault
	Linker *Linker
	// PlexServerToken is the token of the owner's own Plex server at an
	// address (accounts); nil turns the Plex sign-in shortcut off.
	PlexServerToken func(ctx context.Context, url string) (string, error)
	// MediaSignInKey is the API key the owner's first-run Jellyfin or Emby
	// sign-in made on that server.
	MediaSignInKey func(ctx context.Context, kind, url string) (string, error)
	// NameCase gives a server name the computer's own spelling (Windows
	// servers answer in capitals); nil keeps the name as the server said it.
	NameCase func(ctx context.Context, name, url string) string
	// AskedBy lists, per title, the people who asked for it (requests), so
	// the watched clean-up list keeps those titles until they watched.
	AskedBy func(ctx context.Context) (map[int64][]int64, error)
}

// Routes registers the media server routes. Without a vault there is
// nowhere to keep tokens, so none are registered.
func (h *Handlers) Routes(m *http.ServeMux) {
	if h.Vault == nil {
		return
	}
	m.HandleFunc("GET /api/v1/media-servers", h.list)
	m.HandleFunc("POST /api/v1/media-servers", h.add)
	m.HandleFunc("PUT /api/v1/media-servers/{id}", h.edit)
	m.HandleFunc("DELETE /api/v1/media-servers/{id}", h.remove)
	m.HandleFunc("POST /api/v1/media-servers/{id}/test", h.Test)
	m.HandleFunc("GET /api/v1/media-servers/{id}/libraries", h.libraries)
	m.HandleFunc("PUT /api/v1/media-servers/{id}/libraries", h.pickLibraries)
	m.HandleFunc("GET /api/v1/settings/watch-history", h.getWatchHistory)
	m.HandleFunc("PUT /api/v1/settings/watch-history", h.putWatchHistory)
	m.HandleFunc("GET /api/v1/library/stats/watched", h.watched)
	// Where a Library title opens on the Jellyfin and Emby servers.
	m.HandleFunc("GET /api/v1/media-servers/watch-links", h.watchLinks)
	m.HandleFunc("GET /api/v1/my/watched", h.myWatched)
	m.HandleFunc("GET /api/v1/library/watched-by-anyone", h.watchedByAnyone)
	m.HandleFunc("GET /api/v1/library/watched-cleanup", h.watchedCleanup)
	m.HandleFunc("GET /api/v1/settings/plex-labels", h.getLabels)
	m.HandleFunc("PUT /api/v1/settings/plex-labels", h.putLabels)
	m.HandleFunc("POST /api/v1/settings/plex-labels/run", h.runLabels)
	m.HandleFunc("GET /api/v1/settings/default-tracks", h.getTracks)
	m.HandleFunc("PUT /api/v1/settings/default-tracks", h.putTracks)
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	ms, err := listServers(r.Context(), h.Store.Read())
	if err != nil {
		internalError(w, err)
		return
	}
	out := []View{}
	for _, m := range ms {
		out = append(out, ViewOf(m))
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"media_servers": out})
}

func (h *Handlers) add(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	in, ok := readInput(w, r)
	if !ok {
		return
	}
	h.plexSignInToken(r, &in, "")
	m, err := Add(r.Context(), h.Store, h.Vault, in, actorOf(r))
	if errors.Is(err, ErrNameUsed) {
		// During first-run setup the link step may have saved this server
		// already: saving it again updates that one.
		if id, ok := setupTwin(r.Context(), h.Store, in); ok {
			m, err = Change(r.Context(), h.Store, h.Vault, id, in, actorOf(r))
			if err == nil {
				webserver.WriteJSON(w, http.StatusOK, ViewOf(m))
				return
			}
		}
	}
	if err != nil {
		writeError(w, err)
		return
	}
	if WatchHistoryOn(r.Context(), h.Store.Read()) {
		h.Linker.ReadWatchHistorySoon() // the new server's history, without the wait
	}
	webserver.WriteJSON(w, http.StatusCreated, ViewOf(m))
}

func (h *Handlers) edit(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	in, ok := readInput(w, r)
	if !ok {
		return
	}
	keep := ""
	if in.UsePlexSignIn && in.URL == "" {
		if old, err := getServer(r.Context(), h.Store.Read(), id); err == nil {
			keep = old.URL
		}
	}
	h.plexSignInToken(r, &in, keep)
	m, err := Change(r.Context(), h.Store, h.Vault, id, in, actorOf(r))
	if err != nil {
		writeError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, ViewOf(m))
}

func (h *Handlers) remove(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if _, err := getServer(r.Context(), h.Store.Read(), id); err != nil {
		writeError(w, err)
		return
	}
	if err := Remove(r.Context(), h.Store, h.Vault, id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Test checks the address and token: Plex must list a movie library,
// Jellyfin and Emby must answer their system info.
func (h *Handlers) Test(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	m, err := getServer(r.Context(), h.Store.Read(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := h.Linker.Test(ctx, m)
	if err != nil {
		webserver.Fail(w, http.StatusBadGateway, err.Error())
		return
	}
	if h.NameCase != nil && (m.Kind == "jellyfin" || m.Kind == "emby" || (m.Kind == "plex" && !strings.HasPrefix(res, "Libraries: "))) {
		// The answer is "NAME version": only the name part changes case.
		parts := strings.Fields(res)
		if n := len(parts); n > 1 && parts[n-1] != "" && parts[n-1][0] >= '0' && parts[n-1][0] <= '9' {
			res = h.NameCase(ctx, strings.Join(parts[:n-1], " "), m.URL) + " " + parts[n-1]
		} else if n > 0 {
			res = h.NameCase(ctx, res, m.URL)
		}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "found": res})
}

// setupTwin finds the media server of the same kind and name that the
// first-run link step saved, while the wizard is not finished yet and that
// link turned the kind's sign-in on.
func setupTwin(ctx context.Context, s *database.Store, in Input) (int64, bool) {
	db := s.Read()
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	if accounts.FirstRunDone(ctx, db) || database.SettingOr(ctx, db, "this_app", "sign_in_"+kind+"_on", "") != "true" {
		return 0, false
	}
	id, err := serverIDByName(ctx, db, strings.TrimSpace(in.Name), kind)
	return id, err == nil
}

const inputHelp = `send name, kind (plex, jellyfin, emby or subsonic), url, token (for subsonic the password, with username), and optionally plain_password (subsonic, LDAP; https or local only), enabled, on_import, on_rename, on_delete, path_from and path_to, allow_local_network, web_url`

func readInput(w http.ResponseWriter, r *http.Request) (Input, bool) {
	var in Input
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in)
	if mb := (*http.MaxBytesError)(nil); errors.As(err, &mb) {
		webserver.Fail(w, http.StatusRequestEntityTooLarge, "request body too large")
		return in, false
	}
	if err != nil {
		webserver.Fail(w, http.StatusBadRequest, inputHelp)
		return in, false
	}
	if in.Token == secrets.Masked {
		in.Token = ""
	}
	return in, true
}

func idParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, "no such media server")
		return 0, false
	}
	return id, true
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNameUsed):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case IsInputError(err):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, database.ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "no such media server")
	default:
		internalError(w, err)
	}
}

// actorOf is the signed-in person; without sign-in (tests) it is the owner.
func actorOf(r *http.Request) domain.Actor {
	if _, ok := webserver.From(r.Context()); ok {
		return webserver.Actor(r.Context())
	}
	return domain.Actor{Kind: "user", ID: 1}
}

func internalError(w http.ResponseWriter, err error) {
	log.Printf("mediaservers: %v", err)
	webserver.Fail(w, http.StatusInternalServerError, "something went wrong; see the log")
}

// plexSignInToken fills in a Plex server token from the owner's Plex
// sign-in when asked to (first-run setup), so it never passes through the
// browser. Only the owner gets it, only for a Plex server, and only when the
// address is one of the owner's own servers on plex.tv; the token is that
// server's own, never the plex.tv account token. keepURL is the stored
// address an edit keeps when it sends none.
func (h *Handlers) plexSignInToken(r *http.Request, in *Input, keepURL string) {
	if !in.UsePlexSignIn || in.Token != "" {
		return
	}
	if c, ok := webserver.From(r.Context()); !ok || c.Role != webserver.RoleOwner || c.Via == "api_key" {
		return
	}
	u := in.URL
	if u == "" {
		u = keepURL
	}
	if (in.Kind == "jellyfin" || in.Kind == "emby") && h.MediaSignInKey != nil {
		if key, err := h.MediaSignInKey(r.Context(), in.Kind, u); err == nil {
			in.Token = key
		}
		return
	}
	if in.Kind != "plex" || h.PlexServerToken == nil {
		return
	}
	if tok, err := h.PlexServerToken(r.Context(), u); err == nil && tok != "" {
		in.Token = tok
	}
}
