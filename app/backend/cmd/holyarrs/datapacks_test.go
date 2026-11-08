package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles/datapack"
)

func dataPackServer(t *testing.T) (*httptest.Server, *database.Store, string) {
	t.Helper()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	raw, err := os.ReadFile("../../internal/mediafiles/datapack/testdata/synthetic-pack.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(buildHandlers(services{Store: st, Version: "test"}).Handler())
	t.Cleanup(srv.Close)
	return srv, st, string(raw)
}

// route: POST /api/v1/data-packs/validate
// route: POST /api/v1/data-packs
// route: GET /api/v1/data-packs
// route: GET /api/v1/data-packs/{id}
func TestDataPackValidateImportListExport(t *testing.T) {
	t.Parallel()
	srv, st, raw := dataPackServer(t)
	before := st.WriteCount()
	b := string(do(t, "POST", srv.URL+"/api/v1/data-packs/validate", raw, http.StatusOK))
	if !strings.Contains(b, `"valid":true`) || st.WriteCount() != before {
		t.Fatalf("validate response=%s writes=%d", b, st.WriteCount()-before)
	}
	b = string(do(t, "POST", srv.URL+"/api/v1/data-packs", raw, http.StatusCreated))
	if !strings.Contains(b, `"status":"inactive"`) || st.WriteCount() != before+1 {
		t.Fatalf("import response=%s writes=%d", b, st.WriteCount()-before)
	}
	b = string(do(t, "GET", srv.URL+"/api/v1/data-packs", "", http.StatusOK))
	if !strings.Contains(b, `"id":"home-cinema"`) || !strings.Contains(b, `"status":"inactive"`) {
		t.Fatalf("list response=%s", b)
	}
	b = string(do(t, "GET", srv.URL+"/api/v1/data-packs/home-cinema", "", http.StatusOK))
	if !strings.Contains(b, `"format":"holyarrs-data-pack"`) || !strings.Contains(b, `"release_tests"`) {
		t.Fatalf("export response=%s", b)
	}
}

func TestDataPackAPIRejectsInvalidOversizedAndDuplicate(t *testing.T) {
	t.Parallel()
	srv, st, raw := dataPackServer(t)
	before := st.WriteCount()
	do(t, "POST", srv.URL+"/api/v1/data-packs", strings.Replace(raw, `"quality": "1080p_web"`, `"quality": "2160p_web"`, 1), http.StatusBadRequest)
	if st.WriteCount() != before {
		t.Fatalf("invalid import committed %d writes", st.WriteCount()-before)
	}
	tooLarge := strings.Repeat(" ", 256<<10+1)
	do(t, "POST", srv.URL+"/api/v1/data-packs/validate", tooLarge, http.StatusRequestEntityTooLarge)
	do(t, "POST", srv.URL+"/api/v1/data-packs", raw, http.StatusCreated)
	do(t, "POST", srv.URL+"/api/v1/data-packs", raw, http.StatusConflict)
	do(t, "GET", srv.URL+"/api/v1/data-packs/missing", "", http.StatusNotFound)
}

func TestDataPackExportIsContentTypedAndBounded(t *testing.T) {
	t.Parallel()
	srv, _, raw := dataPackServer(t)
	do(t, "POST", srv.URL+"/api/v1/data-packs", raw, http.StatusCreated)
	resp, err := http.Get(srv.URL + "/api/v1/data-packs/home-cinema")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.Header.Get("Content-Type") != "application/json" || len(b) > 256<<10 {
		t.Fatalf("content-type=%q bytes=%d", resp.Header.Get("Content-Type"), len(b))
	}
}

// route: GET /api/v1/data-packs/{id}/preview
// route: POST /api/v1/data-packs/{id}/apply
func TestDataPackPreviewAndExplicitApply(t *testing.T) {
	t.Parallel()
	srv, st, raw := dataPackServer(t)
	do(t, "POST", srv.URL+"/api/v1/data-packs", raw, http.StatusCreated)
	b := do(t, "GET", srv.URL+"/api/v1/data-packs/home-cinema/preview?pick_rule_id=balanced-web&naming_id=readable", "", http.StatusOK)
	var preview struct {
		ApprovalHash string `json:"approval_hash"`
	}
	if err := json.Unmarshal(b, &preview); err != nil || preview.ApprovalHash == "" {
		t.Fatalf("preview=%s err=%v", b, err)
	}
	body := `{"pick_rule_ids":["balanced-web"],"naming_id":"readable","approval_hash":"` + preview.ApprovalHash + `"}`
	b = do(t, "POST", srv.URL+"/api/v1/data-packs/home-cinema/apply", body, http.StatusOK)
	if !strings.Contains(string(b), `"pick_rule_ids":["balanced-web"]`) {
		t.Fatalf("apply=%s", b)
	}
	var n int
	_ = st.Read().QueryRow(`SELECT COUNT(*) FROM pick_rules WHERE name='Balanced web'`).Scan(&n)
	if n != 1 {
		t.Fatalf("created %d rules", n)
	}
	do(t, "POST", srv.URL+"/api/v1/data-packs/home-cinema/apply", body, http.StatusConflict)
}

func TestDataPackApplyRejectsBadSelectionAndApproval(t *testing.T) {
	t.Parallel()
	srv, _, raw := dataPackServer(t)
	do(t, "POST", srv.URL+"/api/v1/data-packs", raw, http.StatusCreated)
	b := do(t, "GET", srv.URL+"/api/v1/data-packs/home-cinema/preview?pick_rule_id=missing", "", http.StatusBadRequest)
	if !strings.Contains(string(b), `pick rule id \"missing\" is not in the data pack`) {
		t.Fatalf("invalid selection response=%s", b)
	}
	do(t, "POST", srv.URL+"/api/v1/data-packs/home-cinema/apply", `{"pick_rule_ids":["balanced-web"],"approval_hash":"bad"}`, http.StatusConflict)
	b = do(t, "POST", srv.URL+"/api/v1/data-packs/home-cinema/apply", `{"pick_rule_ids":["balanced-web"],"approval_hash":"bad","unknown":true}`, http.StatusBadRequest)
	if !strings.Contains(string(b), "send pick_rule_ids, optional naming_id and approval_hash") {
		t.Fatalf("invalid body response=%s", b)
	}
	do(t, "POST", srv.URL+"/api/v1/data-packs/home-cinema/apply", strings.Repeat(" ", 16<<10+1), http.StatusRequestEntityTooLarge)
}

type failingDataPackBody struct{}

func (failingDataPackBody) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestDataPackBodyReadFailuresAreInternal(t *testing.T) {
	t.Parallel()
	_, st, _ := dataPackServer(t)
	a := dataPackMux(st)
	for _, call := range []struct {
		handler func(http.ResponseWriter, *http.Request)
		path    string
	}{
		{a.ServeHTTP, "/api/v1/data-packs/validate"},
		{a.ServeHTTP, "/api/v1/data-packs"},
		{a.ServeHTTP, "/api/v1/data-packs/home-cinema/apply"},
	} {
		r := httptest.NewRequest(http.MethodPost, call.path, nil)
		r.Body = io.NopCloser(failingDataPackBody{})
		r.SetPathValue("id", "home-cinema")
		w := httptest.NewRecorder()
		call.handler(w, r)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("%s status=%d body=%s", call.path, w.Code, w.Body.Bytes())
		}
		assertGenericDataPackError(t, w.Body.Bytes())
	}
}

