package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediaservers"
)

// route: GET /api/v1/media-servers
// route: POST /api/v1/media-servers
// route: PUT /api/v1/media-servers/{id}
// route: DELETE /api/v1/media-servers/{id}
// route: POST /api/v1/media-servers/{id}/test
func TestMediaServersOverHTTP(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	jelly := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != fmt.Sprintf(`MediaBrowser Client="HolyArrs", Device="HolyArrs", DeviceId="holyarrs", Version="1", Token=%q`, "tk-1234") ||
			req.Header.Get("X-MediaBrowser-Token") != "" || req.Header.Get("X-Emby-Token") != "" {
			w.WriteHeader(401)
			return
		}
		_, _ = io.WriteString(w, `{"ServerName":"Lab Jellyfin","Version":"10.9.0"}`)
	}))
	defer jelly.Close()
	r.owner.call("POST", "/api/v1/media-servers", `{"name":"J","kind":"kodi","url":"http://x","token":"tk-1234"}`, 400)
	r.owner.call("POST", "/api/v1/media-servers", `{"name":"J","kind":"jellyfin","url":"`+jelly.URL+`"}`, 400)
	b := r.owner.call("POST", "/api/v1/media-servers", `{"name":"Jellyfin","kind":"jellyfin","url":"`+jelly.URL+`","token":"tk-1234"}`, 201)
	if strings.Contains(string(b), "tk-1234") {
		t.Fatalf("token leaked: %s", b)
	}
	var v mediaservers.View
	_ = json.Unmarshal(b, &v)
	if v.Token != secrets.Masked || !v.OnImport || !v.OnRename || !v.OnDelete || !v.AllowLocal {
		t.Fatalf("view %+v", v)
	}
	r.owner.call("POST", "/api/v1/media-servers", `{"name":"jellyfin","kind":"jellyfin","url":"`+jelly.URL+`","token":"tk-1234"}`, 409)
	r.ana.call("GET", "/api/v1/media-servers", "", 403)
	if b := r.owner.call("GET", "/api/v1/media-servers", "", 200); !strings.Contains(string(b), `"name":"Jellyfin"`) || strings.Contains(string(b), "tk-1234") {
		t.Fatalf("list %s", b)
	}
	if b := r.owner.call("POST", "/api/v1/media-servers/"+itoa(v.ID)+"/test", "", 200); !strings.Contains(string(b), "Lab Jellyfin") {
		t.Fatalf("test %s", b)
	}
	// The masked token keeps the stored one; a wrong one fails the test plainly.
	r.owner.call("PUT", "/api/v1/media-servers/"+itoa(v.ID), fmt.Sprintf(`{"name":"Jellyfin","on_delete":false,"token":%q}`, secrets.Masked), 200)
	r.owner.call("PUT", "/api/v1/media-servers/"+itoa(v.ID), `{"name":"Jellyfin","token":"wrong-1"}`, 200)
	if b := r.owner.call("POST", "/api/v1/media-servers/"+itoa(v.ID)+"/test", "", 502); !strings.Contains(string(b), "turned down the token") {
		t.Fatalf("wrong token %s", b)
	}
	r.owner.call("PUT", "/api/v1/media-servers/999", `{"name":"x"}`, 404)
	r.owner.call("DELETE", "/api/v1/media-servers/"+itoa(v.ID), "", 204)
	r.owner.call("DELETE", "/api/v1/media-servers/"+itoa(v.ID), "", 404)
}
