package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/auth"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
)

func (a *API) moveRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/migrate/plan", a.movePlan)
	m.HandleFunc("POST /api/v1/migrate/dry-run", a.moveDryRun)
	m.HandleFunc("GET /api/v1/migrate/moves", a.listMoves)
	m.HandleFunc("POST /api/v1/migrate/moves", a.startMove)
	m.HandleFunc("GET /api/v1/migrate/moves/{id}", a.getMove)
	m.HandleFunc("GET /api/v1/migrate/moves/{id}/report", a.moveReport)
	m.HandleFunc("POST /api/v1/migrate/moves/{id}/undo", a.undoMove)
}

// mover is the move runner (made on first use in tests).
func (a *API) mover() *migrate.Mover {
	if a.s.Mover == nil {
		a.s.Mover = &migrate.Mover{Store: a.s.Store, Library: a.s.Library, Queue: a.s.Queue}
	}
	if a.s.Mover.PartOn == nil {
		a.s.Mover.PartOn = func(p string) bool { return !a.safeMode() && a.partIsOn(parts.Part(p)) }
	}
	return a.s.Mover
}

func (a *API) safeMode() bool { return a.s.SafeMode || (a.s.Parts != nil && a.s.Parts.SafeMode) }

// MoveInput picks the read apps (kept snapshots) and the owner's choices;
// a move also sends the plan_hash its dry run answered.
type MoveInput struct {
	Snapshots []int64 `json:"snapshots"`
	migrate.Choices
	PlanHash string `json:"plan_hash,omitempty"`
}

// moveInput reads the body; it answers and returns false on a bad one.
func moveInput(w http.ResponseWriter, r *http.Request) (MoveInput, bool) {
	var in MoveInput
	if !decode(w, r, &in, `send {"snapshots": [1, 2], "path_rules": [{"from": "/movies", "to": "/data/movies"}], "skip": ["alerts"], "people": {"person:seerr:2": "merge"}, "plan_hash": "<from the dry run>"}`) {
		return in, false
	}
	if len(in.People) > 1000 {
		fail(w, http.StatusBadRequest, "at most 1000 person choices")
		return in, false
	}
	for k, v := range in.People {
		switch v {
		case migrate.PersonOwner, migrate.PersonAdmin, migrate.PersonMerge, migrate.PersonSkip:
		default:
			fail(w, http.StatusBadRequest, fmt.Sprintf("person %q: choose owner, admin, merge or skip", k))
			return in, false
		}
	}
	for _, s := range in.Skip {
		if migrate.GroupLabels[s] == "" || s == migrate.GroupOther {
			fail(w, http.StatusBadRequest, fmt.Sprintf("unknown group %q", s))
			return in, false
		}
	}
	return in, true
}

func moveFail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, migrate.ErrNoSnapshot), errors.Is(err, migrate.ErrNoMove):
		fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, migrate.ErrMoveBusy), errors.Is(err, migrate.ErrUndone), errors.Is(err, migrate.ErrPlanChanged):
		fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, migrate.ErrNoSnapshotPicked), errors.Is(err, migrate.ErrTooManySnaps):
		fail(w, http.StatusBadRequest, err.Error())
	default:
		if isRuleError(err) {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
		internal(w, err)
	}
}

func isRuleError(err error) bool {
	var re *migrate.RuleError
	return errors.As(err, &re)
}

func (a *API) loadPlan(w http.ResponseWriter, r *http.Request) (*migrate.Plan, MoveInput, bool) {
	if !auth.RequireStrong(w, r) {
		return nil, MoveInput{}, false
	}
	in, ok := moveInput(w, r)
	if !ok {
		return nil, in, false
	}
	p, err := migrate.LoadPlan(r.Context(), a.s.Store, in.Snapshots, in.Choices)
	if err != nil {
		moveFail(w, err)
		return nil, in, false
	}
	return p, in, true
}

func (a *API) movePlan(w http.ResponseWriter, r *http.Request) {
	p, _, ok := a.loadPlan(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plan": p, "counts": p.Counts(), "suggested_path_rules": migrate.SuggestRules(r.Context(), a.s.Store, p),
		"plan_hash": migrate.PlanHash(p)})
}

func (a *API) moveDryRun(w http.ResponseWriter, r *http.Request) {
	p, _, ok := a.loadPlan(w, r)
	if !ok {
		return
	}
	rep := a.mover().Run(r.Context(), 0, p, true, actor(r), nil)
	// The move must start from this very plan: it sends this hash back.
	writeJSON(w, http.StatusOK, map[string]any{"report": rep, "plan_hash": migrate.PlanHash(p)})
}

// refuseSafe answers 409 in safe mode: a move or an undo changes the
// Library, which safe mode keeps still.
func (a *API) refuseSafe(w http.ResponseWriter) bool {
	if a.safeMode() {
		fail(w, http.StatusConflict, "HolyArrs started in safe mode; restart it normally to move or undo")
		return true
	}
	return false
}

func (a *API) startMove(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) || a.refuseSafe(w) {
		return
	}
	in, ok := moveInput(w, r)
	if !ok {
		return
	}
	mv, err := a.mover().Start(r.Context(), in.Snapshots, in.Choices, in.PlanHash, actor(r))
	if err != nil {
		moveFail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"move": mv, "job_id": mv.JobID})
}

func (a *API) listMoves(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	ms, err := migrate.ListMoves(r.Context(), a.s.Store)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"moves": ms})
}

func (a *API) oneMove(w http.ResponseWriter, r *http.Request) (migrate.Move, bool) {
	if !auth.RequireStrong(w, r) {
		return migrate.Move{}, false
	}
	id, ok := pathID(w, r)
	if !ok {
		return migrate.Move{}, false
	}
	mv, err := migrate.GetMove(r.Context(), a.s.Store, id)
	if err != nil {
		moveFail(w, err)
		return mv, false
	}
	return mv, true
}

// getMove answers a move with its Job's progress while it runs.
func (a *API) getMove(w http.ResponseWriter, r *http.Request) {
	mv, ok := a.oneMove(w, r)
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
		_ = a.s.Store.Read().QueryRowContext(r.Context(), "SELECT progress, COALESCE(progress_note, '') FROM jobs WHERE id = ?", job).Scan(&progress, &note)
	}
	if mv.State == "done" || mv.State == "undone" {
		progress = 1
	}
	writeJSON(w, http.StatusOK, map[string]any{"move": mv, "progress": progress, "progress_note": note})
}

// moveReport answers the report as a Markdown file to keep.
func (a *API) moveReport(w http.ResponseWriter, r *http.Request) {
	mv, ok := a.oneMove(w, r)
	if !ok {
		return
	}
	if mv.Report == nil {
		fail(w, http.StatusConflict, "this move has no report yet")
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="holyarrs-move-%d.md"`, mv.ID))
	_, _ = w.Write([]byte(mv.Report.Markdown()))
}

func (a *API) undoMove(w http.ResponseWriter, r *http.Request) {
	mv, ok := a.oneMove(w, r)
	if !ok || a.refuseSafe(w) {
		return
	}
	// The undo runs as its own Job; GET the move for its progress.
	mv, err := a.mover().StartUndo(r.Context(), mv.ID, actor(r))
	if err != nil {
		moveFail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"move": mv, "undo_job_id": mv.UndoJobID})
}
