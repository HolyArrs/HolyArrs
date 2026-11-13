package downloads

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A download where nothing matched gives the folder Import files by
// hand opens with, and its reason still sorts as an import problem.
func TestDecisionGivesTheDownloadFolder(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotNeedsADecision, "finished")
	save := t.TempDir()
	dir := filepath.Join(save, "Some.Pack.S01.1080p-GRP")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"a.mkv", "b.mkv"} {
		if err := os.WriteFile(filepath.Join(dir, n), make([]byte, 4096), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	exec41(t, x.s, `UPDATE downloads SET save_path = ?, files_seen = ? WHERE id = ?`, save,
		`[{"name":"Some.Pack.S01.1080p-GRP/a.mkv","size":4096},{"name":"Some.Pack.S01.1080p-GRP/b.mkv","size":4096}]`, x.dlID)
	exec41(t, x.s, "UPDATE slots SET last_reason = 'no file in the download matched this movie; match the files by hand' WHERE id = ?", x.slotID)
	var out struct {
		Items []DecisionView `json:"items"`
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/needs-decision", "", 200), &out)
	if len(out.Items) != 1 || out.Items[0].ImportFolder != dir || out.Items[0].Why != "conflict" {
		t.Fatalf("items %+v", out.Items)
	}
	if out.Items[0].Reason != "No file in the download matched this movie; match the files by hand." {
		t.Fatalf("reason %q", out.Items[0].Reason)
	}
}

func TestWhyNoMatchIsConflict(t *testing.T) {
	for _, r := range []string{"no file in the download matched an episode; match the files by hand", "nothing could be placed; look at the import plan", "nothing in the download could be matched to a track; look at the import plan"} {
		if got := why(r); got != "conflict" {
			t.Fatalf("%q: %s", r, got)
		}
	}
}
