package updates

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// InstallView is what Settings and Status show about in-app updates.
type InstallView struct {
	// Available says this build can install updates itself (keys pinned,
	// not a packaged build, Linux).
	Available bool   `json:"available"`
	Mode      string `json:"mode"`
	// ModeLocked says HOLYARRS_UPDATES sets the mode.
	ModeLocked bool `json:"mode_locked"`
	// Source is "image" or "update": where the running program came from.
	Source string `json:"source"`
	// Previous is the version "Go back" returns to ("" when none).
	Previous string       `json:"previous,omitempty"`
	Launch   LaunchStatus `json:"launch"`
	Progress Progress     `json:"progress"`
	// Ready is the downloaded, checked version waiting for "Update now".
	Ready   string `json:"ready,omitempty"`
	OneWay  bool   `json:"one_way,omitempty"`
	Running string `json:"running"`
}

// installRoutes adds the in-app update routes.
func (h *Handlers) installRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/app/update/install", h.installGet)
	m.HandleFunc("POST /api/v1/app/update/install", h.installNow)
	m.HandleFunc("POST /api/v1/app/update/go-back", h.goBack)
	m.HandleFunc("PUT /api/v1/app/update/mode", h.setMode)
}

// View is the in-app update state.
func (h *Handlers) View(ctx context.Context) InstallView {
	c := h.Checker
	mode, locked := c.Mode(ctx)
	v := InstallView{Available: c.CanInstall(), Mode: mode, ModeLocked: locked, Source: "image", Running: c.Version, Progress: Progress{Phase: PhaseIdle}}
	if in := c.Installer; in != nil {
		v.Launch = in.Launch()
		v.Progress = in.Progress()
		v.Previous = in.Previous(h.ImageVersion)
		if v.Previous != "" {
			v.Source = "update"
		}
		if ch, ok := in.Staged(); ok {
			v.Ready, v.OneWay = ch.Version, ch.OneWay
		}
	}
	return v
}

func (h *Handlers) installGet(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, h.View(r.Context()))
}

// installNow installs the ready version; waiting for idle can take minutes,
// so it runs on and the page follows the progress.
func (h *Handlers) installNow(w http.ResponseWriter, r *http.Request) {
	in := h.Checker.Installer
	if !h.Checker.CanInstall() {
		webserver.Fail(w, http.StatusConflict, "In-app updates are not available in this build.")
		return
	}
	if _, ok := in.Staged(); !ok {
		webserver.Fail(w, http.StatusConflict, ErrNotReady.Error())
		return
	}
	go func() {
		if err := in.Install(context.WithoutCancel(r.Context())); err != nil {
			log.Printf("updates: update now: %v", err)
		}
	}()
	webserver.WriteJSON(w, http.StatusAccepted, h.View(r.Context()))
}

func (h *Handlers) goBack(w http.ResponseWriter, r *http.Request) {
	in := h.Checker.Installer
	if in == nil {
		webserver.Fail(w, http.StatusConflict, "In-app updates are not available in this build.")
		return
	}
	if err := in.GoBack(); err != nil {
		webserver.Fail(w, http.StatusConflict, err.Error())
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, h.View(r.Context()))
}

func (h *Handlers) setMode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode string `json:"mode"`
	}
	if !webserver.Decode(w, r, &body, "the update mode is off, ask or auto") {
		return
	}
	if err := h.Checker.SetMode(r.Context(), body.Mode); err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, ErrModeLocked) {
			code = http.StatusConflict
		}
		webserver.Fail(w, code, err.Error())
		return
	}
	webserver.WriteJSON(w, http.StatusOK, h.View(r.Context()))
}
