package downloads

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// recordingQB is a qBittorrent that records every action it is sent.
type recordingQB struct {
	mu    sync.Mutex
	calls []string
}

func (f *recordingQB) saw() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.calls, " ")
}

// pauseAllHash is the torrent controlSeed's client holds in each category.
var pauseAllHash = strings.Repeat("c", 40)

func controlSeed(t *testing.T) (*dl41, *recordingQB) {
	t.Helper()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	f := &recordingQB{}
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		path := strings.TrimPrefix(r.URL.Path, "/api/v2/")
		f.mu.Lock()
		f.calls = append(f.calls, path+"="+r.Form.Get("hashes")+r.Form.Get("limit")+r.Form.Get("id")+r.Form.Get("priority"))
		f.mu.Unlock()
		switch path {
		case "sync/maindata":
			_, _ = w.Write([]byte(`{"torrents":{},"server_state":{"free_space_on_disk":42,"dl_info_speed":7,"dl_rate_limit":2048}}`))
		case "torrents/trackers":
			_, _ = w.Write([]byte(`[{"url":"http://t.lab/announce","tier":0,"status":2}]`))
		case "sync/torrentPeers":
			_, _ = w.Write([]byte(`{"peers":{}}`))
		case "torrents/info":
			// One torrent in each category asked for, so Pause all has something of ours.
			if c := r.Form.Get("category"); c != "" {
				_, _ = w.Write([]byte(`[{"hash":"` + pauseAllHash + `","category":"` + c + `","state":"downloading"}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case "torrents/properties":
			_, _ = w.Write([]byte(`{"total_size":2000,"total_downloaded":500,"share_ratio":0.25,"seeds":1,"seeds_total":5,"peers":2,"peers_total":8,"addition_date":1700000000,"save_path":"/downloads","is_private":false}`))
		case "torrents/files":
			_, _ = w.Write([]byte(`[{"index":0,"name":"Movie/movie.mkv","size":1900,"progress":0.25,"priority":1},{"index":1,"name":"Movie/sample.mkv","size":100,"progress":0,"priority":0}]`))
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	for _, w := range []string{"torrents/stop=" + pauseAllHash, "torrents/start=" + pauseAllHash, "transfer/setDownloadLimit=1048576", "transfer/setUploadLimit=0"} {
		if !strings.Contains(got, w) {
			t.Fatalf("missing %s in %s", w, got)
		}
	}
}

// route: GET /api/v1/downloads/{id}/detail
// route: GET /api/v1/downloads/{id}/files
// route: PUT /api/v1/downloads/{id}/files
func TestDownloadDetailAndFiles(t *testing.T) {
	t.Parallel()
	x, f := controlSeed(t)
	base := x.srv.URL + "/api/v1/downloads/" + itoa(x.dlID)
	var d DownloadDetailView
	_ = json.Unmarshal(do(t, "GET", base+"/detail", "", 200), &d)
	if d.Torrent == nil || d.Torrent.Downloaded != 500 || d.Torrent.SeedsTotal != 5 || d.Torrent.SavePath != "/downloads" || d.Downloader == "" || d.Log == nil {
		t.Fatalf("detail %+v", d)
	}
	var fs struct {
		Files []map[string]any `json:"files"`
	}
	_ = json.Unmarshal(do(t, "GET", base+"/files", "", 200), &fs)
	if len(fs.Files) != 2 || fs.Files[1]["priority"] != float64(0) {
		t.Fatalf("files %+v", fs)
	}
	do(t, "PUT", base+"/files", `{"indexes":[1],"priority":3}`, 400)
	do(t, "PUT", base+"/files", `{"indexes":[],"priority":1}`, 400)
	do(t, "PUT", base+"/files", `{"indexes":[0,1],"priority":6}`, 200)
	if got := f.saw(); !strings.Contains(got, "torrents/filePrio=0|16") {
		t.Fatalf("calls %s", got)
	}
	do(t, "GET", x.srv.URL+"/api/v1/downloads/999/detail", "", 404)
	exec41(t, x.s, "UPDATE downloads SET state = 'cleaned'")
	d = DownloadDetailView{}
	_ = json.Unmarshal(do(t, "GET", base+"/detail", "", 200), &d)
	if d.Torrent != nil {
		t.Fatalf("a finished download asks the client: %+v", d)
	}
	do(t, "GET", base+"/files", "", 409)
}

// A download a person picked, sent by a background job, names that person in
// the list and in the pane alike; "HolyArrs" shows only when no person did.
func TestRequestedByAgreesInListAndPane(t *testing.T) {
	t.Parallel()
	x, _ := controlSeed(t)
	exec41(t, x.s, `INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (2, 't', '{}', 't', '{}', 'ana', 'member', 'x')`)
	exec41(t, x.s, `UPDATE downloads SET started_by = '{"kind":"job","id":7}'`)
	both := func() (string, string) {
		var d DownloadDetailView
		_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/"+itoa(x.dlID)+"/detail", "", 200), &d)
		var v viewRows
		_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view", "", 200), &v)
		if len(v.Downloads) == 0 {
			t.Fatal("no list row")
		}
		return v.Downloads[0].RequestedBy, d.RequestedBy
	}
	if list, pane := both(); list != "" || pane != "HolyArrs" {
		t.Fatalf("nobody picked it: list %q pane %q", list, pane)
	}
	exec41(t, x.s, `UPDATE titles SET created_by = '{"kind":"user","id":2}'`)
	if list, pane := both(); list != "ana" || pane != "ana" {
		t.Fatalf("title owner: list %q pane %q", list, pane)
	}
	exec41(t, x.s, `INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (3, 't', '{}', 't', '{}', 'bo', 'member', 'x')`)
	exec41(t, x.s, `UPDATE downloads SET started_by = '{"kind":"user","id":3}'`)
	if list, pane := both(); list != "bo" || pane != "bo" {
		t.Fatalf("person who picked it: list %q pane %q", list, pane)
	}
}
