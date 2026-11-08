package parts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
)

// Errors a control answers with; their text is shown to people.
var (
	ErrSafeMode = errors.New("the app runs in safe mode: every part is off until it starts again without safe mode")
	ErrNoPart   = errors.New("no such part: use movies, shows, music, subtitles or wishlist")
	ErrNoStage  = errors.New("no such stage, or it cannot be paused on its own (restart the app instead)")
	ErrBusy     = errors.New("another control is still running; try again in a moment")
)

// NeedsError is a switch refused because of what another part needs.
type NeedsError struct{ Text string }

func (e NeedsError) Error() string { return e.Text }

// OffError is a stage that cannot start because its part is off.
type OffError struct{ Part Part }

func (e OffError) Error() string { return e.Part.Label() + " is off; turn it on first" }

// Running lists the Jobs the queue's workers run now (jobs.Queue does).
type Running interface {
	Running() []domain.Job
}

// settings row that keeps the parts a person turned off.
const settingGroup, settingKey = "this_app", "parts_off"

// DefaultDrain is how long a restart waits for running Jobs of the part.
const DefaultDrain = 2 * time.Minute

// Control turns parts on, off and restarts them while the app runs, and
// pauses, resumes and restarts single stages. It is the queue's Gate:
// Jobs of a part that is off wait in the queue.
type Control struct {
	Store *database.Store
	Reg   *jobs.Registry
	// SafeMode forces every part off (not saved) and refuses switches.
	SafeMode bool
	// Jobs is the queue, for waiting on running Jobs during a restart.
	Jobs Running
	// Drain caps that wait (default DefaultDrain); a Job still running
	// then simply carries on: it is never cut off.
	Drain time.Duration
	Logf  func(format string, args ...any)

	busy   sync.Mutex // one control at a time
	mu     sync.Mutex
	off    map[Part]string // part -> reason it is off
	hold   map[Part]bool   // parts held for a restart
	active atomic.Bool     // any part off or held
	ctx    context.Context
	loops  []*entry
}

// entry is one stage this Control starts and stops.
type entry struct {
	l        jobs.Loop
	owner    Owner
	optional bool // not started in safe mode
	cancel   context.CancelFunc
	done     <-chan struct{}
	paused   string // why a person paused it; empty when not paused
}

func (c *Control) logf(format string, args ...any) {
	if c.Logf != nil {
		c.Logf(format, args...)
		return
	}
	log.Printf(format, args...)
}

func (c *Control) init() {
	if c.off == nil {
		c.off = map[Part]string{}
		c.hold = map[Part]bool{}
	}
}

// Load reads which parts a person turned off. In safe mode every part is
// off whatever was saved.
func (c *Control) Load(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.init()
	if c.SafeMode {
		for _, p := range All() {
			c.off[p] = "Safe mode: every part is off until the app starts again without safe mode."
		}
		c.flag()
		return nil
	}
	var raw string
	err := c.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = ? AND key = ?", settingGroup, settingKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		c.flag()
		return nil
	}
	if err != nil {
		return err
	}
	var saved []Part
	if err := json.Unmarshal([]byte(raw), &saved); err != nil {
		c.logf("parts: saved part switches unreadable; every part stays on")
	}
	for _, p := range saved {
		if Valid(p) {
			c.off[p] = "Turned off in the part switches."
		}
	}
	c.flag()
	return nil
}

func (c *Control) flag() { c.active.Store(len(c.off) > 0 || len(c.hold) > 0) }

