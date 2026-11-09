package importer

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func rar4(name string, data []byte, total int, splitBefore, splitAfter, compressed, more bool) []byte {
	block := func(typ byte, flags uint16, body []byte) []byte {
		b := make([]byte, 7, 7+len(body))
		b[2] = typ
		binary.LittleEndian.PutUint16(b[3:], flags)
		binary.LittleEndian.PutUint16(b[5:], uint16(7+len(body)))
		b = append(b, body...)
		binary.LittleEndian.PutUint16(b[0:], uint16(crc32.ChecksumIEEE(b[2:])))
		return b
	}
	out := []byte("Rar!\x1a\x07\x00")
	out = append(out, block(0x73, 1, make([]byte, 6))...)
	fl := uint16(0x8000)
	if splitBefore {
		fl |= 1
	}
	if splitAfter {
		fl |= 2
	}
	body := make([]byte, 25+len(name))
	binary.LittleEndian.PutUint32(body[0:], uint32(len(data)))
	binary.LittleEndian.PutUint32(body[4:], uint32(total))
	body[8], body[17], body[18] = 2, 29, 0x30
	binary.LittleEndian.PutUint32(body[9:], crc32.ChecksumIEEE(data))
	if compressed {
		body[18] = 0x33
	}
	binary.LittleEndian.PutUint16(body[19:], uint16(len(name)))
	copy(body[25:], name)
	out = append(out, block(0x74, fl, body)...)
	out = append(out, data...)
	var end uint16
	if more {
		end = 1
	}
	return append(out, block(0x7B, end, nil)...)
}

func packedWorld(t *testing.T, files map[string][]byte) world {
	t.Helper()
	w := setup(t)
	if err := os.Remove(filepath.Join(w.dl, relName+".mkv")); err != nil {
		t.Fatal(err)
	}
	var seen []domain.SeenFile
	for name, b := range files {
		p := filepath.Join(w.dl, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		seen = append(seen, domain.SeenFile{Name: name, Size: int64(len(b))})
	}
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		var d domain.Download
		if err := database.Get(context.Background(), tx, "downloads", w.dlID, &d); err != nil {
			return err
		}
		d.FilesSeen = seen
		return database.Update(tx, "downloads", &d, domain.System)
	})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func snapshot(root string) map[string]string {
	out := map[string]string{}
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			rel, _ := filepath.Rel(root, p)
			out[rel] = string(b)
		}
		return nil
	})
	return out
}

func noLeftovers(t *testing.T, root string) {
	t.Helper()
	var found []string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && strings.HasPrefix(d.Name(), ".holyarrs-unpack") {
			found = append(found, p)
		}
		return nil
	})
	if len(found) > 0 {
		t.Fatalf("temp folder left: %v", found)
	}
}

func lastWhy(imp *mediafiles.Import) string {
	return strings.Join(imp.Plan[len(imp.Plan)-1].Why, " ")
}

var video = bytes.Repeat([]byte("v"), 3000)

func TestPackedMultiVolumeIsUnpackedAndImported(t *testing.T) {
	fakeDisk(t, 10<<30)
	w := packedWorld(t, map[string][]byte{
		"Rel/" + relName + ".rar": rar4(relName+".mkv", video[:1500], len(video), false, true, false, true),
		"Rel/" + relName + ".r00": rar4(relName+".mkv", video[1500:], len(video), true, false, false, false),
	})
	before := snapshot(w.dl)
	im := &Importer{Store: w.s}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "done" {
		t.Fatalf("state %s: %+v", imp.State, imp.Result)
	}
	got, err := os.ReadFile(filepath.Join(w.lib, "Dune - Part Two (2024)", "Dune - Part Two (2024) 1080p WEB.mkv"))
	if err != nil || !bytes.Equal(got, video) {
		t.Fatalf("library file: %v", err)
	}
	after := snapshot(w.dl)
	if len(after) != len(before) {
		t.Fatalf("download folder changed: %v", after)
	}
	for k, v := range before {
		if after[k] != v {
			t.Fatalf("%s changed", k)
		}
	}
	noLeftovers(t, w.dl)
}

func TestPackedSetWithSampleIsUnpacked(t *testing.T) {
	fakeDisk(t, 10<<30)
	w := packedWorld(t, map[string][]byte{
		"Rel/" + relName + ".rar":                                rar4(relName+".mkv", video[:1500], len(video), false, true, false, true),
		"Rel/" + relName + ".r00":                                rar4(relName+".mkv", video[1500:], len(video), true, false, false, false),
		"Rel/" + relName + ".nfo":                                []byte("info"),
		"Rel/Sample/" + strings.ToLower(relName) + "-sample.mkv": bytes.Repeat([]byte("s"), 200),
	})
	im := &Importer{Store: w.s}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "done" {
		t.Fatalf("state %s: %+v", imp.State, imp.Result)
	}
	got, err := os.ReadFile(filepath.Join(w.lib, "Dune - Part Two (2024)", "Dune - Part Two (2024) 1080p WEB.mkv"))
	if err != nil || !bytes.Equal(got, video) {
		t.Fatalf("library file: %v", err)
	}
	noLeftovers(t, w.dl)
}

