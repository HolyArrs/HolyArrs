package migrate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate/secretsfile"
)

// Two Radarrs at two addresses are read as two apps: each read keeps its
// address and its own key, a read again of one address keeps the other's,
// and the plan takes titles and rules from both.
func TestTwoCopiesOfOneApp(t *testing.T) {
	ctx := context.Background()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	v, err := secrets.Open(ctx, st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	r := webserver.NewRouter()
	h := &Handlers{Store: st, Secrets: v, Mover: func() *Mover { return nil }, SafeMode: func() bool { return false }}
	h.Routes(r)
	owner := webserver.ContextWith(ctx, webserver.Caller{UserID: 1, Name: "owner", Role: webserver.RoleOwner, Via: "session"})
	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(owner)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	readAt := func(body string) Saved {
		t.Helper()
		rec := do("POST", "/api/v1/migrate/read", body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("read %s: %d %s", body, rec.Code, rec.Body.String())
		}
		var out struct{ Snapshot Saved }
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out.Snapshot
	}

	hdURL, hdKey := radarrAPIKeyed(t, "S3CR3T_KEY_HD", nil)
	uhdURL, uhdKey := radarrAPIKeyed(t, "S3CR3T_KEY_4K", map[string]any{
		"/api/v3/movie": []any{map[string]any{"id": 1, "tmdbId": 999, "title": "Only In 4K", "year": 2020,
			"path": "/movies4k/Only In 4K (2020)", "rootFolderPath": "/movies4k", "qualityProfileId": 1, "monitored": true}},
		"/api/v3/rootfolder": []any{map[string]any{"id": 1, "path": "/movies4k", "accessible": true, "freeSpace": 100}},
	})
	hd := readAt(`{"kind":"radarr","url":"` + hdURL + `/","key":"` + hdKey + `","allow_local_network":true}`)
	uhd := readAt(`{"kind":"radarr","url":"` + uhdURL + `","key":"` + uhdKey + `","allow_local_network":true}`)
	if hd.Address != strings.ToLower(hdURL) || uhd.Address != strings.ToLower(uhdURL) || hd.Address == uhd.Address {
		t.Fatalf("addresses %q %q", hd.Address, uhd.Address)
	}

	// Each address keeps its own key.
	for addr, want := range map[string]string{hdURL: hdKey, uhdURL: uhdKey} {
		c, _, ok, err := secretsfile.ReadMoveCredentialAt(ctx, st, v, "radarr", addr)
		if err != nil || !ok || c.APIKey != want {
			t.Fatalf("key for %s: ok=%v err=%v", addr, ok, err)
		}
	}

	// Read again with the saved key: each address uses its own key (a wrong
	// one would be refused by the fake app).
	again := readAt(`{"kind":"radarr","url":"` + hdURL + `","use_saved_key":true}`)
	readAt(`{"kind":"radarr","url":"` + uhdURL + `","use_saved_key":true}`)
	if again.Address != hd.Address {
		t.Fatalf("read again address %q", again.Address)
	}
	if rec := do("POST", "/api/v1/migrate/read", `{"kind":"radarr","url":"http://127.0.0.1:1","use_saved_key":true}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("saved key for an unknown address: %d", rec.Code)
	}

	// A read again of one address keeps the other's key.
	readAt(`{"kind":"radarr","url":"` + hdURL + `","key":"` + hdKey + `","allow_local_network":true}`)
	if c, _, ok, _ := secretsfile.ReadMoveCredentialAt(ctx, st, v, "radarr", uhdURL); !ok || c.APIKey != uhdKey {
		t.Fatal("the other copy's key was replaced")
	}
	if list, _ := secretsfile.ListMoveCredentials(ctx, st, v); len(list) != 2 {
		t.Fatalf("saved keys %+v", list)
	}

	// The plan takes both: titles from each, rules named per copy.
	var in []Input
	for _, id := range []int64{again.ID, uhd.ID} {
		_, snap, err := Get(ctx, st, id)
		if err != nil {
			t.Fatal(err)
		}
		in = append(in, Input{ID: id, Snap: snap})
	}
	p := BuildPlan(in, Choices{})
	names := map[string]bool{}
	for _, g := range p.Groups {
		for _, it := range g.Items {
			names[it.Name] = true
		}
	}
	for _, want := range []string{"The Matrix (1999)", "Only In 4K (2020)", "HD (" + instanceLabel(hd.Address) + ")", "HD (" + instanceLabel(uhd.Address) + ")"} {
		if !names[want] {
			t.Errorf("plan lacks %q; has %v", want, names)
		}
	}

	// Forgetting one read forgets only that copy's key.
	if rec := do("DELETE", "/api/v1/migrate/snapshots/"+strconv.FormatInt(uhd.ID, 10), ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete %d", rec.Code)
	}
	if _, _, ok, _ := secretsfile.ReadMoveCredentialAt(ctx, st, v, "radarr", uhdURL); ok {
		t.Fatal("the forgotten copy's key stayed")
	}
	if _, _, ok, _ := secretsfile.ReadMoveCredentialAt(ctx, st, v, "radarr", hdURL); !ok {
		t.Fatal("the other copy's key went too")
	}
}
