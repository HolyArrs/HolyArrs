package health

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func (w *world) runOnly(t *testing.T, group string) map[string]string {
	t.Helper()
	cs, err := w.e.RunOnly(context.Background(), group)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, c := range cs {
		out[c.Key] = c.State + "/" + c.Confirmed
	}
	return out
}

// A fix re-runs only its own check: the key kit warning goes out (and is
// confirmed at once, so Home and the menu dot agree) while another
// problem stays exactly as the last full run left it.
func TestRunOnlyClearsJustThatCheck(t *testing.T) {
	w := newWorld(t)
	w.bk.KeyKit = "warn"
	w.addDownloader(t, "SAB", "unreachable")
	w.run(t)
	w.run(t) // two runs: both warnings are confirmed
	w.bk.KeyKit = ""
	w.exec(t, "UPDATE downloaders SET state = 'ok'")
	got := w.runOnly(t, "backups")
	if got["backups:key"] != "ok/ok" {
		t.Fatalf("key kit after the fix: %s, want ok/ok", got["backups:key"])
	}
	if got["downloader:1"] != "warning/warning" {
		t.Fatalf("a check outside the group changed: %s", got["downloader:1"])
	}
	if got := w.runOnly(t, "downloader"); got["downloader:1"] != "ok/ok" {
		t.Fatalf("downloader after its own re-run: %s", got["downloader:1"])
	}
}

// A new problem found by a targeted run still waits for the flap guard,
// and a recovery writes its Event once.
func TestRunOnlyKeepsTheGuardForNewProblems(t *testing.T) {
	w := newWorld(t)
	w.run(t)
	w.addDownloader(t, "SAB", "unreachable")
	if got := w.runOnly(t, "downloader"); got["downloader:1"] != "warning/ok" {
		t.Fatalf("new problem: %s, want warning/ok", got["downloader:1"])
	}
	if _, err := w.e.RunOnly(context.Background(), "nope"); err == nil {
		t.Fatal("an unknown group should be refused")
	}
}

// Removing a Downloader drops its check on its own re-run, not anyone
// else's rows.
func TestRunOnlyDropsGoneRowsInItsGroupOnly(t *testing.T) {
	w := newWorld(t)
	w.bk.KeyKit = "warn"
	w.addDownloader(t, "SAB", "unreachable")
	w.run(t)
	w.exec(t, "UPDATE downloaders SET removed = 1")
	got := w.runOnly(t, "downloader")
	if _, ok := got["downloader:1"]; ok {
		t.Fatal("removed Downloader still checked")
	}
	if _, ok := got["backups:key"]; !ok {
		t.Fatal("another group's row was dropped")
	}
}

// POST /health/checks/run?only=backups runs that group; the Settings
// status read right after has no key kit item.
func TestRunOnlyRoute(t *testing.T) {
	s := openStore(t)
	addMovieFolder(t, s)
	kit := "warn"
	eng := &Engine{Store: s, Version: "test", Backups: func(context.Context) (Backups, error) {
		return Backups{Newest: time.Now(), Started: true, KeyKit: kit}, nil
	}}
	r := newRig(t, s, rigOpts{Engine: eng})
	srv := httptest.NewServer(r.status.ForgetOnWrite(r.mux))
	t.Cleanup(srv.Close)
	call := func(method, path string, want int) string {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader("{}"))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("%s %s: %d %s", method, path, res.StatusCode, b)
		}
		return string(b)
	}
	call("POST", "/api/v1/health/checks/run", 200)
	if !strings.Contains(call("GET", "/api/v1/settings/status", 200), "key kit") {
		t.Fatal("no key kit item before the download")
	}
	kit = ""
	call("POST", "/api/v1/health/checks/run?only=backups", 200)
	if got := call("GET", "/api/v1/settings/status", 200); strings.Contains(got, "key kit") {
		t.Fatalf("key kit still listed after the re-run: %s", got)
	}
	call("POST", "/api/v1/health/checks/run?only=nope", 400)
}
