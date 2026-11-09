package library

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

func writeFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func crossDevice(t *testing.T) {
	linkFile = func(string, string) error { return &os.LinkError{Op: "link", Err: syscall.EXDEV} }
	t.Cleanup(func() { linkFile = os.Link })
}

// 4.1.3 important 1: a different file of the same size at the target is
// not ours; the source is never deleted for it.
func TestMoveFileRefusesUnrelatedTargetOfSameSize(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b", "a.mkv")
	writeFile(t, from, "aaaa")
	writeFile(t, to, "bbbb")
	err := moveFile(from, to)
	if err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("err %v", err)
	}
	if b, _ := os.ReadFile(from); string(b) != "aaaa" {
		t.Fatal("source lost")
	}
	if b, _ := os.ReadFile(to); string(b) != "bbbb" {
		t.Fatal("target changed")
	}
}

func TestMoveFileTakesItsOwnHardlink(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b", "a.mkv")
	writeFile(t, from, "aaaa")
	_ = os.MkdirAll(filepath.Dir(to), 0o755)
	if err := os.Link(from, to); err != nil {
		t.Skip("no hardlinks here")
	}
	if err := moveFile(from, to); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Fatal("source left")
	}
}

// 4.1.3 important 2: a file that appears at the target after the check is
// never replaced, on the same disk or across disks.
func TestMoveFileNeverReplacesFileThatAppears(t *testing.T) {
	for _, cross := range []bool{false, true} {
		dir := t.TempDir()
		from, to := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b", "a.mkv")
		writeFile(t, from, "aaaa")
		appear := func() { writeFile(t, to, "zz") }
		if cross {
			crossDevice(t)
			copyFile = func(s, d string) error { appear(); return importer.CopyNoReplace(s, d) }
		} else {
			linkFile = func(o, n string) error { appear(); return os.Link(o, n) }
		}
		err := moveFile(from, to)
		copyFile, linkFile = importer.CopyNoReplace, os.Link
		if err == nil {
			t.Fatalf("cross %v: moved over a file that appeared", cross)
		}
		if b, _ := os.ReadFile(to); string(b) != "zz" {
			t.Fatalf("cross %v: target replaced", cross)
		}
		if b, _ := os.ReadFile(from); string(b) != "aaaa" {
			t.Fatalf("cross %v: source lost", cross)
		}
	}
}

// 4.1.3 minor 9: the copy goes through the importer's .holyarrs-*.part temp
// (which its sweep knows); a foreign leftover with the old name is left alone.
func TestMoveFileCrossDeviceUsesSweptTempName(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b", "a.mkv")
	writeFile(t, from, "aaaa")
	writeFile(t, to+".holyarrs-part", "not ours")
	crossDevice(t)
	if err := moveFile(from, to); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(to); string(b) != "aaaa" {
		t.Fatal("copy wrong")
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Fatal("source left")
	}
	if b, _ := os.ReadFile(to + ".holyarrs-part"); string(b) != "not ours" {
		t.Fatal("foreign leftover touched")
	}
	ents, _ := os.ReadDir(filepath.Dir(to))
	if len(ents) != 2 {
		t.Fatalf("temp left behind: %v", ents)
	}
}

// 4.1.3 minor 4: no copy when it would leave the disk too full.
func TestMoveFileRefusesCopyWithoutFreeSpace(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b", "a.mkv")
	writeFile(t, from, "aaaa")
	crossDevice(t)
	checkSpace = func(string, int64) error { return errors.New("not enough free space") }
	t.Cleanup(func() { checkSpace = importer.CheckSpace })
	if err := moveFile(from, to); err == nil || !strings.Contains(err.Error(), "not enough free space") {
		t.Fatalf("err %v", err)
	}
	if _, err := os.Stat(to); !os.IsNotExist(err) {
		t.Fatal("copied anyway")
	}
}

