package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/secrets"
	"github.com/CoderLuii/holyarrs/internal/source"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Bridges (#96) against a fake Jackett and a fake Prowlarr on this machine.

const jackettKey = "jackett-test-key"

// fakeJackett answers the site list the way Jackett does: 200 even for a wrong key.
type fakeJackett struct {
	srv   *httptest.Server
	mu    sync.Mutex
	sites []string // ids, each titled by id with "Site " in front
}

func newFakeJackett(t *testing.T, sites ...string) *fakeJackett {
	f := &fakeJackett{sites: sites}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/UI/Login" {
			_, _ = w.Write([]byte("<html><head><title>Jackett</title></head></html>"))
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Query().Get("apikey") != jackettKey {
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><error code="100" description="Invalid API Key" />`))
			return
		}
		if r.URL.Path != "/api/v2.0/indexers/all/results/torznab/api" || r.URL.Query().Get("t") != "indexers" || r.URL.Query().Get("configured") != "true" {
			http.NotFound(w, r)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><indexers>`)
		for _, id := range f.sites {
			fmt.Fprintf(&b, `<indexer id="%s" configured="true"><title>Site %s</title><type>public</type><caps><categories>`+
				`<category id="2000" name="Movies"><subcat id="2040" name="Movies/HD"/></category>`+
				`<category id="5000" name="TV"><subcat id="5040" name="TV/HD"/></category>`+
				`<category id="100012" name="Own"/></categories></caps></indexer>`, id, id)
		}
		b.WriteString(`</indexers>`)
		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeJackett) set(ids ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sites = ids
}

func openStore(t *testing.T) (*store.Store, *secrets.Vault) {
	t.Helper()
	s, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return s, v
}

func TestJackettSitesAndWrongKey(t *testing.T) {
	f := newFakeJackett(t, "alpha", "beta")
	sites, err := NewJackett(f.srv.URL, jackettKey).Sites(context.Background())
	if err != nil || len(sites) != 2 || sites[0].ID != "alpha" || sites[0].Title != "Site alpha" {
		t.Fatalf("%+v %v", sites, err)
	}
	if got := sites[0].Categories; !slices.Equal(got["movie"], []int{2000, 2040}) || !slices.Equal(got["tv"], []int{5000, 5040}) || len(got["music"]) != 0 {
		t.Fatalf("categories %+v", got)
	}
	if want := f.srv.URL + "/api/v2.0/indexers/alpha/results/torznab/"; NewJackett(f.srv.URL+"/", "k").FeedURL("alpha") != want {
		t.Fatalf("feed %s", NewJackett(f.srv.URL, "k").FeedURL("alpha"))
	}
	if _, err := NewJackett(f.srv.URL, "wrong").Sites(context.Background()); !strings.Contains(Words(err), "turned down the key") {
		t.Fatalf("wrong key: %v", err)
	}
	notJackett := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("<html>hello</html>")) }))
	defer notJackett.Close()
	if _, err := NewJackett(notJackett.URL, jackettKey).Sites(context.Background()); !strings.Contains(Words(err), "not like Jackett") {
		t.Fatalf("not jackett: %v", err)
	}
	if _, err := NewJackett("http://127.0.0.1:1", jackettKey).Sites(context.Background()); !strings.Contains(Words(err), "could not reach") {
		t.Fatalf("down: %v", err)
	}
}

