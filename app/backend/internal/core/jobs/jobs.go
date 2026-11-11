package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// claimPage is how many queued Jobs a gated claim reads at a time; it
// reads page after page until it finds one the gate does not hold.
const claimPage = 200

// errGateOn stops an ungated claim when a part was turned off meanwhile.
var errGateOn = errors.New("a part was turned off; claim through the gate")

// errCancelled is the cause a person's cancel gives the Job's context, so a
// stop that lands later still reads as a cancel.
var errCancelled = errors.New("job cancelled")

// Handler does the work of one Job kind. Returning an error retries the Job
// with back-off until it runs out of attempts.
type Handler func(ctx context.Context, r *Run) error

// ScheduleProbe is a read-only check before a scheduled Job is persisted.
// False means there is no work now. An error queues the durable Job so its
// usual retry and failure record are preserved.
type ScheduleProbe func(ctx context.Context, now time.Time) (needed bool, prepared any, err error)

// Options tunes the Queue. Zero values take the defaults.
type Options struct {
	Workers int // default 2; the size of the one shared pool when Pools is nil
	// Pools gives each pool its own workers (see PoolOf); nil keeps one
	// shared pool of Workers for every kind.
	Pools        map[Pool]int
	MaxAttempts  int           // default 5, for new Jobs
	BaseBackoff  time.Duration // default 5s; doubles on each failed attempt
	MaxBackoff   time.Duration // default 1h
	PollInterval time.Duration // default 1s; a read, never a write
	// OnChange is told about every state or progress change (for the live stream).
	OnChange func(Job)
}

// Gate holds Jobs back while the part they belong to is off (see
// internal/core/parts). Held Jobs stay queued and run once the part is on.
type Gate interface {
	// Active is false while every part is on: the queue then claims as
	// it always did.
	Active() bool
	// Held reports whether a queued Job of kind about subject must wait.
	Held(ctx context.Context, db database.Querier, kind string, subject *database.Subject) bool
	// KindOff reports whether a scheduled kind is not to be queued now.
	KindOff(kind string) bool
}

// OffKinds is a Gate that also lists the kinds held whatever their subject
// (a part's own kinds while it is off), so the claim leaves them out in
// SQL instead of reading past them.
type OffKinds interface {
	OffKinds() []string
}

// Queue runs Jobs stored in the database on a fixed pool of workers.
type Queue struct {
	s     *database.Store
	opt   Options
	mu    sync.Mutex
	hand  map[string]Handler
	probe map[string]ScheduleProbe
	// scheduleNoop is process-local. Restarting forgets every no-work result
	// and safely probes again.
	scheduleNoop          map[string]scheduleNoop
	scheduleProbeInFlight map[string]bool
	prepared              map[int64]preparedJob
	live                  map[int64]context.CancelFunc
	// running are the Jobs a worker of this Queue runs now.
	running map[int64]Job
	gate    Gate
	wake    chan struct{}
	stop    context.CancelFunc
	wg      sync.WaitGroup
}

// gatedCand is one queued Job a gated claim looks at.
type gatedCand struct {
	id      int64
	kind    string
	subject *database.Subject
}

// Run is one attempt at a Job, handed to its Handler.
type Run struct {
	Job      Job
	q        *Queue
	result   map[string]any
	prepared any
	app      context.Context // the queue's own context: done when the app stops
	ctx      context.Context // this attempt's context
}

type preparedJob struct {
	kind    string
	value   any
	expires time.Time
}

type errCode interface{ Code() string }

// New makes a Queue. Call Handle for each kind, then Start.
func New(s *database.Store, opt Options) *Queue {
	if opt.Workers <= 0 {
		opt.Workers = 2
	}
	if opt.MaxAttempts <= 0 {
		opt.MaxAttempts = 5
	}
	if opt.BaseBackoff <= 0 {
		opt.BaseBackoff = 5 * time.Second
	}
	if opt.MaxBackoff <= 0 {
		opt.MaxBackoff = time.Hour
	}
	if opt.PollInterval <= 0 {
		opt.PollInterval = time.Second
	}
	return &Queue{s: s, opt: opt, hand: map[string]Handler{}, probe: map[string]ScheduleProbe{}, scheduleNoop: map[string]scheduleNoop{}, scheduleProbeInFlight: map[string]bool{}, prepared: map[int64]preparedJob{}, live: map[int64]context.CancelFunc{}, running: map[int64]Job{}, wake: make(chan struct{}, 1)}
}

