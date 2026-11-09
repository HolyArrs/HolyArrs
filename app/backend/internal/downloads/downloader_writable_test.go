package downloads

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Only people who may change settings flip "HolyArrs can change this
// Downloader": it rides on the Downloader edit, which needs that permission.
func TestWritableNeedsSettingsPermission(t *testing.T) {
	for _, route := range []string{"POST /api/v1/downloaders", "PUT /api/v1/downloaders/{id}"} {
		if accounts.RoutePerms[route] != webserver.PermSettings {
			t.Errorf("%s: %v", route, accounts.RoutePerms[route])
		}
	}
}

// The switch is saved, shown in the API, and read live: off refuses a
// pause before the client, on lets it through; the environment list locks it.
func TestWritableSwitchLive(t *testing.T) {
	t.Setenv(ReadOnlyEnv, "")
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	f := &allClient{}
	exec41(t, x.s, "UPDATE downloaders SET url = ?, allow_local_network = 1", f.server(t).URL)
	var did int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders").Scan(&did)
	pause := x.srv.URL + "/api/v1/downloaders/" + itoa(did) + "/items/" + outHash + "/pause"
	exec41(t, x.s, "UPDATE downloaders SET writable = 0")
	x.dl.Manager().Forget(did)
	do(t, "POST", pause, "", http.StatusConflict)
	// The Downloads screen learns the row's Downloader is locked, so it offers no client controls.
	if body := string(do(t, "GET", x.srv.URL+"/api/v1/downloads/view?limit=100", "", http.StatusOK)); !strings.Contains(body, `"client_locked":true`) {
		t.Fatalf("the view does not mark the locked Downloader's rows: %.300s", body)
	}
	if strings.Contains(f.saw(), "torrents/stop") || strings.Contains(f.saw(), "torrents/pause") {
		t.Fatalf("a read-only pause reached the client: %s", f.saw())
	}
	var v DownloaderView
	_ = json.Unmarshal(do(t, "PUT", x.srv.URL+"/api/v1/downloaders/"+itoa(did), `{"name":"Q","writable":true}`, http.StatusOK), &v)
	if !v.Writable || v.WritableLocked {
		t.Fatalf("view %+v", v)
	}
	do(t, "POST", pause, "", http.StatusOK)
	t.Setenv(ReadOnlyEnv, itoa(did))
	x.dl.Manager().Forget(did)
	do(t, "POST", pause, "", http.StatusConflict)
	var list struct {
		Downloaders []DownloaderView `json:"downloaders"`
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloaders", "", http.StatusOK), &list)
	if len(list.Downloaders) != 1 || !list.Downloaders[0].Writable || !list.Downloaders[0].WritableLocked {
		t.Fatalf("list %+v", list.Downloaders)
	}
}

// Every change sent straight to a client is in History: who and what.
func TestClientItemChangeEvents(t *testing.T) {
	t.Setenv(ReadOnlyEnv, "")
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	f := &allClient{}
	exec41(t, x.s, "UPDATE downloaders SET url = ?, allow_local_network = 1, writable = 1", f.server(t).URL)
	var did int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders").Scan(&did)
	base := x.srv.URL + "/api/v1/downloaders/" + itoa(did)
	do(t, "POST", base+"/items/"+outHash+"/pause", "", http.StatusOK)
	do(t, "POST", base+"/items/"+outHash+"/resume", "", http.StatusOK)
	do(t, "POST", base+"/bulk", `{"action":"pause","hashes":["`+outHash+`","`+seedHash+`"]}`, http.StatusOK)
	es, _, err := x.s.Events(context.Background(), database.EventFilter{Kinds: []string{"download_changed"}})
	if err != nil || len(es) != 3 {
		t.Fatalf("events %+v %v", es, err)
	}
	actions := map[string]domain.Event{}
	for _, e := range es {
		actions[e.Data["action"].(string)] = e
		if e.Actor.Kind != "user" || e.Data["by_hand"] != true {
			t.Errorf("event %+v", e)
		}
	}
	for k, verb := range map[string]string{"pause": " paused ", "resume": " resumed "} {
		if e, ok := actions[k]; !ok || e.Data["downloader"] != "Q" || !strings.Contains(e.Summary, verb) || !strings.HasSuffix(e.Summary, " in Q") {
			t.Errorf("%s: %+v", k, actions)
		}
	}
	var many bool
	for _, e := range es {
		if e.Data["count"] != nil && strings.Contains(e.Summary, "paused 2 items in Q") {
			many = true
		}
	}
	if !many {
		t.Fatalf("bulk event missing: %+v", es)
	}
}
