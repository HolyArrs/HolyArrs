package downloads

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// a movie on Needs a decision whose download holds one video can be
// imported from Review; a movie with a file needs replace.
func TestDecideImportsTheDownloadedFile(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotNeedsADecision, "finished")
	save := t.TempDir()
	dir := filepath.Join(save, "Ein.Anderer.Name.2020.German.DL.1080p.BluRay.x264-GROUP")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "grp-ean.mkv"), make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	exec41(t, x.s, `UPDATE downloads SET save_path = ?, files_seen = ? WHERE id = ?`, save,
		`[{"name":"Ein.Anderer.Name.2020.German.DL.1080p.BluRay.x264-GROUP/grp-ean.mkv","size":4096}]`, x.dlID)
	exec41(t, x.s, "UPDATE slots SET last_reason = 'no file in the download matched this movie; match the files by hand' WHERE id = ?", x.slotID)

	var out struct {
		Items []DecisionView `json:"items"`
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/needs-decision", "", 200), &out)
	if len(out.Items) != 1 || out.Items[0].ImportFile != "grp-ean.mkv" {
		t.Fatalf("items %+v", out.Items)
	}
	u := x.srv.URL + "/api/v1/needs-decision/" + itoa(x.slotID) + "/decide"
	b := string(do(t, "POST", u, `{"action":"import"}`, 200))
	if !strings.Contains(b, `"outcome":"placed"`) {
		t.Fatalf("import %s", b)
	}
	var st string
	var file *int64
	_ = x.s.Read().QueryRow("SELECT state, file_id FROM slots WHERE id = ?", x.slotID).Scan(&st, &file)
	if file == nil || (st != "in_library" && st != "better_wanted") {
		t.Fatalf("slot %s %v", st, file)
	}
	// No longer waiting: a second import is refused.
	do(t, "POST", u, `{"action":"import"}`, 404)

	// A movie that has a file keeps it unless replace is sent.
	exec41(t, x.s, "UPDATE slots SET state = 'needs_a_decision', last_reason = 'no file in the download matched this movie; match the files by hand' WHERE id = ?", x.slotID)
	_ = os.Remove(filepath.Join(dir, "grp-ean.mkv")) // a hardlink of the Library file: a new copy, not the same one
	if err := os.WriteFile(filepath.Join(dir, "grp-ean.mkv"), make([]byte, 8192), 0o644); err != nil {
		t.Fatal(err)
	}
	do(t, "POST", u, `{"action":"import"}`, 409)
	b = string(do(t, "POST", u, `{"action":"import","replace":true}`, 200))
	if !strings.Contains(b, `"outcome":"placed"`) {
		t.Fatalf("replace %s", b)
	}
	var gone int
	_ = x.s.Read().QueryRow("SELECT count(*) FROM files WHERE state = 'gone'").Scan(&gone)
	if gone != 1 {
		t.Fatalf("%d files deleted, want the replaced one", gone)
	}
}
