package downloads

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// Both adapters offer the controls, and engines without them still compile.
var (
	_ Controller        = (*Qbittorrent)(nil)
	_ TorrentController = (*Qbittorrent)(nil)
	_ Controller        = (*Sabnzbd)(nil)
	_ Retrier           = (*Sabnzbd)(nil)
)

// recorder answers by path (qBittorrent) or by mode and name (SABnzbd) and
// keeps every call it saw as "path?form".
type recorder struct {
	mu      sync.Mutex
	calls   []string
	answers map[string]string // key -> body
	codes   map[string]int    // key -> status
}

func TestQbitControlsSendTheRightActions(t *testing.T) {
	rc := &recorder{}
	q := NewQbittorrent(rc.start(t, qbitKey), nil)
	ctx := context.Background()
	for _, f := range []func() error{
		func() error { return q.PauseItem(ctx, strings.ToUpper(hashA)) },
		func() error { return q.ResumeItem(ctx, hashA) },
		func() error { return q.PauseAll(ctx) },
		func() error { return q.ResumeAll(ctx) },
		func() error { return q.Recheck(ctx, hashA) },
		func() error { return q.Reannounce(ctx, hashA) },
		func() error { return q.MoveTop(ctx, hashA) },
		func() error { return q.SetSpeedLimit(ctx, 1048576, 0) },
	} {
		if err := f(); err != nil {
			t.Fatal(err)
		}
	}
	rc.saw(t,
		"torrents/stop?hashes="+hashA, "torrents/start?hashes="+hashA,
		"torrents/stop?hashes=all", "torrents/start?hashes=all",
		"torrents/recheck?hashes="+hashA, "torrents/reannounce?hashes="+hashA,
		"torrents/topPrio?hashes="+hashA,
		"transfer/setDownloadLimit?limit=1048576", "transfer/setUploadLimit?limit=0")
}

func TestQbitControlsRefuseSelectors(t *testing.T) {
	rc := &recorder{}
	q := NewQbittorrent(rc.start(t, qbitKey), nil)
	for _, id := range []string{"all", hashA + "|" + hashA, ""} {
		if err := q.PauseItem(context.Background(), id); !errors.Is(err, errBadHash) {
			t.Fatalf("%q: %v", id, err)
		}
	}
	rc.saw(t)
}

func TestQbitOldServerFallsBackToPauseResume(t *testing.T) {
	rc := &recorder{codes: map[string]int{"torrents/stop": 404, "torrents/start": 404}}
	q := NewQbittorrent(rc.start(t, qbitKey), nil)
	if err := q.PauseItem(context.Background(), hashA); err != nil {
		t.Fatal(err)
	}
	if err := q.ResumeAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	rc.saw(t, "torrents/stop?hashes="+hashA, "torrents/pause?hashes="+hashA, "torrents/start?hashes=all", "torrents/resume?hashes=all")
}

func TestQbitMoveTopWithQueueOff(t *testing.T) {
	rc := &recorder{codes: map[string]int{"torrents/topPrio": 409}}
	q := NewQbittorrent(rc.start(t, qbitKey), nil)
	err := q.MoveTop(context.Background(), hashA)
	if err == nil || !strings.Contains(err.Error(), "queue turned off") {
		t.Fatal(err)
	}
}

