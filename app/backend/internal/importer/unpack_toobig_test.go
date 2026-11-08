package importer

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer/archive"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer/unrar"
)

func importOnDisk(t *testing.T, free int64, files map[string][]byte) (*mediafiles.Import, world, map[string]string) {
	t.Helper()
	fakeDisk(t, free)
	w := packedWorld(t, files)
	before := snapshot(w.dl)
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return imp, w, before
}

func stubOtherTooBig(t *testing.T) {
	old := extractOther
	t.Cleanup(func() { extractOther = old })
	extractOther = func(context.Context, string, archive.Kind, string, int64) error { return unrar.ErrTooBig }
}

func TestZipPastFreeSpaceSaysNoSpace(t *testing.T) {
	stubOtherTooBig(t)
	zip := append([]byte("PK\x03\x04"), make([]byte, 4000)...)
	imp, w, before := importOnDisk(t, 10<<30, map[string][]byte{"Rel/" + relName + ".zip": zip})
	refusedClearly(t, w, imp, whyNoSpace, before)
}

func TestZipPastSizeCapSaysTooBig(t *testing.T) {
	stubOtherTooBig(t)
	zip := append([]byte("PK\x03\x04"), make([]byte, 4000)...)
	imp, w, before := importOnDisk(t, 1<<50, map[string][]byte{"Rel/" + relName + ".zip": zip})
	refusedClearly(t, w, imp, whyTooBig, before)
}

func TestRARPastFreeSpaceSaysNoSpace(t *testing.T) {
	old := extractCompressed
	t.Cleanup(func() { extractCompressed = old })
	extractCompressed = func(context.Context, string, string, int64) error { return unrar.ErrTooBig }
	imp, w, before := importOnDisk(t, MinFreeSpace+2<<20, rar5Parts(t))
	refusedClearly(t, w, imp, whyNoSpace, before)
}

func TestRARPastItsOwnSizeSaysTooBig(t *testing.T) {
	old := extractCompressed
	t.Cleanup(func() { extractCompressed = old })
	extractCompressed = func(context.Context, string, string, int64) error { return unrar.ErrTooBig }
	imp, w, before := importOnDisk(t, 10<<30, rar5Parts(t))
	refusedClearly(t, w, imp, whyTooBig, before)
}
