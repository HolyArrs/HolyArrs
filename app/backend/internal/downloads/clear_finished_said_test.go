package downloads

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The History line for a cleared Usenet job says whether its files went too,
// and says so plainly when they were left on disk.
func TestClearFinishedSaysWhatHappenedToTheFiles(t *testing.T) {
	const name = "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS"
	cases := []struct {
		name    string
		content func(share string) string // the job folder as the Downloader reports it
		want    string
		notWant string
	}{
		{"folder inside the save folder is deleted", func(string) string { return "/downloads/complete/" + name }, "and its files", "kept"},
		{"folder HolyArrs cannot see is kept and said", func(string) string { return "/elsewhere/complete/" + name }, "files were kept", "and its files"},
		{"no folder reported says nothing about files", func(string) string { return "" }, "Cleared the imported download from", "kept"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			share := t.TempDir()
			if err := os.MkdirAll(filepath.Join(share, "complete", name), 0o755); err != nil {
				t.Fatal(err)
			}
			w, m, id, spy := usenetDownload(t)
			if err := execSQL(w, `UPDATE downloaders SET path_map = '[{"theirs":"/downloads","ours":`+jsonStr(share)+`}]'`); err != nil {
				t.Fatal(err)
			}
			m.Forget(w.download(t, id).DownloaderID)
			spy.content = c.content(share)
			m.ClearFinished(context.Background(), id)
			var said string
			if err := w.s.ReadEvents().QueryRow("SELECT summary FROM events WHERE kind = 'download_removed' AND download_id = ? ORDER BY id DESC LIMIT 1", id).Scan(&said); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(said, c.want) || strings.Contains(said, c.notWant) {
				t.Fatalf("said %q, want %q and not %q", said, c.want, c.notWant)
			}
		})
	}
}
