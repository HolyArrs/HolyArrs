// Package fetchq is the fetch-ahead queue (music speed plan 2026-10-23,
// section 3): jobs with five priorities share one upstream's request rate,
// a job someone is waiting for (P0) goes first, failures back off and end
// dead, and the queue survives restarts in its own SQLite file.
package fetchq

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/upstream"
	_ "modernc.org/sqlite"
)

// Priorities.
const (
	P0Waiting = 0 // someone is waiting (request miss)
	P1Finish  = 1 // pieces of a page just shown
	P2Shared  = 2 // shared lists and their members
	P3Refresh = 3 // refresh of stored items
	P4Deep    = 4 // deep fill, idle only
)

var (
	// ErrNotFound: the upstream has no such item. The job dies at once.
	ErrNotFound = errors.New("fetchq: not found upstream")
	// ErrThrottled: the upstream asked us to slow down (503). The rate
	// halves for a minute and the job is retried without counting.
	ErrThrottled = errors.New("fetchq: upstream throttled")
	// ErrLater: the job waits on another job (its input is not stored
	// yet); it is queued again after LaterWait without counting a failure.
	ErrLater = errors.New("fetchq: waiting on another job")
	// ErrDead: the key failed recently and is not retried yet.
	ErrDead = errors.New("fetchq: recently failed")
	// ErrMore: the job made one step of a longer walk (one upstream call)
	// and has more to do. It is queued again at once, without counting a
	// failure, and its waiters keep waiting for the last step; a job
	// someone waits on runs its next step at P0. One call a step keeps
	// every job short, so a waiting user's job never queues behind a walk.
	ErrMore = errors.New("fetchq: more steps")
	// ErrConnection: the upstream could not be reached at all (refused,
	// reset, a dropped TLS handshake, DNS, a dial timeout). It says nothing
	// about the item, so the job is queued again after ConnWait without
	// counting a failure; with Options.ConnPause the whole queue waits too.
	// Any error upstream.ConnError recognises is treated the same.
	ErrConnection = errors.New("fetchq: upstream unreachable")
)

// ConnWait is the wait after a job's n-th connection failure in a row
// (from 0): 15 s doubling to 5 min. ConnMax connection failures in a row
// (a day and more of an unreachable host) count as one ordinary failure.
func ConnWait(n int) time.Duration {
	d := 15 * time.Second
	for range min(max(n, 0), 5) {
		d *= 2
	}
	return min(d, 5*time.Minute)
}

// ConnMax bounds a job's uncounted connection retries.
var ConnMax = 300

// waitOn is a job's wait for another job (its input), possibly in another
// queue.
type waitOn struct {
	q   *Queue
	key string
}

func (w *waitOn) Error() string { return ErrLater.Error() + " (" + w.key + ")" }
func (w *waitOn) Unwrap() error { return ErrLater }

// WaitOn is the error a job returns while its input, job key in this
// queue, is not done: while that job is queued or running the waiter is
// queued again without counting (it never dies of the wait) and the input
// is raised to the waiter's priority. Once the input is gone (done or
// dead) further waits count as before.
func (q *Queue) WaitOn(key string) error { return &waitOn{q: q, key: key} }

// Raise lifts a queued job to priority p when that is higher (a lower
// number); it never creates a job.
func (q *Queue) Raise(ctx context.Context, key string, p int) {
	if p < 0 || p > 4 {
		return
	}
	if res, err := q.db.ExecContext(ctx, `UPDATE fetch_jobs SET priority=?, not_before=MIN(not_before, ?) WHERE key=? AND state='queued' AND priority>?`, p, q.o.Now().UnixMilli(), key, p); err == nil {
		if n, _ := res.RowsAffected(); n > 0 {
			select {
			case q.wake <- struct{}{}:
			default:
			}
		}
	}
}

// LaterWait is how long a job that waits on another (ErrLater) sits out;
// LaterMax is how often it may wait before the wait counts as an ordinary
// failure (backoff, then dead), so a job that can never finish stops.
var (
	LaterWait = 2 * time.Minute
	LaterMax  = 10
)

// WaitMax is how long a job may wait on a queued input without counting;
// past it each wait counts (LaterMax, then backoff and dead), so a job
// whose input never runs does not sit in the queue for days.
var WaitMax = 24 * time.Hour

// young reports a job created within WaitMax.
func (q *Queue) young(ctx context.Context, key string, now time.Time) bool {
	var created int64
	if q.db.QueryRowContext(ctx, `SELECT created FROM fetch_jobs WHERE key=?`, key).Scan(&created) != nil {
		return true
	}
	return now.Sub(time.UnixMilli(created)) < WaitMax
}

// dead reports a job that died (and is still remembered).
func (q *Queue) dead(ctx context.Context, key string) bool {
	s, _ := q.State(ctx, key)
	return s == "dead"
}

// ReviveAfter is how long a dead job stays dead before Revive queues it
// again, so a source that keeps failing is asked a few times a day, not
// on every sweep.
const ReviveAfter = 6 * time.Hour

