package importer

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func asLink(v []byte) []byte {
	const fileHdr = 7 + 13
	v[fileHdr+15] = 3
	binary.LittleEndian.PutUint32(v[fileHdr+28:], 0xA1FF)
	return v
}

func importPacked(t *testing.T, files map[string][]byte) (*mediafiles.Import, world) {
	t.Helper()
	w := packedWorld(t, files)
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return imp, w
}

func TestSizeOverTheCapIsRefused(t *testing.T) {
	fakeDisk(t, 10<<30)
	old := maxUnpack
	maxUnpack = 1000
	t.Cleanup(func() { maxUnpack = old })
	imp, w := importPacked(t, map[string][]byte{"Rel/" + relName + ".rar": rar4(relName+".mkv", []byte("packed"), 5000, false, false, true, false)})
	if imp.State != "failed" || lastWhy(imp) != whyTooBig {
		t.Fatalf("%s %+v", imp.State, imp.Plan)
	}
	noLeftovers(t, w.dl)
}

func TestLinkInASetKeepsItAwayFromTheTool(t *testing.T) {
	fakeDisk(t, 10<<30)
	imp, w := importPacked(t, map[string][]byte{"Rel/" + relName + ".rar": asLink(rar4("link.mkv", []byte("x"), 5000, false, false, true, false))})
	if imp.State != "failed" || lastWhy(imp) != whySafely {
		t.Fatalf("%s %+v", imp.State, imp.Plan)
	}
	noLeftovers(t, w.dl)
}
