package webserver

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

// LiveTopicDL is the Downloads topic. It keeps its own small ring so large
// patches never push other topics out of the resume history, and a slow
// stream keeps only the newest patch per key instead of being dropped.
const LiveTopicDL = "dl"

// LiveOptions tunes a LiveHub. Zero values take the defaults.
type LiveOptions struct {
	Heartbeat   time.Duration // default 15s
	Ring        int           // events kept for resume, default 256
	DLRing      int           // Downloads events kept for resume, default 64
	ClientQueue int           // events buffered per stream before it is dropped, default 64
	// Merge folds a newer Downloads patch into one that still waits for a
	// slow stream. Nil keeps the newer patch; the browser then sees a gap in
	// the revision and reads that client's page again.
	Merge func(old, cur json.RawMessage) json.RawMessage
}

// LiveViewer is who holds a stream. All marks a manager, who receives every
// event. Anyone else receives only events whose audience lets them in.
type LiveViewer struct {
	UserID int64
	All    bool
	// Approve marks a person who approves asks and so reads everyone's asks.
	Approve bool
}

// LiveAudience says whether a non-manager viewer may receive an event.
type LiveAudience func(LiveViewer) bool

// LiveEvent is one message on the stream. The topic travels inside the data
// so one stream per tab carries every topic.
type LiveEvent struct {
	ID    uint64
	Topic string
	Data  json.RawMessage
	Key   string // Downloads only: the coalescing key, one per download client
	aud   LiveAudience
	open  bool // sent to every viewer (resync)
	only  bool // sent only to viewers aud lets in, never to every manager
}

// LiveHub fans events out to every connected stream.
type LiveHub struct {
	opt     LiveOptions
	boot    string
	mu      sync.Mutex
	next    uint64
	ring    []LiveEvent // oldest first, every topic but Downloads
	dlRing  []LiveEvent
	evicted uint64 // newest id pushed out of ring
	clients map[*liveClient]struct{}
}

// liveClient is one open stream.
type liveClient struct {
	who  LiveViewer
	ch   chan LiveEvent
	gone chan struct{}
	once sync.Once

	mu      sync.Mutex
	pending map[string]LiveEvent // Downloads patches waiting while ch is full
	order   []string
	wake    chan struct{}
	check   chan struct{} // Recheck asks this stream to check its viewer now
}

// LiveUser is the audience of one person's own items.
func LiveUser(id int64) LiveAudience {
	return func(v LiveViewer) bool { return id > 0 && v.UserID == id }
}

// NewLiveHub makes a hub with a fresh boot id.
func NewLiveHub(opt LiveOptions) *LiveHub {
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
	return &LiveHub{opt: opt, boot: hex.EncodeToString(b), clients: map[*liveClient]struct{}{}}
}

// ServeHTTP serves one event stream to a viewer who sees everything.
func (h *LiveHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r, LiveViewer{All: true}, nil)
}

// Handler serves streams to the viewer that view finds on each request.
// Events reach a non-manager only when their audience lets them in.
func (h *LiveHub) Handler(view func(*http.Request) LiveViewer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.serve(w, r, view(r), nil) })
}

// Checked serves streams like Handler and asks view again at every
// heartbeat and after Recheck. A stream whose sign-in stopped working, or
// whose viewer changed, ends. The browser then comes back with the rights it
// has now and resumes from its last event.
func (h *LiveHub) Checked(view func(*http.Request) (LiveViewer, bool)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		who, ok := view(r)
		if !ok {
			http.Error(w, `sign in first`, http.StatusUnauthorized)
			return
		}
		h.serve(w, r, who, func() bool {
			now, ok := view(r)
			return ok && now == who
		})
	})
}

// Boot is this process's boot id. Every event id starts with it.
func (h *LiveHub) Boot() string { return h.boot }

// EventID is the wire id of event n: "<boot>-<n>".
func (h *LiveHub) EventID(n uint64) string { return h.boot + "-" + strconv.FormatUint(n, 10) }

// Publish sends data on topic to managers only and keeps it for resume.
func (h *LiveHub) Publish(topic string, data any) uint64 { return h.publish(topic, "", data, nil) }

// PublishTo sends data on topic to managers and to the non-managers that aud
// lets in.
func (h *LiveHub) PublishTo(topic string, data any, aud LiveAudience) uint64 {
	return h.publish(topic, "", data, aud)
}

// PublishDL sends one Downloads patch for key, a download client. A stream
// that is behind keeps only the newest patch per key instead of being dropped.
func (h *LiveHub) PublishDL(key string, data any, aud LiveAudience) uint64 {
	return h.publish(LiveTopicDL, key, data, aud)
}

// PublishDLUser sends one Downloads patch for key to one non-manager's
// streams only. Managers get their own, fuller patch.
func (h *LiveHub) PublishDLUser(key string, data any, userID int64) uint64 {
	return h.publishEvent(LiveTopicDL, key, data, func(v LiveViewer) bool { return !v.All && userID > 0 && v.UserID == userID }, true)
}

// PublishApprovers sends data on topic to the people who approve asks, the
// same people the asks routes show everyone's asks to.
func (h *LiveHub) PublishApprovers(topic string, data any) uint64 {
	return h.publishEvent(topic, "", data, func(v LiveViewer) bool { return v.Approve }, true)
}

