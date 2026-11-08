package downloads

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

const tinyTorrent = "d4:infod6:lengthi1e4:name1:a12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaaee"

type manageFake struct {
	mu      sync.Mutex
	calls   []string
	answers map[string]string
	codes   map[string]int
}

func TestLinkHash(t *testing.T) {
	for in, want := range map[string]string{
		"magnet:?xt=urn:btih:" + strings.ToUpper(hashA) + "&dn=x": hashA,
		"magnet:?xt=urn:btih:MFQWCYLBMFQWCYLBMFQWCYLBMFQWCYLB":    "6161616161616161616161616161616161616161",
		hashA:                                  hashA,
		"https://example.test/file.torrent":    "",
		"magnet:?dn=no-hash":                   "",
		"magnet:?xt=urn:btih:nothex-nothex-zz": "",
	} {
		if got := linkHash(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestQbitAddTorrentsOldServer(t *testing.T) {
	f := &manageFake{answers: map[string]string{"torrents/info": `[]`, "torrents/add": "Ok."}}
	q := NewQbittorrent(f.start(t), nil)
	out, err := q.AddTorrents(context.Background(), AddBatch{
		Links: []string{"magnet:?xt=urn:btih:" + hashA, ""}, Files: []TorrentUpload{{Name: `C:\dl\a.torrent`, Data: []byte(tinyTorrent)}},
		Category: "linux", SavePath: "/data/linux", Paused: true,
	})
	if err != nil || out.Added != 2 || out.Failed != 0 || out.Duplicates != 0 {
		t.Fatalf("%+v %v", out, err)
	}
	got := f.last("torrents/add")
	for _, want := range []string{"category=linux", "savepath=/data/linux", "stopped=true", "paused=true", "file=a.torrent:d", "urls=magnet:?xt=urn:btih:" + hashA} {
		if !strings.Contains(got, want) {
			t.Fatalf("add call %q lacks %q", got, want)
		}
	}
	f.answers["torrents/add"] = "Fails."
	out, err = q.AddTorrents(context.Background(), AddBatch{Links: []string{hashA}})
	if err != nil || out.Failed != 1 || out.Added != 0 {
		t.Fatalf("fails: %+v %v", out, err)
	}
}

func TestQbitAddTorrentsNewServer(t *testing.T) {
	f := &manageFake{answers: map[string]string{"torrents/info": `[]`, "torrents/add": `{"success_count":1,"failure_count":1,"pending_count":1,"added_torrent_ids":["x"]}`}}
	q := NewQbittorrentKey(f.start(t), "k", nil)
	ctx := context.Background()
	out, err := q.AddTorrents(ctx, AddBatch{Links: []string{"magnet:?xt=urn:btih:" + hashA, "magnet:?xt=urn:btih:" + strings.Repeat("d", 40), "magnet:?xt=urn:btih:" + strings.Repeat("b", 40)}})
	if err != nil || out.Added != 1 || out.Failed != 1 || out.Pending != 1 {
		t.Fatalf("%+v %v", out, err)
	}
	f.codes = map[string]int{"torrents/add": 409}
	f.answers["torrents/add"] = "Conflict"
	out, err = q.AddTorrents(ctx, AddBatch{Links: []string{hashA}})
	if err != nil || out.Failed != 1 {
		t.Fatalf("409: %+v %v", out, err)
	}
	f.codes = map[string]int{"torrents/add": 415}
	if _, err = q.AddTorrents(ctx, AddBatch{Files: []TorrentUpload{{Name: "a.torrent", Data: []byte(tinyTorrent)}}}); err == nil || !strings.Contains(err.Error(), "could not read") {
		t.Fatalf("415: %v", err)
	}
}

func TestQbitAddTorrentsDuplicatesAndBadFiles(t *testing.T) {
	f := &manageFake{answers: map[string]string{"torrents/info": `[{"hash":"` + strings.ToUpper(hashA) + `","state":"uploading"}]`, "torrents/add": "Ok."}}
	q := NewQbittorrent(f.start(t), nil)
	ctx := context.Background()
	out, err := q.AddTorrents(ctx, AddBatch{Links: []string{"magnet:?xt=urn:btih:" + hashA, hashA}, Files: []TorrentUpload{{Name: "notes.txt", Data: []byte("hello")}}})
	if err != nil || out.Duplicates != 2 || out.Failed != 1 || len(out.Bad) != 1 || out.Bad[0] != "notes.txt" {
		t.Fatalf("%+v %v", out, err)
	}
	if n := f.count("torrents/add"); n != 0 {
		t.Fatalf("add was called %d times", n)
	}
	if _, err := q.AddTorrents(ctx, AddBatch{Links: []string{" "}}); err != ErrNothingToAdd {
		t.Fatalf("empty: %v", err)
	}
	two := strings.Repeat("c", 40)
	out, err = q.AddTorrents(ctx, AddBatch{Links: []string{two, "magnet:?xt=urn:btih:" + two}})
	if err != nil || out.Added != 1 || out.Duplicates != 1 {
		t.Fatalf("repeat: %+v %v", out, err)
	}
}

func TestQbitBulkActions(t *testing.T) {
	f := &manageFake{}
	q := NewQbittorrent(f.start(t), nil)
	ctx := context.Background()
	b := strings.Repeat("b", 40)
	hs := []string{strings.ToUpper(hashA), b, hashA}
	want := map[string]string{
		BulkStart: "torrents/start?hashes=" + hashA + "%7C" + b, BulkPause: "torrents/stop?hashes=" + hashA + "%7C" + b,
		BulkRecheck: "torrents/recheck?hashes=" + hashA + "%7C" + b, BulkReannounce: "torrents/reannounce?hashes=" + hashA + "%7C" + b,
		BulkTop: "torrents/topPrio?hashes=" + hashA + "%7C" + b, BulkBottom: "torrents/bottomPrio?hashes=" + hashA + "%7C" + b,
		BulkUp: "torrents/increasePrio?hashes=" + hashA + "%7C" + b, BulkDown: "torrents/decreasePrio?hashes=" + hashA + "%7C" + b,
		BulkForce: "torrents/setForceStart?hashes=" + hashA + "%7C" + b + "&value=true", BulkUnforce: "torrents/setForceStart?hashes=" + hashA + "%7C" + b + "&value=false",
	}
	for _, a := range BulkActions {
		if err := q.Bulk(ctx, a, hs); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
		path := strings.SplitN(want[a], "?", 2)[0]
		if got := f.last(path); got != want[a] {
			t.Fatalf("%s: %q, want %q", a, got, want[a])
		}
	}
	if err := q.RemoveMany(ctx, []string{hashA}, true); err != nil || f.last("torrents/delete") != "torrents/delete?deleteFiles=true&hashes="+hashA {
		t.Fatalf("remove %v %q", err, f.last("torrents/delete"))
	}
	if err := q.Bulk(ctx, BulkStart, []string{"all"}); !errors.Is(err, errBadHash) {
		t.Fatalf("all: %v", err)
	}
	if err := q.Bulk(ctx, "explode", hs); !errors.Is(err, ErrBulkAction) {
		t.Fatalf("unknown: %v", err)
	}
	if err := q.Bulk(ctx, BulkStart, nil); err == nil {
		t.Fatal("empty selection passed")
	}
	f.codes = map[string]int{"torrents/bottomPrio": 409}
	if err := q.Bulk(ctx, BulkBottom, hs); !errors.Is(err, ErrQueueOff) {
		t.Fatalf("queue off: %v", err)
	}
	if n := f.count("torrents/bottomPrio"); n != 2 {
		t.Fatalf("a queue move was sent %d times", n)
	}
}

func TestQbitAddFetchesLinksItself(t *testing.T) {
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a.torrent":
			_, _ = w.Write([]byte(tinyTorrent))
		case "/big.torrent":
			_, _ = w.Write([]byte(strings.Repeat("d", int(addFetchMax)+10)))
		case "/away":
			http.Redirect(w, r, "http://127.0.0.2:9/a.torrent", http.StatusFound)
		case "/text":
			_, _ = w.Write([]byte("not a torrent"))
		}
	}))
	t.Cleanup(src.Close)
	f := &manageFake{answers: map[string]string{"torrents/info": `[]`, "torrents/add": "Ok."}}
	q := NewQbittorrent(f.start(t), nil)
	ctx := context.Background()
	out, err := q.AddTorrents(ctx, AddBatch{Links: []string{src.URL + "/a.torrent?key=secret"}, AllowLocal: true})
	if err != nil || out.Added != 1 || !strings.Contains(f.last("torrents/add"), "file=a.torrent:d") || strings.Contains(f.last("torrents/add"), "urls=") {
		t.Fatalf("fetched %+v %v %q", out, err, f.last("torrents/add"))
	}
	for _, bad := range []string{"/big.torrent", "/away", "/text"} {
		out, err = q.AddTorrents(ctx, AddBatch{Links: []string{src.URL + bad}, AllowLocal: true})
		if err != nil || out.Failed != 1 || len(out.Bad) != 1 || strings.Contains(out.Bad[0], "key=") {
			t.Fatalf("%s: %+v %v", bad, out, err)
		}
	}
	before := f.count("torrents/add")
	for _, l := range []string{src.URL + "/a.torrent", strings.Replace(src.URL, "127.0.0.1", "localhost", 1) + "/a.torrent"} {
		out, err = q.AddTorrents(ctx, AddBatch{Links: []string{l}})
		if err != nil || out.Failed != 1 {
			t.Fatalf("%s without local access: %+v %v", l, out, err)
		}
	}
	if f.count("torrents/add") != before {
		t.Fatal("a refused link reached the client")
	}
}

