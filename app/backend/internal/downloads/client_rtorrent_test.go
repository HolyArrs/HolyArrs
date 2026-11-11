package downloads

import (
	"bytes"
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

const rtHash = "29be7bfa8759643f2d5dac9f94d516c3378696ed"

// fakeRTorrent is an rTorrent behind a web server's /RPC2, holding torrents in memory.
type fakeRTorrent struct {
	mu        sync.Mutex
	user      string
	pass      string
	version   string
	items     map[string]*rtItem // by upper-case hash
	loads     int
	calls     []string
	untrusted bool   // refuse write calls the way ruTorrent's plugins do
	dropLoad  bool   // answer a load with 0 but keep nothing
	tooLarge  bool   // refuse a load for its size
	hideFor   int    // lookups that miss a fresh item
	path      string // where it answers, /RPC2 when empty
}

type rtItem struct {
	name, label, message, dir string
	size, done                int64
	state, open, active       int64
	complete, hashing, multi  int64
	files                     [][]any
}

var rtCallParam = regexp.MustCompile(`(?s)<param>(.*?)</param>`)
var rtMethodName = regexp.MustCompile(`<methodName>([^<]*)</methodName>`)

func startRTorrent(t *testing.T, f *fakeRTorrent) string {
	t.Helper()
	if f.items == nil {
		f.items = map[string]*rtItem{}
	}
	if f.version == "" {
		f.version = "0.16.25"
	}
	if f.path == "" {
		f.path = "/RPC2"
	}
	s := httptest.NewServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(s.Close)
	return s.URL
}

func (f *fakeRTorrent) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != f.path {
		http.NotFound(w, r)
		return
	}
	if f.user != "" || f.pass != "" {
		u, p, ok := r.BasicAuth()
		if !ok || u != f.user || p != f.pass {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}
	body, _ := io.ReadAll(r.Body)
	m := rtMethodName.FindSubmatch(body)
	if m == nil {
		http.Error(w, "bad call", http.StatusBadRequest)
		return
	}
	var params []any
	for _, p := range rtCallParam.FindAllSubmatch(body, -1) {
		v, err := xmlrpcResponse([]byte("<methodResponse><params><param>" + string(p[1]) + "</param></params></methodResponse>"))
		if err != nil {
			http.Error(w, "bad param", http.StatusBadRequest)
			return
		}
		params = append(params, v)
	}
	f.mu.Lock()
	v, fault := f.dispatch(string(m[1]), params)
	f.mu.Unlock()
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0"?><methodResponse>`)
	if fault != nil {
		b.WriteString(`<fault>`)
		_ = xmlrpcValue(&b, map[string]any{"faultCode": fault.Code, "faultString": fault.Message})
		b.WriteString(`</fault>`)
	} else {
		b.WriteString(`<params><param>`)
		_ = xmlrpcValue(&b, v)
		b.WriteString(`</param></params>`)
	}
	b.WriteString(`</methodResponse>`)
	w.Header().Set("Content-Type", "text/xml")
	_, _ = w.Write(b.Bytes())
}

func rtNoHashFault() *xmlrpcFault {
	return &xmlrpcFault{Code: -500, Message: "invalid parameters: info-hash not found"}
}

// dispatch answers one call. The lock is held.
func (f *fakeRTorrent) dispatch(method string, params []any) (any, *xmlrpcFault) {
	f.calls = append(f.calls, method)
	if strings.HasPrefix(method, "execute") || method == "d.delete_tied" || strings.HasPrefix(method, "d.custom5") {
		return nil, &xmlrpcFault{Code: -1, Message: "test: forbidden call " + method}
	}
	write := strings.HasSuffix(method, ".set") || strings.HasPrefix(method, "load.") || method == "d.erase" || method == "d.stop" || method == "d.start"
	if f.untrusted && write {
		return nil, &xmlrpcFault{Code: -500, Message: fmt.Sprintf("Command \"%s\" is not allowed for untrusted connections", method)}
	}
	target := ""
	if len(params) > 0 {
		target, _ = params[0].(string)
	}
	if method == "system.multicall" {
		list, _ := params[0].([]any)
		out := []any{}
		for _, c := range list {
			mc, _ := c.(map[string]any)
			name, _ := mc["methodName"].(string)
			ps, _ := mc["params"].([]any)
			v, fault := f.dispatch(name, ps)
			if fault != nil {
				out = append(out, map[string]any{"faultCode": fault.Code, "faultString": fault.Message})
			} else {
				out = append(out, []any{v})
			}
		}
		return out, nil
	}
	switch method {
	case "system.client_version":
		return f.version, nil
	case "throttle.global_down.rate", "throttle.global_up.rate", "throttle.global_down.total", "throttle.global_up.total":
		return int64(7), nil
	case "throttle.global_down.max_rate", "throttle.global_up.max_rate":
		return int64(0), nil
	case "throttle.global_down.max_rate.set", "throttle.global_up.max_rate.set":
		return int64(0), nil
	case "d.multicall2":
		out := []any{}
		for h, it := range f.items {
			row := []any{}
			for _, field := range params[2:] {
				row = append(row, f.field(h, it, strings.TrimSuffix(field.(string), "=")))
			}
			out = append(out, row)
		}
		return out, nil
	case "load.raw_start", "load.start":
		f.loads++
		if f.tooLarge {
			return nil, &xmlrpcFault{Code: -509, Message: "XML-RPC request too large"}
		}
		if target != "" {
			return nil, &xmlrpcFault{Code: -500, Message: "invalid parameters: target must be empty"}
		}
		it := &rtItem{state: 1, open: 1, active: 1, dir: "/downloads/temp"}
		h := ""
		if method == "load.raw_start" {
			b, _ := params[1].([]byte)
			hash, err := torrentFileHash(b)
			if err != nil {
				return int64(0), nil
			}
			h = strings.ToUpper(hash)
			if m := regexp.MustCompile(`4:name(\d+):`).FindSubmatchIndex(b); m != nil {
				n, _ := strconv.Atoi(string(b[m[2]:m[3]]))
				it.name = string(b[m[1] : m[1]+n])
			}
			it.size, it.done = 1000, 0
			it.files = [][]any{{it.name, int64(1000), int64(0), int64(1)}}
		} else {
			link, _ := params[1].(string)
			if mh, ok := magnetHash(link); ok {
				h = strings.ToUpper(mh)
			}
			it.name = h + ".meta"
		}
		for _, c := range params[2:] {
			cmd, _ := c.(string)
			k, v, _ := strings.Cut(cmd, "=")
			switch k {
			case "d.custom1.set":
				it.label = v
			case "d.directory.set":
				it.dir = strings.Trim(v, `"`)
			}
		}
		if h != "" && !f.dropLoad {
			f.items[h] = it
		}
		return int64(0), nil
	}
	it, ok := f.items[target]
	if !ok {
		return nil, rtNoHashFault()
	}
	switch method {
	case "d.custom1.set":
		it.label, _ = params[1].(string)
		return int64(0), nil
	case "d.erase":
		delete(f.items, target)
		return int64(0), nil
	case "d.stop":
		it.state, it.active = 0, 0
		return int64(0), nil
	case "d.start":
		it.state, it.open, it.active = 1, 1, 1
		return int64(0), nil
	case "d.resume":
		it.active = 1
		return int64(0), nil
	case "f.multicall":
		out := []any{}
		for _, fl := range it.files {
			out = append(out, fl)
		}
		return out, nil
	}
	if f.hideFor > 0 {
		f.hideFor--
		return nil, rtNoHashFault()
	}
	return f.field(target, it, method), nil
}

func (f *fakeRTorrent) field(h string, it *rtItem, name string) any {
	switch name {
	case "d.hash":
		return h
	case "d.name":
		return it.name
	case "d.custom1":
		return it.label
	case "d.size_bytes":
		return it.size
	case "d.completed_bytes":
		return it.done
	case "d.state":
		return it.state
	case "d.is_open":
		return it.open
	case "d.is_active":
		return it.active
	case "d.complete":
		return it.complete
	case "d.hashing":
		return it.hashing
	case "d.message":
		return it.message
	case "d.base_path":
		if it.open == 0 {
			return ""
		}
		if it.multi == 1 {
			return it.dir
		}
		return it.dir + "/" + it.name
	case "d.directory":
		return it.dir
	case "d.is_multi_file":
		return it.multi
	case "d.ratio":
		return int64(1500)
	}
	return int64(0)
}

func (f *fakeRTorrent) called(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == method {
			n++
		}
	}
	return n
}

