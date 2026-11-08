package sources

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Connect Jackett or Prowlarr, through the API, against fakes on this machine.

const bridgeJackettKey = "jackett-api-test-key"

func fakeJackettServer(t *testing.T, sites ...string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Query().Get("apikey") != bridgeJackettKey {
			_, _ = w.Write([]byte(`<error code="100" description="Invalid API Key"/>`))
			return
		}
		var b strings.Builder
		b.WriteString(`<indexers>`)
		for _, id := range sites {
			fmt.Fprintf(&b, `<indexer id="%s" configured="true"><title>Site %s</title><type>public</type><caps><categories><category id="2000"/><category id="5000"/></categories></caps></indexer>`, id, id)
		}
		b.WriteString(`</indexers>`)
		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// route: GET /api/v1/bridges
// route: POST /api/v1/bridges
// route: POST /api/v1/bridges/test
// route: POST /api/v1/bridges/{id}/check
// route: DELETE /api/v1/bridges/{id}
func TestBridgeJackettConnectCheckDisconnect(t *testing.T) {
	t.Parallel()
	srv, _ := sourceServer(t)
	j := fakeJackettServer(t, "alpha", "beta")
	var res struct {
		OK    bool     `json:"ok"`
		Text  string   `json:"text"`
		Sites []string `json:"sites"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/bridges/test", `{"kind":"jackett","url":"`+j.URL+`/","key":"`+bridgeJackettKey+`"}`, 200), &res)
	if !res.OK || res.Text != "Jackett answered with 2 sites." || len(res.Sites) != 2 {
		t.Fatalf("test %+v", res)
	}
	var bad struct {
		Error string `json:"error"`
		Field string `json:"field"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/bridges/test", `{"kind":"jackett","url":"`+j.URL+`","key":"wrong"}`, 502), &bad)
	if !strings.Contains(bad.Error, "turned down the key") {
		t.Fatalf("wrong key %+v", bad)
	}
	for body, field := range map[string]string{
		`{"kind":"sonarr","url":"http://x","key":"k"}`:        "kind",
		`{"kind":"jackett","url":"jackett:9117","key":"k"}`:   "url",
		`{"kind":"jackett","url":"http://jackett:9117"}`:      "key",
		`{"kind":"prowlarr","url":"http://p:9696","key":"k"}`: "callback_url",
	} {
		_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/bridges/test", body, 400), &bad)
		if bad.Field != field {
			t.Errorf("%s: field %q, want %q", body, bad.Field, field)
		}
	}

	var v BridgeView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/bridges", `{"kind":"jackett","url":"`+j.URL+`","key":"`+bridgeJackettKey+`"}`, 201), &v)
	if v.Kind != "jackett" || v.URL != j.URL || v.Sites != 2 || v.Sources != 2 || v.LastCheck == nil || v.LastError != "" {
		t.Fatalf("connect %+v", v)
	}
	// Connecting the same Jackett again takes its key; it is still one connection.
	var again BridgeView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/bridges", `{"kind":"jackett","url":"`+j.URL+`/","key":"`+bridgeJackettKey+`"}`, 201), &again)
	if again.ID != v.ID || again.Sources != 2 {
		t.Fatalf("reconnect %+v", again)
	}
	var list struct {
		Bridges []BridgeView `json:"bridges"`
	}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/bridges", "", 200), &list)
	if len(list.Bridges) != 1 || list.Bridges[0].ID != v.ID {
		t.Fatalf("list %+v", list)
	}
	var sources struct {
		Sources []struct {
			Name string `json:"name"`
		} `json:"sources"`
	}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/sources", "", 200), &sources)
	if len(sources.Sources) != 2 || sources.Sources[0].Name != "Site alpha" {
		t.Fatalf("sources %+v", sources)
	}
	var checked struct {
		Result struct {
			Sites, Added, Updated, Removed int
		} `json:"result"`
	}
	_ = json.Unmarshal(do(t, "POST", fmt.Sprintf("%s/api/v1/bridges/%d/check", srv.URL, v.ID), "", 200), &checked)
	if checked.Result.Sites != 2 || checked.Result.Added+checked.Result.Updated+checked.Result.Removed != 0 {
		t.Fatalf("check %+v", checked)
	}
	var gone struct {
		Sources int  `json:"sources"`
		Kept    bool `json:"kept"`
	}
	_ = json.Unmarshal(do(t, "DELETE", fmt.Sprintf("%s/api/v1/bridges/%d", srv.URL, v.ID), "", 200), &gone)
	if gone.Sources != 2 || gone.Kept {
		t.Fatalf("delete %+v", gone)
	}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/sources", "", 200), &sources)
	if len(sources.Sources) != 0 {
		t.Fatalf("left %+v", sources)
	}
	do(t, "POST", fmt.Sprintf("%s/api/v1/bridges/%d/check", srv.URL, v.ID), "", 404)
	do(t, "DELETE", srv.URL+"/api/v1/bridges/x", "", 400)
}

// route: POST /api/v1/bridges/find
func TestBridgeFindAnswersAList(t *testing.T) {
	t.Parallel()
	srv, _ := sourceServer(t)
	var out struct {
		Found []map[string]string `json:"found"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/bridges/find", "", 200), &out)
	if out.Found == nil {
		t.Fatal("found is not a list")
	}
}