func TestSafeMagnetAllowlist(t *testing.T) {
	m := "magnet:?XT=urn:btih:" + strings.ToUpper(hashA) + "&dn=Some%20Name&TR=http%3A%2F%2F10.0.0.5%2Fannounce&tr=udp%3A%2F%2Ftracker.example%3A80&tr=udp%3A%2F%2Ftracker.example%3A80" +
		"&tr=ftp%3A%2F%2Fx.example%2F&tr=http%3A%2F%2Flocalhost%2Fa&tr=http%3A%2F%2F169.254.169.254%2F&Xs=http%3A%2F%2F127.0.0.1%2Fx.torrent&ws=http%3A%2F%2Fseed.example%2F&x.pe=192.168.1.2%3A6881&dht=1&tr.1=http%3A%2F%2Fa.example%2F"
	got, n, err := safeMagnet(m, false)
	if err != nil || n != 9 || linkHash(got) != hashA {
		t.Fatalf("%d %v %s", n, err, got)
	}
	u, _ := url.Parse(got)
	for k := range u.Query() {
		if k != "xt" && k != "dn" && k != "tr" {
			t.Fatalf("kept %q: %s", k, got)
		}
	}
	if trs := u.Query()["tr"]; len(trs) != 2 || trs[0] != "udp://tracker.example:80" || u.Query().Get("dn") != "Some Name" {
		t.Fatalf("trackers %v in %s", trs, got)
	}
	got, n, err = safeMagnet(m, true)
	q, _ := url.Parse(got)
	if err != nil || n != 4 || q.Query().Get("xs") == "" || q.Query().Get("ws") == "" || len(q.Query()["tr"]) != 4 {
		t.Fatalf("local allowed: %d %v %s", n, err, got)
	}
	for _, bad := range []string{
		"magnet:?xt=urn:btih:" + hashA + "&dn=%zz",
		"magnet:?xt=urn:btih:" + hashA + "&dn=a%0Ab",
		"magnet:?dn=no-hash",
		"magnet:?xt=urn:btih:nothex",
		"https://x.example/?xt=urn:btih:" + hashA,
	} {
		if _, _, err := safeMagnet(bad, false); err == nil {
			t.Errorf("%q passed", bad)
		}
	}
	if got, n, err := safeMagnet("magnet:?xt=urn:btmh:1220"+strings.Repeat("ab", 32), false); err != nil || n != 0 || !strings.HasPrefix(got, "magnet:?xt=urn:btmh:1220") {
		t.Fatalf("v2: %s %d %v", got, n, err)
	}
}

