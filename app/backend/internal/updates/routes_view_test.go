package updates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func TestViewForHidesFeedFromNonSettingsAndStripsCreds(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	u := UpdateInfo{Feed: "https://a:b@h.example/x?tok=1"}
	if got := ViewFor(r, u).Feed; got != "https://h.example/x" {
		t.Fatalf("feed = %q", got)
	}
	if got := ViewFor(r, UpdateInfo{Feed: "%zz"}).Feed; got != "" {
		t.Fatalf("bad feed = %q", got)
	}
	lib := webserver.Caller{UserID: 2, Role: webserver.RoleAdmin, Via: "session", Perms: []webserver.Perm{webserver.PermManageLibrary}}
	if got := ViewFor(r.WithContext(webserver.ContextWith(r.Context(), lib)), u).Feed; got != "" {
		t.Fatalf("non-settings viewer sees feed %q", got)
	}
	set := lib
	set.Perms = append(set.Perms, webserver.PermSettings)
	if got := ViewFor(r.WithContext(webserver.ContextWith(r.Context(), set)), u).Feed; got != "https://h.example/x" {
		t.Fatalf("settings viewer feed = %q", got)
	}
}

func TestClientForGetsLocalFlag(t *testing.T) {
	w := newWorld(t)
	f := newFeed(t)
	f.body = `{"tag_name": "v9.0.0"}`
	var got []bool
	w.e.ReleaseFeed = f.srv.URL
	w.e.ClientFor = func(l bool) *http.Client { got = append(got, l); return f.srv.Client() }
	ctx := context.Background()
	w.e.CheckUpdate(ctx)
	if err := w.e.SaveSetting(ctx, "update_check_local", "true"); err != nil {
		t.Fatal(err)
	}
	w.e.CheckUpdate(ctx)
	if len(got) != 2 || got[0] || !got[1] {
		t.Fatalf("ClientFor got %v, want [false true]", got)
	}
}
