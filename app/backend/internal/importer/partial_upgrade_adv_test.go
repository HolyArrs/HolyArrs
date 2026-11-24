package importer

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Review cases: the first step fails instead of the second, both
// fail, and the old copy is named the 1x01-02 way.
func TestAdvPartialUpgrade(t *testing.T) {
	for _, tc := range []struct {
		name, old, fail string
		keep            []int // episodes that must stay on the old copy
	}{
		{"first fails", "Copper.Valley.S01E01E02.720p-OLD", "S01E01", []int{1}},
		{"both fail", "Copper.Valley.S01E01E02.720p-OLD", "S01E0", []int{1, 2}},
		{"1x01-02 name", "Copper.Valley.1x01-02.720p.HDTV.x264-OLD", "S01E02", []int{2}},
		{"S01E01-E02 name", "Copper.Valley.S01E01-E02.720p-OLD", "S01E01", []int{1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := showSetup(t, []int{1, 2}, ep(1), ep(2))
			oldID, rel := oldDouble(t, w, tc.old)
			imp, err := (&Importer{Store: w.s, Ops: failFor(tc.fail)}).Import(context.Background(), w.dlID, false, domain.System)
			t.Logf("import state %q err %v res %+v", imp.State, err, imp.Result)
			if _, err := os.Stat(filepath.Join(w.lib, filepath.FromSlash(rel))); err != nil {
				t.Fatalf("old copy left its spot: %v", err)
			}
			of := w.file(t, oldID)
			want := []int64{}
			for _, n := range tc.keep {
				want = append(want, w.slots[n])
			}
			if of.State != "present" || !slices.Equal(of.SlotIDs, want) {
				t.Fatalf("old file state %s slots %v, want %v", of.State, of.SlotIDs, want)
			}
			for _, n := range []int{1, 2} {
				sl := w.slot(t, n)
				if sl.FileID == nil {
					t.Fatalf("episode %d has no file", n)
				}
				if slices.Contains(tc.keep, n) != (*sl.FileID == oldID) {
					t.Fatalf("episode %d on file %d (old %d)", n, *sl.FileID, oldID)
				}
			}
			held, _ := filepath.Glob(filepath.Join(w.lib, HoldingDir, "*", "*", "*", "*"))
			if len(held) > 0 {
				t.Fatalf("holding folder not empty: %v", held)
			}
		})
	}
}