// rtTorrentFile is a one-file torrent for name holding payload.
func rtTorrentFile(name string, payload []byte) []byte {
	piece := sha1.Sum(payload)
	info := fmt.Sprintf("d6:lengthi%de4:name%d:%s12:piece lengthi16384e6:pieces20:%se", len(payload), len(name), name, piece[:])
	announce := "http://127.0.0.1:9/announce"
	return []byte(fmt.Sprintf("d8:announce%d:%s4:info%se", len(announce), announce, info))
}

func rtorrentHarness(t *testing.T) conformance {
	t.Helper()
	f := &fakeRTorrent{user: "lab", pass: "<rtorrent-test-password>"}
	base := startRTorrent(t, f)
	return conformance{
		name:    "rtorrent",
		engine:  NewRTorrent(base, f.user, f.pass, nil),
		badKey:  NewRTorrent(base, f.user, "wrong-password", nil),
		at:      func(base string) Engine { return NewRTorrent(base, f.user, f.pass, nil) },
		secret:  f.pass,
		release: AddRequest{URL: "magnet:?xt=urn:btih:" + rtHash + "&dn=Lab", Name: "Lab.Torrent.Movie.2024.1080p.WEB.H264-HOLYARRS", Category: "movies", InfoHash: rtHash},
		unknown: "0000000000000000000000000000000000000001",
		appear: func() {
			f.mu.Lock()
			it := f.items[strings.ToUpper(rtHash)]
			it.name, it.size, it.done = "Lab.Torrent.Movie.2024.1080p.WEB.H264-HOLYARRS.mkv", 1000, 500
			it.files = [][]any{{it.name, int64(1000), int64(1), int64(2)}}
			f.mu.Unlock()
		},
		adds: func() int { f.mu.Lock(); defer f.mu.Unlock(); return f.loads },
		states: func(word string) {
			f.mu.Lock()
			defer f.mu.Unlock()
			it := f.items[strings.ToUpper(rtHash)]
			it.state, it.open, it.active, it.complete, it.hashing, it.message, it.done = 1, 1, 1, 0, 0, "", 500
			switch word {
			case "checking":
				it.hashing = 1
			case "stopped":
				it.state, it.active = 0, 0
			case "paused":
				it.active = 0
			case "seeding":
				it.complete, it.done = 1, 1000
			case "complete":
				it.complete, it.done, it.state, it.active = 1, 1000, 0, 0
			case "error":
				it.open, it.active, it.message = 0, 0, "Storage error: [File chunk read error: No such file or directory]"
			}
		},
		words: map[string]string{
			"downloading": StateDownloading, "checking": StateQueued, "stopped": StatePaused, "paused": StatePaused,
			"seeding": StateFinished, "complete": StateFinished, "error": StatePaused,
		},
	}
}

