package requests

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// An ask's Event names who asked, who decided and the decline note, so on
// /live it reaches people who approve only, not every manager; other
// Events (Reports) still reach managers (coord-1330a).
func TestAskEventsReachOnlyApproversLive(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	hub := webserver.NewLiveHub(webserver.LiveOptions{})
	srv := httptest.NewServer(hub.Handler(func(r *http.Request) webserver.LiveViewer {
		return webserver.LiveViewer{UserID: 2, All: r.URL.Query().Get("all") == "1", Approve: r.URL.Query().Get("approve") == "1"}
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	read := func(q string) <-chan string {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"?"+q, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		out := make(chan string, 16)
		go func() {
			defer resp.Body.Close()
			sc := bufio.NewScanner(resp.Body)
			for sc.Scan() {
				if l := sc.Text(); strings.HasPrefix(l, "data: ") {
					out <- l
				}
			}
		}()
		return out
	}
	manager := read("all=1")
	approver := read("approve=1")
	for hub.Clients() != 2 {
		time.Sleep(5 * time.Millisecond)
	}
	s := &Service{Store: st, Live: hub}
	s.event(ctx, &domain.Event{Kind: domain.EventAskTurnedDown, Summary: "Heat: turned down by luis", Data: map[string]any{"note": "not this one"}})
	s.event(ctx, &domain.Event{Kind: domain.EventReportMade, Summary: "a problem report"})
	collect := func(ch <-chan string) string {
		var all []string
		deadline := time.After(400 * time.Millisecond)
		for {
			select {
			case l := <-ch:
				all = append(all, l)
			case <-deadline:
				return strings.Join(all, "\n")
			}
		}
	}
	m, a := collect(manager), collect(approver)
	if strings.Contains(m, "turned down") || strings.Contains(m, "not this one") {
		t.Errorf("a manager without approve read the turn-down: %s", m)
	}
	if !strings.Contains(m, "a problem report") {
		t.Errorf("the manager missed the report: %s", m)
	}
	if !strings.Contains(a, "turned down by luis") || !strings.Contains(a, "not this one") {
		t.Errorf("the approver missed the turn-down: %s", a)
	}
}
