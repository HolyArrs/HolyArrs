package live

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TopicDL is the Downloads topic. Its events keep their own small ring, so
// large patches never push other topics' history out, and a slow stream
// keeps only the newest patch per key instead of being dropped.
const TopicDL = "dl"

// Options tunes the Hub. Zero values take the defaults.
type Options struct {
	Heartbeat   time.Duration // default 15s
	Ring        int           // events kept for resume, default 256
	DLRing      int           // dl events kept for resume, default 64
	ClientQueue int           // events buffered per client before it is dropped, default 64
	// Merge folds a newer dl patch into one still waiting for a slow
	// stream. Nil keeps the newer one; the browser sees a rev gap and
	// reads that client's page again.
	Merge func(old, cur json.RawMessage) json.RawMessage
}

// Viewer is who holds a stream. All marks a manager, who receives every
// event; anyone else receives only events whose audience lets them in.
type Viewer struct {
	UserID int64
	All    bool
}

// Audience says whether a non-manager viewer may receive an event.
type Audience func(Viewer) bool

// User is the audience of one person's own items.
func User(id int64) Audience { return func(v Viewer) bool { return id > 0 && v.UserID == id } }

// Event is one message on the stream. The topic travels inside the data so
// one stream per tab carries every topic.
type Event struct {
	ID    uint64
	Topic string
	Data  json.RawMessage
	Key   string // dl only: the coalescing key (one per download client)
	aud   Audience
	open  bool // sent to every viewer (resync)
	only  bool // sent only to viewers aud lets in, never to every manager
}

func (e Event) visible(v Viewer) bool {
	if e.only {
		return e.aud != nil && e.aud(v)
	}
	return e.open || v.All || e.aud != nil && e.aud(v)
}

type client struct {
	who  Viewer
	ch   chan Event
	gone chan struct{}
	once sync.Once

	mu      sync.Mutex
	pending map[string]Event // dl patches waiting while ch is full
	order   []string
	wake    chan struct{}
}

func (c *client) drop() { c.once.Do(func() { close(c.gone) }) }

// offer hands a dl event to the client: through ch when there is room and
// nothing for its key waits, else into the key's slot (merged).
func (c *client) offer(e Event, merge func(old, cur json.RawMessage) json.RawMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.pending[e.Key]; ok {
		if merge != nil {
			e.Data = merge(old.Data, e.Data)
		}
		c.pending[e.Key] = e
		return
	}
	select {
	case c.ch <- e:
		return
	default:
	}
	if c.pending == nil {
		c.pending = map[string]Event{}
	}
	c.pending[e.Key] = e
	c.order = append(c.order, e.Key)
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *client) take() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Event, 0, len(c.order))
	for _, k := range c.order {
		out = append(out, c.pending[k])
	}
	c.pending, c.order = nil, nil
	return out
}

// Hub fans events out to every connected stream.
type Hub struct {
	opt     Options
	boot    string
	mu      sync.Mutex
	next    uint64
	ring    []Event // oldest first, every topic but dl
	dlRing  []Event
	evicted uint64 // newest id pushed out of ring
	clients map[*client]struct{}
}

// NewHub makes a Hub with a fresh boot id.
func NewHub(opt Options) *Hub {
	if opt.Heartbeat <= 0 {
		opt.Heartbeat = 15 * time.Second
	}
	if opt.Ring <= 0 {
		opt.Ring = 256
	}
	if opt.DLRing <= 0 {
		opt.DLRing = 64
	}
	if opt.ClientQueue <= 0 {
		opt.ClientQueue = 64
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return &Hub{opt: opt, boot: hex.EncodeToString(b), clients: map[*client]struct{}{}}
}

// Boot is this process's boot id; every event id starts with it.
func (h *Hub) Boot() string { return h.boot }

// EventID is the wire id of event n: "<boot>-<n>".
func (h *Hub) EventID(n uint64) string { return h.boot + "-" + strconv.FormatUint(n, 10) }

// Publish sends data on topic to managers only and keeps it for resume.
func (h *Hub) Publish(topic string, data any) uint64 { return h.publish(topic, "", data, nil) }

// PublishTo sends data on topic to managers and to non-managers aud lets in.
func (h *Hub) PublishTo(topic string, data any, aud Audience) uint64 {
	return h.publish(topic, "", data, aud)
}

// PublishDL sends one dl patch for key (a download client). A stream that
// is behind keeps only the newest patch per key instead of being dropped.
func (h *Hub) PublishDL(key string, data any, aud Audience) uint64 {
	return h.publish(TopicDL, key, data, aud)
}

// PublishDLUser sends one dl patch for key to one non-manager person's
// streams only (managers get their own, fuller patch).
func (h *Hub) PublishDLUser(key string, data any, userID int64) uint64 {
	return h.publishEvent(TopicDL, key, data, func(v Viewer) bool { return !v.All && userID > 0 && v.UserID == userID }, true)
}

// Viewers lists the people holding a stream who are not managers.
func (h *Hub) Viewers() []int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	seen := map[int64]bool{}
	var out []int64
	for c := range h.clients {
		if !c.who.All && c.who.UserID > 0 && !seen[c.who.UserID] {
			seen[c.who.UserID] = true
			out = append(out, c.who.UserID)
		}
	}
	return out
}

