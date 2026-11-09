package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

func waitEnd(t *testing.T, done <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("%s: the /live stream kept its old rights", what)
	}
}

// An open /live stream does not keep the rights it had when it opened
// (coord-1330a): a demoted approver, a sign-out and an admin's end-all each
// end it, and a stream opened again carries only the rights held now.
// route: GET /live
func TestLiveStreamEndsWhenItsRightsGo(t *testing.T) {
	t.Parallel()
	x := setupLive(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	user := "/api/v1/users/" + strconv.FormatInt(x.mem.id, 10)
	if st, out := x.call(t, x.own, "PUT", user, `{"perms":["ask","approve"]}`, true); st != 200 {
		t.Fatalf("make ana an approver: %d %s", st, out)
	}

	// Demoted approver.
	done := x.openStream(t, ctx, x.mem)
	x.waitClients(t, 1)
	if st, out := x.call(t, x.own, "PUT", user, `{"perms":["ask"]}`, true); st != 200 {
		t.Fatalf("take approve away: %d %s", st, out)
	}
	waitEnd(t, done, "approve taken away")
	again := x.stream(t, ctx, x.mem, "")
	x.waitClients(t, 1)
	x.hub.PublishApprovers("ask", "a decline note for approvers")
	if got := frames(again, 300*time.Millisecond); len(got) != 0 {
		t.Fatalf("a demoted approver still reads approvers' asks: %+v", got)
	}
	cancel()
	x.waitClients(t, 0)

	// Sign-out.
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	done = x.openStream(t, ctx, x.mem2)
	x.waitClients(t, 1)
	if st, out := x.call(t, x.mem2, "POST", "/api/v1/auth/logout", "", true); st != 204 {
		t.Fatalf("sign out: %d %s", st, out)
	}
	waitEnd(t, done, "signed out")

	// An admin ends all of someone's sessions.
	done = x.openStream(t, ctx, x.mem)
	x.waitClients(t, 1)
	if st, out := x.call(t, x.own, "POST", user+"/sessions/end-all", "", true); st/100 != 2 {
		t.Fatalf("end-all: %d %s", st, out)
	}
	waitEnd(t, done, "end-all")
	if st, _ := x.call(t, x.mem, "GET", "/live", "", false); st != 401 {
		t.Fatalf("ended session opened /live again: %d", st)
	}
}

// The Events list keeps asks' Events (who asked, who decided, the decline
// note) to people who approve, as /live does; other managers read the rest.
// route: GET /api/v1/events
func TestEventsHideAsksFromManagersWhoDoNotApprove(t *testing.T) {
	t.Parallel()
	x := setupLive(t)
	user := "/api/v1/users/" + strconv.FormatInt(x.mem.id, 10)
	if st, out := x.call(t, x.own, "PUT", user, `{"perms":["ask","manage_library"]}`, true); st != 200 {
		t.Fatalf("make ana a manager: %d %s", st, out)
	}
	ctx := context.Background()
	for _, e := range []*domain.Event{
		{Kind: domain.EventAskTurnedDown, Summary: "Heat: turned down by luis", Data: map[string]any{"note": "not this one"}},
		{Kind: "ai_ask_made", Summary: "Claude asked for Heat for ana"},
		{Kind: domain.EventReportMade, Summary: "a problem report"},
	} {
		if err := x.st.AddEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	kinds := func(u liveUser) []string {
		st, out := x.call(t, u, "GET", "/api/v1/events", "", false)
		if st != 200 {
			t.Fatalf("events: %d %s", st, out)
		}
		var page struct {
			Events []struct {
				Kind string `json:"kind"`
			} `json:"events"`
		}
		if err := json.Unmarshal([]byte(out), &page); err != nil {
			t.Fatalf("events: %v %s", err, out)
		}
		var k []string
		for _, e := range page.Events {
			k = append(k, e.Kind)
		}
		return k
	}
	mgr := strings.Join(kinds(x.mem), ",")
	if strings.Contains(mgr, domain.EventAskTurnedDown) || strings.Contains(mgr, "ai_ask_made") || !strings.Contains(mgr, domain.EventReportMade) {
		t.Fatalf("manager without approve reads %s", mgr)
	}
	if own := strings.Join(kinds(x.own), ","); !strings.Contains(own, domain.EventAskTurnedDown) {
		t.Fatalf("owner reads %s", own)
	}
}

// A title's live update leaves its asks out (who asked, who decided, the
// decline note): pages read the title again through its route, which shows
// them to approvers only.
func TestLiveTitleUpdateLeavesAsksOut(t *testing.T) {
	t.Parallel()
	hub := webserver.NewLiveHub(webserver.LiveOptions{})
	srv := httptest.NewServer(hub.Handler(func(*http.Request) webserver.LiveViewer { return webserver.LiveViewer{All: true} }))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	x := liveSetup{srv: srv, hub: hub}
	ch := x.streamURL(t, ctx, srv.URL)
	x.waitClients(t, 1)
	a := &handlers{s: services{Live: hub}}
	a.tv().PublishView(TitleView{Asks: []requests.View{{ID: 7, Note: "not this one"}}})
	got := frames(ch, 300*time.Millisecond)
	if len(got) != 1 || got[0].topic != "title" || strings.Contains(got[0].data, "not this one") || strings.Contains(got[0].data, `"id":7`) {
		t.Fatalf("title update %+v", got)
	}
}
