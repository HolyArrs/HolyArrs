package main

import (
	"os"
	"path/filepath"
	"testing"
)

var testPass = []byte("test passphrase only")

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, err := keygen(dir, testPass); err != nil {
		t.Fatal(err)
	}
	list := filepath.Join(dir, "list.json")
	if err := os.WriteFile(list, []byte(`{"schema":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sig := list + ".minisig"
	if err := sign(filepath.Join(dir, keyFile), testPass, list, sig); err != nil {
		t.Fatal(err)
	}
	if _, err := verify(filepath.Join(dir, pubFile), list, sig); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(list, []byte(`{"schema":2}`), 0o644)
	if _, err := verify(filepath.Join(dir, pubFile), list, sig); err == nil {
		t.Fatal("a changed list still verified")
	}
}

func TestWrongPassphrase(t *testing.T) {
	dir := t.TempDir()
	if _, err := keygen(dir, testPass); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadKey(filepath.Join(dir, keyFile), []byte("not the passphrase")); err != errPassphrase {
		t.Fatalf("got %v", err)
	}
}

func TestNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	if _, err := keygen(dir, testPass); err != nil {
		t.Fatal(err)
	}
	if _, err := keygen(dir, testPass); err == nil {
		t.Fatal("keygen overwrote an existing key")
	}
}
