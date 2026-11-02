package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCPUProfileOnAsk(t *testing.T) {
	dir := t.TempDir()
	ask := make(chan os.Signal, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { cpuProfileOn(ctx, ask, dir, 50*time.Millisecond); close(done) }()
	ask <- os.Interrupt
	deadline := time.Now().Add(3 * time.Second)
	for {
		m, _ := filepath.Glob(filepath.Join(dir, "cpu-*.pprof"))
		if len(m) == 1 {
			if st, _ := os.Stat(m[0]); st != nil && st.Size() > 0 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("no profile written")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
}

func TestPruneProfilesKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"cpu-20261029T100000.pprof", "cpu-20261029T110000.pprof", "cpu-20261029T120000.pprof", "cpu-20261029T130000.pprof", "other.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pruneProfiles(dir, 2)
	m, _ := filepath.Glob(filepath.Join(dir, "cpu-*.pprof"))
	if len(m) != 2 || filepath.Base(m[0]) != "cpu-20261029T120000.pprof" {
		t.Fatalf("kept %v", m)
	}
	if _, err := os.Stat(filepath.Join(dir, "other.txt")); err != nil {
		t.Fatal("removed a file that is not a profile")
	}
}
