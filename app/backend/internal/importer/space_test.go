package importer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func fakeDisk(t *testing.T, free int64) {
	freeSpaceOf = func(string) int64 { return free }
	t.Cleanup(func() { freeSpaceOf = FreeSpace })
}

const mb = 1 << 20

func TestImportRefusedWhenUnder100MBWouldBeLeft(t *testing.T) {
	for _, c := range []struct {
		name    string
		free    int64
		rules   *FileRules
		refused bool
	}{
		{"plenty free", 10 << 30, nil, false},
		{"exactly the floor plus the file", 100*mb + 2048, nil, false},
		{"one byte short", 100*mb + 2047, nil, true},
		{"nearly full disk", 5 * mb, nil, true},
		{"higher floor in settings", 300 * mb, &FileRules{HoldingDays: 7, MinFreeMB: 500}, true},
		{"check turned off", 5 * mb, &FileRules{HoldingDays: 7, MinFreeMB: 100, SkipFreeSpaceCheck: true}, false},
		{"unknown free space never blocks", -1, nil, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := setup(t)
			if c.rules != nil {
				if err := SaveFileRules(context.Background(), w.s, *c.rules, domain.System); err != nil {
					t.Fatal(err)
				}
			}
			fakeDisk(t, c.free)
			var hooked []string
			im := &Importer{Store: w.s, OnLowSpace: func(_ context.Context, dir string, free, need int64) {
				hooked = append(hooked, dir)
			}}
			dry, err := im.Import(context.Background(), w.dlID, true, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			if !c.refused {
				if imp.State != "done" || dry.Plan[0].Method != "hardlink" || len(hooked) != 0 {
					t.Fatalf("import %s %+v plan %+v hooked %v", imp.State, imp.Result, dry.Plan, hooked)
				}
				return
			}
			if dry.Plan[0].Method != "refuse" || !strings.HasPrefix(dry.Plan[0].Why[0], "not enough free space") {
				t.Fatalf("dry plan %+v", dry.Plan)
			}
			if imp.State != "failed" || imp.Result[0].Outcome != "refused" || !strings.Contains(imp.Result[0].Error, "must stay free") {
				t.Fatalf("import %s %+v", imp.State, imp.Result)
			}
			if ents, _ := os.ReadDir(filepath.Join(w.lib, "Dune - Part Two (2024)")); len(ents) != 0 {
				t.Fatalf("something was written: %v", ents)
			}
			if sl := slotOf(t, w); sl.State != mediafiles.SlotNeedsADecision {
				t.Fatalf("slot %s", sl.State)
			}
			if len(hooked) != 1 || hooked[0] != w.lib {
				t.Fatalf("health hook %v", hooked)
			}
			evs, _ := w.s.RecentEvents(context.Background(), 20)
			found := false
			for _, e := range evs {
				if e.Kind == domain.EventFreeSpaceLow && strings.Contains(e.Summary, "not enough free space") {
					found = true
				}
			}
			if !found {
				t.Fatalf("no free_space_low event in %+v", evs)
			}
		})
	}
}

func TestFileRulesDefaultsAndFloor(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	r, err := ReadFileRules(ctx, w.s.Read())
	if err != nil || r != (FileRules{HoldingDays: 7, MinFreeMB: 100}) {
		t.Fatalf("defaults %+v %v", r, err)
	}
	for _, bad := range []FileRules{{HoldingDays: 7, MinFreeMB: 99}, {HoldingDays: -1, MinFreeMB: 100}, {HoldingDays: 3651, MinFreeMB: 100}} {
		if err := SaveFileRules(ctx, w.s, bad, domain.System); err == nil {
			t.Errorf("%+v saved", bad)
		}
	}
	want := FileRules{HoldingDays: 0, MinFreeMB: 250, SkipFreeSpaceCheck: true}
	if err := SaveFileRules(ctx, w.s, want, domain.System); err != nil {
		t.Fatal(err)
	}
	if r, _ = ReadFileRules(ctx, w.s.Read()); r != want {
		t.Fatalf("saved %+v", r)
	}
}

func TestCheckSpaceSaysWhyInMegabytes(t *testing.T) {
	fakeDisk(t, 50*mb)
	err := CheckSpaceMin(t.TempDir(), 10*mb, 100)
	if err == nil || !strings.Contains(err.Error(), "needs 10 MB and 100 MB must stay free, only 50 MB are free") {
		t.Fatalf("err %v", err)
	}
}

func TestGuardBooksSpaceAcrossImports(t *testing.T) {
	lib, dl := t.TempDir(), t.TempDir()
	fakeDisk(t, 100*mb+3*mb)
	sameDiskOf = func(string, string) bool { return false }
	t.Cleanup(func() { sameDiskOf = sameDisk })
	plan := func(name string) ([]mediafiles.PlanStep, []Candidate) {
		src := filepath.Join(dl, name+".mkv")
		return []mediafiles.PlanStep{{SourceFile: src, Method: "hardlink", TargetPath: filepath.Join(lib, name, name+".mkv"), Why: []string{}}},
			[]Candidate{{Path: src, Size: 2 * mb}}
	}
	im := &Importer{}
	hd, hdC := plan("Show")
	low, releaseHD := im.guard(hd, lib, hdC, FileRules{MinFreeMB: 100})
	if low != nil || hd[0].Method != "hardlink" {
		t.Fatalf("HD refused: %+v", hd)
	}
	uhd, uhdC := plan("Show [4K]")
	if low, release := im.guard(uhd, lib, uhdC, FileRules{MinFreeMB: 100}); low == nil || uhd[0].Method != "refuse" {
		t.Fatalf("4K placed while the HD copy holds the space: %+v", uhd)
	} else {
		release()
	}
	releaseHD()
	uhd, uhdC = plan("Show [4K]")
	if low, release := im.guard(uhd, lib, uhdC, FileRules{MinFreeMB: 100}); low != nil {
		t.Fatalf("4K refused after the HD import: %+v", uhd)
	} else {
		release()
	}
}