func TestRTorrentConformance(t *testing.T) { runConformance(t, rtorrentHarness(t)) }

func TestRTorrentAddressGetsRPC2(t *testing.T) {
	for in, want := range map[string]string{
		"http://box:8000":                            "http://box:8000/RPC2",
		"http://box:8000/":                           "http://box:8000/RPC2",
		"https://seed.example/user/RPC2":             "https://seed.example/user/RPC2",
		"http://box:8080/plugins/httprpc/action.php": "http://box:8080/plugins/httprpc/action.php",
	} {
		if got := NewRTorrent(in, "", "", nil).endpoint; got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
}

func TestRTorrentFetchesTorrentAndSendsBytes(t *testing.T) {
	payload := rtTorrentFile("Lab.Movie.2024.mkv", []byte("x"))
	want, _ := torrentFileHash(payload)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(payload) }))
	defer source.Close()
	f := &fakeRTorrent{}
	e := NewRTorrent(startRTorrent(t, f), "", "", nil)
	id, err := e.Add(context.Background(), AddRequest{URL: source.URL + "/a.torrent", Category: "tv shows", SavePath: "/data/my downloads, new", AllowLocal: true})
	if err != nil || id != want {
		t.Fatalf("add: %q %v", id, err)
	}
	it := f.items[strings.ToUpper(want)]
	if it.label != "tv%20shows" || it.dir != "/data/my downloads, new" || f.called("load.start") != 0 {
		t.Errorf("sent label %q folder %q, load.start %d", it.label, it.dir, f.called("load.start"))
	}
	j, err := e.Get(context.Background(), want)
	if err != nil || j.Category != "tv shows" || len(j.Files) != 1 || j.Ratio != 1.5 {
		t.Errorf("get: %+v %v", j, err)
	}
	jobs, _ := e.List(context.Background(), "tv shows")
	if len(jobs) != 1 {
		t.Errorf("list by decoded label: %d", len(jobs))
	}
}

