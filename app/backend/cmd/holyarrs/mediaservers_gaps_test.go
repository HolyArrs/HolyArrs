package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediaservers"
)

func addJelly(t *testing.T, r *askRig) mediaservers.View {
	t.Helper()
	jelly := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ServerName":"Lab Jellyfin","Version":"10.9.0"}`)
	}))
	t.Cleanup(jelly.Close)
	var v mediaservers.View
	b := r.owner.call("POST", "/api/v1/media-servers", `{"name":"Jellyfin","kind":"jellyfin","url":"`+jelly.URL+`","token":"tk-1234"}`, 201)
	if err := json.Unmarshal(b, &v); err != nil || v.ID == 0 {
		t.Fatalf("%v %s", err, b)
	}
	return v
}

// Moving the address to another host without a token is a plain 400.
func TestMediaServerNewHostNeedsToken(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	v := addJelly(t, r)
	b := r.owner.call("PUT", "/api/v1/media-servers/"+itoa(v.ID), `{"name":"Jellyfin","url":"http://other-host:8096"}`, 400)
	if !strings.Contains(string(b), "token") {
		t.Fatalf("%s", b)
	}
}

// A body over 64 KB answers 413.
func TestMediaServerBodyTooLarge(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	big := `{"name":"` + strings.Repeat("a", 70000) + `"}`
	r.owner.call("POST", "/api/v1/media-servers", big, 413)
}

// Restarting a media server link from Controls runs its test and logs one link_restart Event.
// route: POST /api/v1/links/{kind}/{id}/restart
func TestControlsRestartMediaServer(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	v := addJelly(t, r)
	r.owner.call("POST", "/api/v1/links/media_server/"+itoa(v.ID)+"/restart", "", 200)
	es, _, err := r.st.Events(context.Background(), database.EventFilter{Kinds: []string{domain.EventControl}, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range es {
		if e.Data["action"] == "link_restart" && e.Data["target"] == "media_server:"+itoa(v.ID) {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("events: %+v", es)
	}
}
