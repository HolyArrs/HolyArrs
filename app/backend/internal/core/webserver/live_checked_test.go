package webserver

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// ends reports when a stream's body closes.
func ends(resp *http.Response) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		close(done)
	}()
	return done
}

func ended(t *testing.T, done <-chan struct{}, within time.Duration, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(within):
		t.Fatalf("%s: the stream stayed open", what)
	}
}

func stillOpen(t *testing.T, done <-chan struct{}, for_ time.Duration, what string) {
	t.Helper()
	select {
	case <-done:
		t.Fatalf("%s: the stream ended", what)
	case <-time.After(for_):
	}
}

// A stream through Checked ends once its viewer changes or its sign-in stops
// working: at once after Recheck, and at the next heartbeat without one.
func TestCheckedStreamEndsWhenViewerChanges(t *testing.T) {
	var approve, signedIn atomic.Bool
	approve.Store(true)
	signedIn.Store(true)
	view := func(*http.Request) (LiveViewer, bool) {
		return LiveViewer{UserID: 7, Approve: approve.Load()}, signedIn.Load()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Recheck: rights taken away end the stream straight away.
	h := NewLiveHub(LiveOptions{Heartbeat: time.Hour})
	srv := httptest.NewServer(h.Checked(view))
	defer srv.Close()
	done := ends(connect(t, ctx, srv.URL, ""))
	waitClients(t, h, 1)
	h.Recheck()
	stillOpen(t, done, 100*time.Millisecond, "unchanged viewer after Recheck")
	approve.Store(false)
	h.Recheck()
	ended(t, done, 2*time.Second, "approve taken away")
	waitClients(t, h, 0)

	// Heartbeat: a sign-out nobody announced is noticed at the next beat.
	approve.Store(true)
	hb := NewLiveHub(LiveOptions{Heartbeat: 30 * time.Millisecond})
	srv2 := httptest.NewServer(hb.Checked(view))
	defer srv2.Close()
	done = ends(connect(t, ctx, srv2.URL, ""))
	waitClients(t, hb, 1)
	stillOpen(t, done, 100*time.Millisecond, "beats while signed in")
	signedIn.Store(false)
	ended(t, done, 2*time.Second, "signed out")

	// No sign-in at all: no stream.
	resp, err := http.Get(srv2.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("signed out stream: %d", resp.StatusCode)
	}
}

// A check asked for before an event is handed over runs first, so a
// viewer who just lost a right does not receive one more event.
func TestCheckedChecksBeforeHandingOverAnEvent(t *testing.T) {
	var approve atomic.Bool
	approve.Store(true)
	h := NewLiveHub(LiveOptions{Heartbeat: time.Hour})
	srv := httptest.NewServer(h.Checked(func(*http.Request) (LiveViewer, bool) {
		return LiveViewer{UserID: 7, Approve: approve.Load()}, true
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp := connect(t, ctx, srv.URL, "")
	out := make(chan got, 8)
	go readEvents(t, resp, out, nil)
	waitClients(t, h, 1)
	h.PublishApprovers("ask", "before")
	select {
	case e := <-out:
		if string(e.data) != `"before"` {
			t.Fatalf("got %s", e.data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("approver missed the event")
	}
	// Hold the stream's goroutine out of the way: both land while it waits.
	h.mu.Lock()
	approve.Store(false)
	for c := range h.clients {
		c.check <- struct{}{}
		c.ch <- LiveEvent{ID: 99, Topic: "ask", Data: []byte(`"after"`)}
	}
	h.mu.Unlock()
	select {
	case e := <-out:
		if string(e.data) == `"after"` {
			t.Fatal("a viewer who lost approve was handed one more ask")
		}
	case <-time.After(300 * time.Millisecond):
	}
}