func TestRTorrentLabelWrittenPlainlyStillMatches(t *testing.T) {
	f := &fakeRTorrent{items: map[string]*rtItem{strings.ToUpper(rtHash): {name: "x", label: "tv-sonarr", open: 1, state: 1}}}
	e := NewRTorrent(startRTorrent(t, f), "", "", nil)
	jobs, err := e.List(context.Background(), "tv-sonarr")
	if err != nil || len(jobs) != 1 {
		t.Fatalf("list: %v %v", jobs, err)
	}
	other, _ := e.List(context.Background(), "movies")
	if len(other) != 0 {
		t.Errorf("another label listed %d", len(other))
	}
}

func TestRTorrentNoSaveFolderWithPathPairs(t *testing.T) {
	f := &fakeRTorrent{}
	e := NewRTorrent(startRTorrent(t, f), "", "", nil)
	e.PathMap = []domain.PathMap{{Theirs: "/home/u/downloads", Ours: "/data/seedbox"}}
	if _, err := e.Add(context.Background(), AddRequest{URL: "magnet:?xt=urn:btih:" + rtHash, Category: "movies", SavePath: "/data/downloads"}); err != nil {
		t.Fatal(err)
	}
	if it := f.items[strings.ToUpper(rtHash)]; it.dir != "/downloads/temp" {
		t.Errorf("a HolyArrs path went to a remote rTorrent: %q", it.dir)
	}
}

func TestRTorrentMagnetBase32(t *testing.T) {
	h, ok := magnetHash("magnet:?xt=urn:btih:FG7HX6UHLFSD6LK5VSPZJVIWYM3YNFXN")
	if !ok || h != rtHash {
		t.Errorf("base32 magnet: %q %v", h, ok)
	}
}

func TestRTorrentReconcileOnlySendsNothing(t *testing.T) {
	f := &fakeRTorrent{}
	e := NewRTorrent(startRTorrent(t, f), "", "", nil)
	if _, err := e.Add(context.Background(), AddRequest{InfoHash: rtHash, Category: "movies", ReconcileOnly: true}); !errors.Is(err, ErrUncertain) {
		t.Errorf("unknown torrent: %v", err)
	}
	if f.loads != 0 || f.called("d.custom1.set") != 0 {
		t.Errorf("a reconcile sent %d loads", f.loads)
	}
}

