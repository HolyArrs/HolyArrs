package plugins_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

func eventsManifest(events ...string) func(map[string]any) {
	return func(m map[string]any) {
		m["access"] = map[string]any{"hosts": []string{}, "folders": []string{}, "actions": []string{}, "events": events}
	}
}

func manifestJSON(mod func(map[string]any)) []byte {
	m := map[string]any{
		"id": "tailscale", "name": "Tailscale", "version": "1.0.0", "author": "HolyArrs",
		"access":          map[string]any{"hosts": []string{}, "folders": []string{}, "actions": []string{}},
		"min_app_version": "1.0.0", "sha256": hashOf(pluginFile),
	}
	mod(m)
	b, _ := json.Marshal(m)
	return b
}

func TestManifestEvents(t *testing.T) {
	for _, tc := range []struct {
		name   string
		events any
		ok     bool
	}{
		{"none", nil, true},
		{"empty", []string{}, true},
		{"known", []string{"library.add", "download.done"}, true},
		{"every", plugins.EventNames, true},
		{"unknown", []string{"library.add", "library.everything"}, false},
		{"ask moments are not offered", []string{"ask_waiting"}, false},
		{"twice", []string{"library.add", "library.add"}, false},
		{"case variant", []string{"Library.Add"}, false},
		{"not a list", "library.add", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := plugins.ParseManifest(manifestJSON(func(m map[string]any) {
				if tc.events != nil {
					m["access"].(map[string]any)["events"] = tc.events
				}
			}))
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok %v", err, tc.ok)
			}
			if err == nil && tc.events == nil && m.Access.Events != nil {
				t.Fatalf("events without asking: %v", m.Access.Events)
			}
		})
	}
	// A case variant of the key itself is refused like the other access keys.
	if _, err := plugins.ParseManifest(manifestJSON(func(m map[string]any) {
		m["access"].(map[string]any)["Events"] = []string{"library.add"}
	})); err == nil {
		t.Fatal("Events key variant accepted")
	}
}

// readLine reads one event line from r within d.
func readLine(t *testing.T, r *bufio.Reader, d time.Duration) (map[string]any, error) {
	t.Helper()
	type res struct {
		line string
		err  error
	}
	ch := make(chan res, 1)
	go func() {
		l, err := r.ReadString('\n')
		ch <- res{l, err}
	}()
	select {
	case got := <-ch:
		if got.err != nil {
			return nil, got.err
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(got.line), &out); err != nil {
			t.Fatalf("event line %q: %v", got.line, err)
		}
		return out, nil
	case <-time.After(d):
		return nil, errors.New("no event")
	}
}

// A plugin that asks for library.add gets each one on its stdin within
// seconds, and nothing it did not ask for.
func TestPluginGetsOnlyApprovedEvents(t *testing.T) {
	ls, pub := newListServer(t)
	ls.publishWith("1.0.0", pluginFile, 2, eventsManifest("library.add"))
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	start := run.Starts[len(run.Starts)-1]
	if start.Events == nil {
		t.Fatal("no event stream for a plugin that asked for events")
	}
	p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if ev, _ := p["access"].(map[string]any)["events"].([]any); len(ev) != 1 || ev[0] != "library.add" {
		t.Fatalf("row access events = %v", p["access"])
	}
	r := bufio.NewReader(start.Events)

	e.Deliver("library.delete", map[string]any{"title": "Not asked"})
	e.Deliver("library.add", map[string]any{"subject": "Added: Copper Valley"})
	got, err := readLine(t, r, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got["event"] != "library.add" || got["delivery"] == "" || got["payload"].(map[string]any)["subject"] != "Added: Copper Valley" {
		t.Fatalf("event = %v", got)
	}
	if _, err := readLine(t, r, 300*time.Millisecond); err == nil {
		t.Fatal("got a second event it did not ask for")
	}
}

// Plugins without events start exactly as before: no event stream, and
// Deliver does nothing for them.
func TestPluginWithoutEventsUnchanged(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	if s := run.Starts[len(run.Starts)-1]; s.Events != nil {
		t.Fatalf("event stream given to a plugin that asked for none")
	}
	e.Deliver("library.add", map[string]any{})
	if n := needs(t, m); len(n) != 0 {
		t.Fatalf("needs = %v", n)
	}
}

// An update that asks for a new event waits until the owner approves it,
// like a new host or folder; the running version keeps running meanwhile.
func TestUpdateAddingEventIsHeld(t *testing.T) {
	ls, pub := newListServer(t)
	ls.publishWith("1.0.0", pluginFile, 2, eventsManifest("library.add"))
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	ls.publishWith("1.0.1", []byte("v101"), 3, eventsManifest("library.add", "library.delete"))
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusUnprocessableEntity)
	if n := needs(t, m); len(n) != 1 || n[0] != "event:library.delete" {
		t.Fatalf("needs = %v", n)
	}
	if p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale"); p["installed_version"] != "1.0.0" || p["running"] != true {
		t.Fatalf("while the update waits: %v", p)
	}
	callBody(t, m, "POST", "/api/v1/plugins/tailscale/approve", `{"folders":{}}`, http.StatusOK)
	start := run.Starts[len(run.Starts)-1]
	r := bufio.NewReader(start.Events)
	e.Deliver("library.delete", map[string]any{})
	if got, err := readLine(t, r, 3*time.Second); err != nil || got["event"] != "library.delete" {
		t.Fatalf("after approval: %v %v", got, err)
	}
	// Asking for fewer events applies at once.
	ls.publishWith("1.0.2", []byte("v102"), 4, eventsManifest())
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	if s := run.Starts[len(run.Starts)-1]; s.Events != nil {
		t.Fatal("event stream kept after the plugin stopped asking")
	}
}

