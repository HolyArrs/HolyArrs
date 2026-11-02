package control

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSwitchFileAndDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "control.json")
	s := New(p)
	if s.Get().Prefetch != "on" || !s.Provider("caa") || !s.Lane("p2") || s.Get().P0MaxRPS != 0.54 {
		t.Fatalf("defaults %+v", s.Get())
	}
	os.WriteFile(p, []byte(`{"prefetch":"off","providers":{"caa":false},"queue":{"p2":false},"p0_max_rps":0.2}`), 0o600)
	s.Reload()
	if s.Get().Prefetch != "off" || s.Provider("caa") || !s.Provider("deezer") || s.Lane("p2") || s.Get().P0MaxRPS != 0.2 {
		t.Fatalf("file %+v", s.Get())
	}
	_, e1 := s.Body()
	os.WriteFile(p, []byte(`{broken`), 0o600)
	s.Reload()
	if s.Get().Prefetch != "off" {
		t.Fatal("bad file dropped the last good values")
	}
	if _, e2 := s.Body(); e1 != e2 {
		t.Fatal("etag changed without a change")
	}
}