func TestCompressedRAR5SceneSetIsImportedWithoutATool(t *testing.T) {
	fakeDisk(t, 10<<30)
	files := map[string][]byte{
		"Rel/" + relName + ".nfo":                                []byte("info"),
		"Rel/Sample/" + strings.ToLower(relName) + "-sample.mkv": bytes.Repeat([]byte("s"), 200),
	}
	for i := 1; i <= 4; i++ {
		b, err := os.ReadFile(filepath.Join("unrar", "testdata", "rar5-compressed", fmt.Sprintf("Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.part%02d.rar", i)))
		if err != nil {
			t.Fatal(err)
		}
		files[fmt.Sprintf("Rel/%s.part%02d.rar", relName, i)] = b
	}
	w := packedWorld(t, files)
	im := &Importer{Store: w.s}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "done" {
		t.Fatalf("state %s: %+v", imp.State, imp.Plan)
	}
	got, err := os.ReadFile(filepath.Join(w.lib, "Dune - Part Two (2024)", "Dune - Part Two (2024) 1080p WEB.mkv"))
	if err != nil || len(got) != 720000 {
		t.Fatalf("library file: %d bytes, %v", len(got), err)
	}
	noLeftovers(t, w.dl)
}

func TestPackedDryRunWritesNothing(t *testing.T) {
	w := packedWorld(t, map[string][]byte{"Rel/" + relName + ".rar": rar4(relName+".mkv", video, len(video), false, false, false, false)})
	im := &Importer{Store: w.s}
	if _, err := im.Import(context.Background(), w.dlID, true, domain.System); err != nil {
		t.Fatal(err)
	}
	noLeftovers(t, w.dl)
}

func TestCompressedPackNobodyCanReadGivesPlainReason(t *testing.T) {
	fakeDisk(t, 10<<30)
	w := packedWorld(t, map[string][]byte{"Rel/" + relName + ".rar": rar4(relName+".mkv", []byte("packed"), 5000, false, false, true, false)})
	before := snapshot(w.dl)
	im := &Importer{Store: w.s}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "failed" || lastWhy(imp) != whyDamaged {
		t.Fatalf("state %s, plan %+v", imp.State, imp.Plan)
	}
	var state string
	if err := w.s.Read().QueryRow(`SELECT state FROM slots WHERE id = ?`, w.slotID).Scan(&state); err != nil || state != string(mediafiles.SlotNeedsADecision) {
		t.Fatalf("slot %q %v", state, err)
	}
	if after := snapshot(w.dl); len(after) != len(before) {
		t.Fatal("download folder changed")
	}
	noLeftovers(t, w.dl)
}

func TestPackedPathTraversalIsRefused(t *testing.T) {
	fakeDisk(t, 10<<30)
	w := packedWorld(t, map[string][]byte{"Rel/" + relName + ".rar": rar4(`..\..\evil.mkv`, video, len(video), false, false, false, false)})
	im := &Importer{Store: w.s}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "failed" || lastWhy(imp) != whyUnsafe {
		t.Fatalf("%s %+v", imp.State, imp.Plan)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(w.dl), "evil.mkv")); err == nil {
		t.Fatal("wrote outside")
	}
	noLeftovers(t, w.dl)
}

func TestPackedNeedsFreeSpace(t *testing.T) {
	fakeDisk(t, 1*mb)
	w := packedWorld(t, map[string][]byte{"Rel/" + relName + ".rar": rar4(relName+".mkv", video, len(video), false, false, false, false)})
	im := &Importer{Store: w.s}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "failed" || lastWhy(imp) != whyNoSpace {
		t.Fatalf("%s %+v", imp.State, imp.Plan)
	}
	noLeftovers(t, w.dl)
}

func TestCompressedRAR4IsUnpackedWithoutATool(t *testing.T) {
	fakeDisk(t, 10<<30)
	b, err := os.ReadFile(filepath.Join("unrar", "testdata", "rarfile", "rar3-solid.rar"))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "Rel")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(root, "rel.rar")
	if err := os.WriteFile(first, b, 0o644); err != nil {
		t.Fatal(err)
	}
	extra, cleanup, why := unpackRars(context.Background(), 54, root, []Candidate{{Path: first, Size: int64(len(b))}}, FileRules{}, false)
	defer cleanup()
	if why != "" || len(extra) != 2 {
		t.Fatalf("why %q, extra %+v", why, extra)
	}
	for _, c := range extra {
		if c.Size != 2048 {
			t.Fatalf("unpacked %s: %d bytes", c.Path, c.Size)
		}
	}
	cleanup()
	noLeftovers(t, filepath.Dir(root))
}