func TestRTorrentUnseenAddIsUncertain(t *testing.T) {
	old := rtorrentConfirm
	rtorrentConfirm.tries, rtorrentConfirm.every = 3, time.Millisecond
	t.Cleanup(func() { rtorrentConfirm = old })
	f := &fakeRTorrent{dropLoad: true}
	e := NewRTorrent(startRTorrent(t, f), "", "", nil)
	_, err := e.Add(context.Background(), AddRequest{URL: "magnet:?xt=urn:btih:" + rtHash, Category: "movies"})
	if !errors.Is(err, ErrUncertain) || f.loads != 1 {
		t.Errorf("got %v after %d loads, want uncertain after 1", err, f.loads)
	}
}

func TestRTorrentLateAddConfirmed(t *testing.T) {
	old := rtorrentConfirm
	rtorrentConfirm.every = time.Millisecond
	t.Cleanup(func() { rtorrentConfirm = old })
	f := &fakeRTorrent{}
	base := startRTorrent(t, f)
	e := NewRTorrent(base, "", "", nil)
	f.mu.Lock()
	f.hideFor = 1 + 2*len(rtorrentFields) // the lookup before the send, then two confirm rounds
	f.mu.Unlock()
	id, err := e.Add(context.Background(), AddRequest{URL: "magnet:?xt=urn:btih:" + rtHash, Category: "movies"})
	if err != nil || id != rtHash || f.loads != 1 {
		t.Errorf("late add: %q %v, %d loads", id, err, f.loads)
	}
}

func TestRTorrentForeignLabelNotTaken(t *testing.T) {
	f := &fakeRTorrent{items: map[string]*rtItem{strings.ToUpper(rtHash): {name: "x", label: "tv-sonarr", open: 1, state: 1}}}
	e := NewRTorrent(startRTorrent(t, f), "", "", nil)
	if _, err := e.Add(context.Background(), AddRequest{URL: "magnet:?xt=urn:btih:" + rtHash, Category: "movies"}); err == nil {
		t.Error("a torrent of another label was taken")
	}
	if f.items[strings.ToUpper(rtHash)].label != "tv-sonarr" || f.loads != 0 {
		t.Error("another app's torrent was relabelled or loaded again")
	}
	e.Category = "movies"
	if err := e.Remove(context.Background(), rtHash, true); err == nil {
		t.Error("another app's torrent was removed")
	}
	if err := e.PauseItem(context.Background(), rtHash); !errors.Is(err, ErrNotFound) {
		t.Errorf("pause of another app's torrent: %v", err)
	}
}

func TestRTorrentUntrustedCallsNamed(t *testing.T) {
	f := &fakeRTorrent{untrusted: true}
	_, err := NewRTorrent(startRTorrent(t, f), "", "", nil).Health(context.Background())
	if err == nil || !strings.Contains(err.Error(), "untrusted") || !strings.Contains(err.Error(), "/RPC2") {
		t.Errorf("health: %v", err)
	}
}

func TestRTorrentHealthHints(t *testing.T) {
	f := &fakeRTorrent{path: "/RPC2"}
	base := startRTorrent(t, f)
	if _, err := NewRTorrent(base+"/rutorrent", "", "", nil).Health(context.Background()); err == nil || !strings.Contains(err.Error(), "port 8000") {
		t.Errorf("404: %v", err)
	}
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("<html>ruTorrent</html>")) }))
	defer page.Close()
	if _, err := NewRTorrent(page.URL, "", "", nil).Health(context.Background()); err == nil || !strings.Contains(err.Error(), "check the path") {
		t.Errorf("web page: %v", err)
	}
	f.version = "0.8.9"
	if _, err := NewRTorrent(base, "", "", nil).Health(context.Background()); err == nil || !strings.Contains(err.Error(), "too old") {
		t.Errorf("old: %v", err)
	}
}

