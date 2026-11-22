package backups

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// A backup is refused with a plain reason when the config disk cannot hold
// it unpacked, instead of failing midway with a full disk.
func TestStageRefusesWhenTheDiskCannotHoldTheBackup(t *testing.T) {
	m, _, _, _ := setup(t)
	good, err := m.Create(context.Background(), KindManual)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(good.Path)
	m.FreeSpace = func(string) int64 { return unpackHeadroom + 100 }
	err = m.Stage(context.Background(), bytes.NewReader(b))
	if err == nil || !strings.Contains(err.Error(), "Not enough free space") {
		t.Fatalf("got %v", err)
	}
	m.FreeSpace = func(string) int64 { return 64 << 30 }
	if err := m.Stage(context.Background(), bytes.NewReader(b)); err != nil {
		t.Fatalf("with room: %v", err)
	}
}

// backup.json is read whole into memory, so it has its own small cap even
// when the databases may be large.
func TestStageRefusesAHugeManifest(t *testing.T) {
	m, _, _, _ := setup(t)
	meta := append([]byte(`{"app":"holyarrs"}`), bytes.Repeat([]byte(" "), maxMeta+10)...)
	z := zipBytes(t, map[string][]byte{MetaFile: meta, database.MainFile: []byte("x")})
	err := m.Stage(context.Background(), bytes.NewReader(z))
	if err == nil || !strings.Contains(err.Error(), "backup.json is damaged") {
		t.Fatalf("got %v", err)
	}
}

// Backups have no size cap, so a restore takes more than the 2 GiB it
// used to.
func TestRestoreTakesMoreThanTwoGiB(t *testing.T) {
	if MaxUpload <= 2<<30 || maxUnpacked <= 2<<30 {
		t.Fatalf("restore caps %d / %d", MaxUpload, maxUnpacked)
	}
}

// An upload that takes longer than the server's whole-request read timeout
// still finishes while it keeps sending.
func TestRestoreUploadOutlastsTheReadTimeout(t *testing.T) {
	m, _, _, _ := setup(t)
	good, err := m.Create(context.Background(), KindManual)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(good.Path)
	mux := http.NewServeMux()
	m.Routes(mux, nil)
	// In the app the restore route runs behind webserver.StatusWriter (the
	// status cache's ForgetOnWrite), which must let the deadline through.
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(&webserver.StatusWriter{ResponseWriter: w}, r)
	}))
	srv.Config.ReadTimeout = 300 * time.Millisecond
	srv.Start()
	defer srv.Close()
	pr, pw := io.Pipe()
	go func() {
		// Send in pieces over about a second, three times the timeout.
		step := max(len(b)/10, 1)
		for i := 0; i < len(b); i += step {
			if _, err := pw.Write(b[i:min(i+step, len(b))]); err != nil {
				pw.CloseWithError(err)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		pw.Close()
	}()
	resp, err := http.Post(srv.URL+RestorePath, "application/zip", pr)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("restore: %d %s", resp.StatusCode, body)
	}
}

// Two restores at once take turns: both pass, and one staged restore is
// left, marked ready, never neither.
func TestConcurrentRestoresTakeTurns(t *testing.T) {
	m, _, _, _ := setup(t)
	good, err := m.Create(context.Background(), KindManual)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(good.Path)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { errs <- m.Stage(context.Background(), bytes.NewReader(b)) }()
	}
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(m.Dir, StagedDir, readyFile)); err != nil {
		t.Fatalf("no ready restore is staged: %v", err)
	}
}