// PublishUser sends data on topic to one person's streams when they do not
// approve asks. Approvers get their own, fuller event on the topic.
func (h *LiveHub) PublishUser(topic string, data any, userID int64) uint64 {
	return h.publishEvent(topic, "", data, func(v LiveViewer) bool { return !v.Approve && userID > 0 && v.UserID == userID }, true)
}

// Viewers lists the people holding a stream who are not managers.
func (h *LiveHub) Viewers() []int64 {
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

// Clients returns how many streams are open.
func (h *LiveHub) Clients() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// Recheck asks every stream opened through Checked to check its viewer now,
// after a sign-out, an ended session or changed rights.
func (h *LiveHub) Recheck() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c.check <- struct{}{}:
		default:
		}
	}
}

func (e LiveEvent) visible(v LiveViewer) bool {
	if e.only {
		return e.aud != nil && e.aud(v)
	}
	return e.open || v.All || e.aud != nil && e.aud(v)
}

func (c *liveClient) drop() { c.once.Do(func() { close(c.gone) }) }

// offer hands a Downloads event to the stream: through ch when there is room
// and nothing for its key waits, otherwise into the key's slot, merged.
func (c *liveClient) offer(e LiveEvent, merge func(old, cur json.RawMessage) json.RawMessage) {
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
		c.pending = map[string]LiveEvent{}
	}
	c.pending[e.Key] = e
	c.order = append(c.order, e.Key)
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *liveClient) take() []LiveEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]LiveEvent, 0, len(c.order))
	for _, k := range c.order {
		out = append(out, c.pending[k])
	}
	c.pending, c.order = nil, nil
	return out
}

func (h *LiveHub) publish(topic, key string, data any, aud LiveAudience) uint64 {
	return h.publishEvent(topic, key, data, aud, false)
}

func (h *LiveHub) publishEvent(topic, key string, data any, aud LiveAudience, only bool) uint64 {
	b, err := json.Marshal(data)
	if err != nil {
		b, _ = json.Marshal(map[string]string{"error": err.Error()})
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.next++
	e := LiveEvent{ID: h.next, Topic: topic, Data: b, Key: key, aud: aud, only: only}
	if topic == LiveTopicDL {
		h.dlRing, _, _ = pushRing(h.dlRing, h.opt.DLRing, e)
	} else {
		var out LiveEvent
		var full bool
		if h.ring, out, full = pushRing(h.ring, h.opt.Ring, e); full {
			h.evicted = out.ID
		}
	}
	for c := range h.clients {
		if !e.visible(c.who) {
			continue
		}
		if topic == LiveTopicDL {
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

// subscribe adds a stream and, when resuming from last, returns the events
// after it that who may see. A last from another boot, from the future or
// older than the ring keeps returns one resync event instead.
func (h *LiveHub) subscribe(who LiveViewer, last string) (*liveClient, []LiveEvent) {
	c := &liveClient{who: who, ch: make(chan LiveEvent, h.opt.ClientQueue), gone: make(chan struct{}), wake: make(chan struct{}, 1), check: make(chan struct{}, 1)}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = struct{}{}
	if last == "" {
		return c, nil
	}
	boot, n, ok := parseLiveID(last)
	switch {
	case !ok || boot != h.boot:
		return c, []LiveEvent{liveResync("server restarted")}
	case n > h.next:
		return c, []LiveEvent{liveResync("unknown event id")}
	case n < h.evicted:
		return c, []LiveEvent{liveResync("missed too many events")}
	}
	var out []LiveEvent
	i, j := 0, 0
	for i < len(h.ring) || j < len(h.dlRing) {
		var e LiveEvent
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

func (h *LiveHub) unsubscribe(c *liveClient) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	c.drop()
}

func (h *LiveHub) write(w http.ResponseWriter, e LiveEvent) error {
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

// serve resumes from the Last-Event-ID header or the ?last= query. A stream
// the page opens itself carries no header.
func (h *LiveHub) serve(w http.ResponseWriter, r *http.Request, who LiveViewer, still func() bool) {
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
			// A check asked for meanwhile goes first, so a viewer whose rights
			// just changed is not handed one more event.
			select {
			case <-c.check:
				if still != nil && !still() {
					return
				}
			default:
			}
			if h.write(w, e) != nil || rc.Flush() != nil {
				return
			}
		case <-c.wake:
			select {
			case <-c.check:
				if still != nil && !still() {
					return
				}
			default:
			}
			for _, e := range c.take() {
				if h.write(w, e) != nil {
					return
				}
			}
			if rc.Flush() != nil {
				return
			}
		case <-c.check:
			if still != nil && !still() {
				return
			}
		case <-beat.C:
			if still != nil && !still() {
				return
			}
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}

func pushRing(ring []LiveEvent, max int, e LiveEvent) ([]LiveEvent, LiveEvent, bool) {
	var out LiveEvent
	full := len(ring) == max
	if full {
		out = ring[0]
		copy(ring, ring[1:])
		ring = ring[:len(ring)-1]
	}
	return append(ring, e), out, full
}

func liveResync(reason string) LiveEvent {
	b, _ := json.Marshal(map[string]string{"reason": reason})
	return LiveEvent{Topic: "resync", Data: b, open: true}
}

// parseLiveID reads a wire id. A bare number is an id from before boot ids.
func parseLiveID(s string) (boot string, n uint64, ok bool) {
	b, num, found := strings.Cut(s, "-")
	if !found {
		num = b
		b = ""
	}
	n, err := strconv.ParseUint(num, 10, 64)
	return b, n, err == nil
}
