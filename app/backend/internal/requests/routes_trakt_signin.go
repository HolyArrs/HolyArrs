package requests

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// TraktSignInRoutes registers the Trakt app secret and the Trakt sign-in
// routes (needs Secrets and TraktAuth).
func (h *OutsideListHandlers) TraktSignInRoutes(m *http.ServeMux) {
	m.HandleFunc("PUT /api/v1/settings/trakt-secret", h.putTraktSecret)
	m.HandleFunc("GET /api/v1/settings/trakt-signin", h.getTraktSignIn)
	m.HandleFunc("POST /api/v1/settings/trakt-signin", h.startTraktSignIn)
	m.HandleFunc("POST /api/v1/settings/trakt-signin/check", h.checkTraktSignIn)
	m.HandleFunc("DELETE /api/v1/settings/trakt-signin", h.endTraktSignIn)
}

// putTraktSecret saves the Trakt app's client secret, which Sign in with
// Trakt needs; an empty secret removes it.
func (h *OutsideListHandlers) putTraktSecret(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	var body struct {
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil || len(body.Secret) > 512 || strings.ContainsAny(body.Secret, "\r\n") {
		webserver.Fail(w, http.StatusBadRequest, `send {"secret": "<Trakt app client secret>"}, or {"secret": ""} to remove it`)
		return
	}
	if err := h.TraktAuth.SetSecret(r.Context(), strings.TrimSpace(body.Secret), h.Actor(r)); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, h.TraktAuth.Status(r.Context()))
}

// getTraktSignIn says whether the Trakt app is set up and who is signed in.
func (h *OutsideListHandlers) getTraktSignIn(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, h.TraktAuth.Status(r.Context()))
}

// startTraktSignIn asks Trakt for a code the person types at
// trakt.tv/activate.
func (h *OutsideListHandlers) startTraktSignIn(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	by := h.Actor(r)
	code, err := h.TraktAuth.StartSignIn(r.Context(), by.ID, by)
	if err != nil {
		h.traktFail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, code)
}

// checkTraktSignIn asks Trakt whether the code was approved: state is
// waiting, signed_in, denied or expired.
func (h *OutsideListHandlers) checkTraktSignIn(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	state, err := h.TraktAuth.CheckSignIn(r.Context())
	if err != nil {
		h.traktFail(w, err)
		return
	}
	if state == metadata.TraktSignedIn {
		h.traktChanged()
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"state": state, "status": h.TraktAuth.Status(r.Context())})
}

// endTraktSignIn signs out of Trakt here; public lists still read.
func (h *OutsideListHandlers) endTraktSignIn(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	if err := h.TraktAuth.SignOut(r.Context(), h.Actor(r)); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	h.traktChanged()
	webserver.WriteJSON(w, http.StatusOK, h.TraktAuth.Status(r.Context()))
}

// traktChanged starts a watch history read after a sign-in or sign-out.
func (h *OutsideListHandlers) traktChanged() {
	if h.TraktChanged != nil {
		h.TraktChanged()
	}
}

func (h *OutsideListHandlers) traktFail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, metadata.ErrTraktNoApp), errors.Is(err, metadata.ErrTraktNoSecret), errors.Is(err, metadata.ErrTraktNoSignIn):
		webserver.Fail(w, http.StatusConflict, err.Error())
	default:
		webserver.Fail(w, http.StatusBadGateway, err.Error())
	}
}
