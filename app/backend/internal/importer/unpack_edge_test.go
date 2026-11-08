package importer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func refusedClearly(t *testing.T, w world, imp *mediafiles.Import, why string, before map[string]string) {
	t.Helper()
	if imp.State != "failed" || lastWhy(imp) != why {
		t.Fatalf("state %s, last step %q; want failed, %q (plan %+v)", imp.State, lastWhy(imp), why, imp.Plan)
	}
	var state, reason string
	if err := w.s.Read().QueryRow(`SELECT state, last_reason FROM slots WHERE id = ?`, w.slotID).Scan(&state, &reason); err != nil {
		t.Fatal(err)
	}
	if state != string(mediafiles.SlotNeedsADecision) || reason != why {
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

func importEdge(t *testing.T, files map[string][]byte) (*mediafiles.Import, world, map[string]string) {
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

func TestEdgeOldRARFormatIsRefusedPlainly(t *testing.T) {
	old := append([]byte("RE~^"), make([]byte, 4000)...)
	imp, w, before := importEdge(t, map[string][]byte{"Rel/" + relName + ".rar": old})
	refusedClearly(t, w, imp, whyOldRar, before)
}

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

func TestEdgePasswordRARIsRefusedPlainly(t *testing.T) {
	for _, name := range []string{"pw-files.rar", "pw-headers.rar"} {
		t.Run(name, func(t *testing.T) {
			imp, w, before := importEdge(t, map[string][]byte{"Rel/" + relName + ".rar": edgeFile(t, "testdata", "rar-edge", name)})
			refusedClearly(t, w, imp, whyPassword, before)
		})
	}
}

func rar5Parts(t *testing.T) map[string][]byte {
	files := map[string][]byte{}
	for i := 1; i <= 4; i++ {
		files[fmt.Sprintf("Rel/%s.part%02d.rar", relName, i)] = edgeFile(t, "unrar", "testdata", "rar5-compressed", fmt.Sprintf("Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.part%02d.rar", i))
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

func diskFullErrno() error {
	if filepath.Separator == '\\' {
		return syscall.Errno(112)
	}
	return syscall.ENOSPC
}

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

func TestSevenZipReleaseIsImportedWithoutATool(t *testing.T) {
	files := map[string][]byte{}
	for _, n := range []string{"parts.7z.001", "parts.7z.002"} {
		files["Rel/"+relName+n[len("parts"):]] = edgeFile(t, "archive", "testdata", n)
	}
	files["Rel/"+relName+".nfo"] = []byte("info")
	fakeDisk(t, 10<<30)
	w := packedWorld(t, files)
	setFake(t, w.s, "import_size_floors", "false")
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
