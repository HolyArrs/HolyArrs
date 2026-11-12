package downloads

import (
	"encoding/json"
	"testing"
)

func TestDownloaderAfterSeedingRoundTrip(t *testing.T) {
	t.Parallel()
	srv, _ := dlServer(t)
	var d DownloaderView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders", `{"name":"Q","kind":"qbittorrent","url":"http://10.1.1.1:8080","secret":"k"}`, 201), &d)
	if d.AfterSeeding.Remove || d.AfterSeeding.DeleteFiles || d.AfterSeeding.Ratio != nil {
		t.Fatalf("not off by default: %+v", d.AfterSeeding)
	}
	id := itoa(d.ID)
	_ = json.Unmarshal(do(t, "PUT", srv.URL+"/api/v1/downloaders/"+id, `{"name":"Q","after_seeding":{"remove":true,"ratio":1.5,"minutes":600,"delete_files":true}}`, 200), &d)
	if s := d.AfterSeeding; !s.Remove || !s.DeleteFiles || s.Ratio == nil || *s.Ratio != 1.5 || s.Minutes == nil || *s.Minutes != 600 {
		t.Fatalf("saved %+v", s)
	}
	// Left out of a save, it stays.
	_ = json.Unmarshal(do(t, "PUT", srv.URL+"/api/v1/downloaders/"+id, `{"name":"Q2"}`, 200), &d)
	if !d.AfterSeeding.Remove || !d.AfterSeeding.DeleteFiles {
		t.Fatalf("lost %+v", d.AfterSeeding)
	}
	do(t, "PUT", srv.URL+"/api/v1/downloaders/"+id, `{"name":"Q","after_seeding":{"remove":true,"ratio":0}}`, 400)
	key := "0123456789abcdef0123456789abcdef"
	sab := fakeSAB(t, key)
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders", `{"name":"S","kind":"sabnzbd","url":"`+sab.URL+`","secret":"`+key+`","category":"movies"}`, 201), &d)
	do(t, "PUT", srv.URL+"/api/v1/downloaders/"+itoa(d.ID), `{"name":"S","after_seeding":{"remove":true}}`, 400)
}

func TestDeadRuleSettingsRoundTrip(t *testing.T) {
	t.Parallel()
	srv, _ := dlServer(t)
	var st DownloaderSettings
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/settings/downloaders", "", 200), &st)
	if st.DeadMetadataMinutes == nil || *st.DeadMetadataMinutes != 0 || st.CrawlKBps == nil || *st.CrawlKBps != 0 || st.CrawlHours == nil || *st.CrawlHours != 0 {
		t.Fatalf("not off by default: %+v", st)
	}
	base := `"stalled_after_hours":6,"stalled_act_hours":24,"grab_spacing_seconds":2`
	_ = json.Unmarshal(do(t, "PUT", srv.URL+"/api/v1/settings/downloaders", `{`+base+`,"dead_metadata_minutes":30,"crawl_kbps":50,"crawl_hours":2}`, 200), &st)
	if *st.DeadMetadataMinutes != 30 || *st.CrawlKBps != 50 || *st.CrawlHours != 2 {
		t.Fatalf("saved %+v", st)
	}
	_ = json.Unmarshal(do(t, "PUT", srv.URL+"/api/v1/settings/downloaders", `{`+base+`}`, 200), &st)
	if *st.DeadMetadataMinutes != 30 || *st.CrawlKBps != 50 {
		t.Fatalf("lost %+v", st)
	}
	do(t, "PUT", srv.URL+"/api/v1/settings/downloaders", `{`+base+`,"dead_metadata_minutes":2}`, 400)
	do(t, "PUT", srv.URL+"/api/v1/settings/downloaders", `{`+base+`,"crawl_hours":500}`, 400)
}
