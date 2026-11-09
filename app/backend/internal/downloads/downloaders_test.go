package downloads

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// route: GET /api/v1/downloaders
// route: POST /api/v1/downloaders
// route: PUT /api/v1/downloaders/{id}
// route: DELETE /api/v1/downloaders/{id}
// route: POST /api/v1/downloaders/{id}/test
// route: POST /api/v1/downloaders/{id}/pause
// route: POST /api/v1/downloaders/{id}/resume
func TestDownloadersAddTestPauseDelete(t *testing.T) {
	t.Parallel()
	srv, s := dlServer(t)
	key := "0123456789abcdef0123456789abcdef"
	sab := fakeSAB(t, key)

	// SABnzbd needs its key.
	do(t, "POST", srv.URL+"/api/v1/downloaders", `{"name":"Lab SABnzbd","kind":"sabnzbd","url":"`+sab.URL+`","allow_local_network":true}`, 400)
	do(t, "POST", srv.URL+"/api/v1/downloaders", `{"name":"Bad","kind":"unsupported","url":"http://x"}`, 400)
	do(t, "POST", srv.URL+"/api/v1/downloaders", `{"name":"Bad","kind":"sabnzbd","url":"http://user:pw@x","secret":"k"}`, 400)

	var d DownloaderView
	raw, _ := json.Marshal(map[string]any{"name": "Lab SABnzbd", "kind": "sabnzbd", "url": sab.URL, "secret": key, "category": "movies", "allow_local_network": true, "priority": 2})
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders", string(raw), 201), &d)
	if d.ID == 0 || d.Protocol != "usenet" || d.Secret != secrets.Masked || d.Priority != 2 || d.Health.State != "working" {
		t.Fatalf("added %+v", d)
	}
	do(t, "POST", srv.URL+"/api/v1/downloaders", string(raw), 409) // name used
	list := string(do(t, "GET", srv.URL+"/api/v1/downloaders", "", 200))
	if strings.Contains(list, key) || !strings.Contains(list, "Lab SABnzbd") {
		t.Fatalf("list %s", list)
	}
	id := itoa(d.ID)

	var res struct {
		OK     bool   `json:"ok"`
		Text   string `json:"text"`
		Health struct {
			State string `json:"state"`
		} `json:"health"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/test", "", 200), &res)
	if !res.OK || res.Text != "It answered: sabnzbd 5.1.3." {
		t.Fatalf("test %+v", res)
	}
	// The masked key keeps the stored one; a wrong key is said in plain words.
	keep, wrong := secrets.Masked, "not-the-key"
	do(t, "PUT", srv.URL+"/api/v1/downloaders/"+id, body(map[string]any{"name": "Lab SABnzbd", "secret": keep, "category": "movies"}), 200)
	do(t, "PUT", srv.URL+"/api/v1/downloaders/"+id, body(map[string]any{"name": "Lab SABnzbd", "secret": wrong}), 200)
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/test", "", 200), &res)
	if res.OK || !strings.Contains(res.Text, "turned down the key") || res.Health.State != "sign_in_refused" {
		t.Fatalf("wrong key test %+v", res)
	}
	// A new host needs the key again.
	do(t, "PUT", srv.URL+"/api/v1/downloaders/"+id, `{"name":"Lab SABnzbd","url":"http://10.9.9.9:8080"}`, 409)
	do(t, "PUT", srv.URL+"/api/v1/downloaders/"+id, `{"name":"Lab SABnzbd","secret":"`+key+`"}`, 200)
	// A category SABnzbd lacks.
	do(t, "PUT", srv.URL+"/api/v1/downloaders/"+id, `{"name":"Lab SABnzbd","category":"holyarrs"}`, 200)
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/test", "", 200), &res)
	if res.OK || !strings.Contains(res.Text, "category does not exist") {
		t.Fatalf("category test %+v", res)
	}

	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/pause", "", 200), &d)
	if d.Health.State != "paused" {
		t.Fatalf("paused %+v", d.Health)
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/resume", "", 200), &d)
	if d.Health.State != "working" {
		t.Fatalf("resumed %+v", d.Health)
	}

	// The key never reaches the database in the clear.
	var n int
	_ = s.Read().QueryRow("SELECT count(*) FROM secrets WHERE value LIKE ?", "%"+key+"%").Scan(&n)
	if n != 0 {
		t.Fatal("key stored in the clear")
	}
	do(t, "DELETE", srv.URL+"/api/v1/downloaders/"+id, "", 204)
	do(t, "DELETE", srv.URL+"/api/v1/downloaders/"+id, "", 404)
	do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/test", "", 404)
}

func TestDownloaderOnTheLocalNetworkNeedsTheSwitch(t *testing.T) {
	t.Parallel()
	srv, _ := dlServer(t)
	key := "0123456789abcdef0123456789abcdef"
	sab := fakeSAB(t, key)
	var d DownloaderView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders", `{"name":"S","kind":"sabnzbd","url":"`+sab.URL+`","secret":"`+key+`","category":"movies"}`, 201), &d)
	var res struct {
		OK   bool   `json:"ok"`
		Text string `json:"text"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+itoa(d.ID)+"/test", "", 200), &res)
	if res.OK || !strings.Contains(res.Text, "May reach the local network") {
		t.Fatalf("test %+v", res)
	}
}