func TestRTorrentStates(t *testing.T) {
	for _, c := range []struct {
		row    rtRow
		state  string
		detail string
	}{
		{rtRow{hashing: 1, state: 1, open: 1}, StateQueued, "checking"},
		{rtRow{name: "ABC.meta", state: 1, open: 1, active: 1}, StateDownloading, "metadata"},
		{rtRow{size: 10, state: 1, open: 0, message: "Storage error: http://t.example/a?passkey=secret"}, StatePaused, "error: Storage error: http://t.example"},
		{rtRow{size: 10, state: 0, open: 0, message: "Storage error: old"}, StatePaused, "stopped"},
		{rtRow{size: 10, complete: 1, state: 1, open: 1, active: 1, basePath: "/d/x", name: "x"}, StateFinished, "seeding"},
		{rtRow{size: 10, complete: 1, state: 0, open: 1, basePath: "/d/x", name: "x"}, StateFinished, "complete"},
		{rtRow{size: 10, state: 1, open: 1, active: 0}, StatePaused, "paused"},
		{rtRow{size: 10, state: 1, open: 1, active: 1, message: "Tracker: [Timeout was reached]"}, StateDownloading, "downloading"},
	} {
		state, detail := rtState(c.row, c.row.content())
		if state != c.state || !strings.HasPrefix(detail, c.detail) || strings.Contains(detail, "secret") {
			t.Errorf("%+v: %s %q, want %s %q", c.row, state, detail, c.state, c.detail)
		}
	}
	if !cantUseFiles(Job{State: StatePaused, Detail: "error: Storage error"}) {
		t.Error("an rTorrent storage error does not wait for a late mount")
	}
}

func TestRTorrentContentPath(t *testing.T) {
	for _, c := range []struct {
		row  rtRow
		want string
	}{
		{rtRow{basePath: "/d/x.mkv"}, "/d/x.mkv"},
		{rtRow{dir: "/d", name: "x.mkv"}, "/d/x.mkv"},
		{rtRow{dir: "/d/Show.S01", name: "Show.S01", multi: 1}, "/d/Show.S01"},
		{rtRow{dir: "relative", name: "x"}, ""},
		{rtRow{}, ""},
	} {
		if got := c.row.content(); got != c.want {
			t.Errorf("%+v: %q, want %q", c.row, got, c.want)
		}
	}
}

func TestRTorrentRemoveDeletesOnlySafeFiles(t *testing.T) {
	here := t.TempDir()
	put := func(rel string) string {
		p := filepath.Join(here, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("x"), 0o644)
		return p
	}
	type item struct{ hash, name, dir string }
	for _, c := range []struct {
		name    string
		items   []item
		pathMap []domain.PathMap
		gone    bool
	}{
		{"inside a path pair", []item{{rtHash, "a.mkv", "/remote/movies"}}, []domain.PathMap{{Theirs: "/remote", Ours: here}}, true},
		{"no path pair reaches it", []item{{rtHash, "a.mkv", "/elsewhere/movies"}}, []domain.PathMap{{Theirs: "/remote", Ours: here}}, false},
		{"no pairs at all", []item{{rtHash, "a.mkv", "/remote/movies"}}, nil, false},
		{"another torrent uses it", []item{{rtHash, "a.mkv", "/remote/movies"}, {"1111111111111111111111111111111111111111", "a.mkv", "/remote/movies"}}, []domain.PathMap{{Theirs: "/remote", Ours: here}}, false},
		{"not visible", []item{{rtHash, "missing.mkv", "/remote/movies"}}, []domain.PathMap{{Theirs: "/remote", Ours: here}}, false},
		{"names a Library folder", []item{{rtHash, "library", "/remote"}}, []domain.PathMap{{Theirs: "/remote", Ours: here}}, false},
		{"holds a Library folder", []item{{rtHash, "movies", "/remote"}}, []domain.PathMap{{Theirs: "/remote", Ours: here}}, false},
		{"a name with a backslash", []item{{rtHash, `movies.mkv`, "/remote"}}, []domain.PathMap{{Theirs: "/remote", Ours: here}}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			file := put("movies/a.mkv")
			kept := put("library/Film (2024)/Film.mkv")
			if c.name == "holds a Library folder" {
				kept = put("movies/Film (2024)/Film.mkv")
			}
			f := &fakeRTorrent{items: map[string]*rtItem{}}
			for _, it := range c.items {
				f.items[strings.ToUpper(it.hash)] = &rtItem{name: it.name, label: "movies", dir: it.dir, open: 1, state: 1, complete: 1, size: 1, done: 1}
			}
			e := NewRTorrent(startRTorrent(t, f), "", "", nil)
			e.Category, e.PathMap = "movies", c.pathMap
			e.Keep = func(context.Context) ([]string, error) {
				return []string{filepath.Join(here, "library"), filepath.Join(here, "movies", "Film (2024)")}, nil
			}
			var logged []string
			old := logf
			logf = func(format string, a ...any) { logged = append(logged, fmt.Sprintf(format, a...)) }
			t.Cleanup(func() { logf = old })
			if err := e.Remove(context.Background(), rtHash, true); err != nil {
				t.Fatalf("remove: %v", err)
			}
			if _, ok := f.items[strings.ToUpper(rtHash)]; ok {
				t.Error("the torrent is still in rTorrent")
			}
			_, statErr := os.Stat(file)
			if gone := errors.Is(statErr, os.ErrNotExist); gone != c.gone {
				t.Errorf("files gone %v, want %v (log %v)", gone, c.gone, logged)
			}
			if !c.gone && len(logged) == 0 {
				t.Error("kept files were not logged")
			}
			if _, err := os.Stat(filepath.Join(here, "movies")); err != nil {
				t.Error("the label folder went too")
			}
			if _, err := os.Stat(kept); err != nil {
				t.Errorf("a Library file went: %v", err)
			}
			_ = os.RemoveAll(filepath.Join(here, "movies", "Film (2024)"))
		})
	}
}

