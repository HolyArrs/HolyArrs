package downloads

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A Downloader that cannot be reached when a finished download is removed
// with its files: History says nothing was removed yet, the choice to delete
// the files is kept, and the later try checks again whether they may go.
func TestRemoveDoneUnreachableKeepsDeleteChoiceForLater(t *testing.T) {
	for _, link := range []string{"copy", "inplace"} {
		t.Run(link, func(t *testing.T) {
			w, m, id, eng, dlFile, libFile := doneOnDisk(t, link)
			dr := w.download(t, id).DownloaderID
			m.Engine = func(domain.Downloader) (Engine, error) { return nil, errors.New("connection refused") }
			m.Forget(dr)
			note, err := m.RemoveSaid(context.Background(), id, RemoveChoice{DeleteFiles: true}, luis, "luis")
			if err != nil || note == "" {
				t.Fatalf("%q %v", note, err)
			}
			var n int
			if err := w.s.ReadEvents().QueryRow("SELECT count(*) FROM events WHERE kind = 'download_removed' AND download_id = ?", id).Scan(&n); err != nil || n != 0 {
				t.Fatalf("History says removed before %s answered: %d %v", "the Downloader", n, err)
			}
			m.Engine = func(domain.Downloader) (Engine, error) { return eng, nil }
			m.Forget(dr)
			m.retryRemovals(context.Background())
			d := lastData(t, w, "download_removed", id)
			if d["retried"] != true {
				t.Fatalf("retry data %v", d)
			}
			stillThere(t, libFile, "the Library copy")
			if link == "copy" {
				if _, err := os.Stat(dlFile); !os.IsNotExist(err) || d["delete_files"] != true {
					t.Fatalf("files not deleted on the later try: %v %v", err, d)
				}
				return
			}
			if d["delete_files"] != false || !strings.Contains(fmt.Sprint(d["kept"]), "Library folder") {
				t.Fatalf("kept why missing: %v", d)
			}
		})
	}
}
