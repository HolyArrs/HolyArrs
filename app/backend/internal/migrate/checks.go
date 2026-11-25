package migrate

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// A check is the plan or the test run worked out in the background:
// POST answers at once with its id and GET /api/v1/migrate/checks/{id}
// tells how far it is, the same way Sonarr runs long work as commands. No
// request waits on show lookups or folder checks. Checks live in memory: a
// restart forgets them, GET then answers 410 and the page starts again.
type bgCheck struct {
	mu      sync.Mutex
	id, key string
	kind    string // plan, dry_run
	by      int64
	state   string // running, done, failed
	step    string // reading, folders, titles
	done    int
	total   int
	now     string
	counts  map[string]int
	result  map[string]any
	status  int
	errText string
	ended   time.Time
	cancel  context.CancelFunc
}

const (
	checkKeep    = 10 * time.Minute // a finished check is kept this long
	checkMax     = 8                // checks kept at once
	checkTimeout = time.Hour        // one check never runs longer
)

type checkList struct {
	mu   sync.Mutex
	byID map[string]*bgCheck
}

// start returns the running check for the same input, or starts work.
// A newer check of the same kind by the same person stops the older one:
// the page changed its choices and only wants the latest answer.
func (cl *checkList) start(kind string, in MoveInput, by domain.Actor, work func(ctx context.Context, c *bgCheck) (map[string]any, error)) *bgCheck {
	b, _ := json.Marshal(in)
	sum := sha256.Sum256(append([]byte(kind+"\x00"), b...))
	key := hex.EncodeToString(sum[:])
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if cl.byID == nil {
		cl.byID = map[string]*bgCheck{}
	}
	cl.prune()
	for _, c := range cl.byID {
		c.mu.Lock()
		running := c.state == "running"
		same := c.kind == kind && c.by == by.ID
		c.mu.Unlock()
		if same && running {
			if c.key == key {
				return c
			}
			c.cancel()
		}
	}
	var idb [9]byte
	_, _ = rand.Read(idb[:])
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	c := &bgCheck{id: hex.EncodeToString(idb[:]), key: key, kind: kind, by: by.ID, state: "running", step: "reading", counts: map[string]int{}, cancel: cancel}
	cl.byID[c.id] = c
	go func() {
		defer cancel()
		res, err := safeCheck(ctx, c, work)
		c.mu.Lock()
		defer c.mu.Unlock()
		c.ended = time.Now()
		if err != nil {
			c.state = "failed"
			if ctx.Err() != nil {
				// Stopped by newer choices, or past checkTimeout.
				c.status, c.errText = http.StatusConflict, "this check was stopped; start it again"
				return
			}
			c.status, c.errText = failStatus(err)
			return
		}
		c.state, c.result = "done", res
	}()
	return c
}

func safeCheck(ctx context.Context, c *bgCheck, work func(ctx context.Context, c *bgCheck) (map[string]any, error)) (res map[string]any, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = errors.New("the check stopped unexpectedly")
			log.Printf("migrate check %s: panic: %v", c.id, p)
		}
	}()
	return work(ctx, c)
}

// prune drops finished checks past checkKeep, then the oldest finished
// ones over checkMax. Callers hold cl.mu.
func (cl *checkList) prune() {
	var oldest *bgCheck
	n := 0
	for id, c := range cl.byID {
		c.mu.Lock()
		ended := c.ended
		c.mu.Unlock()
		if !ended.IsZero() && time.Since(ended) > checkKeep {
			delete(cl.byID, id)
			continue
		}
		n++
		if !ended.IsZero() && (oldest == nil || ended.Before(oldest.ended)) {
			oldest = c
		}
	}
	if n >= checkMax && oldest != nil {
		delete(cl.byID, oldest.id)
	}
}

func (cl *checkList) get(id string) *bgCheck {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.byID[id]
}

func (c *bgCheck) setStep(step string) {
	c.mu.Lock()
	c.step = step
	c.mu.Unlock()
}

func (c *bgCheck) item(done, total int, name, result string) {
	c.mu.Lock()
	c.step, c.done, c.total, c.now = "titles", done, total, name
	c.counts[result]++
	c.mu.Unlock()
}