// route: GET /api/v1/settings/downloaders
// route: PUT /api/v1/settings/downloaders
func TestDownloaderSettings(t *testing.T) {
	t.Parallel()
	srv, _ := dlServer(t)
	var st DownloaderSettings
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/settings/downloaders", "", 200), &st)
	if st.StalledAfterHours != 6 || st.StalledActHours != 24 || st.GrabSpacingSeconds != 2 || !st.RemoveFailed || !st.FailDangerous || !st.ReplaceVanished || !st.TryNextDownloader {
		t.Fatalf("defaults %+v", st)
	}
	body := `{"stalled_after_hours":3,"stalled_act_hours":12,"remove_failed":false,"fail_dangerous":true,"replace_vanished":false,"grab_spacing_seconds":5,"try_next_downloader":false}`
	_ = json.Unmarshal(do(t, "PUT", srv.URL+"/api/v1/settings/downloaders", body, 200), &st)
	if st.StalledAfterHours != 3 || st.StalledActHours != 12 || st.RemoveFailed || st.ReplaceVanished || st.GrabSpacingSeconds != 5 || st.TryNextDownloader {
		t.Fatalf("saved %+v", st)
	}
	if st.AutoAdd == nil || *st.AutoAdd {
		t.Fatalf("downloads_auto_add should start off: %+v", st.AutoAdd)
	}
	body = `{"stalled_after_hours":3,"stalled_act_hours":12,"grab_spacing_seconds":5,"downloads_auto_add":true}`
	_ = json.Unmarshal(do(t, "PUT", srv.URL+"/api/v1/settings/downloaders", body, 200), &st)
	if st.AutoAdd == nil || !*st.AutoAdd {
		t.Fatalf("downloads_auto_add not saved: %+v", st.AutoAdd)
	}
	// Left out of a save, it stays.
	_ = json.Unmarshal(do(t, "PUT", srv.URL+"/api/v1/settings/downloaders", `{"stalled_after_hours":3,"stalled_act_hours":12,"grab_spacing_seconds":5}`, 200), &st)
	if st.AutoAdd == nil || !*st.AutoAdd {
		t.Fatalf("downloads_auto_add lost: %+v", st.AutoAdd)
	}
	do(t, "PUT", srv.URL+"/api/v1/settings/downloaders", `{"stalled_after_hours":10,"stalled_act_hours":5,"grab_spacing_seconds":2}`, 400)
	do(t, "PUT", srv.URL+"/api/v1/settings/downloaders", `{"stalled_after_hours":0,"stalled_act_hours":5,"grab_spacing_seconds":2}`, 400)
	do(t, "PUT", srv.URL+"/api/v1/settings/downloaders", `{"stalled_after_hours":6,"stalled_act_hours":24,"grab_spacing_seconds":99}`, 400)
}
