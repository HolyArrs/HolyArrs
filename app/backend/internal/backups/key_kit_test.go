package backups

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

const keyKitPath = "/api/v1/system/key-kit"

// keyPrint is the frozen fingerprint format: the first 8 hex digits of
// SHA-256 over the 32 key bytes.
func keyPrint(key []byte) string {
	s := sha256.Sum256(key)
	return hex.EncodeToString(s[:])[:8]
}

func labKey(seed byte) []byte {
	k := make([]byte, secrets.KeySize)
	for i := range k {
		k[i] = seed + byte(i)
	}
	return k
}

type keyKitState struct {
	Needed      bool   `json:"needed"`
	Fingerprint string `json:"fingerprint"`
}

func listKeyKit(t *testing.T, url string) keyKitState {
	t.Helper()
	resp, err := http.Get(url + "/api/v1/backups")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("list: %v %v", resp, err)
	}
	defer resp.Body.Close()
	var l struct {
		KeyKit *keyKitState `json:"key_kit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&l); err != nil {
		t.Fatal(err)
	}
	if l.KeyKit == nil {
		t.Fatal("the backups list has no key_kit state for the Backups card")
	}
	return *l.KeyKit
}

func keyKitServer(t *testing.T, m *Manager) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	m.Routes(mux, func() {})
	srv := httptest.NewServer(webserver.RecordRouteHits(mux))
	t.Cleanup(srv.Close)
	return srv
}

// route: POST /api/v1/system/key-kit
//
// The key kit is a download of the master key that is itself a valid key
// file: restoring means renaming it to holyarrs.key. It is never cached,
// lands on the audit trail without the key, and the Backups card warns
// until a kit matching the current key has been downloaded (also after a
// restart, and again once the key changes).
func TestKeyKitDownload(t *testing.T) {
	m, s, _, _ := setup(t)
	key := labKey(11)
	m.Key = key
	srv := keyKitServer(t, m)

	if st := listKeyKit(t, srv.URL); !st.Needed || st.Fingerprint != keyPrint(key) {
		t.Fatalf("before any download: %+v, want needed with fingerprint %s", st, keyPrint(key))
	}

	resp, err := http.Post(srv.URL+keyKitPath, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("key kit: %d %s", resp.StatusCode, body)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control %q, want no-store", cc)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") || !strings.Contains(cd, ".key") {
		t.Errorf("Content-Disposition %q, want an attachment named *.key", cd)
	}
	text := string(body)
	if !strings.HasPrefix(text, "#") || !strings.Contains(text, keyPrint(key)) || !strings.Contains(text, secrets.KeyFile) {
		t.Errorf("kit should open with # lines naming the fingerprint and %s:\n%s", secrets.KeyFile, text)
	}
	// The kit, renamed, is the key file.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, secrets.KeyFile), body, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := secrets.LoadKey(dir, "")
	if err != nil || string(got) != string(key) {
		t.Fatalf("the kit is not a working key file: %x %v", got, err)
	}

	// On the audit trail, without the key.
	evs, err := s.RecentEvents(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range evs {
		blob, _ := json.Marshal(e)
		if strings.Contains(string(blob), base64.StdEncoding.EncodeToString(key)) || strings.Contains(string(blob), hex.EncodeToString(key)) {
			t.Fatalf("event holds the key: %s", blob)
		}
		if strings.Contains(strings.ToLower(e.Summary), "key") {
			found = true
		}
	}
	if !found {
		t.Errorf("no audit event for the key kit download in %+v", evs)
	}

	if st := listKeyKit(t, srv.URL); st.Needed || st.Fingerprint != keyPrint(key) {
		t.Fatalf("after the download: %+v, want not needed", st)
	}
	// Kept across a restart (a new Manager on the same database).
	m2 := &Manager{Store: s, Dir: m.Dir, Version: m.Version, Now: m.Now, Key: key}
	if st := listKeyKit(t, keyKitServer(t, m2).URL); st.Needed {
		t.Fatalf("after a restart the card warns again: %+v", st)
	}
	// A different key (say, put back from elsewhere) needs its own copy.
	other := labKey(99)
	m3 := &Manager{Store: s, Dir: m.Dir, Version: m.Version, Now: m.Now, Key: other}
	if st := listKeyKit(t, keyKitServer(t, m3).URL); !st.Needed || st.Fingerprint != keyPrint(other) {
		t.Fatalf("another key: %+v, want needed with fingerprint %s", st, keyPrint(other))
	}
}

// API keys never get the master key, whatever their scope; nor does any
// sign-in that is not a browser session.
func TestKeyKitRefusesAPIKeys(t *testing.T) {
	m, _, _, _ := setup(t)
	m.Key = labKey(5)
	mux := http.NewServeMux()
	m.Routes(mux, func() {})
	for _, who := range []webserver.Caller{
		{UserID: 1, Via: "api_key", Role: webserver.RoleOwner, Perms: webserver.DefaultPerms(webserver.RoleOwner), Scopes: []string{webserver.ScopeAll}},
		{UserID: 1, Via: "local_network", Role: webserver.RoleOwner, Perms: webserver.DefaultPerms(webserver.RoleOwner)},
	} {
		r := httptest.NewRequest(http.MethodPost, keyKitPath, nil).WithContext(webserver.ContextWith(context.Background(), who))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s: %d %s, want 403", who.Via, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), base64.StdEncoding.EncodeToString(m.Key)) {
			t.Fatalf("%s got the key", who.Via)
		}
	}
}

// With HOLYARRS_MASTER_KEY_FILE the key lives outside the config folder and
// the person manages it: the Backups card does not nag.
func TestKeyKitCardSkippedForNamedKeyFile(t *testing.T) {
	m, _, _, _ := setup(t)
	m.Key = labKey(7)
	m.KeyFromEnv = true
	if st := listKeyKit(t, keyKitServer(t, m).URL); st.Needed {
		t.Fatalf("named key file: %+v, want not needed", st)
	}
}