func TestQbitStatusTrackersPeers(t *testing.T) {
	rc := &recorder{answers: map[string]string{
		"sync/maindata": `{"rid":1,"full_update":true,"torrents":{"` + hashA + `":{"state":"stoppedDL"}},
			"server_state":{"alltime_dl":900,"alltime_ul":800,"dl_info_data":90,"up_info_data":80,"dl_info_speed":5,"up_info_speed":4,
			"dl_rate_limit":1024,"up_rate_limit":0,"free_space_on_disk":123456}}`,
		"torrents/trackers": `[{"url":"** [DHT] **","tier":-1,"status":0},
			{"url":"http://tracker.lab/announce","tier":0,"status":2,"msg":"","num_seeds":3,"num_peers":-1}]`,
		"sync/torrentPeers": `{"peers":{"10.0.0.2:6881":{"ip":"10.0.0.2","port":6881,"client":"qBittorrent 5.0","progress":0.5,"dl_speed":10,"up_speed":2,"country":"Lab"}}}`,
	}}
	q := NewQbittorrent(rc.start(t, qbitKey), nil)
	ctx := context.Background()
	st, err := q.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := ClientStatus{Paused: true, Speed: 5, UpSpeed: 4, SpeedLimit: 1024, SessionDown: 90, SessionUp: 80, AllTimeDown: 900, AllTimeUp: 800, FreeSpace: 123456}
	if st != want {
		t.Fatalf("status %+v", st)
	}
	tr, err := q.Trackers(ctx, hashA)
	if err != nil || len(tr) != 2 || !tr[1].Fixed || tr[1].URL != "DHT" || tr[0].Ref == "" || tr[0].Status != "working" || tr[0].Seeds != 3 || tr[0].Peers != 0 {
		t.Fatalf("trackers %+v %v", tr, err)
	}
	ps, err := q.Peers(ctx, hashA)
	if err != nil || len(ps) != 1 || ps[0].Address != "10.0.0.2:6881" || ps[0].Speed != 10 {
		t.Fatalf("peers %+v %v", ps, err)
	}
}

func TestSabControlsSendTheRightActions(t *testing.T) {
	rc := &recorder{answers: map[string]string{
		"switch": `{"result":{"position":0,"priority":0}}`,
		"retry":  `{"status":true,"nzo_id":"SABnzbd_nzo_new"}`,
	}}
	s := NewSabnzbd(rc.start(t, sabKeyOf), sabKey, nil)
	ctx := context.Background()
	for _, f := range []func() error{
		func() error { return s.PauseItem(ctx, "SABnzbd_nzo_1") },
		func() error { return s.ResumeItem(ctx, "SABnzbd_nzo_1") },
		func() error { return s.PauseAll(ctx) },
		func() error { return s.ResumeAll(ctx) },
		func() error { return s.MoveTop(ctx, "SABnzbd_nzo_1") },
		func() error { return s.SetSpeedLimit(ctx, 2*1048576, 0) },
		func() error { return s.SetSpeedLimit(ctx, 0, 0) },
	} {
		if err := f(); err != nil {
			t.Fatal(err)
		}
	}
	id, err := s.Retry(ctx, "SABnzbd_nzo_1")
	if err != nil || id != "SABnzbd_nzo_new" {
		t.Fatal(id, err)
	}
	rc.saw(t,
		"queue/pause?mode=queue&name=pause&value=SABnzbd_nzo_1", "queue/resume?mode=queue&name=resume&value=SABnzbd_nzo_1",
		"pause?mode=pause", "resume?mode=resume",
		"switch?mode=switch&value=SABnzbd_nzo_1&value2=0",
		"config/speedlimit?mode=config&name=speedlimit&value=2048K", "config/speedlimit?mode=config&name=speedlimit&value=0",
		"retry?mode=retry&value=SABnzbd_nzo_1")
}

