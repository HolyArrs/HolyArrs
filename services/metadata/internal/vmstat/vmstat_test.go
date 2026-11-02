package vmstat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSamplesHistoryAndAlert(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "proc"), 0o755)
	os.MkdirAll(filepath.Join(root, "sys", "fs", "cgroup"), 0o755)
	os.WriteFile(filepath.Join(root, "proc", "meminfo"), []byte("MemTotal:       1000 kB\nMemFree:         100 kB\nMemAvailable:    600 kB\n"), 0o644)
	os.WriteFile(filepath.Join(root, "sys", "fs", "cgroup", "memory.current"), []byte("4096\n"), 0o644)
	os.WriteFile(filepath.Join(root, "sys", "fs", "cgroup", "memory.max"), []byte("max\n"), 0o644)
	now := time.Unix(1_800_000_000, 0)
	used := uint64(700)
	r, err := Open(filepath.Join(t.TempDir(), "vm.sqlite"), Options{Root: root, Now: func() time.Time { return now },
		Disk: func(string) (uint64, uint64, error) { return used, 1000, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ctx := context.Background()
	s := r.Record(ctx)
	if s.MemTotal != 1024000 || s.MemUsed != 409600 || s.MemFree != 614400 || s.ContainerMem != 4096 || s.ContainerLimit != 0 || s.DiskFree != 300 {
		t.Fatalf("sample %+v", s)
	}
	now = now.Add(5 * time.Minute)
	used = 850
	r.Record(ctx)
	rep := r.Report(ctx, time.Hour)
	if len(rep.Series) != 2 || rep.Series[1].DiskUsed != 850 {
		t.Fatalf("series %+v", rep.Series)
	}
	if len(rep.Warnings) != 1 || !strings.HasPrefix(rep.Warnings[0], "disk_usage_high") {
		t.Fatalf("warnings %v", rep.Warnings)
	}
	now = now.Add(91 * 24 * time.Hour)
	r.Record(ctx)
	if n := len(r.Report(ctx, 365*24*time.Hour).Series); n != 1 {
		t.Fatalf("history not pruned: %d", n)
	}
}