// 5.1.7 critical 1: a rename that only changes case (.MKV to .mkv) on a
// case-insensitive disk sees the target as the same file; the only copy
// must never be removed. It goes through a temp name, and a crash between
// the two renames is finished on the next try.
func TestMoveFileCaseOnlyRenameKeepsTheOnlyCopy(t *testing.T) {
	for _, crash := range []bool{false, true} {
		dir := t.TempDir()
		from, to := filepath.Join(dir, "Dune.MKV"), filepath.Join(dir, "Dune.mkv")
		writeFile(t, from, "video")
		proof, err := fileProof(from)
		if err != nil {
			t.Fatal(err)
		}
		if crash && !caseInsensitive(dir) {
			continue // a case-sensitive disk: a plain move, no temp name
		}
		if crash {
			caseCrash = func() error { return errors.New("power cut") }
			err := moveRecordedFile(from, to, proof)
			caseCrash = nil
			if err == nil {
				t.Fatal("no crash")
			}
			if b, _ := os.ReadFile(from + caseTempSuffix); string(b) != "video" {
				t.Fatal("temp name not holding the file")
			}
		}
		if err := moveRecordedFile(from, to, proof); err != nil {
			t.Fatalf("crash=%v: %v", crash, err)
		}
		if b, err := os.ReadFile(to); string(b) != "video" {
			t.Fatalf("crash=%v: file lost: %v", crash, err)
		}
		names := []string{}
		ents, _ := os.ReadDir(dir)
		for _, e := range ents {
			names = append(names, e.Name())
		}
		if len(names) != 1 || names[0] != "Dune.mkv" {
			t.Fatalf("crash=%v: on disk %v", crash, names)
		}
		if err := moveRecordedFile(from, to, proof); err != nil {
			t.Fatalf("again: %v", err)
		}
		if b, _ := os.ReadFile(to); string(b) != "video" {
			t.Fatal("lost on second run")
		}
	}
}

// caseInsensitive reports whether dir's disk folds case (Windows, macOS).
func caseInsensitive(dir string) bool {
	p := filepath.Join(dir, "CaseProbe")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		return false
	}
	defer os.Remove(p)
	_, err := os.Stat(filepath.Join(dir, "caseprobe"))
	return err == nil
}

// The same path given twice is nothing to do.
func TestMoveFileSamePathIsNoop(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.mkv")
	writeFile(t, p, "x")
	if err := moveFile(p, p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "x" {
		t.Fatal("file lost")
	}
}

// 5.1.7 review: two spellings of one entry are never taken for a second
// hard link, so the source is never removed for them.
func TestMoveFileNeverRemovesAnAlias(t *testing.T) {
	check := func(t *testing.T, from, to string) {
		t.Helper()
		// A second hard link elsewhere: the link count alone says 2.
		if err := os.Link(from, from+".extra"); err != nil {
			t.Skip("no hardlinks here")
		}
		if err := moveFileDistinct(from, to); err == nil {
			t.Fatal("alias taken as a second name")
		}
		if b, _ := os.ReadFile(from); string(b) != "video" {
			t.Fatal("the only copy was removed")
		}
	}
	t.Run("case variant", func(t *testing.T) {
		dir := t.TempDir()
		from := filepath.Join(dir, "Dune.MKV")
		writeFile(t, from, "video")
		if _, err := os.Stat(filepath.Join(dir, "dune.mkv")); err != nil {
			t.Skip("case-sensitive disk")
		}
		check(t, from, filepath.Join(dir, "dune.mkv"))
	})
	t.Run("symlinked parent", func(t *testing.T) {
		dir := t.TempDir()
		from := filepath.Join(dir, "real", "a.mkv")
		writeFile(t, from, "video")
		if err := os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, "link")); err != nil {
			t.Skip("no symlinks here:", err)
		}
		check(t, from, filepath.Join(dir, "link", "a.mkv"))
	})
	t.Run("junction parent", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("junctions are Windows only")
		}
		dir := t.TempDir()
		from := filepath.Join(dir, "real", "a.mkv")
		writeFile(t, from, "video")
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(dir, "junc"), filepath.Join(dir, "real")).CombinedOutput(); err != nil {
			t.Skip("no junction:", string(out))
		}
		check(t, from, filepath.Join(dir, "junc", "a.mkv"))
	})
	t.Run("real second hard link in one folder", func(t *testing.T) {
		dir := t.TempDir()
		from, to := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b.mkv")
		writeFile(t, from, "video")
		if err := os.Link(from, to); err != nil {
			t.Skip("no hardlinks here")
		}
		if err := moveFileDistinct(from, to); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(from); !os.IsNotExist(err) {
			t.Fatal("source left")
		}
		if b, _ := os.ReadFile(to); string(b) != "video" {
			t.Fatal("target lost")
		}
	})
}