// Handle registers the handler for a Job kind.
func (q *Queue) Handle(kind string, h Handler) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.hand[kind] = h
}

// ProbeSchedule registers a read-only no-work check for one scheduled kind.
// Jobs started by hand bypass it.
func (q *Queue) ProbeSchedule(kind string, probe ScheduleProbe) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if probe == nil {
		delete(q.probe, kind)
		delete(q.scheduleNoop, kind)
		return
	}
	q.probe[kind] = probe
}

// SetGate makes the queue hold Jobs of parts that are off. Call it before
// Start.
func (q *Queue) SetGate(g Gate) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.gate = g
}

// Running returns the Jobs this Queue's workers run now.
func (q *Queue) Running() []Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Job, 0, len(q.running))
	for _, j := range q.running {
		out = append(out, j)
	}
	return out
}

// Enqueue adds a Job. With a dedupe key, an existing queued or running Job
// with that key is returned instead (created is false).
func (q *Queue) Enqueue(ctx context.Context, kind string, subject *database.Subject, params map[string]any, dedupeKey string, by database.Actor) (id int64, created bool, err error) {
	return q.EnqueueAt(ctx, kind, subject, params, dedupeKey, by, time.Time{})
}

// EnqueueAt is Enqueue for a Job that may not start before runAfter (zero
// means now). The time is part of the one insert, so no worker can claim
// the Job before it is set.
func (q *Queue) EnqueueAt(ctx context.Context, kind string, subject *database.Subject, params map[string]any, dedupeKey string, by database.Actor, runAfter time.Time) (id int64, created bool, err error) {
	return q.enqueueAt(ctx, kind, subject, params, dedupeKey, by, runAfter, nil)
}

// Start puts Jobs left running by a crash back in the queue and starts the
// workers.
func (q *Queue) Start(ctx context.Context) error {
	err := q.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE jobs SET state = 'queued', progress_note = 'restarted after a stop', updated_at = ? WHERE state = 'running'",
			database.FormatTime(time.Now()))
		return err
	})
	if err != nil {
		return fmt.Errorf("recovering jobs: %w", err)
	}
	ctx, q.stop = context.WithCancel(ctx)
	if q.opt.Pools == nil {
		for range q.opt.Workers {
			q.wg.Add(1)
			go q.work(ctx, "")
		}
		return nil
	}
	for _, p := range []Pool{PoolCore, PoolIO, PoolHeavy, PoolMigrate} {
		for range q.opt.Pools[p] {
			q.wg.Add(1)
			go q.work(ctx, p)
		}
	}
	return nil
}

// Stop cancels running Jobs and waits for the workers. Cancelled-by-stop
// Jobs stay running in the database and are recovered on the next Start.
func (q *Queue) Stop() {
	if q.stop != nil {
		q.stop()
	}
	q.wg.Wait()
	q.mu.Lock()
	clear(q.scheduleNoop)
	clear(q.prepared)
	q.mu.Unlock()
}

// Stopping says the Job's context ended because the app is stopping, not
// because someone cancelled the Job: the Job stays running in the database
// and is run again after the restart.
func (r *Run) Stopping() bool {
	return r.app != nil && r.app.Err() != nil && !r.cancelledByUser()
}

// TakePrepared returns a successful schedule probe's in-memory result once.
// Retries and Jobs recovered after a restart receive nil and fetch afresh.
func (r *Run) TakePrepared() any {
	prepared := r.prepared
	r.prepared = nil
	return prepared
}

// Progress records how far the Job is (0 to 1) with a short note.
func (r *Run) Progress(ctx context.Context, p float64, note string) error {
	p = min(max(p, 0), 1)
	err := r.q.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE jobs SET progress = ?, progress_note = ?, updated_at = ? WHERE id = ?",
			p, note, database.FormatTime(time.Now()), r.Job.ID)
		return err
	})
	if err == nil {
		r.Job.Progress, r.Job.ProgressNote = p, &note
		r.q.notify(r.Job)
	}
	return err
}

// ProgressOf reads how far a Job is and its note. It is zero and empty for a
// Job that does not exist.
func ProgressOf(ctx context.Context, q database.Querier, id int64) (float64, string, error) {
	var progress float64
	var note string
	err := q.QueryRowContext(ctx, "SELECT progress, COALESCE(progress_note, '') FROM jobs WHERE id = ?", id).Scan(&progress, &note)
	return progress, note, err
}

