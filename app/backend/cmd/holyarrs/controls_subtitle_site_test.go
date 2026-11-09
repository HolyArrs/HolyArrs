package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites/fakesite"
)

// A subtitle site restart runs that site's own test and writes one Event
// naming it.
// route: POST /api/v1/links/{kind}/{id}/restart
func TestControls_SubtitleSiteRestartRunsTest(t *testing.T) {
	f := newCtl(t, false)
	var hits atomic.Int64
	lab := fakesite.Lab()
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		lab.ServeHTTP(w, r)
	}))
	t.Cleanup(site.Close)
	var v struct {
		ID int64 `json:"id"`
	}
	decodeJSON(t, do(t, "POST", f.srv.URL+"/api/v1/subtitle-sites",
		fmt.Sprintf(`{"kind":"fake","url":%q,"allow_local_network":true,"limits":{"gap_ms":1,"per_window":100,"window_seconds":3600}}`, site.URL), 201), &v)
	if v.ID == 0 {
		t.Fatal("no site id")
	}
	before := hits.Load()
	do(t, "POST", f.srv.URL+"/api/v1/links/subtitle_site/"+itoa(v.ID)+"/restart", "", 200)
	if hits.Load() == before {
		t.Fatal("the restart never reached the site")
	}
	es := f.events(t)
	if len(es) != 1 || es[0].Data["target"] != "subtitle_site:"+itoa(v.ID) {
		t.Fatalf("events: %+v", es)
	}
}
