package migrate

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func (h *Handlers) moveRoutes(m *webserver.Router) {
	m.Handle("POST /api/v1/migrate/plan", webserver.NeedUsers, http.HandlerFunc(h.movePlan))
	m.Handle("POST /api/v1/migrate/downloaders", webserver.NeedUsers, http.HandlerFunc(h.bringDownloaders))
	m.Handle("POST /api/v1/migrate/dry-run", webserver.NeedUsers, http.HandlerFunc(h.moveDryRun))
	m.Handle("GET /api/v1/migrate/checks/{id}", webserver.NeedUsers, http.HandlerFunc(h.getCheck))
	m.Handle("GET /api/v1/migrate/moves", webserver.NeedUsers, http.HandlerFunc(h.listMoves))
	m.Handle("POST /api/v1/migrate/moves", webserver.NeedUsers, http.HandlerFunc(h.startMove))
	m.Handle("GET /api/v1/migrate/moves/{id}", webserver.NeedUsers, http.HandlerFunc(h.getMove))
	m.Handle("GET /api/v1/migrate/moves/{id}/report", webserver.NeedUsers, http.HandlerFunc(h.moveReport))
	m.Handle("POST /api/v1/migrate/moves/{id}/undo", webserver.NeedUsers, http.HandlerFunc(h.undoMove))
	m.Handle("DELETE /api/v1/migrate/moves/{id}", webserver.NeedUsers, http.HandlerFunc(h.removeMove))
}

// MoveInput picks the read apps (kept snapshots) and the owner's choices;
// a move also sends the plan_hash its dry run answered.
type MoveInput struct {
	Snapshots []int64 `json:"snapshots"`
	Choices
	PlanHash string `json:"plan_hash,omitempty"`
	// Quick skips the titles preview in the plan (setup's Folders step
	// never shows it), so a folder pick answers at once.
	Quick bool `json:"quick,omitempty"`
}

// moveInput reads the body; it answers and returns false on a bad one.
func moveInput(w http.ResponseWriter, r *http.Request) (MoveInput, bool) {
	var in MoveInput
	if !webserver.Decode(w, r, &in, `send {"snapshots": [1, 2], "path_rules": [{"from": "/movies", "to": "/data/movies"}], "skip": ["alerts"], "people": {"person:seerr:2": "merge"}, "plan_hash": "<from the dry run>"}`) {
		return in, false
	}
	if len(in.People) > 1000 {
		webserver.Fail(w, http.StatusBadRequest, "at most 1000 person choices")
		return in, false
	}
	for k, v := range in.People {
		switch v {
		case PersonOwner, PersonAdmin, PersonMerge, PersonSkip:
		default:
			webserver.Fail(w, http.StatusBadRequest, fmt.Sprintf("person %q: choose owner, admin, merge or skip", k))
			return in, false
		}
	}
	if !validNames(in.Names) {
		webserver.Fail(w, http.StatusBadRequest, "names: choose keep or rename")
		return in, false
	}
	for _, s := range in.Skip {
		if GroupLabels[s] == "" || s == GroupOther {
			webserver.Fail(w, http.StatusBadRequest, fmt.Sprintf("unknown group %q", s))
			return in, false
		}
	}
	return in, true
}

func moveFail(w http.ResponseWriter, err error) {
	code, msg := failStatus(err)
	webserver.Fail(w, code, msg)
}

func isRuleError(err error) bool {
	var re *RuleError
	return errors.As(err, &re)
}

// movePlan starts the plan in the background (a check): folder checks and
// title previews never hold the request.
func (h *Handlers) movePlan(w http.ResponseWriter, r *http.Request) { h.startCheck(w, r, "plan") }

// moveDryRun starts the test run in the background (a check).
func (h *Handlers) moveDryRun(w http.ResponseWriter, r *http.Request) { h.startCheck(w, r, "dry_run") }