func TestJackettSourcesFollowItsSites(t *testing.T) {
	ctx := context.Background()
	s, v := openStore(t)
	f := newFakeJackett(t, "alpha", "beta")
	svc := &Service{Store: s, Vault: v}
	b, err := svc.Add(ctx, KindJackett, f.srv.URL, jackettKey, "", domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(ctx, KindJackett, f.srv.URL+"/", jackettKey, "", domain.System); err == nil {
		// CleanURL drops the slash before Add in the API; the store refuses the same address.
		t.Log("a second add with a slash is the API's to clean")
	}
	// A Source of the person's own with the same name as a site.
	if _, err := source.Add(ctx, s, v, source.Input{Name: "Site beta", Format: "torznab", URL: "https://own.example/api"}, domain.System); err != nil {
		t.Fatal(err)
	}
	res, err := svc.CheckJackett(ctx, b, domain.System)
	if err != nil || res.Added != 2 || res.Sites != 2 {
		t.Fatalf("first check %+v %v", res, err)
	}
	srcs, _ := svc.sourcesOf(ctx, b)
	names := []string{srcs[0].Name, srcs[1].Name}
	slices.Sort(names)
	if !slices.Equal(names, []string{"Site alpha", "Site beta (Jackett)"}) {
		t.Fatalf("names %v", names)
	}
	one := srcs[0]
	if one.Format != "torznab" || !one.AllowLocal || !strings.HasSuffix(one.URL, "/results/torznab/") || !slices.Equal(one.Categories["movie"], []int{2000, 2040}) {
		t.Fatalf("source %+v", one)
	}
	cfg, err := source.ConfigFor(ctx, one, v)
	if err != nil || cfg.APIKey != jackettKey {
		t.Fatalf("key %v %v", cfg.APIKey, err)
	}
	// The person's own changes stay; nothing changed in Jackett: nothing written.
	prio := 7
	if _, err := source.Change(ctx, s, v, one.ID, source.Input{Name: "My alpha", Priority: &prio}, domain.System); err != nil {
		t.Fatal(err)
	}
	b, _ = svc.Get(ctx, b.ID)
	before := s.WriteCount()
	if res, err := svc.CheckJackett(ctx, b, domain.System); err != nil || res.Added+res.Updated+res.Removed != 0 {
		t.Fatalf("quiet check %+v %v", res, err)
	}
	if s.WriteCount() != before {
		t.Fatalf("a quiet check wrote %d times", s.WriteCount()-before)
	}
	// A site added and one removed in Jackett.
	f.set("beta", "gamma")
	b, _ = svc.Get(ctx, b.ID)
	res, err = svc.CheckJackett(ctx, b, domain.System)
	if err != nil || res.Added != 1 || res.Removed != 1 {
		t.Fatalf("after change %+v %v", res, err)
	}
	srcs, _ = svc.sourcesOf(ctx, b)
	for _, src := range srcs {
		if src.Name == "My alpha" {
			t.Fatal("the removed site's Source stayed")
		}
	}
	b, _ = svc.Get(ctx, b.ID)
	if b.SiteCount != 2 || b.LastError != "" || b.LastCheck == nil {
		t.Fatalf("bridge %+v", b)
	}
	// A failing check keeps the Sources and says why.
	f.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<error code="100" description="Invalid API Key"/>`))
	})
	if _, err := svc.CheckJackett(ctx, b, domain.System); err == nil {
		t.Fatal("no error")
	}
	b, _ = svc.Get(ctx, b.ID)
	if !strings.Contains(b.LastError, "turned down the key") || b.SiteCount != 2 || svc.SourceCount(ctx, b) != 2 {
		t.Fatalf("after failure %+v", b)
	}
	// Disconnect, keeping the Sources as ordinary ones.
	n, err := svc.Remove(ctx, b, true)
	if err != nil || n != 2 {
		t.Fatalf("remove %d %v", n, err)
	}
	var left int
	_ = s.Read().QueryRow("SELECT count(*) FROM sources WHERE json_extract(extra, '$.bridge') IS NULL AND name LIKE 'Site %'").Scan(&left)
	if left != 3 { // two kept from the bridge, plus the person's own
		t.Fatalf("kept %d", left)
	}
}

func TestJackettDisconnectRemovesItsSources(t *testing.T) {
	ctx := context.Background()
	s, v := openStore(t)
	f := newFakeJackett(t, "alpha")
	svc := &Service{Store: s, Vault: v}
	b, _ := svc.Add(ctx, KindJackett, f.srv.URL, jackettKey, "", domain.System)
	if _, err := svc.CheckJackett(ctx, b, domain.System); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.Remove(ctx, b, false); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	var n int
	_ = s.Read().QueryRow("SELECT count(*) FROM sources").Scan(&n)
	_ = s.Read().QueryRow("SELECT count(*) FROM bridges").Scan(&n)
	if n != 0 {
		t.Fatalf("left %d", n)
	}
	if ok, _, _ := svc.Probe(ctx, nowish()); ok {
		t.Fatal("the scheduled check runs with no Jackett")
	}
}

// fakeProwlarr keeps the apps written to it.
type fakeProwlarr struct {
	srv     *httptest.Server
	mu      sync.Mutex
	apps    []map[string]any
	nextID  int
	synced  int
	refuse  string
	appName string
}

const prowlarrKey = "prowlarr-test-key"

func newFakeProwlarr(t *testing.T) *fakeProwlarr {
	f := &fakeProwlarr{nextID: 1, appName: "Prowlarr"}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ping" {
			_, _ = w.Write([]byte(`{"status":"OK"}`))
			return
		}
		if r.Header.Get("X-Api-Key") != prowlarrKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.URL.Path == "/api/v1/system/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"appName": f.appName, "version": "2.6.5.5623"})
		case r.URL.Path == "/api/v1/applications/schema":
			var out []map[string]any
			for _, impl := range []string{"Sonarr", "Radarr", "Lidarr"} {
				out = append(out, map[string]any{"id": 0, "name": "", "implementation": impl, "configContract": impl + "Settings", "syncLevel": "addOnly",
					"fields": []any{map[string]any{"name": "prowlarrUrl", "value": "http://localhost:9696"}, map[string]any{"name": "baseUrl", "value": "http://localhost:8989"},
						map[string]any{"name": "apiKey", "value": ""}, map[string]any{"name": "syncCategories", "value": []any{5000, 5040}}}, "presets": []any{}})
			}
			_ = json.NewEncoder(w).Encode(out)
		case r.URL.Path == "/api/v1/applications" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(f.apps)
		case r.URL.Path == "/api/v1/applications" && r.Method == http.MethodPost:
			if f.refuse != "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode([]map[string]any{{"propertyName": "BaseUrl", "errorMessage": f.refuse}})
				return
			}
			var a map[string]any
			_ = json.NewDecoder(r.Body).Decode(&a)
			a["id"] = f.nextID
			f.nextID++
			f.apps = append(f.apps, a)
			_ = json.NewEncoder(w).Encode(a)
		case strings.HasPrefix(r.URL.Path, "/api/v1/applications/") && (r.Method == http.MethodPut || r.Method == http.MethodDelete):
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/applications/")
			for i, a := range f.apps {
				if fmt.Sprint(a["id"]) == id {
					if r.Method == http.MethodDelete {
						f.apps = append(f.apps[:i], f.apps[i+1:]...)
					} else {
						var b map[string]any
						_ = json.NewDecoder(r.Body).Decode(&b)
						f.apps[i] = b
					}
					break
				}
			}
			_, _ = w.Write([]byte("{}"))
		case r.URL.Path == "/api/v1/command":
			var c map[string]any
			_ = json.NewDecoder(r.Body).Decode(&c)
			if c["name"] == "ApplicationIndexerSync" && c["forceSync"] == true {
				f.synced++
			}
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func TestProwlarrConnectAddsOrUpdatesTheApp(t *testing.T) {
	ctx := context.Background()
	f := newFakeProwlarr(t)
	p := NewProwlarr(f.srv.URL, prowlarrKey)
	if v, err := p.Status(ctx); err != nil || v != "2.6.5.5623" {
		t.Fatalf("%q %v", v, err)
	}
	side := Side{Kind: "Sonarr", Name: "HolyArrs TV shows", Base: "http://10.0.0.5:18730/sonarr", Key: "k1"}
	if err := p.Connect(ctx, side); err != nil {
		t.Fatal(err)
	}
	side.Key = "k2"
	if err := p.Connect(ctx, side); err != nil {
		t.Fatal(err)
	}
	if len(f.apps) != 1 {
		t.Fatalf("doubled: %d apps", len(f.apps))
	}
	a := app(f.apps[0])
	if a["implementation"] != "Sonarr" || a["syncLevel"] != "fullSync" || a.field("apiKey") != "k2" || a.field("baseUrl") != side.Base ||
		a.field("prowlarrUrl") != f.srv.URL || fmt.Sprint(a.field("syncCategories")) != "[5000 5040]" {
		t.Fatalf("app %+v", a)
	}
	if err := p.Sync(ctx); err != nil || f.synced != 1 {
		t.Fatalf("sync %d %v", f.synced, err)
	}
	got, _ := p.Connected(ctx, map[string]string{"Sonarr": side.Base, "Radarr": "http://10.0.0.5:18730/radarr"})
	if !got["Sonarr"] || got["Radarr"] {
		t.Fatalf("connected %v", got)
	}
	if err := p.Disconnect(ctx, []string{side.Base}); err != nil || len(f.apps) != 0 {
		t.Fatalf("disconnect %d %v", len(f.apps), err)
	}
	f.refuse = "Unable to connect to HolyArrs"
	if err := p.Connect(ctx, side); !strings.Contains(Words(err), "Unable to connect to HolyArrs") {
		t.Fatalf("refused: %v", err)
	}
	if _, err := NewProwlarr(f.srv.URL, "wrong").Status(ctx); !strings.Contains(Words(err), "turned down the key") {
		t.Fatalf("wrong key: %v", err)
	}
	f.appName = "Sonarr"
	if _, err := p.Status(ctx); !strings.Contains(Words(err), "That is Sonarr") {
		t.Fatalf("sonarr: %v", err)
	}
}

func TestFindLooksOnlyAtTheUsualPlaces(t *testing.T) {
	j := newFakeJackett(t)
	p := newFakeProwlarr(t)
	old := FindHosts
	t.Cleanup(func() { FindHosts = old })
	jh, _ := url.Parse(j.srv.URL)
	ph, _ := url.Parse(p.srv.URL)
	FindHosts = func() []string { return []string{jh.Host, ph.Host, "127.0.0.1:1"} }
	found := Find(context.Background())
	var kinds []string
	for _, f := range found {
		kinds = append(kinds, f.Kind+" "+f.URL)
	}
	if !slices.Contains(kinds, "jackett "+j.srv.URL) || !slices.Contains(kinds, "prowlarr "+p.srv.URL) || slices.Contains(kinds, "jackett "+p.srv.URL) || slices.Contains(kinds, "prowlarr "+j.srv.URL) {
		t.Fatalf("found %v", kinds)
	}
}

func TestCleanURL(t *testing.T) {
	for raw, ok := range map[string]bool{"http://jackett:9117/": true, "https://x.example/jackett": true, "jackett:9117": false, "http://u:p@x": false, "http://x/?a=1": false, "ftp://x": false} {
		if _, err := CleanURL(raw); (err == nil) != ok {
			t.Errorf("%s: %v", raw, err)
		}
	}
	if u, _ := CleanURL(" http://jackett:9117/ "); u != "http://jackett:9117" {
		t.Errorf("cleaned %q", u)
	}
	// A cloud's metadata address is never called.
	if _, _, err := call(context.Background(), client(2_000_000_000), http.MethodGet, "http://169.254.169.254/", nil, nil, 10); err == nil || !strings.Contains(Words(err), "cloud") {
		t.Errorf("metadata: %v", err)
	}
	_ = net.IPv4len
}

func nowish() time.Time { return time.Now() }

// A Source added by hand for a Jackett site joins the bridge instead of
// getting a twin, a very long site title still makes a Source, and a
// Jackett that stays down writes nothing on each check.
func TestJackettAdoptsHandAddedLongTitlesAndQuietWhenDown(t *testing.T) {
	ctx := context.Background()
	s, v := openStore(t)
	long := strings.Repeat("x", 95)
	f := newFakeJackett(t, "alpha", long)
	svc := &Service{Store: s, Vault: v}
	j := NewJackett(f.srv.URL, jackettKey)
	if _, err := source.Add(ctx, s, v, source.Input{Name: "My Jackett alpha", Format: "torznab", URL: j.FeedURL("alpha"), APIKey: jackettKey, AllowLocal: ptrTrue()}, domain.System); err != nil {
		t.Fatal(err)
	}
	b, _ := svc.Add(ctx, KindJackett, f.srv.URL, jackettKey, "", domain.System)
	res, err := svc.CheckJackett(ctx, b, domain.System)
	if err != nil || res.Added != 1 || res.Updated != 1 {
		t.Fatalf("check %+v %v", res, err)
	}
	var total int
	_ = s.Read().QueryRow("SELECT count(*) FROM sources").Scan(&total)
	if total != 2 {
		t.Fatalf("%d Sources, want the hand-added one plus one new", total)
	}
	srcs, _ := svc.sourcesOf(ctx, b)
	for _, src := range srcs {
		if len(src.Name) > 80 {
			t.Fatalf("name %d bytes", len(src.Name))
		}
	}
	b, _ = svc.Get(ctx, b.ID)
	if b.LastError != "" {
		t.Fatalf("error %q", b.LastError)
	}
	// Down, then still down: the second failed check writes nothing.
	f.srv.Close()
	_, _ = svc.CheckJackett(ctx, b, domain.System)
	b, _ = svc.Get(ctx, b.ID)
	before := s.WriteCount()
	_, _ = svc.CheckJackett(ctx, b, domain.System)
	if s.WriteCount() != before {
		t.Fatalf("a check of a Jackett that stays down wrote %d times", s.WriteCount()-before)
	}
	if b.SiteCount != 2 || !strings.Contains(b.LastError, "could not reach") {
		t.Fatalf("down %+v", b)
	}
}

func ptrTrue() *bool { t := true; return &t }

// A Source the person removes stays away at the next check; connecting the
// same Jackett again brings it back.
func TestJackettRemovedSourceStaysAway(t *testing.T) {
	ctx := context.Background()
	s, v := openStore(t)
	f := newFakeJackett(t, "alpha", "beta")
	svc := &Service{Store: s, Vault: v}
	b, _ := svc.Add(ctx, KindJackett, f.srv.URL, jackettKey, "", domain.System)
	if res, err := svc.CheckJackett(ctx, b, domain.System); err != nil || res.Added != 2 {
		t.Fatalf("first %+v %v", res, err)
	}
	srcs, _ := svc.sourcesOf(ctx, b)
	if err := source.Remove(ctx, s, srcs[0].ID); err != nil {
		t.Fatal(err)
	}
	b, _ = svc.Get(ctx, b.ID)
	res, err := svc.CheckJackett(ctx, b, domain.System)
	if err != nil || res.Added != 0 || res.Skipped != 1 || svc.SourceCount(ctx, b) != 1 {
		t.Fatalf("after removal %+v %v", res, err)
	}
	// A new site in Jackett still comes in.
	f.set("alpha", "beta", "gamma")
	b, _ = svc.Get(ctx, b.ID)
	if res, err := svc.CheckJackett(ctx, b, domain.System); err != nil || res.Added != 1 || res.Skipped != 1 {
		t.Fatalf("new site %+v %v", res, err)
	}
	// Connecting again brings every site back.
	b, _ = svc.Get(ctx, b.ID)
	if b, err = svc.Update(ctx, b, jackettKey, "", domain.System); err != nil {
		t.Fatal(err)
	}
	if res, err := svc.CheckJackett(ctx, b, domain.System); err != nil || res.Added != 1 || res.Skipped != 0 || svc.SourceCount(ctx, b) != 3 {
		t.Fatalf("reconnect %+v %v", res, err)
	}
}

// Only a Jackett on the local network gives its Sources the local-network
// allowance, and a change follows on the next check.
func TestJackettLocalAllowanceFollowsItsAddress(t *testing.T) {
	ctx := context.Background()
	s, v := openStore(t)
	f := newFakeJackett(t, "alpha")
	svc := &Service{Store: s, Vault: v}
	local := false
	IsLocal = func(context.Context, string) (bool, bool) { return local, true }
	t.Cleanup(func() { IsLocal = LocalAddress })
	b, _ := svc.Add(ctx, KindJackett, f.srv.URL, jackettKey, "", domain.System)
	if _, err := svc.CheckJackett(ctx, b, domain.System); err != nil {
		t.Fatal(err)
	}
	srcs, _ := svc.sourcesOf(ctx, b)
	b, _ = svc.Get(ctx, b.ID)
	if len(srcs) != 1 || srcs[0].AllowLocal || b.Extra["local"] != "" {
		t.Fatalf("public Jackett: %+v %v", srcs, b.Extra)
	}
	local = true
	if res, err := svc.CheckJackett(ctx, b, domain.System); err != nil || res.Updated != 1 {
		t.Fatalf("now local %+v %v", res, err)
	}
	srcs, _ = svc.sourcesOf(ctx, b)
	b, _ = svc.Get(ctx, b.ID)
	if !srcs[0].AllowLocal || b.Extra["local"] != "1" {
		t.Fatalf("local Jackett: %+v %v", srcs[0], b.Extra)
	}
	// A name that can't be looked up keeps the last answer.
	IsLocal = func(context.Context, string) (bool, bool) { return false, false }
	if res, err := svc.CheckJackett(ctx, b, domain.System); err != nil || res.Updated != 0 {
		t.Fatalf("lookup failed %+v %v", res, err)
	}
}

func TestLocalAddress(t *testing.T) {
	ctx := context.Background()
	for raw, want := range map[string]bool{
		"http://127.0.0.1:9117": true, "http://10.0.0.5:9117": true, "http://192.168.1.2:9117/jackett": true,
		"http://localhost:9117": true, "http://[::1]:9117": true, "http://100.87.70.86:9117": true,
		"http://8.8.8.8:9117": false, "https://1.1.1.1": false,
	} {
		if got, ok := LocalAddress(ctx, raw); got != want || !ok {
			t.Errorf("%s: %v %v", raw, got, ok)
		}
	}
	if _, ok := LocalAddress(ctx, "::bad"); ok {
		t.Error("a bad address looked up")
	}
}
