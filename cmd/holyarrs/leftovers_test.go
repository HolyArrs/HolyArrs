package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A start removes what a cut-off backup or restore upload left in the config
// folder, before the store opens.
func TestStartRemovesCutOffBackupLeftovers(t *testing.T) {
	dir := t.TempDir()
	left := []string{filepath.Join(dir, ".backup-1", "holyarrs.db"), filepath.Join(dir, ".restore-2", "upload.zip"),
		filepath.Join(dir, "backups", "holyarrs-backup-manual-20261101T000000.000Z.zip.part")}
	keep := filepath.Join(dir, "backups", "before-restore-20261101T000000.000Z", "holyarrs.db")
	for _, p := range append(left, keep) {
		_ = os.MkdirAll(filepath.Dir(p), 0o750)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	_ = l.Close()
	c, err := parseFlags([]string{"-config", dir, "-addr", addr}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, c, runOpts{}) }()
	defer func() { cancel(); <-done }()
	for i := 0; i < 200; i++ {
		if resp, err := http.Get("http://" + addr + "/healthz"); err == nil {
			resp.Body.Close()
			if !present(left[0]) && !present(left[2]) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, p := range left {
		if present(p) {
			t.Errorf("%s still there", p)
		}
	}
	if !present(keep) {
		t.Error("before-restore folder was removed")
	}
}

func present(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
