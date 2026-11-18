//go:build windows

package importer

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A file another program holds open for itself alone says it could not be read to check, not that its audio falls short.
func TestMovieImportSaysALockedFileCouldNotBeRead(t *testing.T) {
	w := setup(t)
	setVersionAudio(t, w, "fr")
	if err := execSQL(w.s, "UPDATE slots SET active_download_id = ? WHERE id = ?", w.dlID, w.slotID); err != nil {
		t.Fatal(err)
	}
	name, err := syscall.UTF16PtrFromString(filepath.Join(w.dl, relName+".mkv"))
	if err != nil {
		t.Fatal(err)
	}
	h, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.CloseHandle(h) })
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State == "done" {
		t.Fatalf("import %+v %v", imp, err)
	}
	if sl := slot308(t, w); sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != couldNotPlace(unreadCheckWhy) {
		t.Fatalf("slot %s reason %q", sl.State, ptrText(sl.LastReason))
	}
}
