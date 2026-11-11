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
	m.Handle("POST /api/v1/migrate/dry-run", webserver.NeedUsers, http.HandlerFunc(h.moveDryRun))
	m.Handle("GET /api/v1/migrate/moves", webserver.NeedUsers, http.HandlerFunc(h.listMoves))
	m.Handle("POST /api/v1/migrate/moves", webserver.NeedUsers, http.HandlerFunc(h.startMove))
	m.Handle("GET /api/v1/migrate/moves/{id}", webserver.NeedUsers, http.HandlerFunc(h.getMove))
	m.Handle("GET /api/v1/migrate/moves/{id}/report", webserver.NeedUsers, http.HandlerFunc(h.moveReport))
	m.Handle("POST /api/v1/migrate/moves/{id}/undo", webserver.NeedUsers, http.HandlerFunc(h.undoMove))
}

// MoveInput picks the read apps (kept snapshots) and the owner's choices;
// a move also sends the plan_hash its dry run answered.
type MoveInput struct {
	Snapshots []int64 `json:"snapshots"`
	Choices
	PlanHash string `json:"plan_hash,omitempty"`
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
	for _, s := range in.Skip {
		if GroupLabels[s] == "" || s == GroupOther {
			webserver.Fail(w, http.StatusBadRequest, fmt.Sprintf("unknown group %q", s))
			return in, false
		}
	}
	return in, true
}

func moveFail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNoSnapshot), errors.Is(err, ErrNoMove):
		webserver.Fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrMoveBusy), errors.Is(err, ErrUndone), errors.Is(err, ErrPlanChanged):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNoSnapshotPicked), errors.Is(err, ErrTooManySnaps):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	default:
		if isRuleError(err) {
			webserver.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		webserver.Internal(w, "migrate", err)
	}
}

func isRuleError(err error) bool {
	var re *RuleError
	return errors.As(err, &re)
}

func (h *Handlers) loadPlan(w http.ResponseWriter, r *http.Request) (*Plan, MoveInput, bool) {
	if !webserver.RequireStrong(w, r) {
		return nil, MoveInput{}, false
	}
	in, ok := moveInput(w, r)
	if !ok {
		return nil, in, false
	}
	p, err := LoadPlan(r.Context(), h.Store, in.Snapshots, in.Choices)
	if err != nil {
		moveFail(w, err)
		return nil, in, false
	}
	return p, in, true
}

func (h *Handlers) movePlan(w http.ResponseWriter, r *http.Request) {
	p, _, ok := h.loadPlan(w, r)
	if !ok {
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"plan": p, "counts": p.Counts(), "suggested_path_rules": SuggestRules(r.Context(), h.Store, p),
		"missing_paths": missingTargets(p), "seen_folders": SeenFolders(), "plan_hash": PlanHash(p)})
}

func (h *Handlers) moveDryRun(w http.ResponseWriter, r *http.Request) {
	p, _, ok := h.loadPlan(w, r)
	if !ok {
		return
	}
	rep := h.mover().Run(r.Context(), 0, p, true, actor(r), nil)
	// The move must start from this very plan: it sends this hash back.
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"report": rep, "plan_hash": PlanHash(p)})
}

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
