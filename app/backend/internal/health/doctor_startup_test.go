package health

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
)

func shareKind(kind string) func(string) (bool, string, error) {
	return func(string) (bool, string, error) { return true, kind, nil }
}

func lastCheck(t *testing.T, r Report, has string) Check {
	t.Helper()
	for i := len(r.Checks) - 1; i >= 0; i-- {
		if strings.Contains(r.Checks[i].Text, has) {
			return r.Checks[i]
		}
	}
	t.Fatalf("no check with %q in\n%s", has, r.Text())
	return Check{}
}

// The doctor fix line comes from the same table as the start-up refusal,
// so a Windows user is not told about cache drives.
func TestConfigShareFixFollowsTheKind(t *testing.T) {
	cases := []struct {
		kind   string
		has    []string
		hasNot []string
	}{
		{"9p", []string{"named volume"}, []string{"cache drive", "not NFS or SMB"}},
		{"fakeowner", []string{"Docker Desktop", "named volume"}, []string{"cache drive"}},
		{"fuse", []string{"/mnt/<pool>/appdata"}, []string{"cache"}},
		{"nfs", []string{"local disk"}, nil},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			var r Report
			configChecks(&r, t.TempDir(), 1, 1, shareKind(c.kind))
			if !r.Failed() {
				t.Fatalf("not a FAIL:\n%s", r.Text())
			}
			fix := r.Checks[len(r.Checks)-1].Fix
			if fix != files.ShareFix(c.kind) {
				t.Errorf("fix %q, want files.ShareFix(%q) = %q", fix, c.kind, files.ShareFix(c.kind))
			}
			for _, w := range c.has {
				if !strings.Contains(fix, w) {
					t.Errorf("fix %q lacks %q", fix, w)
				}
			}
			for _, w := range c.hasNot {
				if strings.Contains(fix, w) {
					t.Errorf("fix %q should not say %q", fix, w)
				}
			}
		})
	}
}

// With the Desktop override the fakeowner folder is a WARN (it starts),
// never OK and never FAIL; other kinds stay FAIL.
func TestDesktopOverrideIsAWarning(t *testing.T) {
	t.Setenv("HOLYARRS_ALLOW_DESKTOP_FOLDER", "1")
	var r Report
	configChecks(&r, t.TempDir(), 1, 1, shareKind("fakeowner"))
	if r.Failed() {
		t.Fatalf("override set, still FAIL:\n%s", r.Text())
	}
	if c := r.Checks[len(r.Checks)-1]; c.Status != StatusWarn || !strings.Contains(c.Text+c.Fix, "HOLYARRS_ALLOW_DESKTOP_FOLDER") {
		t.Fatalf("want a WARN naming the override, got %+v", c)
	}
	r = Report{}
	configChecks(&r, t.TempDir(), 1, 1, shareKind("fuse"))
	if !r.Failed() {
		t.Fatalf("the Desktop override must not soften FUSE:\n%s", r.Text())
	}
}

func TestConfigInMemoryFails(t *testing.T) {
	for _, kind := range []string{"tmpfs", "ramfs"} {
		var r Report
		configChecks(&r, t.TempDir(), 1, 1, shareKind(kind))
		c := r.Checks[len(r.Checks)-1]
		if c.Status != StatusFail || !strings.Contains(c.Text, "memory") || strings.Contains(c.Text, "network share") {
			t.Fatalf("%s: %+v", kind, c)
		}
	}
	t.Setenv("HOLYARRS_CONFIG_IN_MEMORY", "yes")
	var r Report
	configChecks(&r, t.TempDir(), 1, 1, shareKind("tmpfs"))
	if c := r.Checks[len(r.Checks)-1]; c.Status != StatusWarn {
		t.Fatalf("override set: want WARN, got %+v", c)
	}
}

// The lock probe only reports: a broken lock is a WARN, never a FAIL.
func TestLockProbeOnlyReports(t *testing.T) {
	var r Report
	lockChecks(&r, t.TempDir(), func(string) error { return errors.New("locks are ignored here") })
	c := lastCheck(t, r, "lock")
	if c.Status != StatusWarn || r.Failed() {
		t.Fatalf("broken lock: want WARN only, got %+v", c)
	}
	if !strings.Contains(c.Fix, "named volume") {
		t.Errorf("fix should point at a named volume: %q", c.Fix)
	}
	r = Report{}
	lockChecks(&r, t.TempDir(), func(string) error { return nil })
	if c := lastCheck(t, r, "lock"); c.Status != StatusOK {
		t.Fatalf("working lock: %+v", c)
	}
}

// Forgot to map /config: doctor fails with the docker cp rescue; mapped
// or unsure stays silent.
func TestUnmappedConfigInDoctor(t *testing.T) {
	for _, kind := range []string{"anonymous", "none"} {
		var r Report
		unmappedChecks(&r, "/config", kind)
		if !r.Failed() {
			t.Fatalf("%s: not a FAIL:\n%s", kind, r.Text())
		}
		c := r.Checks[len(r.Checks)-1]
		if !strings.Contains(c.Fix, "docker cp") || !strings.Contains(c.Text+c.Fix, "/config") {
			t.Errorf("%s: fix should carry the docker cp rescue: %+v", kind, c)
		}
		// A live copy can catch the database and its -wal at different
		// moments: stop first.
		if i, j := strings.Index(c.Fix, "docker stop"), strings.Index(c.Fix, "docker cp"); i < 0 || i > j {
			t.Errorf("%s: fix should stop the container before docker cp: %s", kind, c.Fix)
		}
	}
	var r Report
	unmappedChecks(&r, "/config", "")
	if len(r.Checks) != 0 {
		t.Fatalf("mapped or unsure must stay silent: %+v", r.Checks)
	}
}

