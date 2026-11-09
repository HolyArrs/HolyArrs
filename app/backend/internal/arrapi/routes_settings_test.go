package arrapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func settingsStore(t *testing.T) *database.Store {
	t.Helper()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestHandlersDeleteFilesFollowsSwitch(t *testing.T) {
	ctx := context.Background()
	h := &Handlers{Deps: Deps{Store: settingsStore(t)}}
	d := h.deps()
	if d.DeleteFiles == nil || d.DeleteFiles(ctx) {
		t.Fatal("default must be off")
	}
	on := true
	if err := h.SaveSwitches(ctx, domain.Actor{}, nil, &on); err != nil {
		t.Fatal(err)
	}
	if !h.deps().DeleteFiles(ctx) {
		t.Fatal("switch on not seen")
	}
	if h.On(ctx) {
		t.Fatal("delete-files save turned the API on")
	}
}

func TestGateHidesPrefixesWhileOff(t *testing.T) {
	ctx := context.Background()
	h := &Handlers{Deps: Deps{Store: settingsStore(t)}}
	g := h.Gate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	check := func(want map[string]int) {
		t.Helper()
		for path, code := range want {
			rec := httptest.NewRecorder()
			g.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
			if rec.Code != code {
				t.Errorf("%s: %d want %d", path, rec.Code, code)
			}
		}
	}
	check(map[string]int{"/sonarr/api/v3/system/status": 404, "/radarr/MediaCover/1/poster.jpg": 404, "/api/v1/status": 200, "/sonarrx": 200})
	on := true
	if err := h.SaveSwitches(ctx, domain.Actor{}, &on, nil); err != nil {
		t.Fatal(err)
	}
	check(map[string]int{"/sonarr/api/v3/system/status": 200, "/radarr/MediaCover/1/poster.jpg": 200})
}

func TestSaveSwitchesBothAndNil(t *testing.T) {
	ctx := context.Background()
	st := settingsStore(t)
	t1, f1 := true, false
	if err := saveSwitches(ctx, st, domain.Actor{}, map[string]*bool{settingOn: &t1, settingDeleteFile: &t1}); err != nil {
		t.Fatal(err)
	}
	if err := saveSwitches(ctx, st, domain.Actor{}, map[string]*bool{settingOn: &f1, settingDeleteFile: nil}); err != nil {
		t.Fatal(err)
	}
	if switchValue(ctx, st, settingOn) != "off" || switchValue(ctx, st, settingDeleteFile) != "on" {
		t.Fatal("nil overwrote, or off not saved")
	}
}

// Without sign-in no key can be made or deleted. The settings still list
// both sides, each with no key.
func TestSettingsWithoutSignIn(t *testing.T) {
	h := &Handlers{Deps: Deps{Store: settingsStore(t)}}
	req := httptest.NewRequest("POST", "/api/v1/settings/old-app-api/keys/sonarr", nil)
	req.SetPathValue("side", "sonarr")
	rec := httptest.NewRecorder()
	h.makeKey(rec, req)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "sign-in is off") {
		t.Fatalf("makeKey: %d %s", rec.Code, rec.Body)
	}
	req2 := httptest.NewRequest("DELETE", "/api/v1/settings/old-app-api/keys/sonarr", nil)
	req2.SetPathValue("side", "sonarr")
	rec2 := httptest.NewRecorder()
	h.deleteKey(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("deleteKey: %d", rec2.Code)
	}
	rec3 := httptest.NewRecorder()
	h.getSettings(rec3, httptest.NewRequest("GET", "/api/v1/settings/old-app-api", nil))
	if rec3.Code != http.StatusOK {
		t.Fatalf("getSettings: %d %s", rec3.Code, rec3.Body)
	}
	var out struct {
		Enabled bool             `json:"enabled"`
		Sides   []map[string]any `json:"sides"`
	}
	if err := json.Unmarshal(rec3.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Enabled || len(out.Sides) != 2 {
		t.Fatalf("settings: %s", rec3.Body)
	}
	for _, s := range out.Sides {
		if _, has := s["key"]; has {
			t.Errorf("side %v shows a key", s["side"])
		}
	}
}