// Add registers a background loop of owner (from Loops) for Start. An
// optional loop is not started in safe mode.
func (c *Control) Add(l jobs.Loop, optional bool) {
	owner, ok := startup.OwnerOf(startup.LoopOwners, l.Name)
	if !ok {
		owner = Core
		c.logf("parts: loop %s has no row in the parts table; treated as core", l.Name)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loops = append(c.loops, &entry{l: l, owner: owner, optional: optional})
}

// notStarted is the reason a safe-mode stage shows.
const notStarted = "Safe mode: not started. Restart without safe mode to turn it on."

// Start starts every added loop whose part is on; it keeps ctx for later
// switches.
func (c *Control) Start(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.init()
	c.ctx = ctx
	for _, e := range c.loops {
		c.startLocked(e)
	}
}

// ownerOnLocked reports whether owner is on (ByTitle counts as on).
func (c *Control) ownerOnLocked(o Owner) bool {
	switch o {
	case Core, ByTitle, "":
		return true
	case Media:
		return c.onLocked(Movies) || c.onLocked(Shows) || c.onLocked(Music)
	}
	return c.onLocked(Part(o))
}

func (c *Control) onLocked(p Part) bool {
	_, off := c.off[p]
	return !off && !c.hold[p]
}

// startLocked starts e unless it runs, is paused, or its part is off; a
// loop whose last run has not returned is never started twice.
func (c *Control) startLocked(e *entry) error {
	key := jobs.StageKey(e.l.Name)
	if e.optional && c.SafeMode {
		c.Reg.Set(key, "stage", e.l.Label, jobs.Off, notStarted)
		return ErrSafeMode
	}
	if e.cancel != nil {
		return nil // runs
	}
	if e.done != nil {
		select {
		case <-e.done:
		default:
			return fmt.Errorf("%s: its last pass has not returned (stuck); a whole-app restart clears it", e.l.Label)
		}
	}
	if e.paused != "" {
		c.Reg.Set(key, "stage", e.l.Label, jobs.Off, e.paused)
		return nil
	}
	if !c.ownerOnLocked(e.owner) {
		c.Reg.Set(key, "stage", e.l.Label, jobs.Off, offReason(e.owner))
		return nil
	}
	if c.ctx == nil {
		return nil // Start not called yet
	}
	ctx, cancel := context.WithCancel(c.ctx)
	e.cancel = cancel
	e.done = c.Reg.Go(ctx, e.l)
	return nil
}

func offReason(o Owner) string {
	if o == Media {
		return "Movies, TV shows and Music are all off."
	}
	return Part(o).Label() + " is off."
}

// stop cancels e and waits for it to return (at most wait). It runs
// without c.mu held. False means the loop has not returned yet.
func (c *Control) stop(e *entry, why string, wait time.Duration) bool {
	c.mu.Lock()
	cancel, done := e.cancel, e.done
	e.cancel = nil
	c.mu.Unlock()
	if cancel == nil {
		return true
	}
	cancel()
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-done:
		c.Reg.Set(jobs.StageKey(e.l.Name), "stage", e.l.Label, jobs.Off, why)
		return true
	case <-t.C:
		c.Reg.Update(jobs.StageKey(e.l.Name), func(s *jobs.Status) {
			s.State, s.Stuck = jobs.Failed, true
			s.Reason = "Asked to stop, but its pass has not returned (stuck). It is not started again until it does; a whole-app restart clears it."
		})
		return false
	}
}

func (c *Control) drainWait() time.Duration {
	if c.Drain > 0 {
		return c.Drain
	}
	return DefaultDrain
}

// On reports whether part p is on now.
func (c *Control) On(p Part) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.init()
	return c.onLocked(p)
}

// OwnerOn reports whether owner is on (Media: any of Movies, Shows,
// Music; Core and ByTitle: always).
func (c *Control) OwnerOn(o Owner) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.init()
	return c.ownerOnLocked(o)
}

// Off lists the parts that are off now.
func (c *Control) Off() []Part {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []Part
	for _, p := range All() {
		if !c.onLocked(p) {
			out = append(out, p)
		}
	}
	return out
}

// needsLocked says why the set of on parts in want breaks a dependency.
func needsLocked(on map[Part]bool, changed Part, turnOn bool) error {
	media := on[Movies] || on[Shows] || on[Music]
	if turnOn {
		switch changed {
		case Subtitles:
			if !on[Movies] && !on[Shows] {
				return NeedsError{"Subtitles need Movies or TV shows. Turn Movies or TV shows on first."}
			}
		case Wishlist:
			if !media {
				return NeedsError{"Requests needs Movies, TV shows or Music. Turn one of them on first."}
			}
		}
		return nil
	}
	if on[Subtitles] && !on[Movies] && !on[Shows] {
		return NeedsError{"Subtitles need Movies or TV shows, and " + changed.Label() + " is the last of them that is on. Turn Subtitles off first."}
	}
	if on[Wishlist] && !media {
		return NeedsError{"Requests needs Movies, TV shows or Music, and " + changed.Label() + " is the last of them that is on. Turn Requests off first."}
	}
	return nil
}

