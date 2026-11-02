package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

func TestSettingsChangeAreas(t *testing.T) {
	for _, c := range []struct {
		method, path, area, action string
		ok                         bool
	}{
		{"PUT", "/api/v1/settings/search", "search", "changed", true},
		{"POST", "/api/v1/pick-rules", "pick-rules", "added", true},
		{"PUT", "/api/v1/library-folders/3", "library-folders", "changed", true},
		{"DELETE", "/api/v1/sources/4", "sources", "deleted", true},
		{"PUT", "/api/v1/outside-lists/5", "outside-lists", "changed", true},
		{"POST", "/api/v1/downloaders", "downloaders", "added", true},
		{"POST", "/api/v1/downloaders/2/pause", "", "", false},
		{"POST", "/api/v1/sources/check", "", "", false},
		{"POST", "/api/v1/settings/naming/example", "", "", false},
		{"GET", "/api/v1/settings/search", "", "", false},
		{"PUT", "/api/v1/titles/1", "", "", false},
	} {
		area, _, action, _, ok := settingsChange(c.method, c.path)
		if ok != c.ok || area != c.area || action != c.action {
			t.Errorf("%s %s: %q %q %v", c.method, c.path, area, action, ok)
		}
	}
}

// Removing an item straight in the Downloader is logged as download_removed
// by the person who did it.
func TestClientItemRemoveEvent(t *testing.T) {
	x := seed41(t, domain.SlotDownloading, "downloading")
	f := &allClient{}
	exec41(t, x.s, "UPDATE downloaders SET url = ?", f.server(t).URL)
	var did int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders").Scan(&did)
	do(t, "POST", x.srv.URL+"/api/v1/downloaders/"+itoa(did)+"/items/"+strings.ToUpper(outHash)+"/remove", `{"delete_files":true}`, http.StatusOK)
	es, _, err := x.s.Events(context.Background(), store.EventFilter{Kinds: []string{"download_removed"}})
	if err != nil || len(es) != 1 {
		t.Fatalf("events %v %v", es, err)
	}
	if e := es[0]; e.Data["by_hand"] != true || e.Data["delete_files"] != true || e.Data["downloader"] != "Q" || e.Actor.Kind != "user" {
		t.Fatalf("event %+v", e)
	}
}