func TestDataPackInternalErrorsAreGeneric(t *testing.T) {
	t.Parallel()
	t.Run("corrupt stored pack", func(t *testing.T) {
		srv, st, raw := dataPackServer(t)
		do(t, "POST", srv.URL+"/api/v1/data-packs", raw, http.StatusCreated)
		if err := st.Write(context.Background(), func(tx *sql.Tx) error {
			_, err := tx.Exec(`UPDATE settings SET value = '{' WHERE grp = 'this_app' AND key = 'data_pack:home-cinema'`)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		for _, call := range []struct {
			method string
			path   string
			body   string
		}{
			{http.MethodGet, "/api/v1/data-packs/home-cinema", ""},
			{http.MethodGet, "/api/v1/data-packs/home-cinema/preview?pick_rule_id=balanced-web", ""},
			{http.MethodPost, "/api/v1/data-packs/home-cinema/apply", `{"pick_rule_ids":["balanced-web"],"approval_hash":"0000000000000000000000000000000000000000000000000000000000000000"}`},
		} {
			assertGenericDataPackError(t, do(t, call.method, srv.URL+call.path, call.body, http.StatusInternalServerError))
		}
	})

	t.Run("closed read store", func(t *testing.T) {
		srv, st, _ := dataPackServer(t)
		if err := st.Read().Close(); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{
			"/api/v1/data-packs/home-cinema",
			"/api/v1/data-packs/home-cinema/preview?pick_rule_id=balanced-web",
		} {
			assertGenericDataPackError(t, do(t, http.MethodGet, srv.URL+path, "", http.StatusInternalServerError))
		}
	})

	t.Run("cancelled writes", func(t *testing.T) {
		_, st, raw := dataPackServer(t)
		a := dataPackMux(st)
		for _, call := range []struct {
			handler func(http.ResponseWriter, *http.Request)
			path    string
			body    string
			id      string
		}{
			{a.ServeHTTP, "/api/v1/data-packs", raw, ""},
			{a.ServeHTTP, "/api/v1/data-packs/home-cinema/apply", `{"pick_rule_ids":["balanced-web"],"approval_hash":"0000000000000000000000000000000000000000000000000000000000000000"}`, "home-cinema"},
		} {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			r := httptest.NewRequest(http.MethodPost, call.path, strings.NewReader(call.body)).WithContext(ctx)
			r.SetPathValue("id", call.id)
			w := httptest.NewRecorder()
			call.handler(w, r)
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("%s status=%d body=%s", call.path, w.Code, w.Body.Bytes())
			}
			assertGenericDataPackError(t, w.Body.Bytes())
		}
	})
}

func assertGenericDataPackError(t *testing.T, body []byte) {
	t.Helper()
	if got := string(body); got != "{\"error\":\"something went wrong; see the log\"}\n" {
		t.Fatalf("internal response exposed details: %s", body)
	}
}

func TestDataPackRoutesAreOwnerOnlyAndNeedAdminScope(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	raw, err := os.ReadFile("../../internal/mediafiles/datapack/testdata/synthetic-pack.json")
	if err != nil {
		t.Fatal(err)
	}
	r.owner.call(http.MethodPost, "/api/v1/data-packs", string(raw), http.StatusCreated)

	type routeCall struct{ method, path, body string }
	routes := []routeCall{
		{http.MethodGet, "/api/v1/data-packs", ""},
		{http.MethodGet, "/api/v1/data-packs/home-cinema", ""},
		{http.MethodGet, "/api/v1/data-packs/home-cinema/preview?pick_rule_id=balanced-web", ""},
		{http.MethodPost, "/api/v1/data-packs/validate", string(raw)},
		{http.MethodPost, "/api/v1/data-packs", string(raw)},
		{http.MethodPost, "/api/v1/data-packs/home-cinema/apply", `{"pick_rule_ids":["balanced-web"],"approval_hash":"bad"}`},
	}
	for _, rt := range routes {
		r.ana.call(rt.method, rt.path, rt.body, http.StatusForbidden)
	}

	makeKey := func(scope string) string {
		t.Helper()
		var got struct {
			Key string `json:"key"`
		}
		body := r.owner.call(http.MethodPost, "/api/v1/auth/keys", `{"label":"data pack `+scope+`","scopes":["`+scope+`"]}`, http.StatusCreated)
		if err := json.Unmarshal(body, &got); err != nil || got.Key == "" {
			t.Fatalf("create %s key: %s: %v", scope, body, err)
		}
		return got.Key
	}
	for _, scope := range []string{webserver.ScopeHealthRead, webserver.ScopeRead, webserver.ScopeWrite} {
		key := makeKey(scope)
		for _, rt := range routes {
			if status, body := keyCall(t, r.srv.URL, key, rt.method, rt.path, rt.body); status != http.StatusForbidden {
				t.Errorf("%s key reached %s %s: %d %s", scope, rt.method, rt.path, status, body)
			}
		}
	}
	admin := makeKey(webserver.ScopeAdmin)
	wants := []int{http.StatusOK, http.StatusOK, http.StatusOK, http.StatusOK, http.StatusConflict, http.StatusConflict}
	for i, rt := range routes {
		if status, body := keyCall(t, r.srv.URL, admin, rt.method, rt.path, rt.body); status != wants[i] {
			t.Errorf("admin key %s %s: %d, want %d: %s", rt.method, rt.path, status, wants[i], body)
		}
	}

	patterns := []string{
		"GET /api/v1/data-packs", "GET /api/v1/data-packs/{id}", "GET /api/v1/data-packs/{id}/preview",
		"POST /api/v1/data-packs/validate", "POST /api/v1/data-packs", "POST /api/v1/data-packs/{id}/apply",
	}
	for _, route := range patterns {
		if got, ok := accounts.RoutePerms[route]; !ok || got != webserver.Perm("owner") {
			t.Errorf("%s permission=%q, want owner", route, got)
		}
		if got := accounts.RouteScope(route); got != webserver.ScopeAdmin {
			t.Errorf("%s scope=%s, want admin", route, got)
		}
		for _, scope := range []string{webserver.ScopeHealthRead, webserver.ScopeRead, webserver.ScopeWrite} {
			if webserver.ScopesCover([]string{scope}, webserver.ScopeAdmin) {
				t.Errorf("%s unexpectedly covers data-pack admin scope", scope)
			}
		}
	}
}

// dataPackMux serves only the data pack routes, without the sign-in chain.
func dataPackMux(st *database.Store) *http.ServeMux {
	m := http.NewServeMux()
	(&datapack.Handlers{Deps: datapack.Deps{Store: st}}).Routes(m)
	return m
}
