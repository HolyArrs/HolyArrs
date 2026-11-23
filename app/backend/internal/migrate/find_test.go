package migrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// hostPort splits a test server's address.
func hostPort(t *testing.T, u string) (string, int) {
	t.Helper()
	h, p, err := net.SplitHostPort(strings.TrimPrefix(u, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	n, _ := strconv.Atoi(p)
	return h, n
}

func page(body string, routes map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if b, ok := routes[r.URL.Path]; ok {
			_, _ = w.Write([]byte(b))
			return
		}
		if r.URL.Path == "/" && body != "" {
			_, _ = w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	}))
}

// A search names each app by the calls it answers without a key, skips a
// closed port, an app on the wrong port and a silent host, and sends no key.
func TestFindNamesAppsWithoutAKey(t *testing.T) {
	sonarr := page("", map[string]string{"/ping": `{"status":"OK"}`})
	seerr := page("", map[string]string{"/api/v1/status": `{"version":"2.7.3"}`})
	sab := page("", map[string]string{"/api": `{"version":"4.5.1"}`})
	qbit := page("<html><title>qBittorrent WebUI</title></html>", nil)
	other := page("<html>some other app</html>", map[string]string{"/ping": "pong"})
	keySeen := make(chan bool, 8)
	silent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "" {
			keySeen <- true
		}
		time.Sleep(2 * time.Second)
	}))
	for _, s := range []*httptest.Server{sonarr, seerr, sab, qbit, other, silent} {
		defer s.Close()
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	ch, cp := hostPort(t, closed.URL)
	closed.Close()

	tg := func(u string, k ...Kind) findTarget {
		h, p := hostPort(t, u)
		return findTarget{Host: h, Port: p, Kinds: k}
	}
	old := findTargets
	t.Cleanup(func() { findTargets = old })
	findTargets = func([]string) []findTarget {
		return []findTarget{
			tg(sonarr.URL, Sonarr), tg(seerr.URL, Overseerr), tg(sab.URL, SABnzbd, QBittorrent), tg(qbit.URL, SABnzbd, QBittorrent),
			tg(other.URL, Radarr), tg(other.URL, Bazarr), tg(silent.URL, Lidarr), {Host: ch, Port: cp, Kinds: []Kind{Prowlarr}},
		}
	}
	start := time.Now()
	got, _ := Find(context.Background(), nil)
	if took := time.Since(start); took > 1500*time.Millisecond {
		t.Errorf("a silent host held the search for %v", took)
	}
	want := map[Kind]string{Sonarr: "", Overseerr: "2.7.3", SABnzbd: "4.5.1", QBittorrent: ""}
	if len(got) != len(want) {
		t.Fatalf("found %+v", got)
	}
	for _, f := range got {
		v, ok := want[f.Kind]
		if !ok || f.Version != v || !strings.HasPrefix(f.Address, "http://127.0.0.1:") {
			t.Errorf("unexpected %+v", f)
		}
	}
	select {
	case <-keySeen:
		t.Error("a key was sent")
	default:
	}
}

// The plan tries the Docker names only on their own port, every target on a
// known port, and at most maxNets /24s.
func TestFindTargetsBounded(t *testing.T) {
	ts := findTargets(findNets([]string{"10.1.2.3", "192.168.7.9", "172.17.0.2", "172.18.0.9"}))
	ports := map[int]bool{}
	for _, p := range defaultPorts {
		ports[p.port] = true
	}
	names := 0
	for _, x := range ts {
		if !ports[x.Port] {
			t.Fatalf("port %d is not an app's port", x.Port)
		}
		if x.Host == "sonarr" {
			names++
			if x.Port != 8989 {
				t.Errorf("sonarr tried on %d", x.Port)
			}
		}
	}
	if names != 1 {
		t.Errorf("sonarr tried %d times", names)
	}
	if most := (maxNets*254+3)*len(defaultPorts) + 20; len(ts) > most {
		t.Errorf("%d targets, more than %d", len(ts), most)
	}
}

