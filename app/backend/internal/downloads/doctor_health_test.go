package downloads

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// A cause no one knows reaches the Test line as plain words, never the raw
// Go error.
func TestDownloaderUnknownErrorIsPlain(t *testing.T) {
	_, text := explainDownloader(fmt.Errorf(`Get "http://qbit:8080/api/v2/app/version": EOF`), "http://qbit:8080", false)
	if strings.Contains(text, "EOF") || strings.Contains(text, "Get ") || strings.Contains(text, "qbit:8080") {
		t.Fatalf("raw error on the Test line: %q", text)
	}
}

// A category SABnzbd lacks is recorded, so health says it by name as Test does.
func TestMissingCategoryShowsOnHealth(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	u := startSab(t, &fakeSab{})
	m.FromVault = func(context.Context, int64) (string, error) { return sabKey, nil }
	var ref int64
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO secrets (kind, value, created_at, created_by, updated_at, updated_by) VALUES ('downloader_key', 'sealed', '', '{}', '', '{}')`)
		if err != nil {
			return err
		}
		ref, err = res.LastInsertId()
		return err
	}); err != nil {
		t.Fatal(err)
	}
	d := &domain.Downloader{Name: "SAB no cat", Adapter: "sabnzbd", Protocol: "usenet", URL: u, Category: "nope-cat", Priority: 1, AllowLocal: true, SecretRef: &ref}
	w.addDownloader(t, d)
	if _, err := m.Check(ctx, *d); err == nil {
		t.Fatal("check passed without the category")
	}
	var got domain.Downloader
	if err := database.Get(ctx, w.s.Read(), "downloaders", d.ID, &got); err != nil {
		t.Fatal(err)
	}
	if hv := HealthOf(got, time.Now()); hv.Text != doctor.CategoryLine("nope-cat") || hv.Cause != string(doctor.Category) {
		t.Fatalf("health %+v (last error %q)", hv, got.LastError)
	}
	// A missing category never rests the whole Downloader: grabs for its
	// other categories still go to it.
	if got.State != "ok" || got.BackoffUntil != nil || got.FailCount != 0 {
		t.Fatalf("missing category rested the Downloader: state %q, backoff %v, fails %d", got.State, got.BackoffUntil, got.FailCount)
	}
	if ok, skipped := m.order([]domain.Downloader{got}, time.Now()); len(ok) != 1 {
		t.Fatalf("picker skips it: %v", skipped)
	}
}

// A cause only the Test's second look finds (an https:// address on a plain
// port that drops TLS) reads the same on health afterwards.
func TestHealthKeepsTheTestCause(t *testing.T) {
	ctx := context.Background()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(ctx, s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	a := &Web{Store: s, Metadata: metadata.Lab(), Secrets: v}
	srv := webRig(t, a)
	ds := startDoctorServers(t)
	var d DownloaderView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders", body(map[string]any{"name": "qBit", "kind": "qbittorrent", "url": "https://" + ds.picky,
		"username": "u", "secret": "p", "category": "movies", "allow_local_network": true}), 201), &d)
	var res struct {
		Text   string `json:"text"`
		Health struct {
			Text  string `json:"text"`
			Cause string `json:"cause"`
		} `json:"health"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+itoa(d.ID)+"/test", "", 200), &res)
	if res.Text != doctor.Line(doctor.UseHTTP) {
		t.Fatalf("test line %q", res.Text)
	}
	if res.Health.Text != res.Text || res.Health.Cause != string(doctor.UseHTTP) {
		t.Fatalf("health %+v, test %q", res.Health, res.Text)
	}
}
