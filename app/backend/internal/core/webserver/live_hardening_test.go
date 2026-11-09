package webserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func next(t *testing.T, ch <-chan got) got {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(2 * time.Second):
		t.Fatal("no event")
	}
	return got{}
}

// A browser that resumes after a restart (another boot), with a bare id
// from an older build, or with an id the server never gave, must be told
// to reload, whatever topic it last saw.
func TestResumeAfterRestartAsksForResync(t *testing.T) {
	old := NewLiveHub(LiveOptions{})
	var lastIDs []string
	for _, topic := range []string{"title", "job", "event", "slot", "ask", "downloader", "health", LiveTopicDL} {
		var n uint64
		if topic == LiveTopicDL {
			n = old.PublishDL("1", map[string]int{"toRev": 1}, nil)
		} else {
			n = old.Publish(topic, 1)
		}
		lastIDs = append(lastIDs, old.EventID(n))
	}
	h := NewLiveHub(LiveOptions{}) // the restarted server
	if h.Boot() == old.Boot() {
		t.Fatal("boot ids repeat")
	}
	h.Publish("title", 1)
	lastIDs = append(lastIDs, "3", h.Boot()+"-99")
	for _, id := range lastIDs {
		_, replay := h.subscribe(LiveViewer{All: true}, id)
		if len(replay) != 1 || replay[0].Topic != "resync" {
			t.Errorf("resume from %s: %+v, want one resync", id, replay)
		}
	}
	// The same boot and a current id: nothing to replay, no resync.
	_, replay := h.subscribe(LiveViewer{All: true}, h.EventID(1))
	if len(replay) != 0 {
		t.Errorf("current id replay %+v", replay)
	}
}

func TestQueryLastResumes(t *testing.T) {
	h := NewLiveHub(LiveOptions{})
	srv := httptest.NewServer(h)
	defer srv.Close()
	first := h.Publish("job", 1)
	h.Publish("job", 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp := connect(t, ctx, srv.URL+"?last="+h.EventID(first), "")
	defer resp.Body.Close()
	ch := make(chan got, 8)
	go readEvents(t, resp, ch, nil)
	if e := next(t, ch); string(e.data) != "2" || e.id != h.EventID(first+1) {
		t.Fatalf("got %+v", e)
	}
	// Restart: ?last= from another boot gets resync.
	resp2 := connect(t, ctx, srv.URL+"?last=deadbeef-1", "")
	defer resp2.Body.Close()
	ch2 := make(chan got, 8)
	go readEvents(t, resp2, ch2, nil)
	if e := next(t, ch2); e.topic != "resync" {
		t.Fatalf("got %+v", e)
	}
}

func viewerFromHeader(r *http.Request) LiveViewer {
	id, _ := strconv.ParseInt(r.Header.Get("X-User"), 10, 64)
	return LiveViewer{UserID: id, All: r.Header.Get("X-Manager") == "1", Approve: r.Header.Get("X-Approve") == "1"}
}

// A member receives only events meant for them, live and on replay; the
// manager receives everything.
func TestAudienceFilterLiveAndReplay(t *testing.T) {
	h := NewLiveHub(LiveOptions{})
	srv := httptest.NewServer(h.Handler(viewerFromHeader))
	defer srv.Close()
	start := h.Publish("job", "boot")
	h.Publish("event", "manager only")
	h.PublishTo("ask", "for 7", LiveUser(7))
	h.PublishTo("ask", "for 8", LiveUser(8))
	h.PublishDL("1", "dl for 8", LiveUser(8))
	h.PublishDL("1", "dl for 7", LiveUser(7))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	open := func(user, manager string) chan got {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"?last="+h.EventID(start), nil)
		req.Header.Set("X-User", user)
		req.Header.Set("X-Manager", manager)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		ch := make(chan got, 16)
		go readEvents(t, resp, ch, nil)
		return ch
	}
	member := open("7", "0")
	manager := open("1", "1")
	stranger := open("0", "0")
	waitClients(t, h, 3)
	h.PublishTo("ask", "live for 7", LiveUser(7))
	h.PublishTo("ask", "live for 8", LiveUser(8))
	h.Publish("slot", "live manager only")
	h.PublishDL("2", "live dl for 7", LiveUser(7))

	want := []string{`"for 7"`, `"dl for 7"`, `"live for 7"`, `"live dl for 7"`}
	for _, w := range want {
		if e := next(t, member); string(e.data) != w {
			t.Fatalf("member got %s, want %s", e.data, w)
		}
	}
	select {
	case e := <-member:
		t.Fatalf("member got extra %s", e.data)
	case <-time.After(150 * time.Millisecond):
	}
	for range 9 {
		next(t, manager)
	}
	select {
	case e := <-stranger:
		t.Fatalf("signed-in viewer with no items got %s", e.data)
	case <-time.After(100 * time.Millisecond):
	}
}

