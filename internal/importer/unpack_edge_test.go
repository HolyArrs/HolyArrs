package importer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
)

// When the built-in reader cannot unpack a download (#54, no 7z in the
// image), the import fails in plain words: the Slot waits on Needs a
// decision with the reason, History says it, the download's own files are
// untouched, nothing lands in the Library and no unpack folder is left.

func refusedClearly(t *testing.T, w world, imp *domain.Import, why string, before map[string]string) {
	t.Helper()
	if imp.State != "failed" || lastWhy(imp) != why {
		t.Fatalf("state %s, last step %q; want failed, %q (plan %+v)", imp.State, lastWhy(imp), why, imp.Plan)
	}
	var state, reason string
	if err := w.s.Read().QueryRow(`SELECT state, last_reason FROM slots WHERE id = ?`, w.slotID).Scan(&state, &reason); err != nil {
		t.Fatal(err)
	}
	if state != string(domain.SlotNeedsADecision) || reason != why {
		t.Fatalf("slot %q, reason %q; want needs a decision, %q", state, reason, why)
	}
	var summary string
	if err := w.s.ReadEvents().QueryRow(`SELECT summary FROM events WHERE kind = 'import_failed' AND download_id = ? ORDER BY id DESC LIMIT 1`, w.dlID).Scan(&summary); err != nil {
		t.Fatalf("no History entry: %v", err)
	}
	if !strings.Contains(summary, why) {
		t.Fatalf("History says %q, not why (%q)", summary, why)
	}
	if after := snapshot(w.dl); len(after) != len(before) {
		t.Fatalf("download folder changed: %d files, was %d", len(after), len(before))
	} else {
		for k, v := range before {
			if after[k] != v {
				t.Fatalf("%s changed", k)
			}
		}
	}
	if placed := snapshot(w.lib); len(placed) != 0 {
		t.Fatalf("something reached the Library: %v", placed)
	}
	noLeftovers(t, w.dl)
	noLeftovers(t, filepath.Dir(w.dl))
}

func importEdge(t *testing.T, files map[string][]byte) (*domain.Import, world, map[string]string) {
	t.Helper()
	fakeDisk(t, 10<<30)
	w := packedWorld(t, files)
	before := snapshot(w.dl)
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return imp, w, before
}

