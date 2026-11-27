package plugins_test

import (
	"bytes"
	"compress/bzip2"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

// packServer hosts languages.json, blocklist.json and the pack files.
type packServer struct {
	srv   *httptest.Server
	priv  ed25519.PrivateKey
	pub   ed25519.PublicKey
	mu    sync.Mutex
	files map[string][]byte
	hits  map[string]int
}

func newPackServer(t *testing.T) *packServer {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	ps := &packServer{pub: pub, priv: priv, files: map[string][]byte{}, hits: map[string]int{}}
	ps.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ps.mu.Lock()
		b, ok := ps.files[strings.TrimPrefix(r.URL.Path, "/")]
		ps.hits[r.URL.Path]++
		ps.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	}))
	t.Cleanup(ps.srv.Close)
	ps.lists(1, nil)
	return ps
}

func (ps *packServer) set(name string, b []byte) {
	ps.mu.Lock()
	ps.files[name] = b
	ps.mu.Unlock()
}

// lists signs a languages list with entries and an empty blocklist.
func (ps *packServer) lists(counter uint64, entries []plugins.Entry, block ...plugins.Entry) {
	ps.listsMin(counter, counter, counter, entries, block...)
}

// listsMin signs a languages list needing blocklist minBlock or newer, and
// a blocklist with counter blockCounter.
func (ps *packServer) listsMin(counter, minBlock, blockCounter uint64, entries []plugins.Entry, block ...plugins.Entry) {
	l, _ := plugins.SignList(ps.priv, plugins.List{Kind: plugins.KindLanguages, Counter: counter, MinBlocklistCounter: &minBlock, Entries: entries})
	b, _ := plugins.SignList(ps.priv, plugins.List{Kind: plugins.KindBlocklist, Counter: blockCounter, Entries: block})
	ps.set("languages.json", l)
	ps.set("blocklist.json", b)
}

// bz packs b with the system bzip2 when there is one, else a stored
// pre-packed copy is impossible, so the test is skipped.
func bz(t *testing.T, b []byte) []byte {
	t.Helper()
	path, err := exec.LookPath("bzip2")
	if err != nil {
		t.Skip("bzip2 not installed")
	}
	cmd := exec.Command(path, "-c", "-9")
	cmd.Stdin = bytes.NewReader(b)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func packFile(t *testing.T, lang, app string) []byte {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"lang": lang, "app": app, "fit": "abc123abc123", "values": []any{"Hallo", nil}})
	return bz(t, raw)
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// publish puts a pack on the server and returns its list entry.
func (ps *packServer) publish(t *testing.T, lang, version string) plugins.Entry {
	t.Helper()
	f := packFile(t, lang, version)
	name := "languages/" + lang + "/" + version + ".json.bz2"
	ps.set(name, f)
	return plugins.Entry{ID: "lang-" + lang, Version: version, SHA256: sum(f)}
}

func newLanguages(t *testing.T, ps *packServer, app string) *plugins.Languages {
	t.Helper()
	e := &plugins.Engine{Dir: t.TempDir(), ListURL: ps.srv.URL + "/", Keys: []ed25519.PublicKey{ps.pub}, HTTP: ps.srv.Client(), AppVersion: app}
	return &plugins.Languages{Dir: filepath.Join(t.TempDir(), "languages"), Engine: e}
}