// Revive queues key at priority p even when it is dead, once it has been
// dead ReviveAfter (a dead job's not_before is when it died): for a caller that still wants it (its
// picture is still wanted and no miss is kept). A queued job only rises; a
// missing one is made.
func (q *Queue) Revive(ctx context.Context, key string, p int) {
	if p < 0 || p > 4 {
		p = P4Deep
	}
	now := q.o.Now().UnixMilli()
	if q.dead(ctx, key) {
		res, err := q.db.ExecContext(ctx, `UPDATE fetch_jobs SET state='queued', priority=?, attempts=0, later_count=0, conn_count=0, not_before=? WHERE key=? AND state='dead' AND not_before<=?`, p, now, key, now-ReviveAfter.Milliseconds())
		if err != nil {
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return
		}
	}
	q.Enqueue(ctx, key, p, false)
}

// Lower moves the queued jobs whose key starts with prefix, at priority
// from or below it (a number at least from, under p), down to priority p;
// it answers how many moved. Running and dead jobs stay as they are.
func (q *Queue) Lower(ctx context.Context, prefix string, p, from int) int64 {
	res, err := q.db.ExecContext(ctx, `UPDATE fetch_jobs SET priority=? WHERE state='queued' AND key LIKE ? AND priority>=? AND priority<?`, p, prefix+"%", from, p)
	if err != nil {
		return 0
	}
	n, _ := res.RowsAffected()
	return n
}

// laterLeft reports whether a job may wait (ErrLater) once more.
func (q *Queue) laterLeft(ctx context.Context, key string) bool {
	var n int
	_ = q.db.QueryRowContext(ctx, `SELECT later_count FROM fetch_jobs WHERE key=?`, key).Scan(&n)
	return n < LaterMax
}

// Backoff after each failure; the fifth failure is dead.
var Backoff = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, time.Hour, 6 * time.Hour}

// DefaultShares are the guaranteed minimum shares of P1..P4 and the P0
// maximum, of the per-minute budget (section 3 budget table).
var DefaultShares = [5]float64{0.60, 0.15, 0.15, 0.07, 0.03}

// Options configure a queue.
type Options struct {
	// Name labels log lines and stats ("musicbrainz", "images").
	Name string
	// Rate is requests a second (MusicBrainz 0.9).
	Rate float64
	// Shares: index 0 is the P0 maximum, 1..4 guaranteed minimums.
	Shares [5]float64
	// Do runs one job.
	Do func(ctx context.Context, key string, priority int) error
	// Workers is how many jobs Run keeps going at once (default 1); job
	// starts still follow Rate. With more than one, one worker is kept for
	// P0 and P1, so a waiting user's job never queues behind long ones.
	Workers int
	// HeavyPrefixes mark heavy jobs by key prefix; below P1 one heavy job
	// runs after every HeavyEvery light ones (each kind in its own order),
	// so neither waits behind a long run of the other.
	HeavyPrefixes []string
	HeavyEvery    int
	// Lane reports whether a priority may run now (kill switch); nil: all.
	Lane func(priority int) bool
	// Reserve is the share of Rate (0..0.5) left for another user of the
	// same upstream right now (list matching while names wait).
	Reserve func() float64
	// P0Max caps P0 jobs a second (kill switch p0_max_rps); nil or 0: share only.
	P0Max func() float64
	// MaxRows caps the table; above it P3 and P4 inserts are dropped.
	MaxRows int
	// JobTimeout bounds one job (default 2 min).
	JobTimeout time.Duration
	// ConnPause stops the whole queue for a job's connection wait after a
	// connection failure (a queue with one upstream: when one job cannot
	// connect, the next would not either).
	ConnPause bool
	// Hold is asked before a claimed job runs; true puts it back (for the
	// duration given) without running it or counting anything. nil: none.
	Hold func(key string, priority int) (bool, time.Duration)
	Now  func() time.Time
	Logf func(string, ...any)
}

// Stats are the queue numbers for /v1/stats.
type Stats struct {
	Name          string    `json:"name"`
	Rate          float64   `json:"rate"`
	Queued        [5]int64  `json:"queued"`
	Running       int64     `json:"running"`
	Dead          int64     `json:"dead"`
	Done          int64     `json:"done"`
	Failed        int64     `json:"failed"`
	Dropped       int64     `json:"dropped"`
	Throttled     int64     `json:"throttled"`
	LastMinute    [5]int    `json:"last_minute"`
	OldestP0Secs  float64   `json:"oldest_p0_wait_seconds"`
	LastThrottled time.Time `json:"last_throttled,omitzero"`
	// ConnRetries counts jobs queued again after a connection failure
	// (never counted as failures); ConnRun is the failures in a row now
	// (the alarm is on from 3); PausedUntil is a queue-wide connection
	// pause; Waits counts waits on another job that is still queued; Held
	// counts jobs Hold put back.
	ConnRetries int64     `json:"conn_retries"`
	ConnRun     int       `json:"conn_failures_in_a_row"`
	LastConnErr string    `json:"last_conn_error,omitempty"`
	LastConn    time.Time `json:"last_conn_failure,omitzero"`
	PausedUntil time.Time `json:"paused_until,omitzero"`
	Waits       int64     `json:"input_waits"`
	Held        int64     `json:"held"`
	Workers     int       `json:"workers"`
}

