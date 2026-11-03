package importer

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
)

// asLink marks the one file in a rar4 volume as made on Unix with a
// symlink mode (the header CRC is not checked by the reader).
func asLink(v []byte) []byte {
	const fileHdr = 7 + 13 // signature + main header
	v[fileHdr+15] = 3
	binary.LittleEndian.PutUint32(v[fileHdr+28:], 0xA1FF)
	return v
}

func importPacked(t *testing.T, files map[string][]byte) (*domain.Import, world) {
	t.Helper()
	w := packedWorld(t, files)
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return imp, w
}

func neverRun(t *testing.T) {
	fakeTools(t, func(string) (string, error) { return "unrar", nil },
		func(context.Context, string, []string) error { t.Error("the tool must not run"); return nil })
}

func TestToolOutputPastTheCheckedTotalIsStopped(t *testing.T) {
	fakeDisk(t, 10<<30)
	oldPoll := unpackPoll
	unpackPoll = 5 * time.Millisecond
	t.Cleanup(func() { unpackPoll = oldPoll })
	stopped := false
	fakeTools(t, func(string) (string, error) { return "unrar", nil }, func(ctx context.Context, _ string, a []string) error {
		dest := strings.TrimRight(a[len(a)-1], `/\`)
		if err := os.WriteFile(filepath.Join(dest, "big.mkv"), make([]byte, unpackMargin+1<<20), 0o644); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			stopped = true
			return ctx.Err()
		case <-time.After(20 * time.Second):
			return nil
		}
	})
	imp, w := importPacked(t, map[string][]byte{"Rel/" + relName + ".rar": rar4(relName+".mkv", []byte("packed"), 3000, false, false, true, false)})
	if !stopped || imp.State != "failed" || lastWhy(imp) != whySafely {
		t.Fatalf("stopped %v, state %s, plan %+v", stopped, imp.State, imp.Plan)
	}
	noLeftovers(t, w.dl)
}

func TestSizeOverTheCapIsRefused(t *testing.T) {
	fakeDisk(t, 10<<30)
	old := maxUnpack
	maxUnpack = 1000
	t.Cleanup(func() { maxUnpack = old })
	neverRun(t)
	imp, w := importPacked(t, map[string][]byte{"Rel/" + relName + ".rar": rar4(relName+".mkv", []byte("packed"), 5000, false, false, true, false)})
	if imp.State != "failed" || lastWhy(imp) != whyTooBig {
		t.Fatalf("%s %+v", imp.State, imp.Plan)
	}
	noLeftovers(t, w.dl)
}

func TestLinkInASetKeepsItAwayFromTheTool(t *testing.T) {
	fakeDisk(t, 10<<30)
	neverRun(t)
	// A compressed set, so the tool would be asked, but it holds a link.
	imp, w := importPacked(t, map[string][]byte{"Rel/" + relName + ".rar": asLink(rar4("link.mkv", []byte("x"), 5000, false, false, true, false))})
	if imp.State != "failed" || lastWhy(imp) != whySafely {
		t.Fatalf("%s %+v", imp.State, imp.Plan)
	}
	noLeftovers(t, w.dl)
}

func TestSymlinkLeftByTheToolFailsTheWholeUnpack(t *testing.T) {
	fakeDisk(t, 10<<30)
	outside := t.TempDir()
	linked := false
	fakeTools(t, func(string) (string, error) { return "unrar", nil }, func(_ context.Context, _ string, a []string) error {
		dest := strings.TrimRight(a[len(a)-1], `/\`)
		if err := os.WriteFile(filepath.Join(dest, relName+".mkv"), video, 0o644); err != nil {
			return err
		}
		linked = os.Symlink(outside, filepath.Join(dest, "escape")) == nil
		return nil
	})
	imp, w := importPacked(t, map[string][]byte{"Rel/" + relName + ".rar": rar4(relName+".mkv", []byte("packed"), len(video), false, false, true, false)})
	if !linked {
		t.Skip("symlinks cannot be created in this environment")
	}
	if imp.State != "failed" || lastWhy(imp) != whySafely {
		t.Fatalf("%s %+v", imp.State, imp.Plan)
	}
	noLeftovers(t, w.dl)
	if left, _ := os.ReadDir(outside); len(left) != 0 {
		t.Fatal("touched what the link pointed at")
	}
}

func TestToolFlagsSkipLinksAndNeverOverwrite(t *testing.T) {
	var got [][]string
	fakeTools(t, func(n string) (string, error) {
		if n == "unrar" {
			return "", os.ErrNotExist
		}
		return n, nil
	}, func(_ context.Context, name string, a []string) error { got = append(got, append([]string{name}, a...)); return nil })
	_ = extractWithTool(context.Background(), "a.rar", t.TempDir(), 1<<20)
	if len(got) != 1 || got[0][0] != "7z" || !strings.Contains(strings.Join(got[0], " "), "-aos") || strings.Contains(strings.Join(got[0], " "), "-snl") {
		t.Fatalf("7z: %v", got)
	}
	got = nil
	lookTool = func(n string) (string, error) { return n, nil }
	_ = extractWithTool(context.Background(), "a.rar", t.TempDir(), 1<<20)
	line := strings.Join(got[0], " ")
	if got[0][0] != "unrar" || !strings.Contains(line, "-o-") || strings.Contains(line, "-ol") {
		t.Fatalf("unrar: %v", got)
	}
}