// Set turns part p on or off, saves the choice, and starts or stops the
// part's stages. Turning off stops the part's loops and holds its Jobs in
// the queue (Jobs already running finish; nothing is cut off); its data
// stays. A switch that breaks a dependency is refused with a NeedsError.
func (c *Control) Set(ctx context.Context, p Part, on bool, by domain.Actor) error {
	if !Valid(p) {
		return ErrNoPart
	}
	if c.SafeMode {
		return ErrSafeMode
	}
	if !c.busy.TryLock() {
		return ErrBusy
	}
	defer c.busy.Unlock()
	c.mu.Lock()
	c.init()
	state := map[Part]bool{}
	for _, q := range All() {
		state[q] = c.onLocked(q)
	}
	if state[p] == on {
		c.mu.Unlock()
		return nil
	}
	state[p] = on
	if err := needsLocked(state, p, on); err != nil {
		c.mu.Unlock()
		return err
	}
	saved := c.savedLocked()
	if on {
		saved = slices.DeleteFunc(saved, func(q Part) bool { return q == p })
	} else {
		saved = append(saved, p)
	}
	c.mu.Unlock()
	if err := c.save(ctx, saved, by); err != nil {
		return err
	}
	c.mu.Lock()
	if on {
		delete(c.off, p)
	} else {
		c.off[p] = "Turned off in the part switches."
	}
	c.flag()
	c.mu.Unlock()
	c.apply()
	return nil
}

// savedLocked is the list of parts turned off by a person, in order.
func (c *Control) savedLocked() []Part {
	var out []Part
	for _, q := range All() {
		if _, off := c.off[q]; off {
			out = append(out, q)
		}
	}
	return out
}

func (c *Control) save(ctx context.Context, off []Part, by domain.Actor) error {
	if off == nil {
		off = []Part{}
	}
	raw, _ := json.Marshal(off)
	actor, _ := json.Marshal(by)
	now := database.FormatTime(time.Now())
	return c.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
			settingGroup, settingKey, string(raw), now, string(actor), now, string(actor))
		return err
	})
}

// apply stops the loops whose owner is now off and starts those now on.
func (c *Control) apply() {
	c.mu.Lock()
	var stop []*entry
	for _, e := range c.loops {
		if e.cancel != nil && !c.ownerOnLocked(e.owner) {
			stop = append(stop, e)
		}
	}
	c.mu.Unlock()
	for _, e := range stop {
		c.stop(e, offReason(e.owner), c.drainWait())
	}
	c.mu.Lock()
	for _, e := range c.loops {
		_ = c.startLocked(e)
	}
	c.mu.Unlock()
}

// Restart stops part p (its loops, and no new Jobs of it start), waits for
// its running Jobs to finish (at most Drain; a Job still running then
// carries on and is never cut off), and starts it again. It returns how
// many Jobs were still running when the wait ended.
func (c *Control) Restart(ctx context.Context, p Part) (int, error) {
	if !Valid(p) {
		return 0, ErrNoPart
	}
	if c.SafeMode {
		return 0, ErrSafeMode
	}
	if !c.busy.TryLock() {
		return 0, ErrBusy
	}
	defer c.busy.Unlock()
	c.mu.Lock()
	c.init()
	if !c.onLocked(p) {
		c.mu.Unlock()
		return 0, OffError{p}
	}
	c.hold[p] = true
	c.flag()
	var mine []*entry
	for _, e := range c.loops {
		if e.owner == Of(p) {
			mine = append(mine, e)
		}
	}
	c.mu.Unlock()
	for _, e := range mine {
		c.stop(e, "Restarting.", c.drainWait())
	}
	left := c.waitJobs(ctx, p)
	c.mu.Lock()
	delete(c.hold, p)
	c.flag()
	c.mu.Unlock()
	c.apply()
	return left, nil
}