// Queue is one upstream's job queue.
type Queue struct {
	db   *sql.DB
	o    Options
	wake chan struct{}

	mu      sync.Mutex
	waiters map[string][]chan error
	recent  [5][]time.Time
	// byInstall holds each install's P0 jobs run in the last minute (fair share).
	byInstall map[string][]time.Time
	lightRun  int // light jobs since the last heavy one
	running   int // jobs Run has going
	pruned    time.Time
	slowUntil time.Time
	slowSince time.Time
	done      int64
	failed    int64
	dropped   int64
	// rows is the job count the MaxRows check reads, counted again at
	// most every rowsEvery (and kept up to date by this queue's inserts)
	// instead of a COUNT(*) on every P3/P4 enqueue.
	rows      int
	rowsAt    time.Time
	throttled int64
	lastThr   time.Time
	conns     int64
	connRun   int
	connErr   string
	connLast  time.Time
	connUntil time.Time
	waits     int64
	held      int64
}

// Open opens (or creates) the queue at path. Jobs left running by a stop
// are queued again.
func Open(path string, o Options) (*Queue, error) {
	if o.Rate <= 0 {
		o.Rate = 0.9
	}
	if o.Shares == ([5]float64{}) {
		o.Shares = DefaultShares
	}
	if o.MaxRows <= 0 {
		o.MaxRows = 200000
	}
	if o.JobTimeout <= 0 {
		o.JobTimeout = 2 * time.Minute
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Logf == nil {
		o.Logf = log.Printf
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA busy_timeout=5000; PRAGMA journal_size_limit=67108864;
CREATE TABLE IF NOT EXISTS fetch_jobs(
 key TEXT PRIMARY KEY,
 priority INTEGER NOT NULL,
 state TEXT NOT NULL DEFAULT 'queued',
 not_before INTEGER NOT NULL,
 attempts INTEGER NOT NULL DEFAULT 0,
 last_error TEXT NOT NULL DEFAULT '',
 created INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS fetch_jobs_pick ON fetch_jobs(state, priority, not_before, created);
UPDATE fetch_jobs SET state='queued' WHERE state='running';`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`ALTER TABLE fetch_jobs ADD COLUMN install TEXT NOT NULL DEFAULT ''`); err != nil && !strings.Contains(err.Error(), "duplicate column") {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`ALTER TABLE fetch_jobs ADD COLUMN later_count INTEGER NOT NULL DEFAULT 0`); err != nil && !strings.Contains(err.Error(), "duplicate column") {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`ALTER TABLE fetch_jobs ADD COLUMN conn_count INTEGER NOT NULL DEFAULT 0`); err != nil && !strings.Contains(err.Error(), "duplicate column") {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS fetch_jobs_install ON fetch_jobs(state, priority, install, not_before)`); err != nil {
		db.Close()
		return nil, err
	}
	return &Queue{db: db, o: o, wake: make(chan struct{}, 1), waiters: map[string][]chan error{}}, nil
}

// Close closes the store.
func (q *Queue) Close() error { return q.db.Close() }

type installKey struct{}

// WithInstall names the install a request comes from; its P0 jobs share
// the P0 lane fairly with other installs' (first come, first served, the
// install that ran the fewest in the last minute first).
func WithInstall(ctx context.Context, install string) context.Context {
	return context.WithValue(ctx, installKey{}, install)
}

func installOf(ctx context.Context) string { v, _ := ctx.Value(installKey{}).(string); return v }

// rowsEvery is how often the MaxRows check counts the jobs again.
const rowsEvery = 5 * time.Second

// rowCount answers the job count for the MaxRows check.
func (q *Queue) rowCount(ctx context.Context, now time.Time) int {
	q.mu.Lock()
	n, at := q.rows, q.rowsAt
	q.mu.Unlock()
	if !at.IsZero() && now.Sub(at) < rowsEvery && now.Sub(at) >= 0 {
		return n
	}
	if q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fetch_jobs`).Scan(&n) != nil {
		return n
	}
	q.mu.Lock()
	q.rows, q.rowsAt = n, now
	q.mu.Unlock()
	return n
}

// Enqueue adds key at priority p. A key already queued keeps one row: its
// priority rises to the higher of the two. With wait, the returned channel
// gets the job's result (nil on success); without, it is nil. A key that
// died recently answers ErrDead unless a P0 asks. Over MaxRows, P3 and P4
// inserts are dropped (nil channel, counted).
func (q *Queue) Enqueue(ctx context.Context, key string, p int, wait bool) <-chan error {
	if p < 0 || p > 4 {
		p = P4Deep
	}
	now := q.o.Now()
	var ch chan error
	if wait {
		ch = make(chan error, 1)
	}
	var state string
	var prio int
	var nb int64
	err := q.db.QueryRowContext(ctx, `SELECT state, priority, not_before FROM fetch_jobs WHERE key=?`, key).Scan(&state, &prio, &nb)
	switch {
	case err == sql.ErrNoRows:
		p = q.capPriority(ctx, p)
		if p >= P3Refresh && q.rowCount(ctx, now) >= q.o.MaxRows {
			q.mu.Lock()
			q.dropped++
			q.mu.Unlock()
			return nil
		}
		res, err := q.db.ExecContext(ctx, `INSERT INTO fetch_jobs(key,priority,not_before,created,install) VALUES(?,?,?,?,?) ON CONFLICT(key) DO NOTHING`, key, p, now.UnixMilli(), now.UnixMilli(), installOf(ctx))
		if err != nil {
			return failed(ch, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			q.mu.Lock()
			q.rows++
			q.mu.Unlock()
		}
	case err != nil:
		return failed(ch, err)
	case state == "dead":
		// Dead within 24 h: only a waiting user revives it.
		if p != P0Waiting && now.Sub(time.UnixMilli(nb)) < 24*time.Hour {
			return failed(ch, ErrDead)
		}
		p = q.capPriority(ctx, p)
		_, _ = q.db.ExecContext(ctx, `UPDATE fetch_jobs SET state='queued', priority=?, attempts=0, not_before=?, install=? WHERE key=?`, p, now.UnixMilli(), installOf(ctx), key)
	default:
		if p == P0Waiting && prio != P0Waiting {
			p = q.capPriority(ctx, p)
		}
		if p < prio {
			nbNew := nb
			if p == P0Waiting {
				nbNew = now.UnixMilli()
			}
			_, _ = q.db.ExecContext(ctx, `UPDATE fetch_jobs SET priority=?, not_before=?, install=CASE WHEN install='' THEN ? ELSE install END WHERE key=? AND state='queued'`, p, nbNew, installOf(ctx), key)
		}
	}
	if ch != nil {
		q.mu.Lock()
		q.waiters[key] = append(q.waiters[key], ch)
		q.mu.Unlock()
	}
	select {
	case q.wake <- struct{}{}:
	default:
	}
	return ch
}

func failed(ch chan error, err error) <-chan error {
	if ch == nil {
		return nil
	}
	ch <- err
	return ch
}

// Has reports whether key is queued or running.
func (q *Queue) Has(ctx context.Context, key string) bool {
	var n int
	_ = q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fetch_jobs WHERE key=? AND state IN ('queued','running')`, key).Scan(&n)
	return n > 0
}

// Ready counts queued jobs at priority maxP or higher that may run now.
func (q *Queue) Ready(ctx context.Context, maxP int) int {
	var n int
	_ = q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fetch_jobs WHERE state='queued' AND priority<=? AND not_before<=?`, maxP, q.o.Now().UnixMilli()).Scan(&n)
	return n
}

// State answers a job's state (queued, running or dead; "" when there is
// none) and its not-before time (for a dead job: when it died).
func (q *Queue) State(ctx context.Context, key string) (string, time.Time) {
	var st string
	var nb int64
	if q.db.QueryRowContext(ctx, `SELECT state, not_before FROM fetch_jobs WHERE key=?`, key).Scan(&st, &nb) != nil {
		return "", time.Time{}
	}
	return st, time.UnixMilli(nb)
}

// Position answers a job's state and, while queued, its place in line
// (1: next; jobs at a higher priority, then older ones at the same
// priority, are ahead). Running answers 0; no job answers "" and 0.
func (q *Queue) Position(ctx context.Context, key string) (string, int) {
	var st string
	var p int
	var nb, created int64
	if q.db.QueryRowContext(ctx, `SELECT state, priority, not_before, created FROM fetch_jobs WHERE key=?`, key).Scan(&st, &p, &nb, &created) != nil {
		return "", 0
	}
	if st != "queued" {
		return st, 0
	}
	var n int
	_ = q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fetch_jobs WHERE state='queued' AND (priority<? OR priority=? AND (not_before<? OR not_before=? AND created<?))`, p, p, nb, nb, created).Scan(&n)
	return st, n + 1
}

