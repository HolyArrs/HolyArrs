package mbdump

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func latestIndex(t *testing.T, date string) HTTPIndex {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/LATEST" {
			fmt.Fprintln(w, date)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return HTTPIndex{Base: srv.URL}
}

// TestRefreshDropsLeftoverBuildBeforeCap: a dead build's *.building folder
// would put the data over the build cap; it is removed before the guard
// counts, so the refresh goes ahead.
func TestRefreshDropsLeftoverBuildBeforeCap(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"20261001-000000.building", "20261021-001500.building"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, d, "junk"), make([]byte, 4096), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := &Refresher{Root: root, Index: latestIndex(t, "20261021-001500"), Want: []string{"artist"},
		Guard: &Guard{Root: root, BuildLimit: 1024, Limit: 1 << 30},
		Build: func(ctx context.Context, date, dir string) (Result, error) {
			b, err := NewBuilder(Options{Dir: dir, DumpDate: date})
			if err != nil {
				return Result{}, err
			}
			if err := b.Read(ctx, strings.NewReader(string(dump(t, "31", 1)))); err != nil {
				return Result{}, err
			}
			return b.Finish(ctx)
		}}
	if _, swapped, err := r.Once(context.Background()); err != nil || !swapped {
		t.Fatalf("refresh blocked by leftover build: %v %v", swapped, err)
	}
	left, _ := filepath.Glob(filepath.Join(root, "*.building"))
	if len(left) != 0 {
		t.Fatalf("leftover builds kept: %v", left)
	}
}

// TestRefreshChecksCapAfterSwapAndSweep: with the data over the cap
// after a swap, the refresher logs the cap failure after the swap and
// again after the sweep.
func TestRefreshChecksCapAfterSwapAndSweep(t *testing.T) {
	root := t.TempDir()
	var mu sync.Mutex
	var logs []string
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &Refresher{Root: root, Index: latestIndex(t, "20261021-001500"), Want: []string{"artist"},
		Guard: &Guard{Root: root, Limit: 1, BuildLimit: 1 << 40},
		Logf: func(f string, a ...any) {
			mu.Lock()
			line := fmt.Sprintf(f, a...)
			logs = append(logs, line)
			mu.Unlock()
			if strings.Contains(line, "after sweep") {
				cancel()
			}
		},
		Build: func(ctx context.Context, date, dir string) (Result, error) {
			b, err := NewBuilder(Options{Dir: dir, DumpDate: date})
			if err != nil {
				return Result{}, err
			}
			if err := b.Read(ctx, strings.NewReader(string(dump(t, "31", 1)))); err != nil {
				return Result{}, err
			}
			return b.Finish(ctx)
		}}
	r.Watch(ctx, time.Hour)
	mu.Lock()
	all := strings.Join(logs, "\n")
	mu.Unlock()
	if !strings.Contains(all, "cap check after swap") || !strings.Contains(all, "cap check after sweep") {
		t.Fatalf("cap not checked after swap and sweep:\n%s", all)
	}
	if _, err := r.Guard.Check(context.Background()); !errors.Is(err, ErrOverCap) {
		t.Fatalf("guard: %v", err)
	}
}

// TestWatchSweepsBeforeBuild: a copy retired over Keep ago is gone before
// Watch's next build counts against the cap.
func TestWatchSweepsBeforeBuild(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "20261001-000000")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	line := fmt.Sprintf("20261001-000000 %d\n", time.Now().Add(-2*time.Hour).Unix())
	if err := os.WriteFile(filepath.Join(root, retiredFile), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var kept bool
	r := &Refresher{Root: root, Index: latestIndex(t, "20261021-001500"), Want: []string{"artist"},
		Build: func(context.Context, string, string) (Result, error) {
			_, err := os.Stat(old)
			kept = err == nil
			cancel()
			return Result{}, errors.New("stop")
		}}
	r.Watch(ctx, time.Hour)
	if kept {
		t.Fatal("retired copy still on disk at build")
	}
}
