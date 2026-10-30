package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
)

// fakeClient is a qBittorrent that records every action it is sent.
type fakeClient struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeClient) saw() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.calls, " ")
}

func controlSeed(t *testing.T) (*dl41, *fakeClient) {
	t.Helper()
	x := seed41(t, domain.SlotDownloading, "downloading")
	f := &fakeClient{}
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		path := strings.TrimPrefix(r.URL.Path, "/api/v2/")
		f.mu.Lock()
		f.calls = append(f.calls, path+"="+r.Form.Get("hashes")+r.Form.Get("limit"))
		f.mu.Unlock()
		switch path {
		case "sync/maindata":
			_, _ = w.Write([]byte(`{"torrents":{},"server_state":{"free_space_on_disk":42,"dl_info_speed":7,"dl_rate_limit":2048}}`))
		case "torrents/trackers":
			_, _ = w.Write([]byte(`[{"url":"http://t.lab/announce","tier":0,"status":2}]`))
		case "sync/torrentPeers":
			_, _ = w.Write([]byte(`{"peers":{}}`))
		case "torrents/info":
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	t.Cleanup(qb.Close)
	exec41(t, x.s, "UPDATE downloaders SET url = ?", qb.URL)
	return x, f
}

// route: POST /api/v1/downloads/{id}/pause
// route: POST /api/v1/downloads/{id}/resume
// route: POST /api/v1/downloads/{id}/top
// route: POST /api/v1/downloads/{id}/recheck
// route: POST /api/v1/downloads/{id}/reannounce
func TestDownloadItemControls(t *testing.T) {
	x, f := controlSeed(t)
	base := x.srv.URL + "/api/v1/downloads/" + itoa(x.dlID)
	for _, act := range []string{"pause", "resume", "top", "recheck", "reannounce"} {
		do(t, "POST", base+"/"+act, "", 200)
	}
	h := "abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd"
	want := "torrents/stop=" + h + " torrents/start=" + h + " torrents/topPrio=" + h + " torrents/recheck=" + h + " torrents/reannounce=" + h
	if got := f.saw(); !strings.HasPrefix(got, want) {
		t.Fatalf("calls %s", got)
	}
	do(t, "POST", x.srv.URL+"/api/v1/downloads/999/pause", "", 404)
	exec41(t, x.s, "UPDATE downloads SET state = 'cleaned'")
	do(t, "POST", base+"/pause", "", 409)
}

// route: GET /api/v1/downloads/{id}/swarm
func TestDownloadSwarm(t *testing.T) {
	x, _ := controlSeed(t)
	var out struct {
		Trackers []map[string]any `json:"trackers"`
		Peers    []map[string]any `json:"peers"`
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/"+itoa(x.dlID)+"/swarm", "", 200), &out)
	if len(out.Trackers) != 1 || out.Trackers[0]["status"] != "working" || out.Peers == nil {
		t.Fatalf("swarm %+v", out)
	}
}

// route: GET /api/v1/downloads/clients
// route: POST /api/v1/downloads/pause-all
// route: POST /api/v1/downloads/resume-all
// route: PUT /api/v1/downloaders/{id}/speed-limit
func TestDownloadClientsAndAll(t *testing.T) {
	x, f := controlSeed(t)
	var out struct {
		Clients []ClientView `json:"clients"`
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/clients", "", 200), &out)
	if len(out.Clients) != 1 || !out.Clients[0].Controls || !out.Clients[0].Torrent || out.Clients[0].Status == nil ||
		out.Clients[0].Status.FreeSpace != 42 || out.Clients[0].Status.SpeedLimit != 2048 {
		t.Fatalf("clients %+v", out)
	}
	do(t, "POST", x.srv.URL+"/api/v1/downloads/pause-all", "", 200)
	do(t, "POST", x.srv.URL+"/api/v1/downloads/resume-all", "", 200)
	id := itoa(out.Clients[0].ID)
	do(t, "PUT", x.srv.URL+"/api/v1/downloaders/"+id+"/speed-limit", `{"down":-1}`, 400)
	do(t, "PUT", x.srv.URL+"/api/v1/downloaders/"+id+"/speed-limit", `{"down":1048576,"up":0}`, 200)
	do(t, "PUT", x.srv.URL+"/api/v1/downloaders/999/speed-limit", `{"down":0}`, 404)
	got := f.saw()
	for _, w := range []string{"torrents/stop=all", "torrents/start=all", "transfer/setDownloadLimit=1048576", "transfer/setUploadLimit=0"} {
		if !strings.Contains(got, w) {
			t.Fatalf("missing %s in %s", w, got)
		}
	}
}