func TestTrackerOddLiterals(t *testing.T) {
	for _, h := range []string{"2130706433", "0x7f.1", "127.1", "017700000001", "0x7f000001", "tracker%2eexample.com", "host.123", "a.b.0x10"} {
		if trackerAllowed("http://"+h+"/announce", false) {
			t.Errorf("%s passed without local access", h)
		}
	}
	for _, h := range []string{"tracker.example.com", "1e100.net", "t1.example.org"} {
		if !trackerAllowed("udp://"+h+":80", false) {
			t.Errorf("%s refused", h)
		}
	}
}

func TestPeersRefuseMetadataWithLocal(t *testing.T) {
	m := "magnet:?xt=urn:btih:" + hashA + "&ws=http%3A%2F%2F169.254.169.254%2F&xs=http%3A%2F%2Fmetadata.google.internal%2Fx&as=http%3A%2F%2Ffd00%3Aec2%3A%3A254%2F&x.pe=169.254.169.254%3A80&ws=http%3A%2F%2F192.168.1.5%2Fs&x.pe=192.168.1.2%3A6881"
	got, n, err := safeMagnet(m, true)
	if err != nil || n != 4 {
		t.Fatalf("%d %v %s", n, err, got)
	}
	if strings.Contains(got, "169.254") || strings.Contains(got, "metadata") || !strings.Contains(got, "192.168.1.5") || !strings.Contains(got, "192.168.1.2") {
		t.Fatalf("kept %s", got)
	}
}

