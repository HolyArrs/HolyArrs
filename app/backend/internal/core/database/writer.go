package database

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var errClosed = errors.New("store is closed")

// ErrNestedWrite is returned when a write function calls Write again; the
// one writer is busy running it, so waiting would never end.
var ErrNestedWrite = errors.New("store: Write called from inside a write function")

// ErrWriteTooLong is returned when a write function ran past the writer's
// time cap: its transaction is rolled back and the writer moves on, so one
// slow write never holds up sign-in and every other write.
var ErrWriteTooLong = errors.New("store: a write took too long and was rolled back")

// DefaultWriteCap is the time cap for one write transaction.
const DefaultWriteCap = 20 * time.Second

// goid returns the current goroutine's id, read from its stack header.
func goid() uint64 {
	var buf [64]byte
	b := buf[:runtime.Stack(buf[:], false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	if i := bytes.IndexByte(b, ' '); i > 0 {
		b = b[:i]
	}
	id, _ := strconv.ParseUint(string(b), 10, 64)
	return id
}

type writeReq struct {
	ctx  context.Context
	fn   func(*sql.Tx) error
	done chan error
	enq  time.Time
	// limit caps the transaction (0: no cap, for migrations).
	limit time.Duration
	// rebuild permits a migration to replace a referenced table. It disables
	// foreign keys only on the pinned connection, outside its transaction.
	rebuild bool
	// after holds what AfterCommit asked for, run by the caller once the
	// transaction committed.
	after *[]func()
}

// afterCommit maps a running transaction to the functions to run after it
// commits.
var afterCommit sync.Map // *sql.Tx -> txAfter

type txAfter struct {
	s   *Store
	fns *[]func()
}

// AfterCommit runs fn once tx has committed, on the goroutine that asked
// for the write (so fn may write again). A rolled-back tx drops fn. tx must
// come from Write or WriteEvents.
func AfterCommit(tx *sql.Tx, fn func()) {
	if v, ok := afterCommit.Load(tx); ok {
		p := v.(txAfter).fns
		*p = append(*p, fn)
	}
}

// StoreOf returns the Store a running transaction belongs to (nil outside
// Write and WriteEvents).
func StoreOf(tx *sql.Tx) *Store {
	if v, ok := afterCommit.Load(tx); ok {
		return v.(txAfter).s
	}
	return nil
}

// writer owns the only write connection to one database file and runs every
// write transaction on one goroutine, so writers never fight over the lock.
type writer struct {
	db *sql.DB
	// core is the lane for core writes (sign-in, sessions): the writer
	// takes them before anything waiting in reqs.
	core chan writeReq
	reqs chan writeReq
	// limit is the time cap for one write (0: none).
	limit time.Duration
	quit  chan struct{}
	end   chan struct{}
	// commits counts committed write transactions.
	commits atomic.Int64
	waits   waitRing
	// fnID is the id of the goroutine running the current write
	// function, to catch nested writes.
	fnID atomic.Uint64
	// owner is the Store this writer belongs to.
	owner *Store
}

// waitRing keeps the last queue waits (time from asking to write until the
// writer starts the transaction).
type waitRing struct {
	mu   sync.Mutex
	d    [1024]time.Duration
	n    int
	next int
}

func (r *waitRing) add(d time.Duration) {
	r.mu.Lock()
	r.d[r.next] = d
	r.next = (r.next + 1) % len(r.d)
	r.n = min(r.n+1, len(r.d))
	r.mu.Unlock()
}

// p95 returns the 95th percentile of the kept waits and how many there are.
func (r *waitRing) p95() (time.Duration, int) {
	r.mu.Lock()
	ds := append([]time.Duration(nil), r.d[:r.n]...)
	r.mu.Unlock()
	if len(ds) == 0 {
		return 0, 0
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	return ds[(len(ds)*95+99)/100-1], len(ds)
}

func newWriter(path string, cacheKiB int, limit time.Duration) (*writer, error) {
	db, err := openPool(path, cacheKiB, false, 1)
	if err != nil {
		return nil, err
	}
	// Keep the write connection open: when the last connection to a WAL
	// database closes, SQLite checkpoints, which would be a disk write
	// minutes into an idle period.
	db.SetConnMaxIdleTime(0)
	w := &writer{db: db, core: make(chan writeReq), reqs: make(chan writeReq), limit: limit, quit: make(chan struct{}), end: make(chan struct{})}
	go w.loop()
	return w, nil
}

func (w *writer) loop() {
	defer close(w.end)
	serve := func(r writeReq) {
		w.waits.add(time.Since(r.enq))
		r.done <- w.run(r)
	}
	for {
		select {
		case r := <-w.core:
			serve(r)
			continue
		default:
		}
		select {
		case r := <-w.core:
			serve(r)
		case r := <-w.reqs:
			serve(r)
		case <-w.quit:
			return
		}
	}
}

func (w *writer) run(r writeReq) (err error) {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()
	var tx *sql.Tx
	if r.rebuild {
		conn, e := w.db.Conn(ctx)
		if e != nil {
			return e
		}
		defer func() {
			// Restoration must survive cancellation of the migration. Never
			// return a connection with enforcement disabled to the pool.
			restoreCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			_, restoreErr := conn.ExecContext(restoreCtx, "PRAGMA foreign_keys = ON")
			var enabled int
			if restoreErr == nil {
				restoreErr = conn.QueryRowContext(restoreCtx, "PRAGMA foreign_keys").Scan(&enabled)
				if restoreErr == nil && enabled != 1 {
					restoreErr = errors.New("foreign key enforcement was not restored")
				}
			}
			if restoreErr != nil {
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
				err = errors.Join(err, fmt.Errorf("restore migration foreign keys: %w", restoreErr))
			}
			_ = conn.Close()
		}()
		if _, err = conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
			return err
		}
		var enabled int
		if err = conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
			return err
		}
		if enabled != 0 {
			return errors.New("migration requires foreign keys disabled outside a transaction")
		}
		tx, err = conn.BeginTx(ctx, nil)
	} else {
		tx, err = w.db.BeginTx(ctx, nil)
	}
	if err != nil {
		return err
	}
	defer tx.Rollback()
	afterCommit.Store(tx, txAfter{s: w.owner, fns: r.after})
	defer afterCommit.Delete(tx)
	// The function runs on its own goroutine so the writer can give up on
	// it at the cap; a function still running then only sees a closed
	// transaction.
	res := make(chan error, 1)
	go func() {
		w.fnID.Store(goid())
		defer func() {
			if p := recover(); p != nil {
				res <- fmt.Errorf("write panicked (%T)", p) // the type only: the text may hold a secret
			}
		}()
		res <- r.fn(tx)
	}()
	var capped <-chan time.Time
	if r.limit > 0 {
		t := time.NewTimer(r.limit)
		defer t.Stop()
		capped = t.C
	}
	select {
	case err = <-res:
		w.fnID.Store(0)
	case <-capped:
		cancel() // ends a statement still running
		_ = tx.Rollback()
		w.fnID.Store(0)
		log.Printf("store: a write ran past %s and was rolled back", r.limit)
		return ErrWriteTooLong
	}
	if err != nil {
		_ = tx.Rollback()
		*r.after = nil
		return err
	}
	if r.rebuild {
		rows, checkErr := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
		if checkErr == nil {
			if rows.Next() {
				checkErr = errors.New("migration would violate a foreign key")
			} else {
				checkErr = rows.Err()
			}
			checkErr = errors.Join(checkErr, rows.Close())
		}
		if checkErr != nil {
			*r.after = nil
			return checkErr
		}
	}
	if err := tx.Commit(); err != nil {
		*r.after = nil
		return err
	}
	w.commits.Add(1)
	return nil
}

func (w *writer) do(ctx context.Context, fn func(*sql.Tx) error) error {
	return w.send(ctx, fn, w.reqs, w.limit, false)
}

// doCore is do on the core lane.
func (w *writer) doCore(ctx context.Context, fn func(*sql.Tx) error) error {
	return w.send(ctx, fn, w.core, w.limit, false)
}

// doUncapped is do without a time cap (migrations).
func (w *writer) doUncapped(ctx context.Context, fn func(*sql.Tx) error) error {
	return w.send(ctx, fn, w.reqs, 0, false)
}

// doRebuild is reserved for migrations that rebuild a referenced table.
func (w *writer) doRebuild(ctx context.Context, fn func(*sql.Tx) error) error {
	return w.send(ctx, fn, w.reqs, 0, true)
}

func (w *writer) send(ctx context.Context, fn func(*sql.Tx) error, lane chan writeReq, limit time.Duration, rebuild bool) error {
	if id := w.fnID.Load(); id != 0 && id == goid() {
		return ErrNestedWrite
	}
	r := writeReq{ctx: ctx, fn: fn, done: make(chan error, 1), enq: time.Now(), after: new([]func()), limit: limit, rebuild: rebuild}
	select {
	case lane <- r:
	case <-w.quit:
		return errClosed
	case <-ctx.Done():
		return ctx.Err()
	}
	err := <-r.done
	if err == nil {
		for _, f := range *r.after {
			f()
		}
	}
	return err
}

func (w *writer) close() error {
	select {
	case <-w.quit:
	default:
		close(w.quit)
	}
	<-w.end
	return w.db.Close()
}
