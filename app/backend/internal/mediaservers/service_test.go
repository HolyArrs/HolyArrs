package mediaservers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

var yes, no = true, false

// tok is the fake servers' token (short, so it reads as a test value).
const tok = "tk-1234"

type call struct {
	method, path, query, token string
	body                       string
}

// fakeServer plays Plex (kind plex) or Jellyfin and Emby: it checks the
// token header and records every call.
func fakeServer(t *testing.T, kind string, libRoot string) (*httptest.Server, func() []call) {
	var mu sync.Mutex
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got := r.Header.Get("X-Plex-Token")
		if kind == "jellyfin" {
			if r.Header.Get("Authorization") == fmt.Sprintf(`MediaBrowser Client="HolyArrs", Device="HolyArrs", DeviceId="holyarrs", Version="1", Token=%q`, tok) &&
				r.Header.Get("X-MediaBrowser-Token") == "" && r.Header.Get("X-Emby-Token") == "" {
				got = tok
			}
		} else if kind == "emby" {
			got = r.Header.Get("X-MediaBrowser-Token")
		}
		mu.Lock()
		calls = append(calls, call{r.Method, r.URL.Path, r.URL.RawQuery, got, string(b)})
		mu.Unlock()
		if got != tok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case kind == "plex" && r.URL.Path == "/library/sections":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, `<MediaContainer size="2"><Directory key="1" type="movie" title="Movies"><Location id="1" path="`+libRoot+`"/></Directory>`+
				`<Directory key="2" type="show" title="TV"><Location id="2" path="/tv"/></Directory></MediaContainer>`)
		case kind == "plex" && strings.HasPrefix(r.URL.Path, "/library/sections/"):
			w.WriteHeader(http.StatusOK)
		case kind != "plex" && r.URL.Path == "/System/Info":
			_, _ = io.WriteString(w, `{"ServerName":"Lab `+kind+`","Version":"10.9.0"}`)
		case kind != "plex" && r.URL.Path == "/Library/Media/Updated" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusNoContent)
		case kind == "jellyfin" && r.URL.Path == "/Library/Refresh" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []call { mu.Lock(); defer mu.Unlock(); return append([]call{}, calls...) }
}

type world struct {
	s     *database.Store
	v     *secrets.Vault
	lib   string
	title int64
}

