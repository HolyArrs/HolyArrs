package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

func modeRig(t *testing.T, env map[string]string) (*ModeHandlers, *httptest.Server) {
	t.Helper()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	v, err := secrets.Open(context.Background(), st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	h := &ModeHandlers{Modes: NewModes(envOf(env)), Store: st, Vault: v,
		Test: func(_ context.Context, key, value, pin string) error {
			switch {
			case key == KeyTVDB && pin != "right-pin":
				return ErrTVDBRefused
			case value == "bad":
				return ErrNoAccess
			}
			return nil
		}}
	m := http.NewServeMux()
	h.Routes(m)
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	return h, srv
}

func modeCall(t *testing.T, srv *httptest.Server, method, path, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestSettingsMetadataPickAndKeys(t *testing.T) {
	h, srv := modeRig(t, nil)
	ctx := context.Background()
	// An update with no stored setting is on the metadata server.
	if m, ok := StoredMode(ctx, h.Store.Read()); ok || m != ModeServer {
		t.Fatalf("no row: %q %v", m, ok)
	}
	st, a := modeCall(t, srv, "GET", "/api/v1/settings/metadata", "")
	if st != 200 || a["mode"] != "full" || a["locked"] != false || a["note"] != nil {
		t.Fatalf("default: %d %v", st, a)
	}
	st, a = modeCall(t, srv, "PUT", "/api/v1/settings/metadata", `{"mode":"own","keys":{"`+KeyTMDB+`":"own-tmdb-fixture","mdblist_key":"  "}}`)
	if st != 200 || a["mode"] != "own" || h.Modes.Current() != ModeOwn {
		t.Fatalf("own: %d %v", st, a)
	}
	keys := a["keys"].(map[string]any)
	if keys["tmdb_token"] != true || keys["mdblist_key"] != false {
		t.Fatalf("keys %v", keys)
	}
	if strings.Contains(srvBody(t, srv), "own-tmdb-fixture") {
		t.Fatal("a key came back in the answer")
	}
	if a["note"] != OffNote {
		t.Fatalf("own keys leaves rows off: %v", a["note"])
	}
	// The masked value sent back keeps the key.
	modeCall(t, srv, "PUT", "/api/v1/settings/metadata", `{"keys":{"`+KeyTMDB+`":"`+secrets.Masked+`"}}`)
	if v, _ := h.Vault.Setting(ctx, KeyGroup, KeyTMDB); v != "own-tmdb-fixture" {
		t.Fatalf("mask overwrote the key: %q", v)
	}
	if m, ok := StoredMode(ctx, h.Store.Read()); !ok || m != ModeOwn {
		t.Fatalf("stored: %q %v", m, ok)
	}
	// A restart reads it back.
	again := NewModes(envOf(nil))
	again.LoadMode(ctx, h.Store.Read())
	if again.Current() != ModeOwn {
		t.Fatalf("after a restart: %q", again.Current())
	}
	if st, _ := modeCall(t, srv, "PUT", "/api/v1/settings/metadata", `{"mode":"sometimes"}`); st != 400 {
		t.Fatalf("unknown mode: %d", st)
	}
	if st, _ := modeCall(t, srv, "PUT", "/api/v1/settings/metadata", `{"keys":{"apple_key":"x"}}`); st != 400 {
		t.Fatalf("unknown key: %d", st)
	}
}

func srvBody(t *testing.T, srv *httptest.Server) string {
	resp, err := http.Get(srv.URL + "/api/v1/settings/metadata")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestSettingsMetadataLockedByDocker(t *testing.T) {
	h, srv := modeRig(t, map[string]string{EnvMode: "own"})
	st, a := modeCall(t, srv, "GET", "/api/v1/settings/metadata", "")
	if st != 200 || a["locked"] != true || a["locked_note"] != "Set by your Docker settings" || a["mode"] != "own" {
		t.Fatalf("%d %v", st, a)
	}
	if st, a := modeCall(t, srv, "PUT", "/api/v1/settings/metadata", `{"mode":"off"}`); st != 409 || h.Modes.Current() != ModeOwn {
		t.Fatalf("locked: %d %v", st, a)
	}
	// Keys can still be saved while the mode is locked.
	if st, a := modeCall(t, srv, "PUT", "/api/v1/settings/metadata", `{"keys":{"fanart_key":"fan"}}`); st != 200 || a["keys"].(map[string]any)["fanart_key"] != true {
		t.Fatalf("keys while locked: %d %v", st, a)
	}
}

func TestSettingsMetadataTestKey(t *testing.T) {
	h, srv := modeRig(t, nil)
	ctx := context.Background()
	for _, c := range []struct{ body, want string }{
		{`{"key":"tmdb_token"}`, "No key saved yet"},
		{`{"key":"tmdb_token","value":"good"}`, "Key works"},
		{`{"key":"mdblist_key","value":"bad"}`, "Key refused"},
		{`{"key":"tvdb_key","value":"good","pin":"wrong"}`, "Key or PIN refused"},
		{`{"key":"tvdb_key","value":"good","pin":"right-pin"}`, "Key works"},
	} {
		st, a := modeCall(t, srv, "POST", "/api/v1/settings/metadata/test", c.body)
		if st != 200 || a["message"] != c.want {
			t.Errorf("%s: %d %v", c.body, st, a)
		}
	}
	_ = h.Vault.SetSetting(ctx, KeyGroup, KeyTVDB, "good", database.System)
	_ = h.Vault.SetSetting(ctx, KeyGroup, KeyTVDBPIN, "right-pin", database.System)
	if _, a := modeCall(t, srv, "POST", "/api/v1/settings/metadata/test", `{"key":"tvdb_key"}`); a["message"] != "Key works" {
		t.Fatalf("saved key and PIN: %v", a)
	}
	if st, _ := modeCall(t, srv, "POST", "/api/v1/settings/metadata/test", `{"key":"tvdb_pin"}`); st != 400 {
		t.Fatalf("the PIN alone is not a key: %d", st)
	}
	if !errors.Is(ErrTVDBRefused, ErrNoTVDBKey) {
		t.Fatal("a refused PIN still reads as no usable key")
	}
}

func TestSourceTableStates(t *testing.T) {
	none := func(string) bool { return false }
	all := func(string) bool { return true }
	if !SourceOn("tmdb", ModeServer, none) || SourceOn("tmdb", ModeOwn, none) || !SourceOn("tmdb", ModeOwn, all) || SourceOn("tmdb", ModeOff, all) {
		t.Fatal("tmdb")
	}
	if SourceOn("charts", ModeOwn, all) || !SourceOn("charts", ModeServer, none) {
		t.Fatal("charts come only from the server")
	}
	if SourceOn("tvdb", ModeServer, none) || !SourceOn("tvdb", ModeServer, all) || SourceOn("tvdb", ModeOff, all) {
		t.Fatal("tvdb needs its key, in every mode but off")
	}
	hosts := strings.Join(ModeHosts(ModeServer, none), " ")
	if !strings.Contains(hosts, "api.holyarrs.com") || strings.Contains(hosts, "api.themoviedb.org") {
		t.Fatalf("server hosts: %s", hosts)
	}
	own := strings.Join(ModeHosts(ModeOwn, all), " ")
	if strings.Contains(own, "api.holyarrs.com") || !strings.Contains(own, "api.themoviedb.org") || !strings.Contains(own, "musicbrainz.org") {
		t.Fatalf("own hosts: %s", own)
	}
	if len(ModeHosts(ModeOff, all)) != 0 {
		t.Fatalf("off asks nothing: %v", ModeHosts(ModeOff, all))
	}
}