// waitJobs waits until no running Job belongs to p, at most Drain.
func (c *Control) waitJobs(ctx context.Context, p Part) int {
	if c.Jobs == nil {
		return 0
	}
	deadline := time.Now().Add(c.drainWait())
	for {
		n := 0
		for _, j := range c.Jobs.Running() {
			if c.jobPart(ctx, c.Store.Read(), j.Kind, j.Subject) == Of(p) {
				n++
			}
		}
		if n == 0 || time.Now().After(deadline) || ctx.Err() != nil {
			return n
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// stageLocked finds a stage by name.
func (c *Control) stageLocked(name string) *entry {
	for _, e := range c.loops {
		if e.l.Name == name {
			return e
		}
	}
	return nil
}

// NeverPause lists stages that are never paused or restarted one by one:
// Log masking keeps the list of secrets the log blanks out current, so a
// pause would let a new key or password reach the log in the clear. They
// restart only with the whole app.
var NeverPause = map[string]bool{"log_secrets": true}

// ErrNeverPause answers a pause or restart of a stage in NeverPause.
var ErrNeverPause = errors.New("this stage keeps secrets out of the log and only restarts with the whole app")

// Stages lists the names of the stages that can be paused one by one.
func (c *Control) Stages() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.loops))
	for _, e := range c.loops {
		if NeverPause[e.l.Name] {
			continue
		}
		out = append(out, e.l.Name)
	}
	return out
}

// Pause stops one stage until Resume (or the next start of the app).
func (c *Control) Pause(name, who string) error {
	if NeverPause[name] {
		return ErrNeverPause
	}
	if !c.busy.TryLock() {
		return ErrBusy
	}
	defer c.busy.Unlock()
	c.mu.Lock()
	e := c.stageLocked(name)
	if e == nil {
		c.mu.Unlock()
		return ErrNoStage
	}
	e.paused = "Paused by " + who + "."
	why := e.paused
	c.mu.Unlock()
	if !c.stop(e, why, c.drainWait()) {
		return fmt.Errorf("%s: asked to stop, but its pass has not returned (stuck); a whole-app restart clears it", e.l.Label)
	}
	return nil
}

// Resume starts a paused stage again.
func (c *Control) Resume(name string) error {
	if !c.busy.TryLock() {
		return ErrBusy
	}
	defer c.busy.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.stageLocked(name)
	if e == nil {
		return ErrNoStage
	}
	e.paused = ""
	return c.resumeLocked(e)
}

func (c *Control) resumeLocked(e *entry) error {
	if e.optional && c.SafeMode {
		return ErrSafeMode
	}
	if !c.ownerOnLocked(e.owner) {
		if e.owner == Media {
			return NeedsError{"Movies, TV shows and Music are all off; turn one on first."}
		}
		return OffError{Part(e.owner)}
	}
	return c.startLocked(e)
}