func push(ring []Event, max int, e Event) ([]Event, Event, bool) {
	var out Event
	full := len(ring) == max
	if full {
		out = ring[0]
		copy(ring, ring[1:])
		ring = ring[:len(ring)-1]
	}
	return append(ring, e), out, full
}

func (h *Hub) publish(topic, key string, data any, aud Audience) uint64 {
	return h.publishEvent(topic, key, data, aud, false)
}

func (h *Hub) publishEvent(topic, key string, data any, aud Audience, only bool) uint64 {
	b, err := json.Marshal(data)
	if err != nil {
		b, _ = json.Marshal(map[string]string{"error": err.Error()})
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.next++
	e := Event{ID: h.next, Topic: topic, Data: b, Key: key, aud: aud, only: only}
	if topic == TopicDL {
		h.dlRing, _, _ = push(h.dlRing, h.opt.DLRing, e)
	} else {
		var out Event
		var full bool
		if h.ring, out, full = push(h.ring, h.opt.Ring, e); full {
			h.evicted = out.ID
		}
	}
	for c := range h.clients {
		if !e.visible(c.who) {
			continue
		}
		if topic == TopicDL {
			c.offer(e, h.opt.Merge)
			continue
		}
		select {
		case c.ch <- e:
		default:
			delete(h.clients, c)
			c.drop()
		}
	}
	return e.ID
}

// Clients returns how many streams are open.
func (h *Hub) Clients() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

func resync(reason string) Event {
	b, _ := json.Marshal(map[string]string{"reason": reason})
	return Event{Topic: "resync", Data: b, open: true}
}

// parseID reads a wire id. A bare number is an id from before boot ids.
func parseID(s string) (boot string, n uint64, ok bool) {
	b, num, found := strings.Cut(s, "-")
	if !found {
		num = b
		b = ""
	}
	n, err := strconv.ParseUint(num, 10, 64)
	return b, n, err == nil
}

// subscribe adds a client and, when resuming from last, returns the events
// after it that who may see. A last from another boot, from the future or
// older than the ring keeps returns one "resync" event instead.
func (h *Hub) subscribe(who Viewer, last string) (*client, []Event) {
	c := &client{who: who, ch: make(chan Event, h.opt.ClientQueue), gone: make(chan struct{}), wake: make(chan struct{}, 1)}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = struct{}{}
	if last == "" {
		return c, nil
	}
	boot, n, ok := parseID(last)
	switch {
	case !ok || boot != h.boot:
		return c, []Event{resync("server restarted")}
	case n > h.next:
		return c, []Event{resync("unknown event id")}
	case n < h.evicted:
		return c, []Event{resync("missed too many events")}
	}
	var out []Event
	i, j := 0, 0
	for i < len(h.ring) || j < len(h.dlRing) {
		var e Event
		if j >= len(h.dlRing) || i < len(h.ring) && h.ring[i].ID < h.dlRing[j].ID {
			e = h.ring[i]
			i++
		} else {
			e = h.dlRing[j]
			j++
		}
		if e.ID > n && e.visible(who) {
			out = append(out, e)
		}
	}
	return c, out
}

func (h *Hub) unsubscribe(c *client) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	c.drop()
}

func (h *Hub) write(w http.ResponseWriter, e Event) error {
	env, _ := json.Marshal(struct {
		Topic string          `json:"topic"`
		Data  json.RawMessage `json:"data"`
	}{e.Topic, e.Data})
	var err error
	if e.ID > 0 {
		_, err = fmt.Fprintf(w, "id: %s\ndata: %s\n\n", h.EventID(e.ID), env)
	} else {
		_, err = fmt.Fprintf(w, "data: %s\n\n", env)
	}
	return err
}

// ServeHTTP serves one event stream to a viewer who sees everything.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r, Viewer{All: true})
}

// Handler serves streams to the viewer view finds on each request; events
// reach a non-manager only when their audience lets them in.
func (h *Hub) Handler(view func(*http.Request) Viewer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.serve(w, r, view(r)) })
}

// serve resumes from the Last-Event-ID header or the ?last= query (a new
// stream opened by the page itself carries no header).
func (h *Hub) serve(w http.ResponseWriter, r *http.Request, who Viewer) {
	rc := http.NewResponseController(w)
	hd := w.Header()
	hd.Set("Content-Type", "text/event-stream")
	hd.Set("Cache-Control", "no-cache")
	hd.Set("Connection", "keep-alive")
	hd.Set("X-Accel-Buffering", "no")

	last := r.Header.Get("Last-Event-ID")
	if last == "" {
		last = r.URL.Query().Get("last")
	}
	c, replay := h.subscribe(who, last)
	defer h.unsubscribe(c)

	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprintf(w, "retry: 3000\n\n"); err != nil {
		return
	}
	for _, e := range replay {
		if h.write(w, e) != nil {
			return
		}
	}
	if rc.Flush() != nil {
		return
	}
	beat := time.NewTicker(h.opt.Heartbeat)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-c.gone:
			return
		case e := <-c.ch:
			if h.write(w, e) != nil || rc.Flush() != nil {
				return
			}
		case <-c.wake:
			for _, e := range c.take() {
				if h.write(w, e) != nil {
					return
				}
			}
			if rc.Flush() != nil {
				return
			}
		case <-beat.C:
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}