func TestRTorrentRemoveDeletesNothingWithoutLibraryFolders(t *testing.T) {
	here := t.TempDir()
	file := filepath.Join(here, "a.mkv")
	_ = os.WriteFile(file, []byte("x"), 0o644)
	f := &fakeRTorrent{items: map[string]*rtItem{strings.ToUpper(rtHash): {name: "a.mkv", label: "movies", dir: "/remote", open: 1, state: 1, size: 1}}}
	e := NewRTorrent(startRTorrent(t, f), "", "", nil)
	e.PathMap = []domain.PathMap{{Theirs: "/remote", Ours: here}}
	old := logf
	logf = func(string, ...any) {}
	t.Cleanup(func() { logf = old })
	for _, keep := range []func(context.Context) ([]string, error){nil, func(context.Context) ([]string, error) { return nil, errors.New("locked") }} {
		f.items[strings.ToUpper(rtHash)] = &rtItem{name: "a.mkv", label: "movies", dir: "/remote", open: 1, state: 1, size: 1}
		e.Keep = keep
		if err := e.Remove(context.Background(), rtHash, true); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(file); err != nil {
			t.Errorf("deleted without knowing the Library folders: %v", err)
		}
	}
}

func TestRTorrentRemoveUnknownAndKeep(t *testing.T) {
	f := &fakeRTorrent{items: map[string]*rtItem{strings.ToUpper(rtHash): {name: "x", label: "movies", open: 1, state: 1}}}
	e := NewRTorrent(startRTorrent(t, f), "", "", nil)
	if err := e.Remove(context.Background(), "1111111111111111111111111111111111111111", false); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
	if err := e.Remove(context.Background(), "../etc", false); err == nil {
		t.Error("a bad id was sent")
	}
	if err := e.Remove(context.Background(), rtHash, false); err != nil || len(f.items) != 0 {
		t.Errorf("remove: %v", err)
	}
}