func newWorld(t *testing.T) *world {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	w := &world{s: s, v: v, lib: filepath.Join(t.TempDir(), "movies")}
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		_ = tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr)
		lf := &mediafiles.LibraryFolder{Path: w.lib, Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
		if _, err := database.Insert(tx, "library_folders", lf, domain.System); err != nil {
			return err
		}
		ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "Arrival", SortName: "arrival", MetadataSource: "local", NewPartsPolicy: "all",
			ReleaseGate: "released", WhyAdded: "by_hand", Status: "released", State: "active", SourceIDs: map[string]string{}, Aliases: []string{}, MusicTypes: []string{}, Artwork: []string{}}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		w.title = ti.ID
		_, err := database.Insert(tx, "versions", &mediafiles.Version{TitleID: ti.ID, Label: "HD", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Arrival (2016)", IsMain: true}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func (w *world) folder() string { return filepath.Join(w.lib, "Arrival (2016)") }

func (w *world) emit(t *testing.T, kind string, data map[string]any) {
	t.Helper()
	if err := w.s.AddEvent(context.Background(), &domain.Event{Kind: kind, TitleID: &w.title, Summary: kind, Data: data}); err != nil {
		t.Fatal(err)
	}
}

// Plex scans only the movie folder, in the movie library that holds it.
func TestPlexPathScopedRefresh(t *testing.T) {
	w := newWorld(t)
	srv, calls := fakeServer(t, "plex", w.lib)
	ctx := context.Background()
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: srv.URL + "/", Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	l := &Linker{Store: w.s, Vault: w.v, Delay: time.Hour}
	l.Watch(ctx)
	w.emit(t, "import_done", nil)
	w.emit(t, domain.EventRenamed, nil) // the same folder: one call
	waitPending(t, l, w.folder(), Modified)
	l.Flush(ctx)
	cs := calls()
	if len(cs) != 2 || cs[0].path != "/library/sections" || cs[1].path != "/library/sections/1/refresh" || cs[1].token != tok {
		t.Fatalf("calls %+v", cs)
	}
	if !strings.Contains(cs[1].query, "path=") || !strings.Contains(cs[1].query, "Arrival") {
		t.Fatalf("not path-scoped: %s", cs[1].query)
	}
	es, _, _ := w.s.Events(ctx, database.EventFilter{Kinds: []string{domain.EventMediaRefreshed}})
	if len(es) != 1 || es[0].Data["change"] != Modified {
		t.Fatalf("events %+v", es)
	}
	// A folder outside every movie library scans the movie libraries whole.
	if err := l.Tell(ctx, mustGet(t, w, 1), []Update{{Path: "/elsewhere/Heat (1995)", Change: Created}}); err != nil {
		t.Fatal(err)
	}
	if last := calls()[len(calls())-1]; last.path != "/library/sections/1/refresh" || last.query != "" {
		t.Fatalf("whole-library scan %+v", last)
	}
}

func mustGet(t *testing.T, w *world, id int64) domain.MediaServer {
	var m domain.MediaServer
	if err := database.Get(context.Background(), w.s.Read(), "media_servers", id, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// Jellyfin and Emby get one media-updated call with the folder and change,
// mapped to the server's own path. Jellyfin 12.2 also needs one full refresh
// for the whole coalesced batch before new folders reliably appear.
func TestJellyfinAndEmbyMediaUpdated(t *testing.T) {
	for _, kind := range []string{"jellyfin", "emby"} {
		t.Run(kind, func(t *testing.T) {
			w := newWorld(t)
			srv, calls := fakeServer(t, kind, "")
			ctx := context.Background()
			if _, err := Add(ctx, w.s, w.v, Input{Name: kind, Kind: kind, URL: srv.URL, Token: tok, PathFrom: w.lib, PathTo: "/data/movies"}, domain.System); err != nil {
				t.Fatal(err)
			}
			l := &Linker{Store: w.s, Vault: w.v, Delay: time.Hour}
			if err := l.Tell(ctx, mustGet(t, w, 1), nil); err != nil {
				t.Fatal(err)
			}
			if len(calls()) != 0 {
				t.Fatal("empty update batch called the server")
			}
			l.Watch(ctx)
			w.emit(t, "import_done", nil)
			waitPending(t, l, w.folder(), Created)
			l.Flush(ctx)
			cs := calls()
			wantCalls := 1
			if kind == "jellyfin" {
				wantCalls = 2
			}
			if len(cs) != wantCalls || cs[0].method != "POST" || cs[0].path != "/Library/Media/Updated" || cs[0].token != tok {
				t.Fatalf("calls %+v", cs)
			}
			if kind == "jellyfin" && (cs[1].method != "POST" || cs[1].path != "/Library/Refresh" || cs[1].body != "") {
				t.Fatalf("Jellyfin batch refresh %+v", cs[1])
			}
			var body struct {
				Updates []struct{ Path, UpdateType string }
			}
			_ = json.Unmarshal([]byte(cs[0].body), &body)
			if len(body.Updates) != 1 || body.Updates[0].Path != "/data/movies/Arrival (2016)" || body.Updates[0].UpdateType != Created {
				t.Fatalf("body %s", cs[0].body)
			}
			// A delete that kept the files tells nobody; one that deleted them does.
			w.emit(t, domain.EventTitleRemoved, map[string]any{"delete_files": false})
			l.Flush(ctx)
			if len(calls()) != wantCalls {
				t.Fatal("told about a delete that kept the files")
			}
			w.emit(t, domain.EventTitleRemoved, map[string]any{"delete_files": true})
			waitPending(t, l, w.folder(), Deleted)
			l.Flush(ctx)
			if cs := calls(); len(cs) != 2*wantCalls || !strings.Contains(cs[wantCalls].body, `"Deleted"`) {
				t.Fatalf("delete calls %+v", cs)
			}
			// Switched off for imports: nothing.
			if _, err := Change(ctx, w.s, w.v, 1, Input{Name: kind, OnImport: &no, PathFrom: w.lib, PathTo: "/data/movies"}, domain.System); err != nil {
				t.Fatal(err)
			}
			w.emit(t, "import_done", nil)
			waitPending(t, l, w.folder(), Created)
			l.Flush(ctx)
			if len(calls()) != 2*wantCalls {
				t.Fatal("told about an import with imports off")
			}
		})
	}
}

func TestJellyfinBatchRefreshError(t *testing.T) {
	w := newWorld(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != `MediaBrowser Client="HolyArrs", Device="HolyArrs", DeviceId="holyarrs", Version="1", Token="`+tok+`"` {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/Library/Media/Updated" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	m, err := Add(context.Background(), w.s, w.v, Input{Name: "Jellyfin", Kind: "jellyfin", URL: srv.URL, Token: tok}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	err = (&Linker{Store: w.s, Vault: w.v}).Tell(context.Background(), m, []Update{{Path: w.folder(), Change: Created}})
	if err == nil || err.Error() != "the server answered 500" {
		t.Fatalf("refresh error = %v", err)
	}
}

// The test button: the right answer per kind, a wrong token is said plainly,
// and the token never reaches the log, the result or an Event.
func TestTestButtonAndTokenNeverLogged(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	w := newWorld(t)
	ctx := context.Background()
	l := &Linker{Store: w.s, Vault: w.v, Delay: 5 * time.Millisecond}
	for _, kind := range []string{"plex", "jellyfin", "emby"} {
		srv, _ := fakeServer(t, kind, w.lib)
		m, err := Add(ctx, w.s, w.v, Input{Name: kind, Kind: kind, URL: srv.URL, Token: tok}, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		res, err := l.Test(ctx, m)
		if err != nil || res == "" {
			t.Fatalf("%s test: %q %v", kind, res, err)
		}
		bad, _ := Add(ctx, w.s, w.v, Input{Name: kind + " wrong", Kind: kind, URL: srv.URL, Token: "wrong-1"}, domain.System)
		if _, err := l.Test(ctx, bad); err == nil || err.Error() != "the server turned down the token" {
			t.Fatalf("%s wrong token: %v", kind, err)
		}
	}
	// A server that is down: the failure is recorded without the token.
	srv, _ := fakeServer(t, "jellyfin", "")
	m, _ := Add(ctx, w.s, w.v, Input{Name: "gone", Kind: "jellyfin", URL: srv.URL, Token: tok}, domain.System)
	srv.Close()
	l.Queue(ctx, w.folder(), Created)
	l.Wait()
	m = mustGet(t, w, m.ID)
	es, _, _ := w.s.Events(ctx, database.EventFilter{Kinds: []string{"media_server_*"}})
	evs, _ := json.Marshal(es)
	for _, where := range []string{logs.String(), m.LastError, string(evs)} {
		if strings.Contains(where, tok) || strings.Contains(where, "wrong-1") {
			t.Fatalf("token leaked: %s", where)
		}
	}
	if m.LastError != "the server did not answer" {
		t.Fatalf("last error %q", m.LastError)
	}
}

// Current Jellyfin releases authenticate server API requests through the
// MediaBrowser Authorization header and reject the older token headers.
func TestJellyfinUsesMediaBrowserAuthorization(t *testing.T) {
	w := newWorld(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "MediaBrowser ") || !strings.Contains(auth, `Token="`+tok+`"`) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-MediaBrowser-Token") != "" || r.Header.Get("X-Emby-Token") != "" {
			t.Error("Jellyfin request included a legacy token header")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.URL.Path != "/System/Info" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `{"ServerName":"Live-shape Jellyfin","Version":"12.2.0"}`)
	}))
	t.Cleanup(srv.Close)

	m, err := Add(context.Background(), w.s, w.v, Input{Name: "Jellyfin 12", Kind: "jellyfin", URL: srv.URL, Token: tok}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (&Linker{Store: w.s, Vault: w.v}).Test(context.Background(), m)
	if err != nil || got != "Live-shape Jellyfin 12.2.0" {
		t.Fatalf("test result %q: %v", got, err)
	}
}

func TestJellyfinAuthorizationQuotesToken(t *testing.T) {
	w := newWorld(t)
	const awkward = "quote\"line\nslash\\end"
	want := fmt.Sprintf(`MediaBrowser Client="HolyArrs", Device="HolyArrs", DeviceId="holyarrs", Version="1", Token=%q`, awkward)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"ServerName":"Quoted","Version":"12.2.0"}`)
	}))
	t.Cleanup(srv.Close)

	m, err := Add(context.Background(), w.s, w.v, Input{Name: "Jellyfin quoted", Kind: "jellyfin", URL: srv.URL, Token: awkward}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (&Linker{Store: w.s, Vault: w.v}).Test(context.Background(), m)
	if err != nil || got != "Quoted 12.2.0" {
		t.Fatalf("test result %q: %v", got, err)
	}
}

func TestInputChecksAndTokenAgain(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	for _, in := range []Input{
		{Name: "", Kind: "plex", URL: "http://x:32400", Token: tok},
		{Name: "a", Kind: "kodi", URL: "http://x", Token: tok},
		{Name: "a", Kind: "plex", URL: "x:32400", Token: tok},
		{Name: "a", Kind: "plex", URL: "http://u:p@x:32400", Token: tok},
		{Name: "a", Kind: "plex", URL: "http://x:32400"},
		{Name: "a", Kind: "plex", URL: "http://x:32400", Token: tok, PathFrom: "/m"},
	} {
		if _, err := Add(ctx, w.s, w.v, in, domain.System); !IsInputError(err) {
			t.Errorf("%+v: %v", in, err)
		}
	}
	m, err := Add(ctx, w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: "http://192.168.1.10:32400", Token: tok}, domain.System)
	if err != nil || !m.AllowLocal || !m.OnImport || !m.OnRename || !m.OnDelete || !m.Enabled {
		t.Fatalf("defaults %+v %v", m, err)
	}
	if _, err := Add(ctx, w.s, w.v, Input{Name: "plex", Kind: "plex", URL: "http://x", Token: tok}, domain.System); err != ErrNameUsed {
		t.Fatalf("same name: %v", err)
	}
	if _, err := Change(ctx, w.s, w.v, m.ID, Input{Name: "Plex", URL: "http://10.0.0.9:32400"}, domain.System); err != ErrKeyAgain {
		t.Fatalf("token kept for another host: %v", err)
	}
	if _, err := Change(ctx, w.s, w.v, m.ID, Input{Name: "Plex", URL: "http://10.0.0.9:32400", Token: "tk-5678"}, domain.System); err != nil {
		t.Fatal(err)
	}
	// The scheme counts too; a token never moves to plain
	// http without being typed again.
	if _, err := Change(ctx, w.s, w.v, m.ID, Input{Name: "Plex", URL: "https://10.0.0.9:32400", Token: "tk-5678"}, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := Change(ctx, w.s, w.v, m.ID, Input{Name: "Plex", URL: "http://10.0.0.9:32400"}, domain.System); err != ErrKeyAgain {
		t.Fatalf("token kept on a downgrade to http: %v", err)
	}
	if MapPath(domain.MediaServer{PathFrom: `D:\Movies`, PathTo: "/movies"}, `D:\Movies\Heat (1995)`) != "/movies/Heat (1995)" ||
		MapPath(domain.MediaServer{PathFrom: "/data", PathTo: `M:\`}, "/data/Heat") != `M:\Heat` ||
		MapPath(domain.MediaServer{PathFrom: "/data", PathTo: "/x"}, "/database/Heat") != "/database/Heat" {
		t.Fatal("path map")
	}
	if err := Remove(ctx, w.s, w.v, m.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM secrets WHERE kind = 'media_server_token'").Scan(&n)
	if n != 0 {
		t.Fatalf("%d tokens left after remove", n)
	}
}

// waitPending waits until the Linker has gathered change for folder, so a
// test flushes only after every Event it sent was queued.
func waitPending(t *testing.T, l *Linker, folder, change string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		l.mu.Lock()
		got, ok := l.pending[folder]
		l.mu.Unlock()
		if ok && got == change {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pending %q for %s, want %q", got, folder, change)
		}
		time.Sleep(time.Millisecond)
	}
}

// 5.2 review: a show folder maps like a movie folder, and a Windows path
// map matches whatever case the drive and folders were typed in.
func TestMapPathShowFolder(t *testing.T) {
	for _, c := range []struct{ from, to, p, want string }{
		{"/tv", "/media/tv", "/tv/Copper Valley (2025)", "/media/tv/Copper Valley (2025)"},
		{"/tv/", "/media/tv/", "/tv/Copper Valley (2025)", "/media/tv/Copper Valley (2025)"},
		{`d:\tv`, "/tv", `D:\TV\Copper Valley (2025)`, "/tv/Copper Valley (2025)"},
		{`\\nas\TV`, "/tv", `\\NAS\tv\Copper Valley (2025)`, "/tv/Copper Valley (2025)"},
		{"/tv", "/media/tv", "/TV/Copper Valley (2025)", "/TV/Copper Valley (2025)"}, // Linux paths keep their case
	} {
		if got := MapPath(domain.MediaServer{PathFrom: c.from, PathTo: c.to}, c.p); got != c.want {
			t.Errorf("MapPath(%q -> %q, %q) = %q, want %q", c.from, c.to, c.p, got, c.want)
		}
	}
}
