package library

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Review cases: the source must survive every target that is not
// the same bytes in full, and a true finished copy must still finish.

func adv636Pair(t *testing.T, size int, mutate func([]byte)) (string, string, []byte) {
	t.Helper()
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b", "a.mkv")
	body := make([]byte, size)
	for i := range body {
		body[i] = byte(i * 7)
	}
	other := append([]byte(nil), body...)
	mutate(other)
	writeFile(t, from, string(body))
	writeFile(t, to, string(other))
	return from, to, body
}

func adv636SourceKept(t *testing.T, from string, body []byte) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil || string(b) != string(body) {
		t.Fatalf("source lost or changed: %v", err)
	}
}

// One byte just past the first MiB, and just before the last MiB.
func TestAdv636DiffersAtMiBBoundaries(t *testing.T) {
	for _, at := range []int{1 << 20, 3<<20 - (1 << 20) - 1} {
		from, to, body := adv636Pair(t, 3<<20, func(b []byte) { b[at] ^= 0xff })
		if err := moveFile(from, to); err == nil {
			t.Fatalf("diff at %d: source removed", at)
		}
		adv636SourceKept(t, from, body)
	}
}

// A shorter target (a copy cut off part way) is never taken as finished.
func TestAdv636InterruptedCopyTargetKeepsSource(t *testing.T) {
	from, to, body := adv636Pair(t, 3<<20, func([]byte) {})
	if err := os.Truncate(to, 2<<20); err != nil {
		t.Fatal(err)
	}
	if err := moveFile(from, to); err == nil {
		t.Fatal("a partial target was taken as the finished copy")
	}
	adv636SourceKept(t, from, body)
}

// A target that is a symlink to the source must never let the source go.
func TestAdv636SymlinkTargetToSource(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b", "a.mkv")
	writeFile(t, from, strings.Repeat("z", 3<<20))
	_ = os.MkdirAll(filepath.Dir(to), 0o755)
	if err := os.Symlink(from, to); err != nil {
		t.Skip("no symlinks here")
	}
	if err := moveFile(from, to); err == nil {
		if _, serr := os.Stat(from); serr != nil {
			t.Fatal("source removed through a symlink target: only copy gone")
		}
	}
	adv636SourceKept(t, from, []byte(strings.Repeat("z", 3<<20)))
}

// A hard link target (same file, second real entry) finishes the move.
func TestAdv636HardlinkTargetFinishes(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b", "a.mkv")
	writeFile(t, from, "same file")
	_ = os.MkdirAll(filepath.Dir(to), 0o755)
	if err := os.Link(from, to); err != nil {
		t.Skip("no hard links here")
	}
	if err := moveFile(from, to); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(to); string(b) != "same file" {
		t.Fatal("target lost")
	}
}

// Cross-disk: the link fails, the copy runs; with a different existing
// target nothing is copied or removed, and a fresh target is copied.
func TestAdv636CrossDiskMove(t *testing.T) {
	prev := linkFile
	linkFile = func(string, string) error { return &os.LinkError{Op: "link", Err: syscall.Errno(18)} } // EXDEV
	t.Cleanup(func() { linkFile = prev })
	from, to, body := adv636Pair(t, 3<<20, func(b []byte) { b[len(b)/2] ^= 1 })
	if err := moveFile(from, to); err == nil {
		t.Fatal("cross-disk move removed source over a different target")
	}
	adv636SourceKept(t, from, body)
	fresh := filepath.Join(filepath.Dir(to), "fresh.mkv")
	if err := moveFile(from, fresh); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(fresh); string(b) != string(body) {
		t.Fatal("cross-disk copy wrong")
	}
}

// Disk full / failed copy: the source stays.
func TestAdv636CopyFailsKeepsSource(t *testing.T) {
	pl, pc := linkFile, copyFile
	linkFile = func(string, string) error { return errors.New("cross-device") }
	copyFile = func(string, string) error { return errors.New("no space left on device") }
	t.Cleanup(func() { linkFile, copyFile = pl, pc })
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b", "a.mkv")
	writeFile(t, from, "x")
	if err := moveFile(from, to); err == nil {
		t.Fatal("copy failure reported as success")
	}
	adv636SourceKept(t, from, []byte("x"))
}

// Windows long paths (past 260 characters).
func TestAdv636LongPathDifferentTarget(t *testing.T) {
	dir := t.TempDir()
	deep := dir
	for len(deep) < 300 {
		deep = filepath.Join(deep, strings.Repeat("d", 40))
	}
	from, to := filepath.Join(deep, "a.mkv"), filepath.Join(deep, "b", "a.mkv")
	body := strings.Repeat("q", 3<<20)
	writeFile(t, from, body)
	other := []byte(body)
	other[len(other)/2] = 'w'
	writeFile(t, to, string(other))
	if err := moveFile(from, to); err == nil {
		t.Fatal("long path: source removed over different target")
	}
	adv636SourceKept(t, from, []byte(body))
}

// Huge files: full compare of two 2 GiB files differing in the middle.
// Run with HOLYARRS_ADV_HUGE=1; it reports the time the full read costs.
func TestAdv636HugeSameSizeDifferentMiddle(t *testing.T) {
	if os.Getenv("HOLYARRS_ADV_HUGE") == "" {
		t.Skip("set HOLYARRS_ADV_HUGE=1")
	}
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b", "a.mkv")
	_ = os.MkdirAll(filepath.Dir(to), 0o755)
	const size = 2 << 30
	for _, p := range []string{from, to} {
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(size); err != nil {
			t.Fatal(err)
		}
		if p == to {
			_, _ = f.WriteAt([]byte{1}, size/2)
		}
		f.Close()
	}
	start := time.Now()
	if err := moveFile(from, to); err == nil {
		t.Fatal("huge: source removed over different target")
	}
	t.Logf("full compare of 2x2 GiB refused in %v", time.Since(start))
	if fi, err := os.Stat(from); err != nil || fi.Size() != size {
		t.Fatal("huge source lost")
	}
	if err := os.Remove(to); err != nil {
		t.Fatal(err)
	}
	f, _ := os.Create(to)
	_ = f.Truncate(size)
	f.Close()
	start = time.Now()
	if err := moveFile(from, to); err != nil {
		t.Fatal(err)
	}
	t.Logf("full compare of 2x2 GiB matched in %v", time.Since(start))
}