// RestartStage stops one stage and starts it again (a paused stage too).
func (c *Control) RestartStage(name string) error {
	if NeverPause[name] {
		return ErrNeverPause
	}
	if !c.busy.TryLock() {
		return ErrBusy
	}
	defer c.busy.Unlock()
	c.mu.Lock()
	e := c.stageLocked(name)
	if e == nil {
		c.mu.Unlock()
		return ErrNoStage
	}
	if e.optional && c.SafeMode {
		c.mu.Unlock()
		return ErrSafeMode
	}
	if !c.ownerOnLocked(e.owner) {
		c.mu.Unlock()
		if e.owner == Media {
			return NeedsError{"Movies, TV shows and Music are all off; turn one on first."}
		}
		return OffError{Part(e.owner)}
	}
	c.mu.Unlock()
	if !c.stop(e, "Restarting.", c.drainWait()) {
		return fmt.Errorf("%s: its pass has not returned (stuck); a whole-app restart clears it", e.l.Label)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e.paused = ""
	return c.startLocked(e)
}

// Statuses are the parts' states for the health page: off with the reason,
// starting while a restart waits, else the worst state of the part's
// loops (failed, retrying), else running.
func (c *Control) Statuses() []jobs.Status {
	c.mu.Lock()
	c.init()
	type row struct {
		p      Part
		off    string
		hold   bool
		stages []string
	}
	var rows []row
	for _, p := range All() {
		r := row{p: p, off: c.off[p], hold: c.hold[p]}
		for _, e := range c.loops {
			if e.owner == Of(p) {
				r.stages = append(r.stages, e.l.Name)
			}
		}
		rows = append(rows, r)
	}
	c.mu.Unlock()
	out := make([]jobs.Status, 0, len(rows))
	for _, r := range rows {
		st := jobs.Status{Key: "part:" + string(r.p), Kind: "part", Name: r.p.Label(), State: jobs.Running}
		switch {
		case r.off != "":
			st.State, st.Reason = jobs.Off, r.off
		case r.hold:
			st.State, st.Reason = jobs.Starting, "Restarting: waiting for its running Jobs to finish."
		default:
			for _, name := range r.stages {
				s, ok := c.Reg.Get(jobs.StageKey(name))
				if !ok {
					continue
				}
				if s.State == jobs.Failed || (s.State == jobs.Retrying && st.State != jobs.Failed) ||
					(s.State == jobs.Starting && st.State == jobs.Running) {
					st.State, st.Reason, st.Since = s.State, s.Name+": "+s.Reason, s.Since
				}
			}
			if st.State == jobs.Starting {
				st.Reason = ""
			}
		}
		out = append(out, st)
	}
	return out
}

// Active reports whether any part is off or held (the queue's fast path).
func (c *Control) Active() bool { return c.active.Load() }

// KindOff reports whether scheduled Jobs of kind are not to be queued.
func (c *Control) KindOff(kind string) bool {
	o, ok := startup.OwnerOf(startup.JobOwners, kind)
	if !ok || o == ByTitle {
		return false
	}
	return !c.OwnerOn(o)
}

// OffKinds lists the Job kinds held whatever their subject: the kinds of
// an owner that is off (title-decided kinds are not listed).
func (c *Control) OffKinds() []string {
	var out []string
	for k, o := range startup.OwnersOf(startup.JobOwners) {
		if o != ByTitle && !c.OwnerOn(o) {
			out = append(out, k)
		}
	}
	return out
}

// Held reports whether a queued Job must wait because its part is off.
func (c *Control) Held(ctx context.Context, db database.Querier, kind string, subject *domain.Subject) bool {
	return !c.OwnerOn(c.jobPart(ctx, db, kind, subject))
}

// jobPart resolves a Job's owner; a title-decided Job whose title cannot
// be found counts as shared media work.
func (c *Control) jobPart(ctx context.Context, db database.Querier, kind string, subject *domain.Subject) Owner {
	o, ok := startup.OwnerOf(startup.JobOwners, kind)
	if !ok {
		return Core
	}
	if o != ByTitle {
		return o
	}
	if p, ok := SubjectPart(ctx, db, subject); ok {
		return Of(p)
	}
	return Media
}

// SubjectPart finds the part of the title a Job subject is about.
func SubjectPart(ctx context.Context, db database.Querier, subject *domain.Subject) (Part, bool) {
	if subject == nil {
		return "", false
	}
	var q string
	switch subject.Type {
	case "title":
		q = "SELECT kind FROM titles WHERE id = ?"
	case "slot":
		q = "SELECT t.kind FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id WHERE s.id = ?"
	case "download":
		q = `SELECT t.kind FROM downloads d JOIN slots s ON s.id = json_extract(d.slot_ids, '$[0]')
			JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id WHERE d.id = ?`
	case "file":
		q = "SELECT t.kind FROM files f JOIN versions v ON v.id = f.version_id JOIN titles t ON t.id = v.title_id WHERE f.id = ?"
	case "part":
		q = "SELECT t.kind FROM parts p JOIN titles t ON t.id = p.title_id WHERE p.id = ?"
	case "library_folder":
		q = "SELECT kind FROM library_folders WHERE id = ?"
	default:
		return "", false
	}
	var kind string
	if err := db.QueryRowContext(ctx, q, subject.ID).Scan(&kind); err != nil {
		return "", false
	}
	return ForTitleKind(kind)
}

// TitlePart finds the part of one title by its id.
func TitlePart(ctx context.Context, db database.Querier, id string) (Part, bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
	if err != nil {
		return "", false
	}
	return SubjectPart(ctx, db, &domain.Subject{Type: "title", ID: n})
}