func TestSabControlsRefuseLists(t *testing.T) {
	rc := &recorder{}
	s := NewSabnzbd(rc.start(t, sabKeyOf), sabKey, nil)
	if err := s.PauseItem(context.Background(), "a,b"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if err := s.MoveTop(context.Background(), ""); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	rc.saw(t)
}

func TestSabMoveTopUnknownJob(t *testing.T) {
	rc := &recorder{answers: map[string]string{"switch": `{"result":{"position":-1,"priority":0}}`}}
	s := NewSabnzbd(rc.start(t, sabKeyOf), sabKey, nil)
	if err := s.MoveTop(context.Background(), "SABnzbd_nzo_gone"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestSabStatus(t *testing.T) {
	rc := &recorder{answers: map[string]string{
		"queue":        `{"queue":{"paused":true,"kbpersec":"2.00","speedlimit_abs":"1048576","diskspace1":"10.00","diskspace2":"2.00","slots":[]}}`,
		"server_stats": `{"total":5000,"month":400,"week":300,"day":20,"servers":{}}`,
	}}
	s := NewSabnzbd(rc.start(t, sabKeyOf), sabKey, nil)
	st, err := s.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := ClientStatus{Paused: true, Speed: 2048, SpeedLimit: 1048576, FreeSpace: 2 << 30, AllTimeDown: 5000, MonthDown: 400, DayDown: 20}
	if st != want {
		t.Fatalf("status %+v", st)
	}
}

func TestTrackerHostFallback(t *testing.T) {
	if got := trackerHost("t.example/abc0123456789abcdef0123456789"); got != "tracker" {
		t.Fatalf("trackerHost without a scheme = %q, want tracker", got)
	}
}

func TestHostOnly(t *testing.T) {
	cases := map[string]string{
		"https://user:pass@Tracker.Example.org/announce?k=1": "tracker.example.org",
		"udp://t.example:6969/announce":                      "t.example",
		"https://user:pass@t.example:8443/x":                 "t.example",
		"%%bad":                                              "",
	}
	for in, want := range cases {
		if got := HostOnly(in); got != want {
			t.Errorf("HostOnly(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRedactURLs(t *testing.T) {
	cases := map[string]string{
		"announce failed: https://t.example.org/abc123passkey/announce?x=1": "announce failed: https://t.example.org",
		"two udp://a.example:80/key and http://u:p@b.example/k":             "two udp://a.example:80 and http://b.example",
		"Not working": "Not working",
		"bad request tracker.example/announce?passkey=deadbeef&x=1":  "bad request tracker.example/announce?passkey=…&x=1",
		"unregistered authkey=AbC torrent_pass=zz pid=9 key=k1":      "unregistered authkey=… torrent_pass=… pid=… key=…",
		"denied t.example/0123456789abcdef0123456789abcdef/announce": "denied t.example/…/announce",
		"x ?passkey=abc123": "x ?passkey=…",
		"x &key=abc":        "x &key=…",
		"announce/0123456789abcdef0123456789abcdef": "announce/…",
		"basket=1 monkey=2 sass":                    "basket=1 monkey=2 sass",
		"":                                          "",
		"x api_key=s1 apikey=s2 token=s3 uid=4":     "x api_key=… apikey=… token=… uid=…",
		"tracker says passkey: abc":                 "tracker says passkey: …",
		"monkey=1 basket=2":                         "monkey=1 basket=2",
	}
	for in, want := range cases {
		if got := redactURLs(in); got != want {
			t.Errorf("redactURLs(%q) = %q, want %q", in, got, want)
		}
	}
}

func (rc *recorder) start(t *testing.T, key func(*http.Request) string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		k := key(r)
		form := r.Form
		form.Del("apikey")
		form.Del("output")
		rc.mu.Lock()
		rc.calls = append(rc.calls, k+"?"+form.Encode())
		code, body := rc.codes[k], rc.answers[k]
		rc.mu.Unlock()
		if code != 0 {
			w.WriteHeader(code)
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func (rc *recorder) saw(t *testing.T, want ...string) {
	t.Helper()
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if strings.Join(rc.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s\nwant:\n%s", strings.Join(rc.calls, "\n"), strings.Join(want, "\n"))
	}
}

func qbitKey(r *http.Request) string { return strings.TrimPrefix(r.URL.Path, "/api/v2/") }

func sabKeyOf(r *http.Request) string {
	if n := r.Form.Get("name"); n != "" && (r.Form.Get("mode") == "queue" || r.Form.Get("mode") == "config") {
		return r.Form.Get("mode") + "/" + n
	}
	return r.Form.Get("mode")
}