func edgeFile(t *testing.T, parts ...string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// 1. A RAR from before RAR 1.5 (signature "RE~^"): no reader handles it.
func TestEdgeOldRARFormatIsRefusedPlainly(t *testing.T) {
	old := append([]byte("RE~^"), make([]byte, 4000)...)
	imp, w, before := importEdge(t, map[string][]byte{"Rel/" + relName + ".rar": old})
	refusedClearly(t, w, imp, whyOldRar, before)
}

// 2. A .7z or .zip release is unpacked like a RAR set now (#54, as
// Unpackerr); one that is not really an archive fails as damaged.
func TestEdgeDamagedSevenZipReleaseIsRefusedPlainly(t *testing.T) {
	sz := append([]byte("7z\xbc\xaf\x27\x1c\x00\x04"), make([]byte, 4000)...)
	imp, w, before := importEdge(t, map[string][]byte{"Rel/" + relName + ".7z": sz})
	refusedClearly(t, w, imp, whyDamaged, before)
}

func TestEdgeDamagedZipReleaseIsRefusedPlainly(t *testing.T) {
	zip := append([]byte("PK\x03\x04"), make([]byte, 4000)...)
	imp, w, before := importEdge(t, map[string][]byte{"Rel/" + relName + ".zip": zip})
	refusedClearly(t, w, imp, whyDamaged, before)
}

// 3. Password-protected RAR 5 (WinRAR -p and -hp, testdata/rar-edge), no password known.
func TestEdgePasswordRARIsRefusedPlainly(t *testing.T) {
	for _, name := range []string{"pw-files.rar", "pw-headers.rar"} {
		t.Run(name, func(t *testing.T) {
			imp, w, before := importEdge(t, map[string][]byte{"Rel/" + relName + ".rar": edgeFile(t, "testdata", "rar-edge", name)})
			refusedClearly(t, w, imp, whyPassword, before)
		})
	}
}

// 4. A compressed RAR 5 set with a volume missing, and one with a changed byte.
func rar5Parts(t *testing.T) map[string][]byte {
	files := map[string][]byte{}
	for i := 1; i <= 4; i++ {
		files[fmt.Sprintf("Rel/%s.part%02d.rar", relName, i)] = edgeFile(t, "..", "unrar", "testdata", "rar5-compressed", fmt.Sprintf("Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.part%02d.rar", i))
	}
	return files
}

func TestEdgeMissingVolumeIsRefusedPlainly(t *testing.T) {
	files := rar5Parts(t)
	delete(files, fmt.Sprintf("Rel/%s.part03.rar", relName))
	imp, w, before := importEdge(t, files)
	refusedClearly(t, w, imp, whyDamaged, before)
}

func TestEdgeCorruptVolumeIsRefusedPlainly(t *testing.T) {
	files := rar5Parts(t)
	k := fmt.Sprintf("Rel/%s.part02.rar", relName)
	b := append([]byte(nil), files[k]...)
	b[len(b)/2] ^= 0xff
	files[k] = b
	imp, w, before := importEdge(t, files)
	refusedClearly(t, w, imp, whyDamaged, before)
}

// 5. The disk fills up while the set is being unpacked: the half-written
// file goes with the unpack folder and the reason says the disk is full.
func TestEdgeDiskFullMidUnpackIsRefusedPlainly(t *testing.T) {
	old := extractCompressed
	t.Cleanup(func() { extractCompressed = old })
	extractCompressed = func(_ context.Context, _, dest string, _ int64) error {
		if err := os.WriteFile(filepath.Join(dest, "half.mkv"), make([]byte, 1000), 0o644); err != nil {
			return err
		}
		return &os.PathError{Op: "write", Path: filepath.Join(dest, "half.mkv"), Err: diskFullErrno()}
	}
	imp, w, before := importEdge(t, rar5Parts(t))
	refusedClearly(t, w, imp, whyDiskFull, before)
}

// diskFullErrno is the "disk full" error this system's writes return.
func diskFullErrno() error {
	if filepath.Separator == '\\' {
		return syscall.Errno(112) // ERROR_DISK_FULL
	}
	return syscall.ENOSPC
}

// The same for a show: a season pack in a damaged .7z fails each episode
// with the plain reason, and History says it.
func TestEdgeDamagedSevenZipSeasonPackIsRefusedPlainly(t *testing.T) {
	w := showSetup(t, []int{1, 2}, "Copper.Valley.S01.1080p.WEB.H264-HOLYARRS.7z")
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State != "failed" {
		t.Fatalf("%v %+v", err, imp)
	}
	for _, n := range []int{1, 2} {
		var reason string
		if err := w.s.Read().QueryRow(`SELECT last_reason FROM slots WHERE id = ?`, w.slots[n]).Scan(&reason); err != nil || reason != whyDamaged {
			t.Fatalf("episode %d: reason %q %v", n, reason, err)
		}
	}
	var summary string
	if err := w.s.ReadEvents().QueryRow(`SELECT summary FROM events WHERE kind = 'import_failed' AND download_id = ? ORDER BY id DESC LIMIT 1`, w.dlID).Scan(&summary); err != nil || !strings.Contains(summary, whyDamaged) {
		t.Fatalf("History %q %v", summary, err)
	}
	if placed := snapshot(w.lib); len(placed) != 0 {
		t.Fatalf("something reached the Library: %v", placed)
	}
}

// A real 7-Zip release (two parts, 7-Zip 26.04, see internal/archive/testdata)
// is unpacked with no tool on the system and imported.
func TestSevenZipReleaseIsImportedWithoutATool(t *testing.T) {
	files := map[string][]byte{}
	for _, n := range []string{"parts.7z.001", "parts.7z.002"} {
		files["Rel/"+relName+n[len("parts"):]] = edgeFile(t, "..", "archive", "testdata", n)
	}
	files["Rel/"+relName+".nfo"] = []byte("info")
	fakeDisk(t, 10<<30)
	w := packedWorld(t, files)
	setFake(t, w.s, "import_size_floors", "false") // the test video is small
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("%v state %s plan %+v", err, imp.State, imp.Plan)
	}
	got := snapshot(w.lib)
	if len(got) != 1 {
		t.Fatalf("library %v", got)
	}
	noLeftovers(t, w.dl)
	noLeftovers(t, filepath.Dir(w.dl))
}
