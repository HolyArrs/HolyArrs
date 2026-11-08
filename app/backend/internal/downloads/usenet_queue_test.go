package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type nzbEdits struct {
	mu    sync.Mutex
	calls []string
}

func TestNZBGetQueueControls(t *testing.T) {
	f, n := nzbEditsRig(t)
	ctx := context.Background()
	steps := []func() error{
		func() error { return n.PauseItem(ctx, "nzbid-11") },
		func() error { return n.ResumeItem(ctx, ownMarker) },
		func() error { return n.MoveTop(ctx, "nzbid-11") },
		func() error { return n.MoveItem(ctx, "nzbid-11", MoveUp) },
		func() error { return n.MoveItem(ctx, "nzbid-11", MoveDown) },
		func() error { return n.MoveItem(ctx, "nzbid-11", MoveBottom) },
		func() error { return n.SetPriority(ctx, "nzbid-11", PriorityLow) },
		func() error { return n.SetPriority(ctx, "nzbid-11", PriorityForce) },
		func() error { return n.PauseAll(ctx) },
		func() error { return n.ResumeAll(ctx) },
	}
	for i, s := range steps {
		if err := s(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	want := "GroupPause::[11] GroupResume::[14] GroupMoveTop::[11] GroupMoveOffset:-1:[11] GroupMoveOffset:1:[11] GroupMoveBottom::[11] GroupSetPriority:-50:[11] GroupSetPriority:900:[11] pausedownload resumedownload"
	if got := strings.Join(f.calls, " "); got != want {
		t.Fatalf("calls\n got %s\nwant %s", got, want)
	}
	if err := n.PauseItem(ctx, "nzbid-21"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("history job: %v", err)
	}
	if err := n.MoveItem(ctx, "nzbid-11", "sideways"); !errors.Is(err, ErrMove) {
		t.Fatal(err)
	}
	if err := n.SetPriority(ctx, "nzbid-11", "urgent"); !errors.Is(err, ErrJobPriority) {
		t.Fatal(err)
	}
}

func TestNZBGetReadOnlyQueue(t *testing.T) {
	f, n := nzbEditsRig(t)
	ro := readOnly(n)
	ctx := context.Background()
	q, ok := ro.(UsenetQueue)
	if !ok {
		t.Fatal("read-only NZBGet lost its queue part")
	}
	for _, err := range []error{q.MoveItem(ctx, "nzbid-11", MoveUp), q.SetPriority(ctx, "nzbid-11", PriorityHigh),
		ro.(Controller).PauseAll(ctx), ro.(Controller).MoveTop(ctx, "nzbid-11")} {
		if !errors.Is(err, ErrReadOnly) {
			t.Fatal(err)
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("writes reached NZBGet: %v", f.calls)
	}
}

func TestSabQueueOrderAndPriority(t *testing.T) {
	calls, s := sabOrderRig(t)
	ctx := context.Background()
	for _, c := range []struct{ id, where string }{
		{"SABnzbd_nzo_q1", MoveUp}, {"SABnzbd_nzo_q1", MoveDown}, {"SABnzbd_nzo_q0", MoveBottom},
		{"SABnzbd_nzo_q0", MoveUp}, {"SABnzbd_nzo_q2", MoveDown}, {"SABnzbd_nzo_q2", MoveBottom},
	} {
		if err := s.MoveItem(ctx, c.id, c.where); err != nil {
			t.Fatalf("%s %s: %v", c.id, c.where, err)
		}
	}
	for _, p := range []string{PriorityLow, PriorityNormal, PriorityHigh, PriorityForce} {
		if err := s.SetPriority(ctx, "SABnzbd_nzo_q1", p); err != nil {
			t.Fatal(err)
		}
	}
	want := "switch:SABnzbd_nzo_q1:0 switch:SABnzbd_nzo_q1:2 switch:SABnzbd_nzo_q0:2 priority:SABnzbd_nzo_q1:-1 priority:SABnzbd_nzo_q1:0 priority:SABnzbd_nzo_q1:1 priority:SABnzbd_nzo_q1:2"
	if got := strings.Join(*calls, " "); got != want {
		t.Fatalf("calls\n got %s\nwant %s", got, want)
	}
	if err := s.MoveItem(ctx, "SABnzbd_nzo_gone", MoveUp); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if err := s.SetPriority(ctx, "SABnzbd_nzo_q1", "urgent"); !errors.Is(err, ErrJobPriority) {
		t.Fatal(err)
	}
	ro := readOnly(s).(UsenetQueue)
	n := len(*calls)
	if err := ro.MoveItem(ctx, "SABnzbd_nzo_q1", MoveUp); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
	if err := ro.SetPriority(ctx, "SABnzbd_nzo_q1", PriorityHigh); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
	if len(*calls) != n {
		t.Fatal("read-only change reached SABnzbd")
	}
}

func nzbEditsRig(t *testing.T) (*nzbEdits, *NZBGet) {
	t.Helper()
	f := &nzbEdits{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		defer f.mu.Unlock()
		var result any = true
		switch in.Method {
		case "listgroups":
			result = []map[string]any{
				{"NZBID": 11, "NZBName": "Outside.Job", "Category": "movies", "Status": "QUEUED"},
				{"NZBID": 14, "NZBName": "Own.Movie", "Category": "movies", "Status": "QUEUED",
					"Parameters": []map[string]any{{"Name": nzbgetMarker, "Value": ownMarker}}},
			}
		case "history":
			result = []map[string]any{{"NZBID": 21, "NZBName": "Done", "Category": "movies", "Status": "SUCCESS/ALL"}}
		case "editqueue":
			var cmd, param string
			var ids []int
			_ = json.Unmarshal(in.Params[0], &cmd)
			_ = json.Unmarshal(in.Params[1], &param)
			_ = json.Unmarshal(in.Params[2], &ids)
			f.calls = append(f.calls, fmt.Sprintf("%s:%s:%v", cmd, param, ids))
		case "pausedownload", "resumedownload":
			f.calls = append(f.calls, in.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "1.1", "result": result})
	}))
	t.Cleanup(s.Close)
	n := NewNZBGet(s.URL, "u", "p", nil)
	n.Category = "movies"
	return f, n
}

func sabOrderRig(t *testing.T) (*[]string, *Sabnzbd) {
	t.Helper()
	var mu sync.Mutex
	calls := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		q := r.URL.Query()
		switch q.Get("mode") {
		case "queue":
			if q.Get("name") == "priority" {
				calls = append(calls, "priority:"+q.Get("value")+":"+q.Get("value2"))
				_, _ = w.Write([]byte(`{"position":0}`))
				return
			}
			slots := []string{}
			for i := 0; i < 3; i++ {
				id := fmt.Sprintf("SABnzbd_nzo_q%d", i)
				if ids := q.Get("nzo_ids"); ids == "" || ids == id {
					slots = append(slots, fmt.Sprintf(`{"index":%d,"nzo_id":%q}`, i, id))
				}
			}
			fmt.Fprintf(w, `{"queue":{"noofslots_total":3,"slots":[%s]}}`, strings.Join(slots, ","))
		case "switch":
			calls = append(calls, "switch:"+q.Get("value")+":"+q.Get("value2"))
			_, _ = w.Write([]byte(`{"result":{"position":1,"priority":0}}`))
		default:
			_, _ = w.Write([]byte(`{"status":true}`))
		}
	}))
	t.Cleanup(srv.Close)
	return &calls, NewSabnzbd(srv.URL, sabKey, nil)
}