// rate is the current request rate: halved for 60 s after a throttle,
// then stepping back up by 10% a minute.
func (q *Queue) rateLocked(now time.Time) float64 {
	r := q.o.Rate
	if q.o.Reserve != nil {
		r *= 1 - min(0.5, max(0, q.o.Reserve()))
	}
	if q.slowSince.IsZero() {
		return r
	}
	if now.Before(q.slowUntil) {
		return r / 2
	}
	steps := now.Sub(q.slowUntil).Minutes()
	f := 0.5 + 0.1*(1+float64(int(steps)))
	if f >= 1 {
		q.slowSince = time.Time{}
		return r
	}
	return r * f
}

// pick chooses the priority to run now from those with ready jobs: P0
// while under its cap, then any lane under its guaranteed minimum, then
// the highest ready lane (unused shares flow down). While someone waits
// (P0 ready) the server's own caching work (P2 to P4) never runs, past
// the P0 cap too; only P1 (pieces of a page just shown) may.
func (q *Queue) pickLocked(now time.Time, ready [5]bool) int {
	perMin := q.rateLocked(now) * 60
	for p := range 5 {
		cut := 0
		for cut < len(q.recent[p]) && now.Sub(q.recent[p][cut]) > time.Minute {
			cut++
		}
		q.recent[p] = q.recent[p][cut:]
	}
	lane := func(p int) bool { return q.o.Lane == nil || q.o.Lane(p) }
	if ready[0] {
		limit := q.o.Shares[0] * perMin
		if q.o.P0Max != nil {
			if m := q.o.P0Max(); m > 0 && m*60 < limit {
				limit = m * 60
			}
		}
		if float64(len(q.recent[0])) < limit {
			return 0
		}
		if ready[1] && lane(1) {
			return 1
		}
		return -1
	}
	for p := 1; p < 5; p++ {
		if ready[p] && lane(p) && float64(len(q.recent[p])) < q.o.Shares[p]*perMin {
			return p
		}
	}
	for p := 1; p < 5; p++ {
		if ready[p] && lane(p) && (p < P4Deep || !ready[0] && !ready[1] && !ready[2] && !ready[3]) {
			return p
		}
	}
	return -1
}