// A plugin that never reads its stdin never slows HolyArrs down: Deliver
// drops what does not fit and returns at once.
func TestDeliverNeverBlocks(t *testing.T) {
	ls, pub := newListServer(t)
	ls.publishWith("1.0.0", pluginFile, 2, eventsManifest("library.add"))
	e, _ := newEngine(t, ls, pub)
	call(t, mux(e), "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	big := map[string]any{"text": strings.Repeat("x", 4000)}
	done := make(chan struct{})
	go func() {
		for i := 0; i < 5000; i++ {
			e.Deliver("library.add", big)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Deliver blocked on a plugin that does not read")
	}
}

// After a stop the stream ends and later events are not kept for it.
func TestStoppedPluginStreamEnds(t *testing.T) {
	ls, pub := newListServer(t)
	ls.publishWith("1.0.0", pluginFile, 2, eventsManifest("library.add"))
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	r := bufio.NewReader(run.Starts[len(run.Starts)-1].Events)
	call(t, m, "POST", "/api/v1/plugins/tailscale/stop", http.StatusOK)
	e.Deliver("library.add", map[string]any{})
	if _, err := readLine(t, r, 3*time.Second); !errors.Is(err, io.EOF) {
		t.Fatalf("after stop: %v, want EOF", err)
	}
}

// A plugin started again catches up on what happened while it was stopped,
// with ids, and never gets an event twice; a first start begins at now.
func TestStoppedPluginCatchesUp(t *testing.T) {
	ls, pub := newListServer(t)
	ls.publishWith("1.0.0", pluginFile, 2, eventsManifest("library.add"))
	e, run := newEngine(t, ls, pub)
	var asked []int64
	last := int64(10)
	e.LastEventID = func(context.Context) (int64, error) { return last, nil }
	e.Replay = func(_ context.Context, evs []string, after int64, limit int) []plugins.Replayed {
		asked = append(asked, after)
		return []plugins.Replayed{{Event: "library.add", ID: 11, Payload: map[string]any{"n": 11}}, {Event: "library.delete", ID: 12}, {Event: "library.add", ID: 13}}
	}
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	if len(asked) != 0 {
		t.Fatalf("first start caught up from %v", asked)
	}
	r := bufio.NewReader(run.Starts[len(run.Starts)-1].Events)
	e.DeliverEvent("library.add", 11, map[string]any{})
	if got, err := readLine(t, r, 3*time.Second); err != nil || got["id"] != float64(11) {
		t.Fatalf("live: %v %v", got, err)
	}
	call(t, m, "POST", "/api/v1/plugins/tailscale/stop", http.StatusOK)
	last = 13
	call(t, m, "POST", "/api/v1/plugins/tailscale/start", http.StatusOK)
	if len(asked) == 0 || asked[0] != 11 {
		t.Fatalf("caught up from %v, want after 11", asked)
	}
	r = bufio.NewReader(run.Starts[len(run.Starts)-1].Events)
	var ids []float64
	for {
		got, err := readLine(t, r, 500*time.Millisecond)
		if err != nil {
			break
		}
		ids = append(ids, got["id"].(float64))
	}
	// 11 is sent again (the cursor is the plugin's to dedupe), 12 was not
	// asked for, 13 is new.
	if len(ids) != 2 || ids[0] != 11 || ids[1] != 13 {
		t.Fatalf("catch-up ids %v", ids)
	}
}

// A restore that rewound the history clamps the cursor, and a plugin told
// other events than last time (the owner approved more) starts from now
// instead of replaying old history of the new event.
func TestCatchUpCursorEdges(t *testing.T) {
	ls, pub := newListServer(t)
	ls.publishWith("1.0.0", pluginFile, 2, eventsManifest("library.add"))
	e, _ := newEngine(t, ls, pub)
	var asked []int64
	last := int64(50)
	e.LastEventID = func(context.Context) (int64, error) { return last, nil }
	e.Replay = func(_ context.Context, _ []string, after int64, _ int) []plugins.Replayed {
		asked = append(asked, after)
		return nil
	}
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	call(t, m, "POST", "/api/v1/plugins/tailscale/stop", http.StatusOK)
	last = 20 // a backup restore rewound the history
	call(t, m, "POST", "/api/v1/plugins/tailscale/start", http.StatusOK)
	if len(asked) == 0 || asked[0] != 20 {
		t.Fatalf("after a restore caught up from %v, want 20", asked)
	}
	ls.publishWith("1.0.1", []byte("v101"), 3, eventsManifest("library.add", "library.delete"))
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusUnprocessableEntity)
	last = 90
	n := len(asked)
	callBody(t, m, "POST", "/api/v1/plugins/tailscale/approve", `{"folders":{}}`, http.StatusOK)
	if len(asked) != n {
		t.Fatalf("new events replayed old history: %v", asked)
	}
}

// A slow catch-up query never holds up live events for other plugins, and
// an event that happens during it still arrives, after the missed ones.
func TestSlowCatchUpDoesNotBlockDelivery(t *testing.T) {
	ls, pub := newListServer(t)
	ls.publishWith("1.0.0", pluginFile, 2, eventsManifest("library.add"))
	e, run := newEngine(t, ls, pub)
	last := int64(10)
	e.LastEventID = func(context.Context) (int64, error) { return last, nil }
	slow := make(chan struct{})
	var calls int
	e.Replay = func(_ context.Context, _ []string, after int64, _ int) []plugins.Replayed {
		calls++
		if calls == 1 {
			<-slow
			return []plugins.Replayed{{Event: "library.add", ID: 11}}
		}
		if after >= 11 {
			return []plugins.Replayed{{Event: "library.add", ID: 12}}
		}
		return nil
	}
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	call(t, m, "POST", "/api/v1/plugins/tailscale/stop", http.StatusOK)
	started := make(chan struct{})
	go func() {
		call(t, m, "POST", "/api/v1/plugins/tailscale/start", http.StatusOK)
		close(started)
	}()
	time.Sleep(200 * time.Millisecond)
	delivered := make(chan struct{})
	go func() {
		e.DeliverEvent("library.add", 12, map[string]any{})
		close(delivered)
	}()
	select {
	case <-delivered:
	case <-time.After(2 * time.Second):
		close(slow)
		t.Fatal("a live event waited for a catch-up query")
	}
	close(slow)
	<-started
	r := bufio.NewReader(run.Starts[len(run.Starts)-1].Events)
	var ids []float64
	for {
		got, err := readLine(t, r, 500*time.Millisecond)
		if err != nil {
			break
		}
		ids = append(ids, got["id"].(float64))
	}
	if len(ids) != 2 || ids[0] != 11 || ids[1] != 12 {
		t.Fatalf("ids %v, want 11 then 12", ids)
	}
}

// When the history can't be read at start, the saved cursor is kept: no
// clamp to 0 and no flood of old events.
func TestCatchUpKeepsCursorWhenHistoryFails(t *testing.T) {
	ls, pub := newListServer(t)
	ls.publishWith("1.0.0", pluginFile, 2, eventsManifest("library.add"))
	e, _ := newEngine(t, ls, pub)
	var asked []int64
	fail := false
	e.LastEventID = func(context.Context) (int64, error) {
		if fail {
			return 0, errors.New("database is locked")
		}
		return 40, nil
	}
	e.Replay = func(_ context.Context, _ []string, after int64, _ int) []plugins.Replayed {
		asked = append(asked, after)
		return nil
	}
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	call(t, m, "POST", "/api/v1/plugins/tailscale/stop", http.StatusOK)
	fail = true
	call(t, m, "POST", "/api/v1/plugins/tailscale/start", http.StatusOK)
	for _, a := range asked {
		if a != 40 {
			t.Fatalf("caught up from %v, want 40", asked)
		}
	}
	if len(asked) == 0 {
		t.Fatal("no catch-up at all")
	}
}