// A stream that cannot keep up keeps the newest dl patch per key instead
// of being dropped; other topics still drop it as before.
func TestDLCoalescesForSlowStream(t *testing.T) {
	h := NewLiveHub(LiveOptions{ClientQueue: 2, Merge: func(old, cur json.RawMessage) json.RawMessage {
		var a, b []int
		_ = json.Unmarshal(old, &a)
		_ = json.Unmarshal(cur, &b)
		out, _ := json.Marshal(append(a, b...))
		return out
	}})
	c, _ := h.subscribe(LiveViewer{All: true}, "")
	for i := range 6 {
		h.PublishDL("1", []int{i}, nil)
	}
	h.PublishDL("2", []int{100}, nil)
	h.PublishDL("2", []int{101}, nil)
	if h.Clients() != 1 {
		t.Fatal("slow stream dropped on dl")
	}
	if len(c.ch) != 2 {
		t.Fatalf("queue %d", len(c.ch))
	}
	select {
	case <-c.wake:
	default:
		t.Fatal("no wake for pending patches")
	}
	p := c.take()
	if len(p) != 2 || p[0].Key != "1" || string(p[0].Data) != "[2,3,4,5]" || p[1].Key != "2" || string(p[1].Data) != "[100,101]" {
		t.Fatalf("pending %+v", p)
	}
	if p[0].ID != 6 {
		t.Fatalf("merged id %d, want newest", p[0].ID)
	}
	// While a key waits, a new patch merges even when the queue has room.
	<-c.ch
	h.PublishDL("1", []int{7}, nil)
	h.PublishDL("1", []int{8}, nil) // queue full again: into the slot
	h.PublishDL("1", []int{9}, nil)
	if p := c.take(); len(p) != 1 || string(p[0].Data) != "[8,9]" {
		t.Fatalf("pending %+v", p)
	}
	for range 3 {
		h.Publish("x", 0)
	}
	if h.Clients() != 0 {
		t.Fatal("other topics no longer drop a full stream")
	}
}

// dl events keep their own ring: a flood of patches does not push other
// topics out of resume history.
func TestDLRingSeparate(t *testing.T) {
	h := NewLiveHub(LiveOptions{Ring: 4, DLRing: 2})
	first := h.Publish("title", 1)
	h.Publish("title", 2)
	for i := range 50 {
		h.PublishDL("1", i, nil)
	}
	h.Publish("title", 3)
	_, replay := h.subscribe(LiveViewer{All: true}, h.EventID(first))
	var topics []string
	for _, e := range replay {
		topics = append(topics, e.Topic+":"+string(e.Data))
	}
	want := []string{"title:2", "dl:48", "dl:49", "title:3"}
	if len(topics) != len(want) {
		t.Fatalf("replay %v", topics)
	}
	for i := range want {
		if topics[i] != want[i] {
			t.Fatalf("replay %v, want %v", topics, want)
		}
	}
}

// The coalesced patches reach the browser over the wire.
func TestDLSlotDeliveredOverStream(t *testing.T) {
	h := NewLiveHub(LiveOptions{ClientQueue: 1})
	srv := httptest.NewServer(h)
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp := connect(t, ctx, srv.URL, "")
	defer resp.Body.Close()
	waitClients(t, h, 1)
	for i := range 200 {
		h.PublishDL("1", i, nil)
	}
	ch := make(chan got, 256)
	go readEvents(t, resp, ch, nil)
	deadline := time.After(3 * time.Second)
	for {
		select {
		case e := <-ch:
			if string(e.data) == "199" {
				if h.Clients() != 1 {
					t.Fatal("stream dropped")
				}
				return
			}
		case <-deadline:
			t.Fatal("newest patch never arrived")
		}
	}
}

// A patch for one person reaches only that person's streams, live and on
// replay: never a manager (who gets a fuller one) nor anyone else.
func TestDLUserOnly(t *testing.T) {
	h := NewLiveHub(LiveOptions{})
	mgr, _ := h.subscribe(LiveViewer{All: true, UserID: 1}, "")
	ana, _ := h.subscribe(LiveViewer{UserID: 2}, "")
	bo, _ := h.subscribe(LiveViewer{UserID: 3}, "")
	h.PublishDLUser("1", "for ana", 2)
	if len(mgr.ch) != 0 || len(bo.ch) != 0 || len(ana.ch) != 1 {
		t.Fatalf("live: manager %d, bo %d, ana %d", len(mgr.ch), len(bo.ch), len(ana.ch))
	}
	for _, v := range []LiveViewer{{All: true, UserID: 1}, {UserID: 3}} {
		if _, replay := h.subscribe(v, h.EventID(0)); len(replay) != 0 {
			t.Fatalf("replay to %+v: %+v", v, replay)
		}
	}
	if _, replay := h.subscribe(LiveViewer{UserID: 2}, h.EventID(0)); len(replay) != 1 {
		t.Fatalf("replay to ana: %+v", replay)
	}
	if got := h.Viewers(); len(got) != 2 {
		t.Fatalf("viewers %v", got)
	}
}

// Ask events follow the asks routes: the full one goes to people
// who approve, each asker who does not approve gets their own copy, and
// someone who only runs the Library (or another member) gets neither.
func TestAskEventsFollowApprove(t *testing.T) {
	h := NewLiveHub(LiveOptions{})
	srv := httptest.NewServer(h.Handler(viewerFromHeader))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	open := func(user, manager, approve string) chan got {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
		req.Header.Set("X-User", user)
		req.Header.Set("X-Manager", manager)
		req.Header.Set("X-Approve", approve)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		ch := make(chan got, 16)
		go readEvents(t, resp, ch, nil)
		return ch
	}
	member := open("7", "0", "0")
	other := open("8", "0", "0")
	approver := open("1", "0", "1")
	librarian := open("9", "1", "0")
	waitClients(t, h, 4)
	h.PublishApprovers("ask", "full")
	h.PublishUser("ask", "for 7", 7)
	if e := next(t, member); string(e.data) != `"for 7"` {
		t.Fatalf("member got %s", e.data)
	}
	if e := next(t, approver); string(e.data) != `"full"` {
		t.Fatalf("approver got %s", e.data)
	}
	select {
	case e := <-approver:
		t.Fatalf("approver got the member's copy too: %s", e.data)
	case e := <-other:
		t.Fatalf("another member got %s", e.data)
	case e := <-librarian:
		t.Fatalf("a Library manager who does not approve got %s", e.data)
	case <-time.After(150 * time.Millisecond):
	}
}
