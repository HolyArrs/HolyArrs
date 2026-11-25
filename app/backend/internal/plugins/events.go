package plugins

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// feedSize is how many events wait for a plugin that reads slowly; more are
// dropped, so a plugin never slows HolyArrs down. A start with missed events
// to catch up on gets room for those too.
const feedSize = 256

// maxReplay is how many missed events a start catches up on at most.
const maxReplay = 1000

// EventLine is one line a plugin reads on its stdin. ID is the event's
// number in HolyArrs's history: it grows, survives restarts and is the same
// when an event is sent again, so a plugin can skip one it already handled.
type EventLine struct {
	Event    string    `json:"event"`
	ID       int64     `json:"id,omitempty"`
	Delivery string    `json:"delivery"`
	At       time.Time `json:"at"`
	Payload  any       `json:"payload"`
}

// Replayed is one missed event to catch up on.
type Replayed struct {
	Event   string
	ID      int64
	Payload any
}

type feedItem struct {
	line []byte
	id   int64
}

// feed is one running plugin's event stream.
type feed struct {
	events  []string
	q       chan feedItem
	w       *os.File
	closed  bool
	dropped int
	// seen are the ids queued since the start, so a catch-up never sends an
	// event the live stream already did.
	seen map[int64]bool
	// cursor is the highest id written to the plugin; it is kept in
	// cursorPath so the next start knows where to catch up from.
	cursor     atomic.Int64
	cursorPath string
	// done closes when the writer has written its last line and cursor.
	done chan struct{}
}

// eventsFor are the events a start is told: asked for by m and approved in a.
func eventsFor(m *Manifest, a Approval) []string {
	if m == nil {
		return nil
	}
	var out []string
	for _, ev := range m.Access.Events {
		if slices.Contains(a.Events, ev) {
			out = append(out, ev)
		}
	}
	return out
}

// readCursor is the id saved at path and the events it was saved for; ok
// false when there is none.
func readCursor(path string) (int64, string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, "", false
	}
	id, events, _ := strings.Cut(strings.TrimSpace(string(b)), "\n")
	n, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
	return n, strings.TrimSpace(events), err == nil && n >= 0
}

func (f *feed) saveCursor() {
	if f.cursorPath == "" {
		return
	}
	tmp := f.cursorPath + ".tmp"
	if os.WriteFile(tmp, []byte(strconv.FormatInt(f.cursor.Load(), 10)+"\n"+strings.Join(f.events, ",")), 0o600) == nil {
		_ = os.Rename(tmp, f.cursorPath)
	}
}

// openFeed makes the pipe for a start that is told events and sets
// acc.Stdin to its read end; nil when acc has no events. extra is room for
// the events it will catch up on.
func openFeed(acc *StartAccess, cursorPath string, cursor int64, extra int) (*feed, error) {
	if len(acc.Events) == 0 {
		return nil, nil
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	acc.Stdin = r
	f := &feed{events: acc.Events, q: make(chan feedItem, feedSize+extra), w: w, seen: map[int64]bool{}, cursorPath: cursorPath, done: make(chan struct{})}
	f.cursor.Store(cursor)
	f.saveCursor()
	go func() {
		defer close(f.done)
		saved := time.Now()
		for it := range f.q {
			if _, err := w.Write(it.line); err != nil {
				continue // the plugin is gone; the rest is dropped until close
			}
			if it.id > f.cursor.Load() {
				f.cursor.Store(it.id)
			}
			if time.Since(saved) > 2*time.Second {
				f.saveCursor()
				saved = time.Now()
			}
		}
		_ = w.Close()
		f.saveCursor()
	}()
	return f, nil
}

func eventLine(event string, id int64, payload any) []byte {
	d := make([]byte, 8)
	_, _ = rand.Read(d)
	b, err := json.Marshal(EventLine{Event: event, ID: id, Delivery: hex.EncodeToString(d), At: time.Now().UTC(), Payload: payload})
	if err != nil {
		return nil
	}
	return append(b, '\n')
}

// prepareFeed opens the event stream for a start that is told events. after
// is the cursor to catch up from, or -1 when there is nothing to catch up
// on (no events, or a first start, which begins with what happens from now).
func (e *Engine) prepareFeed(id string, acc *StartAccess) (*feed, int64, error) {
	if len(acc.Events) == 0 {
		return nil, -1, nil
	}
	path := filepath.Join(e.Dir, id, "events.cursor")
	after, savedFor, ok := readCursor(path)
	var last int64
	known := false
	if e.LastEventID != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		var err error
		last, err = e.LastEventID(ctx)
		cancel()
		if known = err == nil; !known {
			log.Printf("plugins: %s: reading the event history: %v", id, err)
		}
	}
	// A first start, or one told other events than last time (the owner
	// approved more), begins with what happens from now: no old history of
	// a newly approved event. A restore that rewound the history clamps.
	ok = ok && savedFor == strings.Join(acc.Events, ",")
	extra := maxReplay + 1
	if !ok {
		after, extra = last, 0
	} else if known && after > last {
		after = last
	}
	f, err := openFeed(acc, path, after, extra)
	if err != nil || !ok {
		return f, -1, err
	}
	return f, after, nil
}

