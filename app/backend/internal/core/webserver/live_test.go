package webserver

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type got struct {
	id    string
	topic string
	data  json.RawMessage
}

// readEvents reads SSE frames from a stream and sends each data frame.
func readEvents(t *testing.T, resp *http.Response, out chan<- got, comments chan<- string) {
	t.Helper()
	sc := bufio.NewScanner(resp.Body)
	var cur got
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if cur.id != "" || cur.data != nil {
				out <- cur
			}
			cur = got{}
		case strings.HasPrefix(line, ":"):
			if comments != nil {
				comments <- line
			}
		case strings.HasPrefix(line, "id: "):
			cur.id = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "data: "):
			var env struct {
				Topic string          `json:"topic"`
				Data  json.RawMessage `json:"data"`
			}
			_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &env)
			cur.topic, cur.data = env.Topic, env.Data
		}
	}
}

func connect(t *testing.T, ctx context.Context, url, lastID string) *http.Response {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func waitClients(t *testing.T, h *LiveHub, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for h.Clients() != n {
		if time.Now().After(deadline) {
			t.Fatalf("clients %d, want %d", h.Clients(), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestHeaders(t *testing.T) {
	h := NewLiveHub(LiveOptions{})
	srv := httptest.NewServer(h)
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp := connect(t, ctx, srv.URL, "")
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type %q", ct)
	}
	if resp.Header.Get("X-Accel-Buffering") != "no" {
		t.Error("X-Accel-Buffering should be no")
	}
	if resp.Header.Get("Cache-Control") != "no-cache" {
		t.Error("Cache-Control should be no-cache")
	}
}

func TestTenClientsReceiveAndCleanUp(t *testing.T) {
	h := NewLiveHub(LiveOptions{})
	srv := httptest.NewServer(h)
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	results := make(chan got, 10)
	for range 10 {
		resp := connect(t, ctx, srv.URL, "")
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer resp.Body.Close()
			ch := make(chan got, 4)
			go readEvents(t, resp, ch, nil)
			select {
			case e := <-ch:
				results <- e
			case <-time.After(3 * time.Second):
			}
		}()
	}
	waitClients(t, h, 10)
	id := h.Publish("download", map[string]any{"progress": 0.5})
	wg.Wait()
	close(results)
	n := 0
	for e := range results {
		n++
		if e.topic != "download" || e.id != h.EventID(id) || string(e.data) != `{"progress":0.5}` {
			t.Errorf("bad event %+v", e)
		}
	}
	if n != 10 {
		t.Fatalf("%d clients got the event, want 10", n)
	}
	cancel()
	waitClients(t, h, 0)
}

func TestResumeReplaysMissed(t *testing.T) {
	h := NewLiveHub(LiveOptions{})
	srv := httptest.NewServer(h)
	defer srv.Close()
	first := h.Publish("job", 1)
	h.Publish("job", 2)
	h.Publish("job", 3)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp := connect(t, ctx, srv.URL, h.EventID(first))
	defer resp.Body.Close()
	ch := make(chan got, 8)
	go readEvents(t, resp, ch, nil)
	for _, want := range []string{"2", "3"} {
		select {
		case e := <-ch:
			if string(e.data) != want {
				t.Fatalf("replayed %s, want %s", e.data, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("replay missing")
		}
	}
	h.Publish("job", 4)
	if e := <-ch; string(e.data) != "4" {
		t.Fatalf("live event after replay: %s", e.data)
	}
}

func TestResumeTooOldAsksForResync(t *testing.T) {
	h := NewLiveHub(LiveOptions{Ring: 4})
	for i := range 10 {
		h.Publish("job", i)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp := connect(t, ctx, srv.URL, h.EventID(1))
	defer resp.Body.Close()
	ch := make(chan got, 8)
	go readEvents(t, resp, ch, nil)
	if e := <-ch; e.topic != "resync" {
		t.Fatalf("first event %+v, want resync", e)
	}
}

func TestHeartbeat(t *testing.T) {
	h := NewLiveHub(LiveOptions{Heartbeat: 20 * time.Millisecond})
	srv := httptest.NewServer(h)
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp := connect(t, ctx, srv.URL, "")
	defer resp.Body.Close()
	comments := make(chan string, 8)
	go readEvents(t, resp, make(chan got, 8), comments)
	select {
	case c := <-comments:
		if !strings.HasPrefix(c, ": ping") {
			t.Fatalf("heartbeat %q", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no heartbeat")
	}
}

func TestDefaults(t *testing.T) {
	h := NewLiveHub(LiveOptions{})
	if h.opt.Heartbeat != 15*time.Second || h.opt.Ring != 256 {
		t.Fatalf("defaults %+v", h.opt)
	}
}

func TestSlowClientIsDropped(t *testing.T) {
	h := NewLiveHub(LiveOptions{ClientQueue: 2})
	c, _ := h.subscribe(LiveViewer{All: true}, "")
	for i := range 5 {
		h.Publish("x", i)
	}
	if h.Clients() != 0 {
		t.Fatal("slow client was not dropped")
	}
	select {
	case <-c.gone:
	default:
		t.Fatal("dropped client was not told")
	}
}

func TestRingKeepsLast(t *testing.T) {
	h := NewLiveHub(LiveOptions{Ring: 3})
	for i := range 5 {
		h.Publish("x", i)
	}
	_, replay := h.subscribe(LiveViewer{All: true}, h.EventID(2))
	if len(replay) != 3 || replay[0].ID != 3 || replay[2].ID != 5 {
		t.Fatalf("replay %+v", replay)
	}
}
