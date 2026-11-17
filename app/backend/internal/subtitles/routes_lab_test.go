package subtitles

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
)

func labCall(t *testing.T, m *http.ServeMux, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func kindNames(t *testing.T, m *http.ServeMux) []string {
	t.Helper()
	rec := labCall(t, m, "GET", "/api/v1/subtitle-sites/kinds", "")
	var out struct {
		Kinds []struct{ Kind string } `json:"kinds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, k := range out.Kinds {
		names = append(names, k.Kind)
	}
	return names
}

func hasKind(names []string, k string) bool {
	for _, n := range names {
		if n == k {
			return true
		}
	}
	return false
}

// The lab's fake sites exist for HolyArrs's own tests: a real install neither
// lists them nor takes a new one.
func TestLabKinds_HiddenAndRefusedWithoutLabSwitch(t *testing.T) {
	t.Setenv("HOLYARRS_FLAGS", "")
	f := newFix(t)
	m := http.NewServeMux()
	NewHandlers(f.st, nil, &Service{Store: f.st}).Routes(m)
	names := kindNames(t, m)
	if len(names) == 0 || !hasKind(names, "podnapisi") {
		t.Fatalf("kinds = %v, want the real sites", names)
	}
	for _, k := range []string{"fake", "fake_account"} {
		if hasKind(names, k) {
			t.Errorf("kinds list %s with no lab switch: %v", k, names)
		}
		body := `{"kind": "` + k + `", "url": "http://127.0.0.1:9", "api_key": "x"}`
		if k == "fake" {
			body = `{"kind": "fake", "url": "http://127.0.0.1:9"}`
		}
		rec := labCall(t, m, "POST", "/api/v1/subtitle-sites", body)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "unknown kind of subtitle site") {
			t.Errorf("adding %s: %d %s, want 400 unknown kind", k, rec.Code, rec.Body.String())
		}
	}
}

// A lab site saved while the switch was on stays: it is listed and can be
// removed, but cannot be changed with the switch off.
func TestLabKinds_SavedRowKeptWithoutLabSwitch(t *testing.T) {
	t.Setenv("HOLYARRS_FLAGS", startup.Lab)
	f := newFix(t)
	row, err := AddSite(context.Background(), f.st, nil, SiteInput{Kind: "fake", URL: "http://127.0.0.1:9", AllowLocalNetwork: true}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOLYARRS_FLAGS", "")
	m := http.NewServeMux()
	NewHandlers(f.st, nil, &Service{Store: f.st}).Routes(m)
	if rec := labCall(t, m, "GET", "/api/v1/subtitle-sites", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"kind":"fake"`) {
		t.Fatalf("saved lab site not listed: %d %s", rec.Code, rec.Body.String())
	}
	path := "/api/v1/subtitle-sites/" + strconv.FormatInt(row.ID, 10)
	if rec := labCall(t, m, "PUT", path, `{"name": "Lab", "url": "http://127.0.0.1:9"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("changing a lab site with no switch: %d %s, want 400", rec.Code, rec.Body.String())
	}
	if rec := labCall(t, m, "DELETE", path, ""); rec.Code != http.StatusNoContent {
		t.Errorf("removing a lab site: %d %s, want 204", rec.Code, rec.Body.String())
	}
}

// With the lab switch on, the tests and the lab get both fake kinds.
func TestLabKinds_ListedAndAddedWithLabSwitch(t *testing.T) {
	t.Setenv("HOLYARRS_FLAGS", "ai_apps, "+startup.Lab)
	f := newFix(t)
	m := http.NewServeMux()
	NewHandlers(f.st, nil, &Service{Store: f.st}).Routes(m)
	names := kindNames(t, m)
	if !hasKind(names, "fake") || !hasKind(names, "fake_account") {
		t.Fatalf("kinds = %v, want both lab kinds", names)
	}
	rec := labCall(t, m, "POST", "/api/v1/subtitle-sites", `{"kind": "fake", "url": "http://127.0.0.1:9", "allow_local_network": true}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("adding a lab site with the switch: %d %s", rec.Code, rec.Body.String())
	}
}