// abandonFeed ends the stream of a start that failed.
func (e *Engine) abandonFeed(f *feed, acc StartAccess) {
	if f == nil {
		return
	}
	if r, ok := acc.Stdin.(*os.File); ok {
		r.Close()
	}
	e.feedMu.Lock()
	f.close()
	e.feedMu.Unlock()
}

// attachFeed lists f as id's stream. With after >= 0 it first queues the
// events id missed since after, oldest first, at most maxReplay, then a
// "missed" line when there were more: the plugin should read what it needs
// from scratch. The slow catch-up query runs before feedMu is taken, so
// live events for other plugins never wait on it; under feedMu a short
// second query picks up what happened meanwhile, so live events still come
// after the missed ones and none is lost between the two.
func (e *Engine) attachFeed(id string, f *feed, after int64) {
	if f == nil {
		e.addFeed(id, nil)
		return
	}
	var items []Replayed
	more := false
	if after >= 0 && e.Replay != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		items = e.Replay(ctx, f.events, after, maxReplay+1)
		cancel()
		if more = len(items) > maxReplay; more {
			items = items[:maxReplay]
		}
	}
	e.feedMu.Lock()
	defer e.feedMu.Unlock()
	if e.feeds == nil {
		e.feeds = map[string]*feed{}
	}
	if old := e.feeds[id]; old != nil && old != f {
		old.close()
	}
	e.feeds[id] = f
	if after < 0 || e.Replay == nil {
		return
	}
	if !more {
		from := after
		if len(items) > 0 {
			from = items[len(items)-1].ID
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		late := e.Replay(ctx, f.events, from, maxReplay+1-len(items))
		cancel()
		items = append(items, late...)
		if more = len(items) > maxReplay; more {
			items = items[:maxReplay]
		}
	}
	for _, it := range items {
		if !slices.Contains(f.events, it.Event) || f.seen[it.ID] {
			continue
		}
		if line := eventLine(it.Event, it.ID, it.Payload); line != nil {
			f.seen[it.ID] = true
			select {
			case f.q <- feedItem{line, it.ID}:
			default:
			}
		}
	}
	if more {
		if line := eventLine("missed", 0, map[string]any{"after": items[len(items)-1].ID}); line != nil {
			select {
			case f.q <- feedItem{line: line}:
			default:
			}
		}
		log.Printf("plugins: %s missed more than %d events while stopped", id, maxReplay)
	}
}

// addFeed lists f as id's stream (nil drops any old one).
func (e *Engine) addFeed(id string, f *feed) {
	e.feedMu.Lock()
	defer e.feedMu.Unlock()
	if e.feeds == nil {
		e.feeds = map[string]*feed{}
	}
	if old := e.feeds[id]; old != nil && old != f {
		old.close()
	}
	if f == nil {
		delete(e.feeds, id)
		return
	}
	e.feeds[id] = f
}

// dropFeed ends id's stream when it is still f (nil: whatever it is).
func (e *Engine) dropFeed(id string, f *feed) {
	e.feedMu.Lock()
	cur := e.feeds[id]
	if cur == nil || (f != nil && cur != f) {
		e.feedMu.Unlock()
		return
	}
	cur.close()
	delete(e.feeds, id)
	e.feedMu.Unlock()
	// Outside the lock, let the writer save its cursor before the plugin's
	// folder can go (a stop, a removal).
	select {
	case <-cur.done:
	case <-time.After(time.Second):
	}
}

// close ends the stream: the plugin reads to the end and then EOF. The
// write end is closed too, so a writer stuck on a full pipe stops. Called
// with feedMu held.
func (f *feed) close() {
	if f.closed {
		return
	}
	f.closed = true
	close(f.q)
	_ = f.w.Close()
}

// Deliver tells every running plugin that asked for event, and whose owner
// approved it, what happened: one JSON line on its stdin. It never waits; a
// plugin that does not keep up loses events.
func (e *Engine) Deliver(event string, payload any) { e.DeliverEvent(event, 0, payload) }

// DeliverEvent is Deliver for the history event with id (0: none).
func (e *Engine) DeliverEvent(event string, id int64, payload any) {
	e.feedMu.Lock()
	defer e.feedMu.Unlock()
	var line []byte
	for pid, f := range e.feeds {
		if !slices.Contains(f.events, event) || (id > 0 && f.seen[id]) {
			continue
		}
		if line == nil {
			if line = eventLine(event, id, payload); line == nil {
				return
			}
		}
		if id > 0 {
			if len(f.seen) > 4096 {
				clear(f.seen)
			}
			f.seen[id] = true
		}
		select {
		case f.q <- feedItem{line, id}:
		default:
			f.dropped++
			if f.dropped == 1 || f.dropped%100 == 0 {
				log.Printf("plugins: %s does not keep up with its events; %d dropped", pid, f.dropped)
			}
		}
	}
}