// p0PerInstall caps the queued P0 jobs one install may hold; past it its
// new asks wait at P2, so no install fills the P0 lane.
const p0PerInstall = 200

// capPriority is the priority a job gets when p is P0: P2 when the asking
// install already holds p0PerInstall queued P0 jobs. Every path that sets
// P0 (insert, upgrade, revive) goes through it.
func (q *Queue) capPriority(ctx context.Context, p int) int {
	in := installOf(ctx)
	if p != P0Waiting || in == "" {
		return p
	}
	var n int
	_ = q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fetch_jobs WHERE state='queued' AND priority=0 AND install=?`, in).Scan(&n)
	if n >= p0PerInstall {
		return P2Shared
	}
	return p
}

// pickP0 chooses the P0 job to run: each install's oldest ready job, then
// the install that ran the fewest P0 jobs in the last minute (ties: the
// oldest job), so no install's burst pushes another's ask back.
func (q *Queue) pickP0(ctx context.Context, now time.Time) (string, int, bool) {
	rows, err := q.db.QueryContext(ctx, `SELECT install, key, attempts, MIN(not_before) FROM fetch_jobs WHERE state='queued' AND priority=0 AND not_before<=? GROUP BY install`, now.UnixMilli())
	if err != nil {
		return "", 0, false
	}
	defer rows.Close()
	q.mu.Lock()
	defer q.mu.Unlock()
	best, bestN, bestA, bestIn, bestAt := "", -1, 0, "", int64(0)
	for rows.Next() {
		var k, in string
		var a int
		var at int64
		if rows.Scan(&in, &k, &a, &at) != nil {
			continue
		}
		n := q.ranLocked(in, now)
		if bestN < 0 || n < bestN || n == bestN && at < bestAt {
			best, bestN, bestA, bestIn, bestAt = k, n, a, in, at
		}
	}
	if best == "" {
		return "", 0, false
	}
	q.chargeLocked(bestIn, now)
	return best, bestA, true
}

// ranLocked is an install's P0 runs in the last minute.
func (q *Queue) ranLocked(in string, now time.Time) int {
	l := q.byInstall[in]
	cut := 0
	for cut < len(l) && now.Sub(l[cut]) > time.Minute {
		cut++
	}
	return len(l) - cut
}

// chargeLocked counts a P0 run against an install. Lists are trimmed to
// the window and only installs with nothing left are dropped (at most once
// a minute), so rotating install IDs never reset anyone else's history.
func (q *Queue) chargeLocked(in string, now time.Time) {
	if q.byInstall == nil {
		q.byInstall = map[string][]time.Time{}
	}
	q.byInstall[in] = append(q.byInstall[in], now)
	if now.Sub(q.pruned) < time.Minute {
		return
	}
	q.pruned = now
	for k, l := range q.byInstall {
		cut := 0
		for cut < len(l) && now.Sub(l[cut]) > time.Minute {
			cut++
		}
		if cut == len(l) {
			delete(q.byInstall, k)
		} else if cut > 0 {
			q.byInstall[k] = l[cut:]
		}
	}
}

// Step runs at most one job; it reports whether one ran.
func (q *Queue) Step(ctx context.Context) bool {
	key, attempts, p, ok := q.claim(ctx, P4Deep)
	if !ok {
		return false
	}
	q.run(ctx, key, attempts, p)
	return true
}

// claim picks the next job at a priority up to maxP and marks it running.
func (q *Queue) claim(ctx context.Context, maxP int) (string, int, int, bool) {
	now := q.o.Now()
	q.mu.Lock()
	paused := now.Before(q.connUntil)
	q.mu.Unlock()
	if paused {
		return "", 0, 0, false
	}
	// skip: priorities whose claimed job was held this time; the next try
	// takes another priority, so held jobs never starve the rest.
	var skip [5]bool
	for range 50 {
		key, attempts, p, ok := q.claimOne(ctx, now, maxP, skip)
		if !ok || q.o.Hold == nil {
			return key, attempts, p, ok
		}
		hold, d := q.o.Hold(key, p)
		if !hold {
			return key, attempts, p, ok
		}
		// Held back: queued again for d, nothing counted, the slot unused.
		_, _ = q.db.ExecContext(ctx, `UPDATE fetch_jobs SET state='queued', not_before=? WHERE key=?`, now.Add(max(d, time.Second)).UnixMilli(), key)
		skip[p] = true
		q.mu.Lock()
		q.held++
		if n := len(q.recent[p]); n > 0 {
			q.recent[p] = q.recent[p][:n-1]
		}
		q.mu.Unlock()
	}
	return "", 0, 0, false
}

// readySQL finds one due job of a priority with an index seek
// (fetch_jobs_pick); a DISTINCT over every queued row cost 30% of the
// container's CPU at 60k queued picture jobs (2026-10-29 profile).
const readySQL = `SELECT 1 FROM fetch_jobs WHERE state='queued' AND priority=? AND not_before<=? LIMIT 1`

// readyPriorities reports which priorities up to maxP have a job due.
func (q *Queue) readyPriorities(ctx context.Context, now time.Time, maxP int) [5]bool {
	var ready [5]bool
	for p := 0; p <= maxP && p < len(ready); p++ {
		var one int
		ready[p] = q.db.QueryRowContext(ctx, readySQL, p, now.UnixMilli()).Scan(&one) == nil
	}
	return ready
}

func (q *Queue) claimOne(ctx context.Context, now time.Time, maxP int, skip [5]bool) (string, int, int, bool) {
	ready := q.readyPriorities(ctx, now, maxP)
	for p, s := range skip {
		ready[p] = ready[p] && !s
	}
	q.mu.Lock()
	p := q.pickLocked(now, ready)
	q.mu.Unlock()
	if p < 0 {
		return "", 0, 0, false
	}
	var key string
	var attempts int
	switch {
	case p == P0Waiting:
		var ok bool
		if key, attempts, ok = q.pickP0(ctx, now); !ok {
			return "", 0, 0, false
		}
	case p >= P2Shared && len(q.o.HeavyPrefixes) > 0:
		var ok bool
		if key, attempts, ok = q.pickMixed(ctx, p, now); !ok {
			return "", 0, 0, false
		}
	default:
		if err := q.db.QueryRowContext(ctx, `SELECT key, attempts FROM fetch_jobs WHERE state='queued' AND priority=? AND not_before<=? ORDER BY not_before, created LIMIT 1`, p, now.UnixMilli()).Scan(&key, &attempts); err != nil {
			return "", 0, 0, false
		}
	}
	// Claimed only while still queued: a second claimer (Step beside
	// Run) never runs the same job twice.
	res, err := q.db.ExecContext(ctx, `UPDATE fetch_jobs SET state='running' WHERE key=? AND state='queued'`, key)
	if err != nil {
		return "", 0, 0, false
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return "", 0, 0, false
	}
	q.mu.Lock()
	q.recent[p] = append(q.recent[p], now)
	q.mu.Unlock()
	return key, attempts, p, true
}

// pickMixed picks a job of priority p alternating heavy and light ones:
// a heavy job once HeavyEvery light ones ran since the last, else light;
// the other kind when one kind has none ready.
func (q *Queue) pickMixed(ctx context.Context, p int, now time.Time) (string, int, bool) {
	var like []string
	var args []any
	for _, pre := range q.o.HeavyPrefixes {
		like = append(like, "key LIKE ?")
		args = append(args, pre+"%")
	}
	heavy := "(" + strings.Join(like, " OR ") + ")"
	every := max(q.o.HeavyEvery, 1)
	q.mu.Lock()
	heavyTurn := q.lightRun >= every
	q.mu.Unlock()
	order := []string{"NOT " + heavy, heavy}
	if heavyTurn {
		order = []string{heavy, "NOT " + heavy}
	}
	for _, cond := range order {
		var k string
		var a int
		sqlq := `SELECT key, attempts FROM fetch_jobs WHERE state='queued' AND priority=? AND not_before<=? AND ` + cond + ` ORDER BY not_before, created LIMIT 1`
		if q.db.QueryRowContext(ctx, sqlq, append([]any{p, now.UnixMilli()}, args...)...).Scan(&k, &a) == nil {
			q.mu.Lock()
			if cond == heavy {
				q.lightRun = 0
			} else {
				q.lightRun++
			}
			q.mu.Unlock()
			return k, a, true
		}
	}
	return "", 0, false
}

// run runs one claimed job and records its result.
func (q *Queue) run(ctx context.Context, key string, attempts, p int) {
	jctx, cancel := context.WithTimeout(ctx, q.o.JobTimeout)
	// A job that panics counts as a failed try (dead after the usual
	// tries) instead of taking the server down with it.
	err := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				q.o.Logf("fetchq %s ALARM: job %s panicked: %v\n%s", q.o.Name, key, r, debug.Stack())
				err = fmt.Errorf("panic: %v", r)
			}
		}()
		return q.o.Do(jctx, key, p)
	}()
	cancel()
	q.finish(ctx, key, attempts, p, err)
}

// connRetry queues a job again after a connection failure without counting
// it; false once the job ran out of uncounted retries (ConnMax).
func (q *Queue) connRetry(ctx context.Context, key string, now time.Time, err error) bool {
	var n int
	_ = q.db.QueryRowContext(ctx, `SELECT conn_count FROM fetch_jobs WHERE key=?`, key).Scan(&n)
	if n >= ConnMax {
		return false
	}
	q.mu.Lock()
	run := q.connRun
	q.mu.Unlock()
	wait := ConnWait(max(n, run))
	_, _ = q.db.ExecContext(ctx, `UPDATE fetch_jobs SET state='queued', not_before=?, last_error=?, conn_count=conn_count+1 WHERE key=?`, now.Add(wait).UnixMilli(), trim(err.Error()), key)
	q.mu.Lock()
	q.conns++
	q.connRun++
	q.connErr = trim(err.Error())
	q.connLast = now
	if q.connRun == 3 || q.connRun > 3 && q.connRun%50 == 0 {
		q.o.Logf("fetchq %s ALARM: %s did not connect %d times in a row; these jobs pause, retry on their own (up to every 5 min) and continue when it answers, nothing is lost", q.o.Name, connSource(q.connErr), q.connRun)
	}
	if q.o.ConnPause {
		if until := now.Add(ConnWait(q.connRun - 1)); until.After(q.connUntil) {
			q.connUntil = until
		}
	}
	q.mu.Unlock()
	return true
}

// connError reports a failure to reach the upstream (see ErrConnection).
func connError(err error) bool {
	if err == nil || errors.Is(err, ErrNotFound) || errors.Is(err, ErrThrottled) || errors.Is(err, ErrLater) || errors.Is(err, ErrMore) || errors.Is(err, ErrDead) || errors.Is(err, upstream.ErrBusy) {
		return false
	}
	return errors.Is(err, ErrConnection) || upstream.ConnError(err)
}

func (q *Queue) finish(ctx context.Context, key string, attempts, p int, err error) {
	now := q.o.Now()
	notify := true
	conn := connError(err)
	if err == nil || errors.Is(err, ErrMore) || errors.Is(err, ErrNotFound) {
		q.mu.Lock()
		if q.connRun >= 3 {
			q.o.Logf("fetchq %s: connections work again after %d failures in a row", q.o.Name, q.connRun)
		}
		q.connRun = 0
		q.connUntil = time.Time{}
		q.mu.Unlock()
	}
	var w *waitOn
	switch {
	case err == nil:
		_, _ = q.db.ExecContext(ctx, `DELETE FROM fetch_jobs WHERE key=?`, key)
		q.mu.Lock()
		q.done++
		q.mu.Unlock()
	case errors.Is(err, upstream.ErrBusy):
		// The shared MetaBrainz pace was full: nothing was sent. Later,
		// uncounted, and the queue slows down like after a throttle.
		_, _ = q.db.ExecContext(ctx, `UPDATE fetch_jobs SET state='queued', not_before=?, last_error=? WHERE key=?`, now.Add(15*time.Second).UnixMilli(), err.Error(), key)
		q.mu.Lock()
		q.throttled++
		q.lastThr = now
		q.slowSince, q.slowUntil = now, now.Add(time.Minute)
		q.mu.Unlock()
	case conn && q.connRetry(ctx, key, now, err):
	case errors.As(err, &w) && w.q.dead(ctx, w.key):
		// The input died: it will not come, so the waiter ends too.
		_, _ = q.db.ExecContext(ctx, `UPDATE fetch_jobs SET state='dead', not_before=?, attempts=?, last_error=? WHERE key=?`, now.UnixMilli(), attempts+1, trim("fetchq: input died ("+w.key+")"), key)
		q.mu.Lock()
		q.failed++
		q.mu.Unlock()
	case errors.As(err, &w) && w.q.Has(ctx, w.key) && q.young(ctx, key, now):
		// The input is still queued or running: wait without counting, and
		// lift the input to this job's priority.
		w.q.Raise(ctx, w.key, p)
		_, _ = q.db.ExecContext(ctx, `UPDATE fetch_jobs SET state='queued', not_before=?, last_error=? WHERE key=?`, now.Add(LaterWait).UnixMilli(), trim(err.Error()), key)
		q.mu.Lock()
		q.waits++
		q.mu.Unlock()
	case errors.Is(err, ErrMore):
		q.mu.Lock()
		waiting := len(q.waiters[key]) > 0
		q.mu.Unlock()
		_, _ = q.db.ExecContext(ctx, `UPDATE fetch_jobs SET state='queued', not_before=?, conn_count=0, priority=CASE WHEN ? THEN 0 ELSE priority END WHERE key=?`, now.UnixMilli()+1, waiting, key) // behind jobs already queued
		notify = false
		select {
		case q.wake <- struct{}{}:
		default:
		}
	case errors.Is(err, ErrLater) && q.laterLeft(ctx, key):
		_, _ = q.db.ExecContext(ctx, `UPDATE fetch_jobs SET state='queued', not_before=?, last_error=?, later_count=later_count+1 WHERE key=?`, now.Add(LaterWait).UnixMilli(), err.Error(), key)
	case errors.Is(err, ErrThrottled):
		_, _ = q.db.ExecContext(ctx, `UPDATE fetch_jobs SET state='queued', not_before=?, last_error=? WHERE key=?`, now.Add(30*time.Second).UnixMilli(), err.Error(), key)
		q.mu.Lock()
		q.throttled++
		q.lastThr = now
		q.slowSince, q.slowUntil = now, now.Add(time.Minute)
		q.mu.Unlock()
	case errors.Is(err, ErrNotFound) || attempts+1 >= len(Backoff):
		_, _ = q.db.ExecContext(ctx, `UPDATE fetch_jobs SET state='dead', not_before=?, attempts=?, last_error=? WHERE key=?`, now.UnixMilli(), attempts+1, trim(err.Error()), key)
		q.mu.Lock()
		q.failed++
		q.mu.Unlock()
	default:
		_, _ = q.db.ExecContext(ctx, `UPDATE fetch_jobs SET state='queued', not_before=?, attempts=?, last_error=? WHERE key=?`, now.Add(Backoff[attempts]).UnixMilli(), attempts+1, trim(err.Error()), key)
		q.mu.Lock()
		q.failed++
		q.mu.Unlock()
	}
	if !notify {
		return
	}
	q.mu.Lock()
	ws := q.waiters[key]
	delete(q.waiters, key)
	q.mu.Unlock()
	for _, w := range ws {
		w <- err
	}
}

func trim(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

// Run works the queue at the current rate until ctx ends. Dead rows older
// than 7 days are removed hourly.
func (q *Queue) Run(ctx context.Context) {
	var last time.Time
	purge := time.NewTicker(time.Hour)
	defer purge.Stop()
	workers := max(q.o.Workers, 1)
	done := make(chan struct{}, workers)
	var wg sync.WaitGroup
	defer wg.Wait()
	for ctx.Err() == nil {
		q.mu.Lock()
		gap := time.Duration(float64(time.Second) / q.rateLocked(q.o.Now()))
		busy := q.running
		q.mu.Unlock()
		if busy >= workers {
			// Every worker is busy: wait for one to finish.
			select {
			case <-ctx.Done():
				return
			case <-done:
			}
			continue
		}
		if d := gap - time.Since(last); d > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(d):
			}
		}
		// The last free worker is kept for P0 and P1.
		maxP := P4Deep
		if workers > 1 && busy == workers-1 {
			maxP = P1Finish
		}
		started := time.Now()
		if key, attempts, p, ok := q.claim(ctx, maxP); ok {
			last = started
			q.mu.Lock()
			q.running++
			q.mu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				q.run(ctx, key, attempts, p)
				q.mu.Lock()
				q.running--
				q.mu.Unlock()
				select { // never blocks: Run only needs to know one finished
				case done <- struct{}{}:
				default:
				}
			}()
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-q.wake:
		case <-done:
		case <-time.After(time.Second):
		case <-purge.C:
			_, _ = q.db.ExecContext(ctx, `DELETE FROM fetch_jobs WHERE state='dead' AND not_before<?`, q.o.Now().Add(-7*24*time.Hour).UnixMilli())
		}
	}
}

// Stats reads the queue numbers.
func (q *Queue) Stats(ctx context.Context) Stats {
	now := q.o.Now()
	st := Stats{Name: q.o.Name}
	rows, err := q.db.QueryContext(ctx, `SELECT state, priority, COUNT(*), MIN(created) FROM fetch_jobs GROUP BY state, priority`)
	if err == nil {
		for rows.Next() {
			var state string
			var p int
			var n, oldest int64
			if rows.Scan(&state, &p, &n, &oldest) != nil {
				continue
			}
			switch state {
			case "queued":
				if p >= 0 && p < 5 {
					st.Queued[p] += n
				}
				if p == 0 && n > 0 {
					st.OldestP0Secs = now.Sub(time.UnixMilli(oldest)).Seconds()
				}
			case "running":
				st.Running += n
			case "dead":
				st.Dead += n
			}
		}
		rows.Close()
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	st.Rate = q.rateLocked(now)
	st.Done, st.Failed, st.Dropped, st.Throttled, st.LastThrottled = q.done, q.failed, q.dropped, q.throttled, q.lastThr
	st.ConnRetries, st.ConnRun, st.LastConnErr, st.LastConn, st.Waits, st.Held = q.conns, q.connRun, q.connErr, q.connLast, q.waits, q.held
	st.Workers = max(q.o.Workers, 1)
	if now.Before(q.connUntil) {
		st.PausedUntil = q.connUntil
	}
	for p := range 5 {
		for _, t := range q.recent[p] {
			if now.Sub(t) <= time.Minute {
				st.LastMinute[p]++
			}
		}
	}
	return st
}

// connSource is the source named in a connection error ("Apple Music
// covers (is1-ssl.mzstatic.com)" from ErrConnection wrapped with it), else
// the error itself, else "the source".
func connSource(msg string) string {
	if s, ok := strings.CutPrefix(msg, ErrConnection.Error()+": "); ok && s != "" {
		return s
	}
	if msg == "" || msg == ErrConnection.Error() {
		return "the source"
	}
	return msg
}
