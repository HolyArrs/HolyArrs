package plugins

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// LanguageHandlers serves /api/v1/languages: the language packs.
type LanguageHandlers struct {
	Languages *Languages
	// SetupOpen lets the first-run screen install languages before anyone can
	// sign in; it answers the request itself when the caller may not.
	SetupOpen func(http.ResponseWriter, *http.Request) bool
}

// Routes registers the language pack routes.
func (h *LanguageHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/languages", h.list)
	m.HandleFunc("POST /api/v1/languages/{code}", h.install)
	m.HandleFunc("DELETE /api/v1/languages/{code}", h.remove)
	m.HandleFunc("GET /api/v1/languages/{code}/pack", h.pack)
	m.HandleFunc("GET /api/v1/public/languages", h.installed)
	m.HandleFunc("POST /api/v1/auth/setup/language/{code}", h.setupInstall)
	m.HandleFunc("DELETE /api/v1/auth/setup/language/{code}", h.setupRemove)
}

// installed lists the codes a picker may offer (English and the installed
// packs); public, so the sign-in page lists only languages it can show.
func (h *LanguageHandlers) installed(w http.ResponseWriter, r *http.Request) {
	codes := []string{"en"}
	if h.Languages != nil {
		for _, l := range h.Languages.List() {
			if l.State == "installed" {
				codes = append(codes, l.Code)
			}
		}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"installed": codes})
}

// setupRemove takes a pack off again when it is unticked on the first-run language step.
func (h *LanguageHandlers) setupRemove(w http.ResponseWriter, r *http.Request) {
	if h.SetupOpen == nil || !h.SetupOpen(w, r) {
		if h.SetupOpen == nil {
			webserver.Fail(w, http.StatusNotFound, "not found")
		}
		return
	}
	h.remove(w, r)
}

// setupInstall is install for the first-run language step, before an account exists.
func (h *LanguageHandlers) setupInstall(w http.ResponseWriter, r *http.Request) {
	if h.SetupOpen == nil || !h.SetupOpen(w, r) {
		if h.SetupOpen == nil {
			webserver.Fail(w, http.StatusNotFound, "not found")
		}
		return
	}
	// Already installed for this version: answer without fetching, so the
	// open setup route can't be used to make the server fetch again and again.
	if h.Languages != nil && h.Languages.Current(r.PathValue("code")) {
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"languages": h.Languages.List()})
		return
	}
	h.install(w, r)
}

func (h *LanguageHandlers) ready(w http.ResponseWriter) bool {
	if h.Languages == nil {
		webserver.Fail(w, http.StatusServiceUnavailable, "language packs are not set up in this build")
		return false
	}
	return true
}

func (h *LanguageHandlers) list(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	h.Languages.LoadSizes(r.Context())
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"languages": h.Languages.List(), "auto_browser": h.Languages.AutoBrowser()})
}

func (h *LanguageHandlers) install(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	// The download goes on when the page is closed; the page asks again.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Minute)
	defer cancel()
	if err := h.Languages.Install(ctx, r.PathValue("code")); err != nil {
		failLanguage(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"languages": h.Languages.List()})
}

func (h *LanguageHandlers) remove(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	if err := h.Languages.Remove(r.PathValue("code")); err != nil {
		failLanguage(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"languages": h.Languages.List()})
}

// pack needs no sign-in: the sign-in page shows in the picked language too.
func (h *LanguageHandlers) pack(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	b, sum, err := h.Languages.Pack(r.PathValue("code"))
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, "this language is not installed")
		return
	}
	tag := `"` + sum + `"`
	w.Header().Set("ETag", tag)
	w.Header().Set("Cache-Control", "no-cache")
	if r.Header.Get("If-None-Match") == tag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

func failLanguage(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotListed):
		webserver.Fail(w, http.StatusNotFound, "no pack for this language and app version")
	case errors.Is(err, ErrNoKey):
		webserver.Fail(w, http.StatusConflict, "language packs can't be installed: this build has no signing key")
	case errors.Is(err, ErrFetch):
		webserver.Fail(w, http.StatusBadGateway, "the language couldn't be downloaded; check this server's internet connection")
	default:
		webserver.Fail(w, http.StatusUnprocessableEntity, "the language pack failed its safety check, so it wasn't installed")
	}
}
