package live

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Options tunes the Hub. Zero values take the defaults.
type Options struct {
	Heartbeat   time.Duration // default 25s
	Ring        int           // events kept for resume, default 256
	ClientQueue int           // events buffered per client before it is dropped, default 64
}

// Event is one message on the stream. The topic travels inside the data so
// one stream per tab carries every topic.
type Event struct {
	ID    uint64
	Topic string
	Data  json.RawMessage
}

type client struct {
	ch   chan Event
	gone chan struct{}
	once sync.Once
}

func (c *client) drop() { c.once.Do(func() { close(c.gone) }) }

// Hub fans events out to every connected stream.
type Hub struct {
	opt     Options
	mu      sync.Mutex
	next    uint64
	ring    []Event // oldest first
	clients map[*client]struct{}
}

// NewHub makes a Hub.
func NewHub(opt Options) *Hub {
	if opt.Heartbeat <= 0 {
		opt.Heartbeat = 25 * time.Second
	}
	if opt.Ring <= 0 {
		opt.Ring = 256
	}
	if opt.ClientQueue <= 0 {
		opt.ClientQueue = 64
	}
	return &Hub{opt: opt, clients: map[*client]struct{}{}}
}

func itoa(n uint64) string { return strconv.FormatUint(n, 10) }

// Publish sends data on topic to every client and keeps it for resume.
// A client whose queue is full is dropped; its browser reconnects and resumes.
func (h *Hub) Publish(topic string, data any) uint64 {
	b, err := json.Marshal(data)
	if err != nil {
		b, _ = json.Marshal(map[string]string{"error": err.Error()})
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.next++
	e := Event{ID: h.next, Topic: topic, Data: b}
	if len(h.ring) == h.opt.Ring {
		copy(h.ring, h.ring[1:])
		h.ring = h.ring[:len(h.ring)-1]
	}
	h.ring = append(h.ring, e)
	for c := range h.clients {
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

// subscribe adds a client and, when resuming, returns the events after last.
// If those events are no longer kept, it returns one "resync" event instead.
func (h *Hub) subscribe(last uint64, resume bool) (*client, []Event) {
	c := &client{ch: make(chan Event, h.opt.ClientQueue), gone: make(chan struct{})}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = struct{}{}
	if !resume || last >= h.next {
		return c, nil
	}
	if len(h.ring) == 0 || h.ring[0].ID > last+1 {
		return c, []Event{{Topic: "resync", Data: json.RawMessage(`{"reason":"missed too many events"}`)}}
	}
	var out []Event
	for _, e := range h.ring {
		if e.ID > last {
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

func write(w http.ResponseWriter, e Event) error {
	env, _ := json.Marshal(struct {
		Topic string          `json:"topic"`
		Data  json.RawMessage `json:"data"`
	}{e.Topic, e.Data})
	var err error
	if e.ID > 0 {
		_, err = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", e.ID, env)
	} else {
		_, err = fmt.Fprintf(w, "data: %s\n\n", env)
	}
	return err
}

// ServeHTTP serves one event stream. It resumes from the Last-Event-ID header.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	hd := w.Header()
	hd.Set("Content-Type", "text/event-stream")
	hd.Set("Cache-Control", "no-cache")
	hd.Set("Connection", "keep-alive")
	hd.Set("X-Accel-Buffering", "no")

	var last uint64
	resume := false
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			last, resume = n, true
		}
	}
	c, replay := h.subscribe(last, resume)
	defer h.unsubscribe(c)

	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprintf(w, "retry: 3000\n\n"); err != nil {
		return
	}
	for _, e := range replay {
		if write(w, e) != nil {
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
			if write(w, e) != nil || rc.Flush() != nil {
				return
			}
		case <-beat.C:
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}