// refuseSafe answers 409 in safe mode: a move or an undo changes the
// Library, which safe mode keeps still.
func (h *Handlers) refuseSafe(w http.ResponseWriter) bool {
	if h.safeMode() {
		webserver.Fail(w, http.StatusConflict, "HolyArrs started in safe mode; restart it normally to import or undo")
		return true
	}
	return false
}

func (h *Handlers) startMove(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) || h.refuseSafe(w) {
		return
	}
	in, ok := moveInput(w, r)
	if !ok {
		return
	}
	mv, err := h.mover().Start(r.Context(), in.Snapshots, in.Choices, in.PlanHash, actor(r))
	if err != nil {
		moveFail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"move": mv, "job_id": mv.JobID})
}

func (h *Handlers) listMoves(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	ms, err := ListMoves(r.Context(), h.Store)
	if err != nil {
		webserver.Internal(w, "migrate", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"moves": ms})
}

func (h *Handlers) oneMove(w http.ResponseWriter, r *http.Request) (Move, bool) {
	if !webserver.RequireStrong(w, r) {
		return Move{}, false
	}
	id, ok := moveID(w, r)
	if !ok {
		return Move{}, false
	}
	mv, err := GetMove(r.Context(), h.Store, id)
	if err != nil {
		moveFail(w, err)
		return mv, false
	}
	return mv, true
}

// moveID reads the move id from the address; a bad one is a missing move.
func moveID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusNotFound, ErrNoMove.Error())
		return 0, false
	}
	return id, true
}

// getMove answers a move with its Job's progress while it runs.
func (h *Handlers) getMove(w http.ResponseWriter, r *http.Request) {
	mv, ok := h.oneMove(w, r)
	if !ok {
		return
	}
	var progress float64
	var note string
	job := mv.JobID
	if mv.UndoState == "queued" || mv.UndoState == "running" {
		job = mv.UndoJobID
	}
	if job != 0 {
		progress, note, _ = jobs.ProgressOf(r.Context(), h.Store.Read(), job)
	}
	if mv.State == "done" || mv.State == "undone" {
		progress = 1
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"move": mv, "progress": progress, "progress_note": note})
}

// moveReport answers the report as a Markdown file to keep.
func (h *Handlers) moveReport(w http.ResponseWriter, r *http.Request) {
	mv, ok := h.oneMove(w, r)
	if !ok {
		return
	}
	if mv.Report == nil {
		webserver.Fail(w, http.StatusConflict, "this import has no report yet")
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="holyarrs-move-%d.md"`, mv.ID))
	_, _ = w.Write([]byte(mv.Report.Markdown()))
}

func (h *Handlers) undoMove(w http.ResponseWriter, r *http.Request) {
	mv, ok := h.oneMove(w, r)
	if !ok || h.refuseSafe(w) {
		return
	}
	// The undo runs as its own Job; GET the move for its progress.
	mv, err := h.mover().StartUndo(r.Context(), mv.ID, actor(r))
	if err != nil {
		moveFail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"move": mv, "undo_job_id": mv.UndoJobID})
}

// removeMove drops an undone (or failed, empty) import from Earlier imports.
func (h *Handlers) removeMove(w http.ResponseWriter, r *http.Request) {
	mv, ok := h.oneMove(w, r)
	if !ok {
		return
	}
	if err := RemoveMove(r.Context(), h.Store, mv.ID); err != nil {
		moveFail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// bringDownloaders adds the read apps' Downloaders at once (the welcome
// Downloader step then shows them connected).
func (h *Handlers) bringDownloaders(w http.ResponseWriter, r *http.Request) {
	if h.refuseSafe(w) {
		return
	}
	var in struct {
		Snapshots []int64 `json:"snapshots"`
	}
	if !webserver.Decode(w, r, &in, `send {"snapshots": [1, 2]}`) {
		return
	}
	out, err := h.mover().BringDownloaders(r.Context(), in.Snapshots, actor(r))
	if err != nil {
		moveFail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"downloaders": out})
}