// Rootless Docker/Podman: container root is the user's own account, so
// the root note must not push PUID/PGID there (item 10, fixed text).
func TestRootNoteMentionsRootless(t *testing.T) {
	var r Report
	appChecks(&r, "1.0.0", 0, 0)
	c := lastCheck(t, r, "running as 0:0")
	if c.Status != StatusWarn || !strings.Contains(c.Fix, "rootless") {
		t.Fatalf("root note: %+v", c)
	}
}

// writeFix branches on why the write failed, so a read-only mount does
// not get chown advice.
func TestWriteFixBranchesOnTheError(t *testing.T) {
	ro := writeFix("/media", 1000, 1000, "1000:1000", syscall.EROFS)
	if !strings.Contains(ro, "remove :ro") || strings.Contains(ro, "chown") {
		t.Errorf("EROFS fix: %q", ro)
	}
	right := writeFix("/media", 1000, 1000, "1000:1000", syscall.EACCES)
	if strings.Contains(right, "chown -R") || !(strings.Contains(right, "label") || strings.Contains(right, "ACL")) {
		t.Errorf("owner already right: %q", right)
	}
	wrong := writeFix("/media", 1000, 1000, "0:0", syscall.EACCES)
	if !strings.Contains(wrong, "chown -R 1000:1000") {
		t.Errorf("owner wrong: %q", wrong)
	}
}

func TestSlowFolderWording(t *testing.T) {
	c := (Folder{Role: "downloads", Path: "/d", State: "slow"}).Check(1, 1)
	if !strings.Contains(c.Text, "waking from sleep or a slow network share") {
		t.Fatalf("slow text: %q", c.Text)
	}
}

// --- doctor-race: doctor run while the server still sets up the data ---

func runDoctorStartup(t *testing.T, dir string) Report {
	t.Helper()
	return RunDoctor(context.Background(), DoctorOptions{ConfigDir: dir, MetadataURL: "https://meta.invalid",
		HTTP: &http.Client{Transport: failRT{}}, NetTimeout: 100 * time.Millisecond, FolderBudget: time.Second})
}

func rawExec(t *testing.T, dir, q string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(dir, database.MainFile))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}

func migratedStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{NetworkShare: func(string) (bool, string, error) { return false, "", nil }})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	return dir
}

// dataFailed reports a FAIL outside the Network section (the tests point
// the metadata check at an address that never answers).
func dataFailed(r Report) bool {
	for _, c := range r.Checks {
		if c.Status == StatusFail && c.Section != "Network" {
			return true
		}
	}
	return false
}

func assertSetupWarn(t *testing.T, r Report) {
	t.Helper()
	text := r.Text()
	if dataFailed(r) {
		t.Fatalf("mid-setup must not FAIL:\n%s", text)
	}
	c := lastCheck(t, r, "still being set up")
	if c.Status != StatusWarn {
		t.Fatalf("want WARN, got %+v", c)
	}
	for _, bad := range []string{"SQL logic error", "no such", "could not read the folders", "no Library or download folders set yet"} {
		if strings.Contains(text, bad) {
			t.Errorf("report says %q:\n%s", bad, text)
		}
	}
}

func TestDoctorDuringMigrationIsOneWarning(t *testing.T) {
	dir := migratedStore(t)
	rawExec(t, dir, "DELETE FROM schema_migrations WHERE version > 10")
	assertSetupWarn(t, runDoctorStartup(t, dir))
}

func TestDoctorOnAnEmptyDatabaseIsOneWarning(t *testing.T) {
	dir := t.TempDir()
	rawExec(t, dir, "PRAGMA user_version = 0") // creates an empty database file
	assertSetupWarn(t, runDoctorStartup(t, dir))
}

func TestDoctorOnNewerDataSaysSo(t *testing.T) {
	dir := migratedStore(t)
	know, _ := database.SchemaVersions()
	rawExec(t, dir, "INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, 'future.sql', 't')", know+1)
	r := runDoctorStartup(t, dir)
	if dataFailed(r) {
		t.Fatalf("newer data is a WARN:\n%s", r.Text())
	}
	c := lastCheck(t, r, "newer HolyArrs")
	if c.Status != StatusWarn {
		t.Fatalf("want WARN, got %+v", c)
	}
	if strings.Contains(r.Text(), "no Library or download folders set yet") {
		t.Errorf("folders should be skipped:\n%s", r.Text())
	}
}

func TestDoctorOnAMigratedStoreAddsNoSetupLine(t *testing.T) {
	r := runDoctorStartup(t, migratedStore(t))
	text := r.Text()
	if strings.Contains(text, "still being set up") || strings.Contains(text, "newer HolyArrs") {
		t.Fatalf("fully migrated store:\n%s", text)
	}
}