func TestOddMetadataLiteralsWithLocal(t *testing.T) {
	for _, h := range []string{"2852039166", "0xa9fea9fe", "169.254.43518", "0251.0376.0251.0376", "169.254.169.254.", "999.1.1.1"} {
		if trackerAllowed("http://"+h+"/a", true) {
			t.Errorf("tracker %s passed with local access", h)
		}
		if peerAllowed("ws", "http://"+h+"/s") || peerAllowed("x.pe", h+":80") {
			t.Errorf("peer %s passed with local access", h)
		}
	}
	if trackerAllowed("http://[fe80::1%25eth0]/a", true) || trackerAllowed("http://[fe80::1%25eth0]/a", false) || peerAllowed("x.pe", "[fe80::1%eth0]:80") {
		t.Error("zoned link-local passed")
	}
	if !trackerAllowed("http://3232235777/a", true) || trackerAllowed("http://3232235777/a", false) {
		t.Error("decimal 192.168.1.1: kept only with local access")
	}
	if ip, _ := hostIP("127.1"); !ip.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Errorf("127.1 read as %v", ip)
	}
}

func (f *manageFake) start(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v2/")
		var form string
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			_ = r.ParseMultipartForm(1 << 20)
			parts := []string{}
			for k, v := range r.MultipartForm.Value {
				parts = append(parts, k+"="+strings.Join(v, ","))
			}
			for _, fh := range r.MultipartForm.File["torrents"] {
				fd, _ := fh.Open()
				b, _ := io.ReadAll(fd)
				fd.Close()
				parts = append(parts, "file="+fh.Filename+":"+string(b[:1]))
			}
			form = strings.Join(sorted(parts), "&")
		} else {
			_ = r.ParseForm()
			form = r.Form.Encode()
		}
		f.mu.Lock()
		f.calls = append(f.calls, path+"?"+form)
		code, body := f.codes[path], f.answers[path]
		f.mu.Unlock()
		if code != 0 {
			w.WriteHeader(code)
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func (f *manageFake) last(path string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		if strings.HasPrefix(f.calls[i], path+"?") {
			return f.calls[i]
		}
	}
	return ""
}

func (f *manageFake) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, path+"?") {
			n++
		}
	}
	return n
}

func sorted(s []string) []string {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	return s
}