// SetResult stores a result to save when the Job is done.
func (r *Run) SetResult(v map[string]any) { r.result = v }

// SaveResult stores a result now, while the Job still runs, so the part
// done so far is kept if the Job is cancelled or the app stops; it is
// also the result saved when the Job is done.
func (r *Run) SaveResult(ctx context.Context, v map[string]any) error {
	r.result = v
	res, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return r.q.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE jobs SET result = ?, updated_at = ? WHERE id = ?", string(res), database.FormatTime(time.Now()), r.Job.ID)
		return err
	})
}

// Cancel stops a queued or running Job.
func (q *Queue) Cancel(ctx context.Context, id int64) error {
	var state string
	err := q.s.Write(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRow("SELECT state FROM jobs WHERE id = ?", id).Scan(&state); err != nil {
			return err
		}
		if state == "queued" {
			ts := database.FormatTime(time.Now())
			_, err := tx.Exec("UPDATE jobs SET state = 'cancelled', ended_at = ?, updated_at = ? WHERE id = ?", ts, ts, id)
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	switch state {
	case "queued":
		q.clearPrepared(id)
		return nil
	case "running":
		q.mu.Lock()
		c := q.live[id]
		q.mu.Unlock()
		if c == nil {
			return fmt.Errorf("job %d is running elsewhere", id)
		}
		c()
		return nil
	}
	return fmt.Errorf("job %d is %s and cannot be cancelled", id, state)
}

func (q *Queue) activeGate() Gate {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.gate == nil || !q.gate.Active() {
		return nil
	}
	return q.gate
}

func (q *Queue) enqueueAt(ctx context.Context, kind string, subject *database.Subject, params map[string]any, dedupeKey string, by database.Actor, runAfter time.Time, prepared any) (id int64, created bool, err error) {
	if params == nil {
		params = map[string]any{}
	}
	if runAfter.IsZero() {
		runAfter = time.Now()
	}
	j := &Job{Kind: kind, Subject: subject, Params: params, Priority: "normal", State: "queued",
		MaxAttempts: q.opt.MaxAttempts, RunAfter: runAfter.UTC(), AskedBy: by}
	if dedupeKey != "" {
		j.DedupeKey = &dedupeKey
	}
	err = q.s.Write(ctx, func(tx *sql.Tx) error {
		if dedupeKey != "" {
			err := tx.QueryRow("SELECT id FROM jobs WHERE dedupe_key = ? AND state IN ('queued', 'running')", dedupeKey).Scan(&id)
			if err == nil {
				return nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		var err error
		id, err = database.Insert(tx, "jobs", j, by)
		created = err == nil
		if created && prepared != nil {
			q.mu.Lock()
			q.prepared[id] = preparedJob{kind: kind, value: prepared}
			q.mu.Unlock()
		}
		return err
	})
	if err != nil {
		q.clearPrepared(id)
		return 0, false, err
	}
	if created {
		q.notify(*j)
		q.poke()
	}
	return id, created, nil
}

func (q *Queue) poke() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *Queue) work(ctx context.Context, pool Pool) {
	defer q.wg.Done()
	t := time.NewTicker(q.opt.PollInterval)
	defer t.Stop()
	stage := "jobs"
	if pool != "" {
		stage = "jobs_" + string(pool)
	}
	for {
		for ctx.Err() == nil {
			more := false
			// A crash outside a Handler (which has its own recover) is
			// caught here, so the worker keeps going.
			Guard(stage, func() {
				j, ok, err := q.claim(ctx, pool)
				if err != nil || !ok {
					return
				}
				more = true
				q.run(ctx, j)
			})
			if !more {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-q.wake:
		}
	}
}

func (q *Queue) claim(ctx context.Context, pool Pool) (Job, bool, error) {
	var j Job
	found := false
	kinds, args := poolFilter(pool)
	if g := q.activeGate(); g != nil {
		return q.claimGated(ctx, g, kinds, args)
	}
	// Look first on the read pool, so an idle queue never opens a write.
	var one int
	err := q.s.Read().QueryRowContext(ctx, "SELECT 1 FROM jobs WHERE state = 'queued' AND run_after <= ?"+kinds+" LIMIT 1",
		append([]any{database.FormatTime(time.Now())}, args...)...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return j, false, nil
	}
	if err != nil {
		return j, false, err
	}
	err = q.s.Write(ctx, func(tx *sql.Tx) error {
		// A part may have been turned off since the check above, and a
		// Job of it queued after that: claim through the gate instead.
		if q.activeGate() != nil {
			return errGateOn
		}
		var id int64
		now := database.FormatTime(time.Now())
		err := tx.QueryRow(`SELECT id FROM jobs WHERE state = 'queued' AND run_after <= ?`+kinds+`
			ORDER BY CASE priority WHEN 'now' THEN 0 WHEN 'normal' THEN 1 ELSE 2 END, run_after, id LIMIT 1`, append([]any{now}, args...)...).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE jobs SET state = 'running', attempt = attempt + 1, started_at = ?, updated_at = ? WHERE id = ?", now, now, id); err != nil {
			return err
		}
		found = true
		return database.Get(ctx, tx, "jobs", id, &j)
	})
	if errors.Is(err, errGateOn) {
		if g := q.activeGate(); g != nil {
			return q.claimGated(ctx, g, kinds, args)
		}
		return j, false, nil
	}
	if found && err == nil {
		q.notify(j)
	}
	return j, found && err == nil, err
}

// claimGated is claim while a part is off: it reads the next Jobs on the
// read pool (leaving out in SQL the kinds held whatever their subject),
// skips those the gate holds, page after page, and claims the first other
// one only if it is still queued and still not held.
func (q *Queue) claimGated(ctx context.Context, g Gate, kinds string, args []any) (Job, bool, error) {
	var j Job
	q.clearPreparedHeld(g)
	if ok, has := g.(OffKinds); has {
		if off := ok.OffKinds(); len(off) > 0 {
			kinds += " AND kind NOT IN (" + strings.TrimSuffix(strings.Repeat("?, ", len(off)), ", ") + ")"
			args = append([]any(nil), args...)
			for _, k := range off {
				args = append(args, k)
			}
		}
	}
	now := database.FormatTime(time.Now())
	for offset := 0; ; offset += claimPage {
		c, more, err := q.gatedPage(ctx, g, kinds, append([]any{now}, args...), offset)
		if err != nil || c == nil {
			if err == nil && more {
				continue
			}
			return j, false, err
		}
		return q.claimHeldCheck(ctx, g, *c)
	}
}

// gatedPage reads one page of queued Jobs and returns the first the gate
// does not hold; more says a further page may hold one.
func (q *Queue) gatedPage(ctx context.Context, g Gate, kinds string, args []any, offset int) (*gatedCand, bool, error) {
	db := q.s.Read()
	rows, err := db.QueryContext(ctx, `SELECT id, kind, subject FROM jobs WHERE state = 'queued' AND run_after <= ?`+kinds+`
		ORDER BY CASE priority WHEN 'now' THEN 0 WHEN 'normal' THEN 1 ELSE 2 END, run_after, id LIMIT ? OFFSET ?`,
		append(args, claimPage, offset)...)
	if err != nil {
		return nil, false, err
	}
	type cand = gatedCand
	var cs []cand
	for rows.Next() {
		var c cand
		var subj sql.NullString
		if err := rows.Scan(&c.id, &c.kind, &subj); err != nil {
			rows.Close()
			return nil, false, err
		}
		if subj.Valid && subj.String != "" && subj.String != "null" {
			c.subject = &database.Subject{}
			if json.Unmarshal([]byte(subj.String), c.subject) != nil {
				c.subject = nil
			}
		}
		cs = append(cs, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, false, err
	}
	for _, c := range cs {
		if !g.Held(ctx, db, c.kind, c.subject) {
			return &c, false, nil
		}
	}
	return nil, len(cs) == claimPage, nil
}

// claimHeldCheck claims c if it is still queued and, checked again inside
// the write, still not held (a part may have been turned off meanwhile).
func (q *Queue) claimHeldCheck(ctx context.Context, g Gate, c gatedCand) (Job, bool, error) {
	var j Job
	id := c.id
	found := false
	err := q.s.Write(ctx, func(tx *sql.Tx) error {
		if g.Held(ctx, tx, c.kind, c.subject) {
			return nil // turned off meanwhile: it waits
		}
		now := database.FormatTime(time.Now())
		res, err := tx.Exec("UPDATE jobs SET state = 'running', attempt = attempt + 1, started_at = ?, updated_at = ? WHERE id = ? AND state = 'queued'", now, now, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil // another worker took it
		}
		found = true
		return database.Get(ctx, tx, "jobs", id, &j)
	})
	if found && err == nil {
		q.notify(j)
	}
	return j, found && err == nil, err
}

// cancelledByUser says someone cancelled the Job before anything else
// ended its context.
func (r *Run) cancelledByUser() bool {
	return r.ctx != nil && errors.Is(context.Cause(r.ctx), errCancelled)
}

func (q *Queue) run(ctx context.Context, j Job) {
	q.mu.Lock()
	h := q.hand[j.Kind]
	entry := q.prepared[j.ID]
	prepared := entry.value
	if !entry.expires.IsZero() && !time.Now().Before(entry.expires) {
		prepared = nil
	}
	delete(q.prepared, j.ID)
	runCtx, cancel := context.WithCancelCause(ctx)
	q.live[j.ID] = func() { cancel(errCancelled) }
	q.running[j.ID] = j
	q.mu.Unlock()
	defer func() {
		q.mu.Lock()
		delete(q.live, j.ID)
		delete(q.running, j.ID)
		q.mu.Unlock()
		cancel(nil)
	}()

	r := &Run{Job: j, q: q, prepared: prepared, app: ctx, ctx: runCtx}
	var err error
	code := "failed"
	if h == nil {
		err, code = fmt.Errorf("no handler for job kind %q", j.Kind), "no_handler"
	} else {
		err = safeCall(runCtx, h, r)
		var c errCode
		if errors.As(err, &c) {
			code = c.Code()
		}
	}
	if ctx.Err() != nil && !r.cancelledByUser() {
		return // stopping: leave it running, Start recovers it
	}
	cancelled := runCtx.Err() != nil && err != nil
	q.finish(j, r.result, err, code, cancelled)
}

func safeCall(ctx context.Context, h Handler, r *Run) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("job panicked (%T)", p) // the type only: the text may hold a secret
		}
	}()
	return h(ctx, r)
}

func (q *Queue) backoff(attempt int) time.Duration {
	d := q.opt.BaseBackoff
	for i := 1; i < attempt && d < q.opt.MaxBackoff; i++ {
		d *= 2
	}
	return min(d, q.opt.MaxBackoff)
}

func (q *Queue) finish(j Job, result map[string]any, runErr error, code string, cancelled bool) {
	q.clearPrepared(j.ID)
	now := time.Now().UTC()
	ctx := context.Background()
	_ = q.s.Write(ctx, func(tx *sql.Tx) error {
		ts := database.FormatTime(now)
		switch {
		case cancelled:
			_, err := tx.Exec("UPDATE jobs SET state = 'cancelled', ended_at = ?, updated_at = ? WHERE id = ?", ts, ts, j.ID)
			return err
		case runErr == nil:
			res, _ := json.Marshal(result)
			if result == nil {
				res = []byte("{}")
			}
			_, err := tx.Exec("UPDATE jobs SET state = 'done', progress = 1, result = ?, error = NULL, ended_at = ?, updated_at = ? WHERE id = ?",
				string(res), ts, ts, j.ID)
			return err
		}
		e, _ := json.Marshal(JobError{Code: code, Message: runErr.Error()})
		var fe interface{ Final() bool }
		final := errors.As(runErr, &fe) && fe.Final()
		if j.Attempt < j.MaxAttempts && code != "no_handler" && !final {
			next := database.FormatTime(now.Add(q.backoff(j.Attempt)))
			_, err := tx.Exec("UPDATE jobs SET state = 'queued', run_after = ?, error = ?, updated_at = ? WHERE id = ?", next, string(e), ts, j.ID)
			return err
		}
		_, err := tx.Exec("UPDATE jobs SET state = 'failed', error = ?, ended_at = ?, updated_at = ? WHERE id = ?", string(e), ts, ts, j.ID)
		return err
	})
	var fresh Job
	if database.Get(ctx, q.s.Read(), "jobs", j.ID, &fresh) == nil {
		q.notify(fresh)
	}
}

func (q *Queue) clearPrepared(id int64) {
	if id == 0 {
		return
	}
	q.mu.Lock()
	delete(q.prepared, id)
	q.mu.Unlock()
}

func (q *Queue) clearPreparedHeld(g Gate) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for id, prepared := range q.prepared {
		if g.KindOff(prepared.kind) {
			delete(q.prepared, id)
		}
	}
}

func (q *Queue) notify(j Job) {
	if q.opt.OnChange != nil {
		q.opt.OnChange(j)
	}
}
