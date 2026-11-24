package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

var testPass = []byte("test passphrase only")

func setup(t *testing.T) (dir, payload string) {
	t.Helper()
	dir = t.TempDir()
	if _, err := keygen(dir, testPass); err != nil {
		t.Fatal(err)
	}
	payload = filepath.Join(dir, "payload.json")
	body := `{"kind":"blocklist","counter":5,"entries":[{"id":"bad-plugin","version":"","sha256":""}]}`
	if err := os.WriteFile(payload, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, payload
}

func TestRoundTrip(t *testing.T) {
	dir, payload := setup(t)
	out := filepath.Join(dir, "list.json")
	if err := sign(filepath.Join(dir, keyFile), testPass, plugins.KindBlocklist, payload, out, 4); err != nil {
		t.Fatal(err)
	}
	l, err := verify(filepath.Join(dir, pubFile), out)
	if err != nil {
		t.Fatal(err)
	}
	if l.Counter != 5 || len(l.Entries) != 1 {
		t.Fatalf("got %+v", l)
	}
}

func TestMinCounter(t *testing.T) {
	dir, payload := setup(t)
	err := sign(filepath.Join(dir, keyFile), testPass, plugins.KindBlocklist, payload, filepath.Join(dir, "l.json"), 5)
	if err == nil {
		t.Fatal("signed a counter that is not above -min-counter")
	}
}

func TestNoOverwrite(t *testing.T) {
	dir, _ := setup(t)
	before, _ := os.ReadFile(filepath.Join(dir, keyFile))
	if _, err := keygen(dir, testPass); err == nil {
		t.Fatal("keygen overwrote an existing key")
	}
	after, _ := os.ReadFile(filepath.Join(dir, keyFile))
	if string(before) != string(after) {
		t.Fatal("key file changed")
	}
}

func TestWrongPassphrase(t *testing.T) {
	dir, payload := setup(t)
	err := sign(filepath.Join(dir, keyFile), []byte("not the passphrase"), plugins.KindBlocklist, payload, filepath.Join(dir, "l.json"), 0)
	if !errors.Is(err, errPassphrase) {
		t.Fatalf("got %v", err)
	}
}

func TestTampered(t *testing.T) {
	dir, payload := setup(t)
	out := filepath.Join(dir, "list.json")
	if err := sign(filepath.Join(dir, keyFile), testPass, plugins.KindBlocklist, payload, out, 0); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	var sl plugins.SignedList
	if err := json.Unmarshal(data, &sl); err != nil {
		t.Fatal(err)
	}
	sl.Payload = []byte(strings.Replace(string(sl.Payload), `"counter":5`, `"counter":9`, 1))
	data, _ = json.Marshal(sl)
	if err := os.WriteFile(out, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := verify(filepath.Join(dir, pubFile), out); !errors.Is(err, plugins.ErrListSignature) {
		t.Fatalf("got %v", err)
	}
}

func TestPassEnv(t *testing.T) {
	t.Setenv("PLUGINKEY_TEST_PASS", string(testPass))
	p, err := readPassphrase("", "PLUGINKEY_TEST_PASS")
	if err != nil || string(p) != string(testPass) {
		t.Fatalf("got %q, %v", p, err)
	}
	t.Setenv("PLUGINKEY_TEST_PASS", "")
	if _, err := readPassphrase("", "PLUGINKEY_TEST_PASS"); err == nil {
		t.Fatal("accepted an empty passphrase variable")
	}
}

func TestCheck(t *testing.T) {
	dir, _ := setup(t)
	key, pub := filepath.Join(dir, keyFile), filepath.Join(dir, pubFile)
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	file := write("plugin", "plugin bytes")
	fileSum := sha256.Sum256([]byte("plugin bytes"))
	man := `{"id":"demo","name":"Demo","version":"1.0.0","author":"x","access":{"hosts":[],"folders":[],"actions":[]},"min_app_version":"1.2.0","sha256":"` + hex.EncodeToString(fileSum[:]) + `"}`
	manPath := write("manifest.json", man)
	manSum := sha256.Sum256([]byte(man))
	signed := func(name, kind, body string) string {
		in := write(name+".payload", body)
		out := filepath.Join(dir, name)
		if err := sign(key, testPass, kind, in, out, 0); err != nil {
			t.Fatal(err)
		}
		return out
	}
	list := signed("plugins.json", plugins.KindPlugins, `{"kind":"plugins","counter":1,"min_blocklist_counter":1,"entries":[{"id":"demo","version":"1.0.0","sha256":"`+hex.EncodeToString(fileSum[:])+`","manifest_sha256":"`+hex.EncodeToString(manSum[:])+`"}]}`)
	open := signed("open.json", plugins.KindBlocklist, `{"kind":"blocklist","counter":1,"entries":[]}`)
	if _, err := check(pub, list, open, manPath, file); err != nil {
		t.Fatal(err)
	}
	blocked := signed("blocked.json", plugins.KindBlocklist, `{"kind":"blocklist","counter":2,"entries":[{"id":"demo","version":"1.0.0","sha256":""}]}`)
	if _, err := check(pub, list, blocked, manPath, file); !errors.Is(err, plugins.ErrBlocked) {
		t.Fatalf("got %v, want blocked", err)
	}
}

// A plain key signs with no passphrase and verifies with its public key.
func TestPlainKey(t *testing.T) {
	dir := t.TempDir()
	pubPath := filepath.Join(dir, pubFile)
	var secret bytes.Buffer
	pub, err := keygenPlain(&secret, pubPath)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(dir, "secret.key")
	if err := os.WriteFile(keyPath, secret.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if sealed, err := isSealed(keyPath); err != nil || sealed {
		t.Fatalf("plain key seen as sealed: %v", err)
	}
	payload := filepath.Join(dir, "payload.json")
	if err := os.WriteFile(payload, []byte(`{"kind":"plugins","counter":1,"min_blocklist_counter":0,"entries":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "plugins.json")
	if err := sign(keyPath, nil, plugins.KindPlugins, payload, out, 0); err != nil {
		t.Fatal(err)
	}
	l, err := verify(pubPath, out)
	if err != nil || l.Counter != 1 {
		t.Fatalf("got %+v, %v", l, err)
	}
	data, _ := os.ReadFile(out)
	if _, err := plugins.VerifyList(pub, data, plugins.KindPlugins, 0); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), strings.TrimSpace(secret.String())) {
		t.Fatal("the signed list holds the private key")
	}
}

func TestSignsLanguageList(t *testing.T) {
	dir, _ := setup(t)
	payload := filepath.Join(dir, "languages.json")
	body := `{"kind":"languages","counter":2,"min_blocklist_counter":1,"entries":[{"id":"lang-de","version":"1.2.1","sha256":"` + strings.Repeat("a", 64) + `"}]}`
	if err := os.WriteFile(payload, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "signed.json")
	if err := sign(filepath.Join(dir, keyFile), testPass, plugins.KindLanguages, payload, out, 0); err != nil {
		t.Fatal(err)
	}
	if l, err := verify(filepath.Join(dir, pubFile), out); err != nil || l.Kind != plugins.KindLanguages {
		t.Fatalf("%v %+v", err, l)
	}
}