func plain(t *testing.T, l *plugins.Languages, code string) map[string]any {
	t.Helper()
	b, _, err := l.Pack(code)
	if err != nil {
		t.Fatalf("pack %s: %v", code, err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestVerifyListLanguagesKind(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	ok := plugins.Entry{ID: "lang-de", Version: "1.2.1", SHA256: strings.Repeat("a", 64)}
	zero := uint64(0)
	raw, _ := plugins.SignList(priv, plugins.List{Kind: plugins.KindLanguages, Counter: 5, MinBlocklistCounter: &zero, Entries: []plugins.Entry{ok}})
	if _, err := plugins.VerifyList(pub, raw, plugins.KindLanguages, 5); err != nil {
		t.Fatalf("good list refused: %v", err)
	}
	if _, err := plugins.VerifyList(pub, raw, plugins.KindPlugins, 0); !errors.Is(err, plugins.ErrListKind) {
		t.Fatalf("languages list taken as the plugin list: %v", err)
	}
	if _, err := plugins.VerifyList(pub, raw, plugins.KindLanguages, 6); !errors.Is(err, plugins.ErrListOlder) {
		t.Fatalf("older list accepted: %v", err)
	}
	other, _, _ := ed25519.GenerateKey(nil)
	if _, err := plugins.VerifyList(other, raw, plugins.KindLanguages, 0); !errors.Is(err, plugins.ErrListSignature) {
		t.Fatalf("list signed by another key accepted: %v", err)
	}
	bad, _ := plugins.SignList(priv, plugins.List{Kind: plugins.KindLanguages, Counter: 5, MinBlocklistCounter: &zero, Entries: []plugins.Entry{{ID: "lang-de", Version: "1.2.1"}}})
	if _, err := plugins.VerifyList(pub, bad, plugins.KindLanguages, 0); !errors.Is(err, plugins.ErrListFormat) {
		t.Fatalf("entry without a hash accepted: %v", err)
	}
}

func TestLanguageInstallAndServe(t *testing.T) {
	ps := newPackServer(t)
	ps.lists(2, []plugins.Entry{ps.publish(t, "de", "1.2.0"), ps.publish(t, "de", "1.2.1")})
	l := newLanguages(t, ps, "1.2.1")
	if _, _, err := l.Pack("de"); !errors.Is(err, plugins.ErrNotInstalled) {
		t.Fatalf("pack before install: %v", err)
	}
	if err := l.Install(context.Background(), "de"); err != nil {
		t.Fatal(err)
	}
	if m := plain(t, l, "de"); m["lang"] != "de" || m["app"] != "1.2.1" {
		t.Fatalf("served pack %v", m)
	}
	rows := l.List()
	var de *plugins.LanguageRow
	for i := range rows {
		if rows[i].Code == "de" {
			de = &rows[i]
		}
	}
	if de == nil || de.State != "installed" || de.Version != "1.2.1" || de.Size == 0 {
		t.Fatalf("row %+v", de)
	}
	if rows[0].Code != "en" || rows[0].State != "built_in" {
		t.Fatalf("English is not first and built in: %+v", rows[0])
	}
	// The stored file is the packed copy.
	if _, err := os.Stat(filepath.Join(l.Dir, "de", "1.2.1.json.bz2")); err != nil {
		t.Fatal(err)
	}
	if err := l.Remove("de"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.Pack("de"); !errors.Is(err, plugins.ErrNotInstalled) {
		t.Fatalf("pack after remove: %v", err)
	}
}

func TestLanguageRefusals(t *testing.T) {
	ps := newPackServer(t)
	good := ps.publish(t, "fr", "1.2.1")
	cases := map[string]func(){
		"hash mismatch": func() {
			e := good
			e.SHA256 = strings.Repeat("0", 64)
			ps.lists(3, []plugins.Entry{e})
		},
		"pack for another app version": func() {
			f := packFile(t, "fr", "1.1.0")
			ps.set("languages/fr/1.2.1.json.bz2", f)
			ps.lists(3, []plugins.Entry{{ID: "lang-fr", Version: "1.2.1", SHA256: sum(f)}})
		},
		"pack for another language": func() {
			f := packFile(t, "de", "1.2.1")
			ps.set("languages/fr/1.2.1.json.bz2", f)
			ps.lists(3, []plugins.Entry{{ID: "lang-fr", Version: "1.2.1", SHA256: sum(f)}})
		},
		"blocked": func() {
			ps.lists(3, []plugins.Entry{ps.publish(t, "fr", "1.2.1")}, plugins.Entry{ID: "lang-fr", Version: "1.2.1"})
		},
		"only a newer version listed": func() {
			ps.lists(3, []plugins.Entry{ps.publish(t, "fr", "1.2.2")})
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			setup()
			l := newLanguages(t, ps, "1.2.1")
			if err := l.Install(context.Background(), "fr"); err == nil {
				t.Fatal("install accepted")
			}
			if _, _, err := l.Pack("fr"); !errors.Is(err, plugins.ErrNotInstalled) {
				t.Fatalf("pack served after a refused install: %v", err)
			}
			if rows := l.List(); rows[indexOf(rows, "fr")].State != "failed" {
				t.Fatalf("failed install not shown: %+v", rows[indexOf(rows, "fr")])
			}
		})
	}
	l := newLanguages(t, ps, "1.2.1")
	if err := l.Install(context.Background(), "xx"); !errors.Is(err, plugins.ErrNotListed) {
		t.Fatalf("unknown language: %v", err)
	}
}

func indexOf(rows []plugins.LanguageRow, code string) int {
	for i, r := range rows {
		if r.Code == code {
			return i
		}
	}
	return -1
}

func TestLanguageChangedOnDiskIsNotServed(t *testing.T) {
	ps := newPackServer(t)
	ps.lists(1, []plugins.Entry{ps.publish(t, "it", "1.2.1")})
	l := newLanguages(t, ps, "1.2.1")
	if err := l.Install(context.Background(), "it"); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(l.Dir, "it", "1.2.1.json.bz2")
	if err := os.WriteFile(p, packFile(t, "it", "1.2.1")[:20], 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.Pack("it"); !errors.Is(err, plugins.ErrNotInstalled) {
		t.Fatalf("changed pack served: %v", err)
	}
}

func TestLanguageOfflineShowsNothingInstalled(t *testing.T) {
	ps := newPackServer(t)
	l := newLanguages(t, ps, "1.2.1")
	ps.srv.Close()
	if err := l.Install(context.Background(), "de"); !errors.Is(err, plugins.ErrFetch) {
		t.Fatalf("offline install: %v", err)
	}
	if err := l.Ensure(context.Background(), []string{"de"}); err == nil {
		t.Fatal("ensure offline reported success")
	}
}

func TestLanguageEnsureFollowsTheAppVersion(t *testing.T) {
	ps := newPackServer(t)
	ps.lists(1, []plugins.Entry{ps.publish(t, "de", "1.2.0"), ps.publish(t, "es", "1.2.0")})
	old := newLanguages(t, ps, "1.2.0")
	if err := old.Ensure(context.Background(), []string{"de", "en", ""}); err != nil {
		t.Fatal(err)
	}
	if old.NewVersion() {
		t.Fatal("1.2.0 still seen as new after its packs were installed")
	}
	// The app is updated: the installed pack follows it, and a newly asked language joins.
	ps.lists(2, []plugins.Entry{ps.publish(t, "de", "1.2.1"), ps.publish(t, "es", "1.2.1")})
	l := &plugins.Languages{Dir: old.Dir, Engine: &plugins.Engine{Dir: old.Engine.Dir, ListURL: old.Engine.ListURL, Keys: old.Engine.Keys, HTTP: old.Engine.HTTP, AppVersion: "1.2.1"}}
	if !l.NewVersion() {
		t.Fatal("first start of 1.2.1 not seen as a new version")
	}
	if err := l.Ensure(context.Background(), []string{"es"}); err != nil {
		t.Fatal(err)
	}
	if l.NewVersion() {
		t.Fatal("still a new version after the packs were brought up to date")
	}
	for _, c := range []string{"de", "es"} {
		if m := plain(t, l, c); m["app"] != "1.2.1" {
			t.Fatalf("%s pack is for %v", c, m["app"])
		}
	}
	if _, err := os.Stat(filepath.Join(l.Dir, "de", "1.2.0.json.bz2")); !os.IsNotExist(err) {
		t.Fatalf("old pack kept: %v", err)
	}
	// A second Ensure with everything current asks the server nothing.
	ps.mu.Lock()
	before := ps.hits["/languages.json"]
	ps.mu.Unlock()
	if err := l.Ensure(context.Background(), []string{"de", "es"}); err != nil {
		t.Fatal(err)
	}
	ps.mu.Lock()
	after := ps.hits["/languages.json"]
	ps.mu.Unlock()
	if after != before {
		t.Fatal("Ensure fetched the list with every pack current")
	}
}

func TestLanguageDevBuildTakesNewest(t *testing.T) {
	ps := newPackServer(t)
	ps.lists(1, []plugins.Entry{ps.publish(t, "nl", "1.2.0"), ps.publish(t, "nl", "1.2.1")})
	l := newLanguages(t, ps, "dev")
	if err := l.Install(context.Background(), "nl"); err != nil {
		t.Fatal(err)
	}
	if m := plain(t, l, "nl"); m["app"] != "1.2.1" {
		t.Fatalf("dev build took %v", m["app"])
	}
}

func TestLanguageHandlers(t *testing.T) {
	ps := newPackServer(t)
	ps.lists(1, []plugins.Entry{ps.publish(t, "de", "1.2.1")})
	l := newLanguages(t, ps, "1.2.1")
	m := http.NewServeMux()
	(&plugins.LanguageHandlers{Languages: l}).Routes(m)
	do := func(method, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		m.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return w
	}
	if w := do("GET", "/api/v1/languages/de/pack"); w.Code != http.StatusNotFound {
		t.Fatalf("pack before install: %d", w.Code)
	}
	if w := do("POST", "/api/v1/languages/de"); w.Code != http.StatusOK {
		t.Fatalf("install: %d %s", w.Code, w.Body)
	}
	w := do("GET", "/api/v1/languages/de/pack")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"lang":"de"`) || w.Header().Get("ETag") == "" {
		t.Fatalf("pack: %d %q %s", w.Code, w.Header().Get("ETag"), w.Body)
	}
	if w := do("POST", "/api/v1/languages/xx"); w.Code != http.StatusNotFound {
		t.Fatalf("unknown language: %d", w.Code)
	}
	var got struct {
		Languages []plugins.LanguageRow `json:"languages"`
	}
	w = do("GET", "/api/v1/languages")
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || len(got.Languages) != 44 {
		t.Fatalf("list: %v %d", err, len(got.Languages))
	}
	if w := do("DELETE", "/api/v1/languages/de"); w.Code != http.StatusOK {
		t.Fatalf("remove: %d", w.Code)
	}
}

// The packs are plain bzip2: Go's reader opens what the system tool wrote.
func TestLanguagePackIsBzip2(t *testing.T) {
	f := packFile(t, "de", "1.2.1")
	b, err := io.ReadAll(bzip2.NewReader(bytes.NewReader(f)))
	if err != nil || !bytes.Contains(b, []byte(`"lang":"de"`)) {
		t.Fatalf("%v %s", err, b)
	}
}

// An install from before language packs installs browsers' languages without
// asking; a new one asks. The answer is kept from the first start.
func TestLanguageAutoBrowserKeptFromFirstStart(t *testing.T) {
	ps := newPackServer(t)
	for _, upgraded := range []bool{true, false} {
		l := newLanguages(t, ps, "1.2.1")
		l.Upgraded = func() bool { return upgraded }
		if l.AutoBrowser() != upgraded {
			t.Fatalf("upgraded=%v: auto_browser %v", upgraded, !upgraded)
		}
		again := &plugins.Languages{Dir: l.Dir, Engine: l.Engine, Upgraded: func() bool { return !upgraded }}
		if again.AutoBrowser() != upgraded {
			t.Fatalf("upgraded=%v: answer not kept", upgraded)
		}
	}
}

func TestLanguageRecheckDropsWithdrawnPacks(t *testing.T) {
	ps := newPackServer(t)
	de, fr, es := ps.publish(t, "de", "1.2.1"), ps.publish(t, "fr", "1.2.1"), ps.publish(t, "es", "1.2.1")
	ps.lists(1, []plugins.Entry{de, fr, es})
	l := newLanguages(t, ps, "1.2.1")
	if err := l.Ensure(context.Background(), []string{"de", "fr", "es"}); err != nil {
		t.Fatal(err)
	}
	if err := l.Recheck(context.Background()); err != nil {
		t.Fatal(err)
	}
	plain(t, l, "de")
	// fr is blocked by hash, es leaves the list, de stays.
	ps.lists(2, []plugins.Entry{de, fr}, plugins.Entry{SHA256: fr.SHA256})
	if err := l.Recheck(context.Background()); err != nil {
		t.Fatal(err)
	}
	plain(t, l, "de")
	for _, c := range []string{"fr", "es"} {
		if _, _, err := l.Pack(c); !errors.Is(err, plugins.ErrNotInstalled) {
			t.Fatalf("%s still served after withdrawal: %v", c, err)
		}
	}
	// An older list is refused, not used to drop more.
	ps.lists(1, nil)
	if err := l.Recheck(context.Background()); err == nil {
		t.Fatal("older lists accepted")
	}
	plain(t, l, "de")
}

func TestLanguageVersionMustBeAFileName(t *testing.T) {
	ps := newPackServer(t)
	en := ps.publish(t, "de", "1.2.1")
	en.Version = "../x"
	ps.lists(1, []plugins.Entry{en})
	l := newLanguages(t, ps, "dev")
	if err := l.Install(context.Background(), "de"); err == nil {
		t.Fatal("path-like version installed")
	}
}

func TestLanguagesListShowsSizeBeforeInstall(t *testing.T) {
	ps := newPackServer(t)
	fr := ps.publish(t, "fr", "1.2.1")
	fr.Size = 4321
	ps.lists(1, []plugins.Entry{fr})
	l := newLanguages(t, ps, "1.2.1")
	if rows := l.List(); rows[indexOf(rows, "fr")].Size != 0 {
		t.Fatalf("size before the lists were read: %d", rows[indexOf(rows, "fr")].Size)
	}
	l.LoadSizes(context.Background())
	if r := l.List()[indexOf(l.List(), "fr")]; r.State != "available" || r.Size != 4321 {
		t.Fatalf("fr row %+v", r)
	}
}

// A damaged state.json must not forget the packs on disk: they are read
// back from their files, and the damaged copy is kept beside it.
func TestLanguageDamagedStateKeepsPacks(t *testing.T) {
	ps := newPackServer(t)
	ps.lists(2, []plugins.Entry{ps.publish(t, "de", "1.2.1")})
	l := newLanguages(t, ps, "1.2.1")
	if err := l.Install(context.Background(), "de"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.Dir, "state.json"), []byte("{\"packs\": {\"de\""), 0o600); err != nil {
		t.Fatal(err)
	}
	again := &plugins.Languages{Dir: l.Dir, Engine: l.Engine}
	if m := plain(t, again, "de"); m["lang"] != "de" {
		t.Fatalf("served pack %v", m)
	}
	if _, err := os.Stat(filepath.Join(l.Dir, "state.json.bad")); err != nil {
		t.Fatalf("damaged copy not kept: %v", err)
	}
	// A file that isn't a good pack is not taken back.
	os.MkdirAll(filepath.Join(l.Dir, "fr"), 0o700)
	os.WriteFile(filepath.Join(l.Dir, "fr", "1.2.1.json.bz2"), []byte("junk"), 0o600)
	os.WriteFile(filepath.Join(l.Dir, "state.json"), []byte("nope"), 0o600)
	third := &plugins.Languages{Dir: l.Dir, Engine: l.Engine}
	if _, _, err := third.Pack("fr"); !errors.Is(err, plugins.ErrNotInstalled) {
		t.Fatalf("junk pack taken back: %v", err)
	}
	if _, _, err := third.Pack("de"); err != nil {
		t.Fatalf("good pack lost on second damage: %v", err)
	}
}

// The open setup route answers from what is installed instead of fetching again.
func TestLanguageSetupInstallSkipsCurrentPack(t *testing.T) {
	ps := newPackServer(t)
	ps.lists(1, []plugins.Entry{ps.publish(t, "de", "1.2.1")})
	l := newLanguages(t, ps, "1.2.1")
	m := http.NewServeMux()
	(&plugins.LanguageHandlers{Languages: l, SetupOpen: func(http.ResponseWriter, *http.Request) bool { return true }}).Routes(m)
	do := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		m.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/auth/setup/language/de", nil))
		return w
	}
	if w := do(); w.Code != http.StatusOK {
		t.Fatalf("first install: %d %s", w.Code, w.Body)
	}
	ps.mu.Lock()
	before := 0
	for _, n := range ps.hits {
		before += n
	}
	ps.mu.Unlock()
	w := do()
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"languages"`) {
		t.Fatalf("second install: %d %s", w.Code, w.Body)
	}
	ps.mu.Lock()
	after := 0
	for _, n := range ps.hits {
		after += n
	}
	ps.mu.Unlock()
	if after != before {
		t.Fatalf("second call fetched again: %d requests before, %d after", before, after)
	}
}

// a release with no packs of its own yet takes the newest earlier
// release pack, never a newer one or a test build's, and keeps it without
// fetching it again.
func TestLanguageReleaseFallsBackToEarlierPack(t *testing.T) {
	ps := newPackServer(t)
	ps.lists(1, []plugins.Entry{
		ps.publish(t, "fr", "1.2.9"), ps.publish(t, "fr", "1.3.0"),
		ps.publish(t, "fr", "1.3.1-test.1"), ps.publish(t, "fr", "1.3.2"),
	})
	l := newLanguages(t, ps, "1.3.1")
	if err := l.Install(context.Background(), "fr"); err != nil {
		t.Fatal(err)
	}
	if m := plain(t, l, "fr"); m["app"] != "1.3.0" {
		t.Fatalf("1.3.1 took the %v pack", m["app"])
	}
	ps.mu.Lock()
	before := ps.hits["/languages/fr/1.3.0.json.bz2"]
	ps.mu.Unlock()
	if err := l.Ensure(context.Background(), []string{"fr"}); err != nil {
		t.Fatal(err)
	}
	ps.mu.Lock()
	after := ps.hits["/languages/fr/1.3.0.json.bz2"]
	ps.mu.Unlock()
	if before != 1 || after != before {
		t.Fatalf("pack fetched %d then %d times", before, after)
	}
	// Once the release's own pack is listed, it replaces the fallback.
	ps.lists(2, []plugins.Entry{ps.publish(t, "fr", "1.3.0"), ps.publish(t, "fr", "1.3.1")})
	if err := l.Ensure(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if m := plain(t, l, "fr"); m["app"] != "1.3.1" {
		t.Fatalf("own pack not taken: %v", m["app"])
	}
}
