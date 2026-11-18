package main

import (
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