func TestRTorrentPauseResumeAndStatus(t *testing.T) {
	f := &fakeRTorrent{items: map[string]*rtItem{
		strings.ToUpper(rtHash):                    {name: "ours", label: "movies", open: 1, state: 1, active: 1, size: 10},
		"2222222222222222222222222222222222222222": {name: "theirs", label: "tv-sonarr", open: 1, state: 1, active: 1, size: 10},
	}}
	e := NewRTorrent(startRTorrent(t, f), "", "", nil)
	e.Category = "movies"
	ctx := context.Background()
	if err := e.PauseItem(ctx, rtHash); err != nil {
		t.Fatal(err)
	}
	if j, _ := e.Get(ctx, rtHash); j.State != StatePaused {
		t.Errorf("paused: %s", j.State)
	}
	if st, err := e.Status(ctx); err != nil || !st.Paused {
		t.Errorf("status: %+v %v", st, err)
	}
	if err := e.ResumeItem(ctx, rtHash); err != nil {
		t.Fatal(err)
	}
	if err := e.PauseAll(ctx); err != nil {
		t.Fatal(err)
	}
	if f.items["2222222222222222222222222222222222222222"].state != 1 {
		t.Error("pause all stopped another app's torrent")
	}
	if err := e.ResumeAll(ctx); err != nil || f.items[strings.ToUpper(rtHash)].active != 1 {
		t.Errorf("resume all: %v", err)
	}
	if err := e.MoveTop(ctx, rtHash); !errors.Is(err, ErrNotSupported) {
		t.Errorf("move top: %v", err)
	}
	var _ Controller = e
}

func TestRTorrentSizeRefusalIsFinal(t *testing.T) {
	f := &fakeRTorrent{tooLarge: true}
	e := NewRTorrent(startRTorrent(t, f), "", "", nil)
	_, err := e.Add(context.Background(), AddRequest{URL: "magnet:?xt=urn:btih:" + rtHash, Category: "movies"})
	if err == nil || errors.Is(err, ErrUncertain) || !errors.Is(err, ErrTooLarge) || !strings.Contains(err.Error(), "size_limit") || f.loads != 1 {
		t.Errorf("got %v after %d loads, want a final size refusal", err, f.loads)
	}
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusRequestEntityTooLarge) }))
	defer big.Close()
	_, err = NewRTorrent(big.URL, "", "", nil).Add(context.Background(), AddRequest{URL: "magnet:?xt=urn:btih:" + rtHash, Category: "movies"})
	if err == nil || errors.Is(err, ErrUncertain) || !errors.Is(err, ErrTooLarge) {
		t.Errorf("413: %v", err)
	}
}

func TestXMLRPCOddNameKeepsTheList(t *testing.T) {
	body := "<methodResponse><params><param><value><array><data>" +
		"<value><array><data><value><string>badnameÿ&#1;</string></value></data></array></value>" +
		"<value><array><data><value><string>good</string></value></data></array></value>" +
		"</data></array></value></param></params></methodResponse>"
	v, err := xmlrpcResponse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	rows := v.([]any)
	if len(rows) != 2 || rows[1].([]any)[0] != "good" || !strings.HasPrefix(rows[0].([]any)[0].(string), "bad") {
		t.Errorf("rows: %#v", rows)
	}
}

func TestRTorrentMultiFilePaths(t *testing.T) {
	f := &fakeRTorrent{items: map[string]*rtItem{strings.ToUpper(rtHash): {name: "Show.S01", label: "tv", dir: "/downloads/Show.S01", multi: 1, open: 1, state: 1, size: 10,
		files: [][]any{{"Show.S01E01.mkv", int64(5), int64(1), int64(1)}, {"Subs/en.srt", int64(5), int64(0), int64(1)}}}}}
	j, err := NewRTorrent(startRTorrent(t, f), "", "", nil).Get(context.Background(), rtHash)
	if err != nil {
		t.Fatal(err)
	}
	if j.SavePath != "/downloads" || j.ContentPath != "/downloads/Show.S01" || len(j.Files) != 2 || j.Files[0].Name != "Show.S01/Show.S01E01.mkv" || j.Files[1].Name != "Show.S01/Subs/en.srt" {
		t.Errorf("paths: save %q content %q files %+v", j.SavePath, j.ContentPath, j.Files)
	}
}
