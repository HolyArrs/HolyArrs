package arrapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"slices"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Prowlarr links music indexers as a Lidarr app. The Lidarr side
// answers its status and indexer calls under /api/v1, and what Prowlarr
// writes there becomes a music Source.
func TestLidarrSide_ProwlarrMakesMusicSource(t *testing.T) {
	st := settingsStore(t)
	v, err := secrets.Open(context.Background(), st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	var saved []domain.Source
	srv := Lidarr(Deps{Store: st, Secrets: v, SourceSaved: func(s domain.Source) { saved = append(saved, s) }})
	ctx := webserver.ContextWith(context.Background(), webserver.Caller{UserID: 1, Via: "api_key", Role: webserver.RoleOwner,
		Perms: webserver.DefaultPerms(webserver.RoleOwner), Scopes: []string{webserver.ScopeCompat}})
	send := func(method, path string, body any) *httptest.ResponseRecorder {
		var b []byte
		if body != nil {
			b, _ = json.Marshal(body)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(method, path, bytes.NewReader(b)).WithContext(ctx))
		return rec
	}

	rec := send("GET", "/lidarr/api/v1/system/status", nil)
	var status struct {
		AppName string `json:"appName"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil || rec.Code != 200 || status.AppName != "Lidarr" {
		t.Fatalf("status: %d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("X-Application-Version") != LidarrVersion || status.Version != LidarrVersion {
		t.Fatalf("version header %q body %q", rec.Header().Get("X-Application-Version"), status.Version)
	}

	if rec = send("GET", "/lidarr/api/v1/indexer/schema", nil); rec.Code != 200 {
		t.Fatalf("schema: %d %s", rec.Code, rec.Body)
	}
	h := &handler{side: "lidarr"}
	body := prowlarrBody(h, "torznab", map[string]any{"categories": []any{3000.0, 3010.0}})
	if rec = send("POST", "/lidarr/api/v1/indexer?forceSave=true", body); rec.Code != 201 || len(saved) != 1 {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	src := saved[0]
	if !slices.Equal(src.Kinds, []string{"music"}) || !slices.Equal(src.Categories["music"], []int{3000, 3010}) {
		t.Fatalf("source kinds %v categories %v", src.Kinds, src.Categories)
	}
	if rec = send("GET", "/lidarr/api/v1/indexer", nil); rec.Code != 200 || !bytes.Contains(rec.Body.Bytes(), []byte(`"Lab Tracker (Prowlarr)"`)) {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}

	// Only what Prowlarr needs is answered: no library, no v3 path.
	for _, p := range []string{"/lidarr/api/v1/artist", "/lidarr/api/v1/movie", "/lidarr/api/v1/qualityprofile"} {
		if rec = send("GET", p, nil); rec.Code != 404 {
			t.Fatalf("%s: %d, want 404", p, rec.Code)
		}
	}
	if rec = send("GET", "/lidarr/api/v3/system/status", nil); rec.Code != 404 {
		t.Fatalf("v3: %d, want 404", rec.Code)
	}
}

// The settings key routes take the Lidarr side too, so a key can be made
// for a Lidarr app added in Prowlarr by hand, and removed again.
func TestLidarrSide_KeyRouteAcceptsSide(t *testing.T) {
	if got := sideAllowed("lidarr"); !got {
		t.Fatal("lidarr refused")
	}
	if sideAllowed("readarr") || sideAllowed("") {
		t.Fatal("unknown side allowed")
	}
}