// view is what GET answers: progress while it runs, the result when done.
func (c *bgCheck) view() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	counts := make(map[string]int, len(c.counts))
	for k, v := range c.counts {
		counts[k] = v
	}
	v := map[string]any{"check_id": c.id, "kind": c.kind, "state": c.state, "step": c.step, "done": c.done, "total": c.total, "now": c.now, "counts": counts}
	for k, x := range c.result {
		v[k] = x
	}
	return v
}

// failStatus maps an error to the answer the old waiting routes gave.
func failStatus(err error) (int, string) {
	switch {
	case errors.Is(err, ErrNoSnapshot), errors.Is(err, ErrNoMove):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, ErrMoveBusy), errors.Is(err, ErrUndone), errors.Is(err, ErrPlanChanged), errors.Is(err, ErrMoveKept):
		return http.StatusConflict, err.Error()
	case errors.Is(err, ErrNoSnapshotPicked), errors.Is(err, ErrTooManySnaps), isRuleError(err):
		return http.StatusBadRequest, err.Error()
	}
	log.Printf("migrate: %v", err)
	return http.StatusInternalServerError, "something went wrong; see the log"
}

// startCheck answers 202 with the check; GET it for progress and result.
func (h *Handlers) startCheck(w http.ResponseWriter, r *http.Request, kind string) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	in, ok := moveInput(w, r)
	if !ok {
		return
	}
	in.PlanHash = ""
	// Bad input answers at once, as before; only real work goes to the check.
	var bad error
	switch {
	case len(in.Snapshots) == 0:
		bad = ErrNoSnapshotPicked
	case len(in.Snapshots) > 12:
		bad = ErrTooManySnaps
	default:
		bad = ValidRules(in.PathRules)
	}
	if bad != nil {
		moveFail(w, bad)
		return
	}
	work := h.planWork(in)
	if kind == "dry_run" {
		work = h.dryWork(in, actor(r))
	}
	c := h.checks.start(kind, in, actor(r), work)
	webserver.WriteJSON(w, http.StatusAccepted, c.view())
}

func (h *Handlers) planWork(in MoveInput) func(ctx context.Context, c *bgCheck) (map[string]any, error) {
	return func(ctx context.Context, c *bgCheck) (map[string]any, error) {
		p, err := LoadPlan(ctx, h.Store, in.Snapshots, in.Choices)
		if err != nil {
			return nil, err
		}
		c.setStep("folders")
		hash, rules, missing := PlanHash(p), SuggestRules(ctx, h.Store, p), missingTargets(p)
		c.setStep("titles")
		// The titles list says what the import will do, as the dry run would.
		if !in.Quick {
			h.mover().PreviewTitles(ctx, p)
		}
		MarkUnseen(p)
		// Folder checks stay inside the mounted and Library folders.
		roots := browseRoots(ctx, h.Store)
		rt := newRooted(roots)
		MarkFound(p, rt)
		here := HereFolders(ctx, h.Store)
		if needsSuggest(p) {
			MarkSuggest(p, deepFolders(roots, func(d string) []string { return rt.sub(d, hereMax) }), rt.dir)
		}
		rt.close()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return map[string]any{"plan": p, "plan_counts": p.Counts(), "suggested_path_rules": rules,
			"missing_paths": missing, "seen_folders": SeenFolders(), "here_folders": here, "plan_hash": hash}, nil
	}
}

func (h *Handlers) dryWork(in MoveInput, by domain.Actor) func(ctx context.Context, c *bgCheck) (map[string]any, error) {
	return func(ctx context.Context, c *bgCheck) (map[string]any, error) {
		p, err := LoadPlan(ctx, h.Store, in.Snapshots, in.Choices)
		if err != nil {
			return nil, err
		}
		c.setStep("titles")
		rep := h.mover().DryCheck(ctx, p, by, c.item)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// The move must start from this very plan: it sends this hash back.
		return map[string]any{"report": rep, "plan_hash": PlanHash(p)}, nil
	}
}

// getCheck answers a check's progress, its result once done, or its error
// with the status the waiting routes used. 410: unknown (a restart).
func (h *Handlers) getCheck(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	c := h.checks.get(r.PathValue("id"))
	if c == nil || c.by != actor(r).ID {
		webserver.Fail(w, http.StatusGone, "this check is no longer kept; start it again")
		return
	}
	c.mu.Lock()
	failed, status, msg := c.state == "failed", c.status, c.errText
	c.mu.Unlock()
	if failed {
		webserver.Fail(w, status, msg)
		return
	}
	webserver.WriteJSONNoStore(w, http.StatusOK, c.view())
}