// The route runs at most one search every findGap (owner only is the
// route permission, pinned in cmd/holyarrs).
func TestFindRouteOwnerAndLimited(t *testing.T) {
	old := findTargets
	t.Cleanup(func() { findTargets = old })
	findTargets = func([]string) []findTarget { return nil }
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r := webserver.NewRouter()
	h := &Handlers{Store: st, Mover: func() *Mover { return nil }, SafeMode: func() bool { return false }}
	h.Routes(r)
	as := func(role string) int {
		ctx := webserver.ContextWith(context.Background(), webserver.Caller{UserID: 2, Name: role, Role: role, Via: "session"})
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/migrate/find", nil).WithContext(ctx))
		if rec.Code == 200 {
			var out struct{ Found []Found }
			if json.Unmarshal(rec.Body.Bytes(), &out) != nil || out.Found == nil {
				t.Errorf("body %s", rec.Body.String())
			}
		}
		return rec.Code
	}
	if c := as(webserver.RoleOwner); c != http.StatusOK {
		t.Errorf("owner: %d", c)
	}
	// A second search right after waits its turn, then runs.
	if c := as(webserver.RoleOwner); c != http.StatusOK {
		t.Errorf("second search right after: %d", c)
	}
	// With findWaiters already waiting, one more is refused at once.
	for _, tc := range []struct {
		waiting int32
		want    int
	}{{findWaiters, http.StatusTooManyRequests}, {findWaiters + 2, http.StatusTooManyRequests}} {
		h.findWaiting.Store(tc.waiting)
		if c := as(webserver.RoleOwner); c != tc.want {
			t.Errorf("%d waiting: got %d, want %d", tc.waiting, c, tc.want)
		}
	}
	h.findWaiting.Store(0)
}

// Overseerr's Sonarr and Radarr come with their keys: listed without them,
// checked from here, and with add read and kept in the vault.
func TestLinkedFromOverseerr(t *testing.T) {
	ctx := context.Background()
	rURL, rKey := radarrAPI(t)
	rh, rp := hostPort(t, rURL)
	rAddr := "http://" + net.JoinHostPort(rh, strconv.Itoa(rp))
	seerrKey := "S3CR3T_SEERR_KEY"
	srv := fakeAPI(t, "X-Api-Key", seerrKey, map[string]any{
		"/api/v1/settings/radarr": []any{map[string]any{"id": 0, "name": "Radarr", "hostname": rh, "port": rp, "apiKey": rKey,
			"useSsl": false, "baseUrl": "", "is4k": false, "isDefault": true}},
		"/api/v1/settings/sonarr": []any{map[string]any{"id": 0, "name": "Sonarr", "hostname": "sonarr-only-overseerr-knows.invalid", "port": 8989,
			"apiKey": "S3CR3T_SONARR_KEY", "useSsl": false, "baseUrl": "/sonarr/", "is4k": true}},
	})
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	v, err := secrets.Open(ctx, st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	r := webserver.NewRouter()
	h := &Handlers{Store: st, Secrets: v, Mover: func() *Mover { return nil }, SafeMode: func() bool { return false }}
	h.Routes(r)
	owner := webserver.ContextWith(ctx, webserver.Caller{UserID: 1, Name: "owner", Role: webserver.RoleOwner, Via: "session"})
	do := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/migrate/linked", strings.NewReader(body)).WithContext(owner))
		return rec
	}
	base := `{"kind":"overseerr","url":"` + srv.URL + `","key":"` + seerrKey + `","allow_local_network":true`
	rec := do(base + `}`)
	if rec.Code != 200 {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "S3CR3T") {
		t.Fatalf("a key was answered: %s", rec.Body.String())
	}
	var out struct {
		Apps []struct {
			Kind      Kind
			Address   string
			FourK     bool `json:"four_k"`
			HasKey    bool `json:"has_key"`
			Reachable bool
			Version   string
		}
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Apps) != 2 {
		t.Fatalf("apps %+v", out.Apps)
	}
	for _, a := range out.Apps {
		switch a.Kind {
		case Radarr:
			if !a.Reachable || !a.HasKey || a.Version != "6.4.4" || a.Address != rAddr {
				t.Errorf("radarr %+v", a)
			}
		case Sonarr:
			if a.Reachable || !a.HasKey || !a.FourK || a.Address != "http://sonarr-only-overseerr-knows.invalid:8989/sonarr" {
				t.Errorf("sonarr %+v", a)
			}
		}
	}
	rec = do(base + `,"add":["` + rAddr + `"]}`)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "S3CR3T") {
		t.Fatalf("add: %d %s", rec.Code, rec.Body.String())
	}
	var added struct {
		Read []struct {
			Kind     Kind
			Snapshot Saved
		}
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &added)
	if len(added.Read) != 1 || added.Read[0].Kind != Radarr || added.Read[0].Snapshot.ID == 0 {
		t.Fatalf("read %s", rec.Body.String())
	}
	saved := httptest.NewRecorder()
	r.ServeHTTP(saved, httptest.NewRequest("GET", "/api/v1/migrate/saved-keys", nil).WithContext(owner))
	if !strings.Contains(saved.Body.String(), `"radarr"`) || strings.Contains(saved.Body.String(), "S3CR3T") {
		t.Errorf("saved keys %s", saved.Body.String())
	}
}

