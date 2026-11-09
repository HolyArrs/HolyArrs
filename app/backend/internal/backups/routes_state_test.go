package backups

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// The Settings page reads preset and restore_waiting from the list.
func TestListShowsPresetAndWaitingRestore(t *testing.T) {
	m, _, _, _ := setup(t)
	if err := m.SetPreset(t.Context(), Light, domain.Actor{}); err != nil {
		t.Fatal(err)
	}
	b, err := m.Create(t.Context(), KindManual)
	if err != nil {
		t.Fatal(err)
	}
	z, _ := os.ReadFile(b.Path)
	if err := m.Stage(t.Context(), bytes.NewReader(z)); err != nil {
		t.Fatal(err)
	}
	srv := keyKitServer(t, m)
	resp, err := http.Get(srv.URL + "/api/v1/backups")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("list: %v %v", resp, err)
	}
	defer resp.Body.Close()
	var l struct {
		Preset         string           `json:"preset"`
		RestoreWaiting bool             `json:"restore_waiting"`
		KeyKit         *json.RawMessage `json:"key_kit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&l); err != nil {
		t.Fatal(err)
	}
	if l.Preset != "light" || !l.RestoreWaiting || l.KeyKit == nil {
		t.Fatalf("listed %+v", l)
	}
}

// The health card is fed from State, KeyPrint and KeyKitPrint.
func TestBackupsHealthFromManager(t *testing.T) {
	m, _, c, _ := setup(t)
	m.Key = labKey(5)
	if _, err := m.Create(t.Context(), KindManual); err != nil {
		t.Fatal(err)
	}
	newest, every, _, err := m.State(t.Context())
	if err != nil || every <= 0 || c.t.Sub(newest).Abs() > time.Minute {
		t.Fatalf("state %v %v %v (now %v)", newest, every, err, c.t)
	}
	if m.KeyPrint() == "" || m.KeyKitPrint(t.Context()) != "" {
		t.Fatalf("before kit: print %q kit %q", m.KeyPrint(), m.KeyKitPrint(t.Context()))
	}
	srv := keyKitServer(t, m)
	resp, err := http.Post(srv.URL+keyKitPath, "", nil)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("key kit: %v %v", resp, err)
	}
	resp.Body.Close()
	if got := m.KeyKitPrint(t.Context()); got != m.KeyPrint() {
		t.Fatalf("after kit: %q, want %q", got, m.KeyPrint())
	}
}
