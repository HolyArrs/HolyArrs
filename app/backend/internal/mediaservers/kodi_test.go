package mediaservers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

type rpcCall struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
	user   string
	pass   string
}

// fakeKodi plays Kodi's JSON-RPC web server: basic auth, one POST /jsonrpc,
// HTTP 200 with an error object for a call it refuses.
func fakeKodi(t *testing.T, user, pass string, version int, refuse map[string]int) (*httptest.Server, func() []rpcCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []rpcCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if r.Method != http.MethodPost || r.URL.Path != "/jsonrpc" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if user != "" && (!ok || u != user || p != pass) {
			w.Header().Set("WWW-Authenticate", `Basic realm="XBMC"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		b, _ := io.ReadAll(r.Body)
		c := rpcCall{user: u, pass: p}
		_ = json.Unmarshal(b, &c)
		mu.Lock()
		calls = append(calls, c)
		mu.Unlock()
		if code, bad := refuse[c.Method]; bad {
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"error":{"code":`+itoa(code)+`,"message":"Invalid params."}}`)
			return
		}
		switch c.Method {
		case "Application.GetProperties":
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"name":"Kodi","version":{"major":`+itoa(version)+`,"minor":1,"revision":"x","tag":"stable"}}}`)
		default:
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":"OK"}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []rpcCall { mu.Lock(); defer mu.Unlock(); return append([]rpcCall{}, calls...) }
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// An import scans only the new folder, a delete that removed files cleans it,
// both as Kodi sees the path, with the login Kodi asked for.
func TestKodiScanAndClean(t *testing.T) {
	w := newWorld(t)
	srv, calls := fakeKodi(t, "kodi", "pw-1234", 21, nil)
	ctx := context.Background()
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Living room", Kind: "kodi", URL: srv.URL, Username: "kodi", Token: "pw-1234",
		PathFrom: w.lib, PathTo: "smb://nas/movies"}, domain.System); err != nil {
		t.Fatal(err)
	}
	l := &Linker{Store: w.s, Vault: w.v, Delay: time.Hour}
	l.Watch(ctx)
	w.emit(t, "import_done", nil)
	waitPending(t, l, w.folder(), Created)
	l.Flush(ctx)
	cs := calls()
	// Kodi reads the parent folder again first, then scans only the new folder.
	if len(cs) != 2 || cs[0].Method != "Files.GetDirectory" || cs[0].Params["directory"] != "smb://nas/movies/" ||
		cs[1].Method != "VideoLibrary.Scan" || cs[1].Params["directory"] != "smb://nas/movies/Arrival (2016)/" ||
		cs[1].Params["showdialogs"] != false || cs[1].user != "kodi" || cs[1].pass != "pw-1234" {
		t.Fatalf("scan calls %+v", cs)
	}
	// A delete that kept the files tells nobody.
	w.emit(t, domain.EventTitleRemoved, map[string]any{"delete_files": false})
	l.Flush(ctx)
	if len(calls()) != 2 {
		t.Fatal("told about a delete that kept the files")
	}
	w.emit(t, domain.EventTitleRemoved, map[string]any{"delete_files": true})
	waitPending(t, l, w.folder(), Deleted)
	l.Flush(ctx)
	cs = calls()
	// Clean names the content type: Kodi 20 cleans nothing for the default "video".
	if len(cs) != 3 || cs[2].Method != "VideoLibrary.Clean" || cs[2].Params["directory"] != "smb://nas/movies/Arrival (2016)/" ||
		cs[2].Params["content"] != "movies" || cs[2].Params["showdialogs"] != false {
		t.Fatalf("clean calls %+v", cs)
	}
	// Imports off: nothing.
	if _, err := Change(ctx, w.s, w.v, 1, Input{Name: "Living room", OnImport: &no, PathFrom: w.lib, PathTo: "smb://nas/movies"}, domain.System); err != nil {
		t.Fatal(err)
	}
	w.emit(t, "import_done", nil)
	waitPending(t, l, w.folder(), Created)
	l.Flush(ctx)
	if len(calls()) != 3 {
		t.Fatal("told about an import with imports off")
	}
}

// Music is scanned in Kodi's music library; a deleted music folder is left,
// since Kodi can clean that library only whole.
func TestKodiMusic(t *testing.T) {
	w := newWorld(t)
	srv, calls := fakeKodi(t, "", "", 21, nil)
	ctx := context.Background()
	m, err := Add(ctx, w.s, w.v, Input{Name: "Kodi", Kind: "kodi", URL: srv.URL}, domain.System)
	if err != nil || m.TokenSecretID != nil {
		t.Fatalf("a Kodi with no password: %+v %v", m, err)
	}
	l := &Linker{Store: w.s, Vault: w.v}
	if err := l.Tell(ctx, m, []Update{{Path: "/music/Artist/Album", Change: Created, Kind: "artist"}, {Path: "/music/Artist/Album", Change: Deleted, Kind: "artist"}}); err != nil {
		t.Fatal(err)
	}
	cs := calls()
	if len(cs) != 2 || cs[0].Method != "Files.GetDirectory" || cs[0].Params["directory"] != "/music/Artist/" ||
		cs[1].Method != "AudioLibrary.Scan" || cs[1].Params["directory"] != "/music/Artist/Album/" || cs[1].user != "" {
		t.Fatalf("music calls %+v", cs)
	}
}

func TestKodiTest(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	l := &Linker{Store: w.s, Vault: w.v}
	srv, _ := fakeKodi(t, "kodi", "pw-1234", 21, nil)
	m, err := Add(ctx, w.s, w.v, Input{Name: "ok", Kind: "kodi", URL: srv.URL, Username: "kodi", Token: "pw-1234"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := l.Test(ctx, m); err != nil || got != "Kodi 21.1" {
		t.Fatalf("test %q %v", got, err)
	}
	// A wrong password is a refused login, and the password stays out of the line.
	bad, err := Add(ctx, w.s, w.v, Input{Name: "bad", Kind: "kodi", URL: srv.URL, Username: "kodi", Token: "wrong-pw"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	_, err = l.Test(ctx, bad)
	if err == nil || strings.Contains(err.Error(), "wrong-pw") || strings.Contains(err.Error(), srv.URL) {
		t.Fatalf("wrong password: %v", err)
	}
	// Kodi 18 cannot clean one folder.
	old, _ := fakeKodi(t, "", "", 18, nil)
	om, _ := Add(ctx, w.s, w.v, Input{Name: "old", Kind: "kodi", URL: old.URL}, domain.System)
	if _, err := l.Test(ctx, om); err == nil {
		t.Fatal("Kodi 18 passed")
	}
	// Something else answering is not Kodi.
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "<html>hello</html>") }))
	t.Cleanup(other.Close)
	xm, _ := Add(ctx, w.s, w.v, Input{Name: "other", Kind: "kodi", URL: other.URL}, domain.System)
	if _, err := l.Test(ctx, xm); err == nil {
		t.Fatal("a web page passed as Kodi")
	}
}

// Kodi refuses a call with HTTP 200 and an error object: that is a failed update.
func TestKodiRefusedCall(t *testing.T) {
	w := newWorld(t)
	srv, _ := fakeKodi(t, "", "", 21, map[string]int{"VideoLibrary.Clean": -32602})
	ctx := context.Background()
	m, _ := Add(ctx, w.s, w.v, Input{Name: "Kodi", Kind: "kodi", URL: srv.URL}, domain.System)
	l := &Linker{Store: w.s, Vault: w.v}
	if err := l.Tell(ctx, m, []Update{{Path: "/movies/A", Change: Deleted}}); err == nil || !strings.Contains(err.Error(), "VideoLibrary.Clean") {
		t.Fatalf("refused clean: %v", err)
	}
	if err := l.Tell(ctx, m, []Update{{Path: "/movies/A", Change: Created}}); err != nil {
		t.Fatal(err)
	}
}

func TestKodiDirAndUsername(t *testing.T) {
	for in, want := range map[string]string{"smb://nas/m": "smb://nas/m/", "smb://nas/m/": "smb://nas/m/", `D:\Movies\A`: `D:\Movies\A\`, "/m/A": "/m/A/"} {
		if got := kodiDir(in); got != want {
			t.Errorf("kodiDir(%q) = %q, want %q", in, got, want)
		}
	}
	w := newWorld(t)
	m, err := Add(context.Background(), w.s, w.v, Input{Name: "K", Kind: "kodi", URL: "http://x:8080", Username: " kodi "}, domain.System)
	if err != nil || m.Username != "kodi" || ViewOf(m).Username != "kodi" || ViewOf(m).Token != "" {
		t.Fatalf("%+v %v", m, err)
	}
	// The user name belongs to Kodi only.
	p, _ := Add(context.Background(), w.s, w.v, Input{Name: "P", Kind: "plex", URL: "http://x:32400", Token: tok, Username: "x"}, domain.System)
	if p.Username != "" {
		t.Fatalf("plex kept a user name: %q", p.Username)
	}
}

// A show is cleaned under tvshows, and a batch reads each parent folder once.
func TestKodiShowCleanAndParentOnce(t *testing.T) {
	w := newWorld(t)
	srv, calls := fakeKodi(t, "", "", 21, nil)
	ctx := context.Background()
	m, _ := Add(ctx, w.s, w.v, Input{Name: "Kodi", Kind: "kodi", URL: srv.URL}, domain.System)
	l := &Linker{Store: w.s, Vault: w.v}
	err := l.Tell(ctx, m, []Update{
		{Path: "/tv/Show A", Change: Created, Kind: "show"},
		{Path: "/tv/Show B", Change: Modified, Kind: "show"},
		{Path: "/tv/Show C", Change: Deleted, Kind: "show"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range calls() {
		got = append(got, c.Method)
	}
	if strings.Join(got, " ") != "Files.GetDirectory VideoLibrary.Scan VideoLibrary.Scan VideoLibrary.Clean" {
		t.Fatalf("calls %v", got)
	}
	if c := calls()[3]; c.Params["content"] != "tvshows" || c.Params["directory"] != "/tv/Show C/" {
		t.Fatalf("clean %+v", c)
	}
}

// A failed refresh of the parent does not stop the scan.
func TestKodiRefreshFailureIsNotFatal(t *testing.T) {
	w := newWorld(t)
	srv, calls := fakeKodi(t, "", "", 21, map[string]int{"Files.GetDirectory": -32602})
	ctx := context.Background()
	m, _ := Add(ctx, w.s, w.v, Input{Name: "Kodi", Kind: "kodi", URL: srv.URL}, domain.System)
	l := &Linker{Store: w.s, Vault: w.v}
	if err := l.Tell(ctx, m, []Update{{Path: "/movies/A", Change: Created}}); err != nil {
		t.Fatal(err)
	}
	if cs := calls(); len(cs) != 2 || cs[1].Method != "VideoLibrary.Scan" {
		t.Fatalf("calls %+v", cs)
	}
}

func TestKodiParent(t *testing.T) {
	for in, want := range map[string]string{
		"smb://nas/movies/Arrival (2016)/": "smb://nas/movies/",
		"smb://nas/Arrival/":               "smb://nas/",
		"smb://nas/":                       "",
		"/Arrival/":                        "",
		"/m/Arrival/":                      "/m/",
		`D:\Movies\A\`:                     `D:\Movies\`,
		`D:\A\`:                            `D:\`,
		"nfs://10.0.0.5/export/media/A/":   "nfs://10.0.0.5/export/media/",
	} {
		if got := kodiParent(in); got != want {
			t.Errorf("kodiParent(%q) = %q, want %q", in, got, want)
		}
	}
}