// Prowlarr's synced apps give addresses only: it masks their keys.
func TestLinkedFromProwlarr(t *testing.T) {
	key := "S3CR3T_PROWLARR"
	srv := fakeAPI(t, "X-Api-Key", key, map[string]any{
		"/api/v1/applications": []any{
			map[string]any{"name": "Sonarr", "implementation": "Sonarr", "fields": []any{
				map[string]any{"name": "prowlarrUrl", "value": "http://prowlarr:9696"},
				map[string]any{"name": "baseUrl", "value": "http://sonarr:8989"},
				map[string]any{"name": "apiKey", "value": "********"}}},
			map[string]any{"name": "Readarr", "implementation": "Readarr", "fields": []any{map[string]any{"name": "baseUrl", "value": "http://readarr:8787"}}},
		},
	})
	got, err := ReadLinked(context.Background(), api(Prowlarr, srv.URL, key))
	if err != nil || len(got) != 1 || got[0].Kind != Sonarr || got[0].Address != "http://sonarr:8989" || got[0].HasKey || got[0].key != "" {
		t.Fatalf("%+v %v", got, err)
	}
}

// The /24s come from the address the browser used first, then the addresses
// already known, then this server's own; names, public, loopback and
// metadata addresses are skipped, and at most maxNets are searched.
func TestFindNetsFromHints(t *testing.T) {
	got := findNets([]string{"", "holyarrs.example.com", "8.8.8.8", "127.0.0.1:6262", "169.254.169.254",
		"10.20.30.40:18790", "10.20.30.41", "http://192.168.5.7:7878/radarr", "[::1]:80", "172.20.1.2", "172.21.1.2"})
	want := []string{"10.20.30", "192.168.5", "172.20.1"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("nets %v, want %v", got, want)
	}
	for _, h := range []string{"10.20.30.40", "192.168.5.200", "172.20.1.254"} {
		ok := false
		for _, x := range findTargets(got) {
			if x.Host == h {
				ok = true
			}
		}
		if !ok {
			t.Errorf("%s not tried", h)
		}
	}
}

// The route takes the browser's host from the body, the Host header, the
// typed addresses and the Downloaders already added, and says what it
// searched.
func TestFindRouteSearchesBrowserNetwork(t *testing.T) {
	var asked []string
	old := findTargets
	t.Cleanup(func() { findTargets = old })
	findTargets = func(nets []string) []findTarget { asked = nets; return nil }
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	err = st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url)
			VALUES ('c','x','c','x','q','qbittorrent','torrent','http://192.168.40.5:8080')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	r := webserver.NewRouter()
	h := &Handlers{Store: st, Mover: func() *Mover { return nil }, SafeMode: func() bool { return false }}
	h.Routes(r)
	ctx := webserver.ContextWith(context.Background(), webserver.Caller{UserID: 1, Name: "o", Role: webserver.RoleOwner, Via: "session"})
	req := httptest.NewRequest("POST", "/api/v1/migrate/find", strings.NewReader(`{"host":"10.9.8.7","addresses":["holyarrs.lan","http://172.30.0.4:5055"]}`)).WithContext(ctx)
	req.Host = "holyarrs.lan:18790"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if len(asked) != maxNets || asked[0] != "10.9.8" || asked[1] != "172.30.0" || asked[2] != "192.168.40" {
		t.Errorf("nets %v", asked)
	}
	var out struct{ Searched []string }
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Searched) == 0 || out.Searched[0] != "10.9.8.0/24" {
		t.Errorf("searched %v", out.Searched)
	}
}
