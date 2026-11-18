package downloads

import (
	"context"
	"database/sql"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// NZBGet may reach only the Source's own address on the home network, as on
// the SABnzbd path: a Release link on another local host is refused even when
// the Source may reach the local network.
func TestNZBGetAllowsOnlyTheSourcesOwnLocalHost(t *testing.T) {
	file := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<?xml version=\"1.0\"?><nzb></nzb>"))
	}))
	t.Cleanup(file.Close)
	f, base := startNZBGet(t)
	n := NewNZBGet(base, f.user, f.pass, nil)
	_, err := n.Add(context.Background(), AddRequest{URL: file.URL + "/getnzb/1.nzb", Name: "Other", Category: "tv",
		OperationKey: "other", AllowLocal: true, SourceURL: "http://192.168.1.10:9696"})
	if err == nil || f.adds != 0 {
		t.Fatalf("a link on another local host was fetched: adds=%d err=%v", f.adds, err)
	}
	if _, err := n.Add(context.Background(), AddRequest{URL: file.URL + "/getnzb/1.nzb", Name: "Own", Category: "tv",
		OperationKey: "own", AllowLocal: true, SourceURL: file.URL}); err != nil || f.adds != 1 {
		t.Fatalf("the Source's own link: adds=%d err=%v", f.adds, err)
	}
}

// A Source that may already reach the local network gets a hint that names
// the real cause instead of asking to turn that setting on.
func TestLinkToAnotherLocalAddressSaysSo(t *testing.T) {
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://10.0.0.7:8080/getnzb/1.nzb", http.StatusFound)
	}))
	t.Cleanup(src.Close)
	_, _, err := fetchGrabLink(context.Background(), src.URL+"/x", 1<<20, nil, sourceLocalHost(src.URL, true))
	if err != errLinkOtherLocal {
		t.Fatalf("with local access: %v", err)
	}
	fakeDNS(t, map[string]string{"idx.example": hostPort(t, src)}, true)
	_, _, err = fetchGrabLink(context.Background(), "http://idx.example:"+portOf(t, src)+"/x", 1<<20, nil, "")
	if err != errLinkLocal {
		t.Fatalf("without local access: %v", err)
	}
}

// A send refused because the Release link led to a refused address is never
// shown as sent: the Slot waits for a person with the reason, so the failure
// shows where problems are listed, not only in the history.
func TestRefusedLinkWaitsForAPerson(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://10.0.0.7:8080/x.torrent", http.StatusFound)
	}))
	t.Cleanup(src.Close)
	fakeDNS(t, map[string]string{"idx.example": hostPort(t, src)}, true)
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE sources SET allow_local_network = 0, url = 'http://idx.example'`); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE releases SET link_ref = ?`, "http://idx.example:"+portOf(t, src)+"/r")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	run := &jobs.Run{Job: domain.Job{Subject: &domain.Subject{Type: "slot", ID: w.slotID}, Params: map[string]any{"release_id": float64(w.relID)}, Attempt: 1, MaxAttempts: 5}}
	if err := m.HandleGrab(ctx, run); err != nil {
		t.Fatalf("the Job should end, not retry: %v", err)
	}
	var sl mediafiles.Slot
	if err := database.Get(ctx, w.s.Read(), "slots", w.slotID, &sl); err != nil {
		t.Fatal(err)
	}
	if sl.State != mediafiles.SlotNeedsADecision || sl.LastReason == nil || *sl.LastReason != "not sent: "+errLinkLocal.msg {
		t.Fatalf("slot %s, reason %v", sl.State, sl.LastReason)
	}
	if why(*sl.LastReason) != "link_refused" {
		t.Fatalf("why = %q", why(*sl.LastReason))
	}
	var n int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM downloads").Scan(&n)
	if n != 0 || len(w.fake.added) != 0 {
		t.Fatalf("something was sent: %d downloads", n)
	}
}

func portOf(t *testing.T, s *httptest.Server) string {
	t.Helper()
	_, p, _ := net.SplitHostPort(hostPort(t, s))
	return p
}
