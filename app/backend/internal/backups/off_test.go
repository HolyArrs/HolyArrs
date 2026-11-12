package backups

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Off stops scheduled backups only: nothing is made or pruned on a tick,
// manual backups still work and State reports no schedule.
func TestOffStopsScheduledBackups(t *testing.T) {
	m, _, c, _ := setup(t)
	old, err := m.Create(t.Context(), KindScheduled)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SetPreset(t.Context(), Off, domain.Actor{}); err != nil {
		t.Fatal(err)
	}
	if p, _ := m.Preset(t.Context()); p != Off {
		t.Fatalf("preset %q, want off", p)
	}
	c.t = c.t.Add(90 * 24 * time.Hour)
	if err := m.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	all, _ := m.List()
	if len(all) != 1 || all[0].Name != old.Name {
		t.Fatalf("after a tick with Off: %+v", all)
	}
	if _, err := m.Create(t.Context(), KindManual); err != nil {
		t.Fatalf("manual backup with Off: %v", err)
	}
	if _, every, _, err := m.State(t.Context()); err != nil || every != 0 {
		t.Fatalf("state every %v err %v, want 0", every, err)
	}
	if !m.Off(t.Context()) {
		t.Fatal("Off() false with the off preset")
	}
}

// With Off the Backups card does not ask for a key kit.
func TestOffHidesKeyKitReminder(t *testing.T) {
	m, _, _, _ := setup(t)
	m.Key = labKey(7)
	if !m.keyKitState(t.Context()).Needed {
		t.Fatal("key kit not asked for before Off")
	}
	if err := m.SetPreset(t.Context(), Off, domain.Actor{}); err != nil {
		t.Fatal(err)
	}
	if m.keyKitState(t.Context()).Needed {
		t.Fatal("key kit still asked for with Off")
	}
}

// The settings route takes "off"; unknown values name every choice.
func TestSettingsRouteTakesOff(t *testing.T) {
	m, _, _, _ := setup(t)
	srv := keyKitServer(t, m)
	put := func(body string) *http.Response {
		req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/backups/settings", strings.NewReader(body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	resp := put(`{"preset":"off"}`)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("put off: %d", resp.StatusCode)
	}
	if p, _ := m.Preset(t.Context()); p != Off {
		t.Fatalf("preset %q", p)
	}
	resp = put(`{"preset":"weekly"}`)
	defer resp.Body.Close()
	var e map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&e)
	if resp.StatusCode != 400 || !strings.Contains(strings.ToLower(toString(e)), "off") {
		t.Fatalf("bad preset: %d %v", resp.StatusCode, e)
	}
}

func toString(v any) string { b, _ := json.Marshal(v); return string(b) }
