package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
)

// fakeOldApps records what the bridge flow asks of the arr side.
type fakeOldApps struct {
	calls    []string
	on       bool
	canMake  bool
	haveKeys map[string]bool
}

func (f *fakeOldApps) CanMakeKeys() bool { return f.canMake }
func (f *fakeOldApps) KeySides(context.Context) (map[string]bool, error) {
	return f.haveKeys, nil
}
func (f *fakeOldApps) MakeKey(_ *http.Request, side string) (string, error) {
	f.calls = append(f.calls, "make "+side)
	return "key-" + side, nil
}
func (f *fakeOldApps) DeleteKey(_ *http.Request, side string) error {
	f.calls = append(f.calls, "delete "+side)
	return nil
}
func (f *fakeOldApps) On(context.Context) bool { return f.on }
func (f *fakeOldApps) SetOn(_ context.Context, _ domain.Actor, on bool) error {
	f.on = on
	f.calls = append(f.calls, map[bool]string{true: "on", false: "off"}[on])
	return nil
}

func addProwlarrBridge(t *testing.T, h *Handlers, url string, keys string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"kind":"prowlarr","url":"` + url + `","key":"` + prowlarrKey + `","callback_url":"http://10.0.0.5:6262"` + keys + `}`
	rec := httptest.NewRecorder()
	h.addBridge(rec, httptest.NewRequest("POST", "/api/v1/bridges", strings.NewReader(body)))
	return rec
}

// When Prowlarr refuses the app entries, the keys this request made are
// deleted again and the arr API goes back off. A key typed in is never
// made or deleted.
func TestAddProwlarrBridgeRollsBackOldAppKeysOnFailure(t *testing.T) {
	f := newFakeProwlarr(t)
	f.refuse = "Unable to connect to HolyArrs"
	s, v := openStore(t)
	both := func(parts.Part) bool { return true }

	old := &fakeOldApps{canMake: true}
	h := &Handlers{Store: s, Vault: v, OldApps: old, PartOn: both}
	rec := addProwlarrBridge(t, h, f.srv.URL, "")
	if rec.Code/100 == 2 {
		t.Fatalf("added anyway: %d %s", rec.Code, rec.Body)
	}
	want := []string{"make sonarr", "make radarr", "on", "delete sonarr", "delete radarr", "off"}
	if !slices.Equal(old.calls, want) {
		t.Fatalf("calls %v want %v", old.calls, want)
	}
	if old.on {
		t.Fatal("arr API left on")
	}
	if bs, err := h.bridges().List(context.Background()); err != nil || len(bs) != 0 {
		t.Fatalf("bridge kept: %v %v", bs, err)
	}

	// Keys typed in: nothing is made, and nothing typed in is deleted.
	typed := &fakeOldApps{canMake: true}
	h2 := &Handlers{Store: s, Vault: v, OldApps: typed, PartOn: both}
	rec = addProwlarrBridge(t, h2, f.srv.URL, `,"keys":{"sonarr":"a","radarr":"b"}`)
	if rec.Code/100 == 2 {
		t.Fatalf("added anyway: %d %s", rec.Code, rec.Body)
	}
	if !slices.Equal(typed.calls, []string{"on", "off"}) {
		t.Fatalf("typed keys calls %v", typed.calls)
	}

	// The switch was already on: it stays on after a failure.
	already := &fakeOldApps{canMake: true, on: true}
	h3 := &Handlers{Store: s, Vault: v, OldApps: already, PartOn: both}
	addProwlarrBridge(t, h3, f.srv.URL, `,"keys":{"sonarr":"a","radarr":"b"}`)
	if len(already.calls) != 0 || !already.on {
		t.Fatalf("already on: %v on=%v", already.calls, already.on)
	}

	// A side that has a key needs a say first; nothing is made or switched.
	have := &fakeOldApps{canMake: true, haveKeys: map[string]bool{"sonarr": true}}
	h4 := &Handlers{Store: s, Vault: v, OldApps: have, PartOn: both}
	if rec = addProwlarrBridge(t, h4, f.srv.URL, ""); rec.Code != http.StatusConflict || len(have.calls) != 0 {
		t.Fatalf("existing key: %d %v", rec.Code, have.calls)
	}
}
