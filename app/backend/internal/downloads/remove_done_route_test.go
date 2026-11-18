package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// postRemove sends POST /api/v1/downloads/{id}/remove as the owner.
func postRemove(t *testing.T, w *world, m *Manager, id int64, body string) *httptest.ResponseRecorder {
	t.Helper()
	a := &Web{Store: w.s, Downloads: m}
	r := httptest.NewRequest("POST", "/api/v1/downloads/"+strconv.FormatInt(id, 10)+"/remove", strings.NewReader(body))
	r.SetPathValue("id", strconv.FormatInt(id, 10))
	r = r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{UserID: 1, Role: webserver.RoleOwner, Via: "session"}))
	rec := httptest.NewRecorder()
	a.removeDownload(rec, r)
	return rec
}

// The Downloader's refusal reaches the answer's client_note; a read-only
// Downloader and a running import answer 409.
func TestRemoveDoneRouteSaysClientNoteAndConflicts(t *testing.T) {
	w, m, id, se := seededDownload(t, "qbittorrent", `{}`, Job{Detail: "uploading"})
	m.Engine = func(domain.Downloader) (Engine, error) {
		return &diskEngine{seedEngine: se, fail: errors.New("qBittorrent said no")}, nil
	}
	rec := postRemove(t, w, m, id, `{"delete_files":false}`)
	var out struct {
		ClientNote string `json:"client_note"`
		Phase      string `json:"phase"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil || !strings.Contains(out.ClientNote, "qBittorrent said no") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}

	w, m, id, _ = seededDownload(t, "qbittorrent", `{}`, Job{Detail: "uploading"})
	if err := execSQL(w, `UPDATE downloaders SET writable = 0`); err != nil {
		t.Fatal(err)
	}
	m.Forget(w.download(t, id).DownloaderID)
	if rec := postRemove(t, w, m, id, `{}`); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "read-only") {
		t.Fatalf("read-only: %d %s", rec.Code, rec.Body.String())
	}

	w, m, id, _ = seededDownload(t, "qbittorrent", `{}`, Job{Detail: "uploading"})
	if err := execSQL(w, `UPDATE imports SET state = 'placing'`); err != nil {
		t.Fatal(err)
	}
	if rec := postRemove(t, w, m, id, `{}`); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "library") {
		t.Fatalf("importing: %d %s", rec.Code, rec.Body.String())
	}
}

// A member who may not manage the Library never gets Remove on a finished row.
func TestDownloadsViewDoneRowNotRemovableWithoutManage(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotInLibrary, "seeding")
	exec41(t, x.s, `UPDATE downloaders SET writable = 1`)
	exec41(t, x.s, `INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES
		(1, 't', '{}', 't', '{}', 'owner', 'owner', 'x'), (2, 't', '{}', 't', '{}', 'ana', 'member', 'x')`)
	exec41(t, x.s, `INSERT INTO asks (id, created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, title_id, asked_by, asked_at)
		SELECT 1, 't', '{}', 't', '{}', 'movie', '1', 'Salt Choir', 'approved', t.id, 2, 't' FROM titles t LIMIT 1`)
	exec41(t, x.s, `INSERT INTO ask_people (ask_id, user_id, asked_at) VALUES (1, 2, 't')`)
	asker := webserver.Caller{UserID: 2, Role: webserver.RoleMember, Perms: []webserver.Perm{webserver.PermAsk}, Via: "session"}
	if r := viewAs(t, x, asker); len(r) != 1 || r[0].CanAct {
		t.Fatalf("asker %+v", r)
	}
	approver := webserver.Caller{UserID: 3, Role: webserver.RoleMember, Perms: []webserver.Perm{webserver.PermAsk, webserver.PermApprove}, Via: "session"}
	if r := viewAt(t, x, approver, "/api/v1/downloads/view?asked=1"); len(r) != 1 || r[0].CanAct {
		t.Fatalf("approver %+v", r)
	}
	admin := webserver.Caller{UserID: 1, Role: webserver.RoleAdmin, Perms: []webserver.Perm{webserver.PermManageLibrary}, Via: "session"}
	if r := viewAs(t, x, admin); len(r) != 1 || !r[0].CanAct {
		t.Fatalf("admin who manages %+v", r)
	}
}

// lockFolder makes the download's files impossible to delete: an open file
// on Windows, a folder without write rights elsewhere.
func lockFolder(t *testing.T, file string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		f, err := os.Open(file)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = f.Close() })
		return
	}
	if os.Geteuid() == 0 {
		t.Skip("root deletes anything")
	}
	dir := filepath.Dir(file)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}

// Locked or no-permission files: when the Downloader says so, the person is
// told and it is asked again later; when it drops the torrent and leaves the
// files, the row ends. Either way the Library copy stays.
func TestRemoveDoneLockedFiles(t *testing.T) {
	w, m, id, eng, dlFile, libFile := doneOnDisk(t, "hardlink")
	lockFolder(t, dlFile)
	note, err := m.RemoveSaid(context.Background(), id, RemoveChoice{DeleteFiles: true}, luis, "luis")
	if err != nil || !strings.Contains(note, "tries again") {
		t.Fatalf("%q %v", note, err)
	}
	var pending *bool
	if err := m.Store.Read().QueryRow("SELECT removal_pending FROM downloads WHERE id = ?", id).Scan(&pending); err != nil || pending == nil {
		t.Fatalf("not waiting to retry: %v", err)
	}
	stillThere(t, dlFile, "locked")
	stillThere(t, libFile, "locked")
	if !strings.Contains(lastSummary(t, w, "download_not_removed", id), "tries again") {
		t.Fatal("History does not say the remove failed")
	}
	if len(eng.removed) != 0 {
		t.Fatalf("removed %v", eng.removed)
	}

	w, m, id, eng, dlFile, libFile = doneOnDisk(t, "hardlink")
	lockFolder(t, dlFile)
	eng.quiet = true
	if note, err := m.RemoveSaid(context.Background(), id, RemoveChoice{DeleteFiles: true}, luis, "luis"); err != nil || note != "" {
		t.Fatalf("%q %v", note, err)
	}
	if d := w.download(t, id); d.State != "cleaned" {
		t.Fatalf("download %s", d.State)
	}
	stillThere(t, libFile, "locked, torrent dropped")
}
