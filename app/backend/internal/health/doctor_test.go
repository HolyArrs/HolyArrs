package health

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func TestDoctorReportOnAFreshStore(t *testing.T) {
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	dl := t.TempDir()
	_ = s.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" || r.URL.Path != "/" {
			t.Errorf("asked %s", r.URL)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	// srv.Client: the app's own dial policy refuses this loopback test server.
	r := RunDoctor(context.Background(), DoctorOptions{Version: "9.9.9", ConfigDir: dir, SavePath: dl, HTTP: srv.Client(),
		MetadataURL: srv.URL + "/v1/secret?apikey=SHOULD_NOT_SHOW", Now: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)})
	text := r.Text()
	for _, want := range []string{"App", "version 9.9.9", "Config folder", "writable", "local disk", "Network", "answered (HTTP 204)", "Clock", "downloads " + dl, "Result:"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
	if strings.Contains(text, "SHOULD_NOT_SHOW") || strings.Contains(text, "/v1/secret") {
		t.Fatalf("link details shown:\n%s", text)
	}
	if r.Failed() {
		t.Fatalf("failed:\n%s", text)
	}
	if !strings.Contains(string(r.JSON()), `"ok": true`) {
		t.Fatalf("json %s", r.JSON())
	}
}

func TestDoctorFolders(t *testing.T) {
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	movies, dl := t.TempDir(), t.TempDir()
	gone := filepath.Join(t.TempDir(), "gone")
	ctx := context.Background()
	for _, p := range []string{movies, gone} {
		if err := writeFolder(s, p); err != nil {
			t.Fatal(err)
		}
	}
	fs, err := CheckFolders(ctx, s.Read(), dl, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, f := range fs {
		states[f.Path] = f.State
	}
	if states[movies] != "ok" || states[gone] != "missing" || states[dl] != "ok" {
		t.Fatalf("states %v", states)
	}
	sum, warns := DoctorStartup(ctx, s.Read(), dl, 10*time.Second)
	if sum != "folders: 2 ok, 1 missing" || len(warns) != 1 || !strings.Contains(warns[0], gone) || !strings.Contains(warns[0], "add the volume") {
		t.Fatalf("summary %q warns %q", sum, warns)
	}
}

func writeFolder(s *database.Store, path string) error {
	return s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO library_folders (created_at, created_by, updated_at, updated_by, path, kind, label, health, same_disk_as)
			VALUES ('t', '{}', 't', '{}', ?, 'movie', 'Movies', 'ok', '[]')`, path)
		return err
	})
}

func TestFolderChecksWordsAndSlow(t *testing.T) {
	f := Folder{Role: "movie library", Path: "/mnt/user/media", State: "read_only", Owner: "0:0"}
	c := f.Check(99, 100)
	if c.Status != StatusFail || !strings.Contains(c.Fix, "chown -R 99:100") || !strings.Contains(c.Fix, "/mnt/user/media") || !strings.Contains(c.Fix, "PUID/PGID") {
		t.Fatalf("%+v", c)
	}
	if c := (Folder{Role: "movie library", Path: "/m", State: "no_hardlink", Free: 1 << 30}).Check(1, 1); c.Status != StatusWarn || !strings.Contains(c.Fix, "one mount") {
		t.Fatalf("%+v", c)
	}
	if c := (Folder{Role: "downloads", Path: "/d", State: "slow"}).Check(1, 1); c.Status != StatusWarn || !strings.Contains(c.Text, "skipped") {
		t.Fatalf("%+v", c)
	}
	if got := Summary([]Folder{{State: "ok"}, {State: "ok"}, {State: "read_only"}, {State: "slow"}}); got != "folders: 2 ok, 1 not writable, 1 not checked (slow to answer)" {
		t.Fatal(got)
	}
}

func TestConfigOnANetworkShareFails(t *testing.T) {
	var r Report
	configChecks(&r, t.TempDir(), 1, 1, func(string) (bool, string, error) { return true, "nfs", nil })
	if !r.Failed() || !strings.Contains(r.Text(), "must be on a local disk") {
		t.Fatal(r.Text())
	}
	r = Report{}
	configChecks(&r, filepath.Join(t.TempDir(), "nope"), 1, 1, func(string) (bool, string, error) { return false, "", nil })
	if !r.Failed() {
		t.Fatal(r.Text())
	}
}

func TestNetworkAndClock(t *testing.T) {
	var r Report
	networkChecks(context.Background(), &r, "https://meta.example", time.Second, &http.Client{Transport: failRT{}},
		func(context.Context, string) ([]string, error) { return nil, errors.New("no such host") })
	if !r.Failed() || !strings.Contains(r.Text(), "cannot resolve meta.example") {
		t.Fatal(r.Text())
	}
	r = Report{}
	clockChecks(&r, time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC), "Europe/Berlin")
	if r.Failed() || !strings.Contains(r.Text(), "WARN") || !strings.Contains(r.Text(), "Europe/Berlin") {
		t.Fatal(r.Text())
	}
	if parseUmask("Name:\tx\nUmask:\t0022\n") != "0022" || parseUmask("") != "unknown" {
		t.Fatal("umask")
	}
	if Size(3<<30) != "3.0 GB" || Size(-1) != "unknown" {
		t.Fatal(Size(3 << 30))
	}
}

type failRT struct{}

func (failRT) RoundTrip(*http.Request) (*http.Response, error) { return nil, errors.New("refused") }

func TestOpenReadOnlyNeverCreates(t *testing.T) {
	dir := t.TempDir()
	if _, err := database.OpenReadOnly(dir); err == nil {
		t.Fatal("opened a missing database")
	}
	if _, err := os.Stat(filepath.Join(dir, database.MainFile)); err == nil {
		t.Fatal("created the database")
	}
}

func dbChecks(t *testing.T, dir string) []Check {
	t.Helper()
	r := RunDoctor(context.Background(), DoctorOptions{ConfigDir: dir, MetadataURL: "https://meta.invalid",
		HTTP: &http.Client{Transport: failRT{}}, NetTimeout: 100 * time.Millisecond, FolderBudget: time.Second})
	var out []Check
	for _, c := range r.Checks {
		if strings.Contains(c.Text, "no database") || strings.Contains(c.Text, "open the database") {
			out = append(out, c)
		}
	}
	return out
}

func TestDoctorMissingDatabaseIsAWarning(t *testing.T) {
	got := dbChecks(t, t.TempDir())
	if len(got) != 1 || got[0].Status != StatusWarn || !strings.Contains(got[0].Text, "no database yet") {
		t.Fatalf("got %+v", got)
	}
}

func TestDoctorUnopenableDatabaseFails(t *testing.T) {
	dir := t.TempDir()
	// A folder where the database file should be: it exists but cannot be
	// opened, like a database this user cannot read.
	if err := os.Mkdir(filepath.Join(dir, database.MainFile), 0o755); err != nil {
		t.Fatal(err)
	}
	got := dbChecks(t, dir)
	if len(got) != 1 || got[0].Status != StatusFail || !strings.Contains(got[0].Text, "cannot open the database") || got[0].Fix == "" {
		t.Fatalf("got %+v", got)
	}
}

func TestDoctorClientIgnoresProxyEnv(t *testing.T) {
	tr, ok := metadataClient().Transport.(*http.Transport)
	if !ok || tr.Proxy != nil || tr.DialContext == nil {
		t.Fatalf("the doctor must dial like the app: no proxy, policy dialer (%T)", metadataClient().Transport)
	}
}
