package backups

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// RestorePath takes the uploaded zip; its body may be larger than other API bodies.
const RestorePath = "/api/v1/backups/restore"

// Routes adds the backup routes. restart, when set, is called after a
// restore is staged so the swap happens right away.
func (m *Manager) Routes(mux *http.ServeMux, restart func()) {
	m.keyKitRoute(mux)
	mux.HandleFunc("GET /api/v1/backups", func(w http.ResponseWriter, r *http.Request) {
		list, err := m.List()
		if err != nil {
			webserver.Fail(w, http.StatusInternalServerError, "could not list backups")
			return
		}
		if list == nil {
			list = []Info{}
		}
		p, _ := m.Preset(r.Context())
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"backups": list, "preset": p, "restore_waiting": Staged(m.Dir), "key_note": KeyNote,
			"key_kit": m.keyKitState(r.Context())})
	})
	mux.HandleFunc("POST /api/v1/backups", func(w http.ResponseWriter, r *http.Request) {
		if err := m.roomFor(); err != nil {
			log.Printf("backup: %v", err)
			webserver.Fail(w, http.StatusInsufficientStorage, err.Error()+" and try again.")
			return
		}
		// A dropped request (closed tab, proxy timeout) must not cut the
		// backup off; only a stop of HolyArrs does.
		ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
		defer cancel()
		if m.Stop != nil {
			defer context.AfterFunc(m.Stop, cancel)()
		}
		b, err := m.Create(ctx, KindManual)
		if err != nil {
			log.Printf("backup: %v", err)
			if diskFull(err) {
				webserver.Fail(w, http.StatusInsufficientStorage, noSpace+" and try again.")
				return
			}
			webserver.Fail(w, http.StatusInternalServerError, "the backup failed; see the log")
			return
		}
		webserver.WriteJSON(w, http.StatusCreated, b)
	})
	mux.HandleFunc("PUT /api/v1/backups/settings", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Preset Preset `json:"preset"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
			webserver.Fail(w, http.StatusBadRequest, `send {"preset": "balanced"|"light"|"careful"|"off"}`)
			return
		}
		if err := m.SetPreset(r.Context(), body.Preset, webserver.Actor(r.Context())); err != nil {
			webserver.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"preset": body.Preset})
	})
	mux.HandleFunc("GET /api/v1/backups/{name}", func(w http.ResponseWriter, r *http.Request) {
		if !webserver.RequireStrong(w, r) {
			return
		}
		f, err := m.Open(r.PathValue("name"))
		if err != nil {
			webserver.Fail(w, http.StatusNotFound, "no such backup")
			return
		}
		defer f.Close()
		// A backup holds the whole database (password hashes too), so taking one is on the audit trail.
		m.audit(r, "downloaded", r.PathValue("name"))
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+r.PathValue("name")+`"`)
		if fi, err := f.Stat(); err == nil {
			w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
		}
		_, _ = io.Copy(w, f)
	})
	mux.HandleFunc("DELETE /api/v1/backups/{name}", func(w http.ResponseWriter, r *http.Request) {
		if !webserver.RequireStrong(w, r) {
			return
		}
		if err := m.Delete(r.PathValue("name")); err != nil {
			webserver.Fail(w, http.StatusNotFound, "no such backup")
			return
		}
		m.audit(r, "deleted", r.PathValue("name"))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/backups/restore", func(w http.ResponseWriter, r *http.Request) {
		if !webserver.RequireStrong(w, r) {
			return
		}
		if err := m.Stage(r.Context(), http.MaxBytesReader(w, r.Body, MaxUpload)); err != nil {
			webserver.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Printf("backup: a restore is staged; it is put in place on the next start")
		webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"staged": true, "restarting": restart != nil,
			"note": "The backup is checked and staged. HolyArrs puts it in place when it starts again."})
		if restart != nil {
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			go restart()
		}
	})
}

// audit writes a backup_used Event: who downloaded or deleted a backup.
func (m *Manager) audit(r *http.Request, action, name string) {
	if m.Store == nil {
		return
	}
	who := "someone"
	if w, ok := webserver.From(r.Context()); ok && w.Name != "" {
		who = w.Name
	}
	_ = m.Store.AddEvent(context.WithoutCancel(r.Context()), &domain.Event{Actor: webserver.Actor(r.Context()), Kind: "backup_used",
		Summary: "Backup " + name + " " + action + " by " + who, Data: map[string]any{"action": action, "name": name}})
}
