package downloads

import (
	"os"
	"path/filepath"
	"testing"
)

// SABnzbd can name a folder or file inside the job as a finished job's place
// (a season pack unpacked into one folder per episode). The job's own folder
// is taken instead, so every episode of the pack is read, as Sonarr does.
func TestSabnzbdStorageInsideTheJob(t *testing.T) {
	pack := "MobLand.S02.MULTi.1080p.WEB.AV1-SUPPLY"
	ep := "MobLand.S02E01.MULTi.1080p.WEB.AV1-SUPPLY"
	for storage, want := range map[string]string{
		"/media/complete/" + pack + "/" + ep:                     "/media/complete/" + pack,
		"/media/complete/" + pack + "/" + ep + "/" + ep + ".mkv": "/media/complete/" + pack,
		"/media/complete/" + pack:                                "/media/complete/" + pack,
		`C:\dl\complete\` + pack + `\` + ep:                      `C:\dl\complete\` + pack,
		"/media/complete/Other.Name":                             "/media/complete/Other.Name",
	} {
		j := sabHistorySlot{ID: "SABnzbd_nzo_1", Name: pack, Status: "Completed", Storage: storage}.job()
		if j.ContentPath != want || j.SavePath != parentDir(want) {
			t.Errorf("%s: content %q save %q, want %q", storage, j.ContentPath, j.SavePath, want)
		}
	}

	// The whole pack is read from the job's folder, not one episode.
	root := t.TempDir()
	for _, e := range []string{"S02E01", "S02E02", "S02E10.FiNAL"} {
		n := "MobLand." + e + ".MULTi.1080p.WEB.AV1-SUPPLY"
		p := filepath.Join(root, pack, n, n+".mkv")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	j := sabHistorySlot{ID: "SABnzbd_nzo_1", Name: pack, Status: "Completed", Storage: filepath.Join(root, pack, ep)}.job()
	save, files := readFinished(j, nil, root)
	if filepath.Clean(save) != filepath.Clean(root) || len(files) != 3 {
		t.Fatalf("save %q files %+v", save, files)
	}
}
