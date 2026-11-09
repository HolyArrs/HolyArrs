package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

const (
	sabFailed = "SABnzbd_nzo_failed1"
	sabNew    = "SABnzbd_nzo_again2"
)

// usenetFake is a SABnzbd answering the calls the Downloads page makes on
// one job; it records them.
type usenetFake struct {
	mu    sync.Mutex
	calls []string
}

func (f *usenetFake) saw() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.calls, " ")
}

func (f *usenetFake) server(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		f.mu.Lock()
		f.calls = append(f.calls, q.Get("mode")+":"+q.Get("name")+":"+q.Get("value"))
		f.mu.Unlock()
		switch q.Get("mode") {
		case "retry":
			fmt.Fprintf(w, `{"status":true,"nzo_id":%q}`, sabNew)
		case "queue", "switch":
			_, _ = w.Write([]byte(`{"status":true,"result":{"position":0},"queue":{"slots":[]}}`))
		case "history":
			fmt.Fprintf(w, `{"history":{"noofslots":1,"last_history_update":3,"slots":[{"nzo_id":%q,"name":"Quiet.Orchard.2024","status":"Failed","stage_log":[
{"name":"Source","actions":["https://idx.example/api?apikey=TOPSECRET&t=get"]},
{"name":"Servers","actions":["news.example=812.4 MB","backup=1 KB"]},
{"name":"Repair","actions":["Repair failed, not enough repair blocks (3 short)"]}]}]}}`, q.Get("nzo_ids"))
		case "server_stats":
			_, _ = w.Write([]byte(`{"total":900,"month":800,"week":700,"day":600,"servers":{"news.example":{"total":900,"month":800,"week":700,"day":600,"articles_tried":10,"articles_success":9}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func addSab(t *testing.T, st *database.Store, url string) int64 {
	t.Helper()
	var did int64
	err := st.Write(context.Background(), func(tx *sql.Tx) error {
		dr := &domain.Downloader{Writable: true, Name: "S", Adapter: "sabnzbd", Protocol: "usenet", URL: url, Category: "holyarrs", Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		_, err := database.Insert(tx, "downloaders", dr, domain.System)
		did = dr.ID
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return did
}

// route: POST /api/v1/downloaders/{id}/items/{hash}/retry
// route: GET /api/v1/downloaders/{id}/items/{hash}/usenet
// Outside SABnzbd jobs: pause, resume and top need only manage-library;
// Retry and remove need the settings permission. A retried job's log and
// link follow it to its new id.
func TestUsenetItemRetryAndTabs(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	f := &usenetFake{}
	did := addSab(t, r.st, f.server(t))
	r.owner.call("PUT", fmt.Sprintf("/api/v1/users/%d", r.ana.id), `{"perms":["ask","manage_library"]}`, 200)
	item := fmt.Sprintf("/api/v1/downloaders/%d/items/%s", did, sabFailed)
	for _, act := range []string{"pause", "resume", "top"} {
		r.ana.call("POST", item+"/"+act, "", 200)
	}
	r.ana.call("POST", item+"/retry", "", 403)
	r.ana.call("POST", item+"/remove", "", 403)
	// Ids that are not one job never reach the client.
	r.owner.call("POST", fmt.Sprintf("/api/v1/downloaders/%d/items/a,b/pause", did), "", 404)

	// What HolyArrs keeps about the job, under its old id.
	err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		low := strings.ToLower(sabFailed)
		if _, err := tx.Exec("INSERT INTO torrent_log (downloader_id, hash, at, kind, summary) VALUES (?, ?, '2026-10-30T10:00:00Z', 'failed', 'Failed')", did, low); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO client_links (downloader_id, hash, name, state, matched_at) VALUES (?, ?, 'Quiet.Orchard.2024', 'not_media', '2026-10-30T10:00:00Z')", did, low)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		OK bool   `json:"ok"`
		ID string `json:"id"`
	}
	_ = json.Unmarshal(r.owner.call("POST", item+"/retry", "", 200), &out)
	if !out.OK || out.ID != sabNew {
		t.Fatalf("retry answered %+v", out)
	}
	newLow, oldLow := strings.ToLower(sabNew), strings.ToLower(sabFailed)
	if n := count(t, r.st, "SELECT COUNT(*) FROM torrent_log WHERE downloader_id = ? AND hash = ?", did, newLow); n != 2 {
		t.Fatalf("%d log lines under the new id", n)
	}
	if n := count(t, r.st, "SELECT COUNT(*) FROM torrent_log WHERE hash = ?", oldLow) + count(t, r.st, "SELECT COUNT(*) FROM client_links WHERE hash = ?", oldLow); n != 0 {
		t.Fatalf("%d rows left under the old id", n)
	}
	if n := count(t, r.st, "SELECT COUNT(*) FROM client_links WHERE hash = ?", newLow); n != 1 {
		t.Fatal("the link did not follow the job")
	}
	got := f.saw()
	for _, w := range []string{"queue:pause:" + sabFailed, "queue:resume:" + sabFailed, "switch::" + sabFailed, "retry::" + sabFailed} {
		if !strings.Contains(got, w) {
			t.Fatalf("missing %s in %s", w, got)
		}
	}

	// The Servers and Repair tabs.
	var u downloads.UsenetView
	_ = json.Unmarshal(r.ana.call("GET", item+"/usenet", "", 200), &u)
	if len(u.Stages) != 3 || u.Stages[2].Stage != "repair" || len(u.JobServers) != 2 || u.JobServers[0].Amount != "812.4 MB" {
		t.Fatalf("usenet view %+v", u)
	}
	if u.Servers == nil || u.Servers.Total != 900 || len(u.Servers.Servers) != 1 || u.Servers.Servers[0].ArticlesFound != 9 {
		t.Fatalf("server stats %+v", u.Servers)
	}
	if strings.Contains(fmt.Sprint(u.Stages), "TOPSECRET") {
		t.Fatal("an indexer key reached the answer")
	}
}

// route: GET /api/v1/downloads/{id}/usenet
// A job HolyArrs sent is not retried here (its failure path searched again
// already), and its tabs read through the download.
func TestUsenetOwnDownload(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	f := &usenetFake{}
	exec41(t, x.s, "UPDATE downloaders SET adapter = 'sabnzbd', protocol = 'usenet', url = ?", f.server(t))
	exec41(t, x.s, "UPDATE downloads SET external_id = ?", sabFailed)
	var did int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders").Scan(&did)
	do(t, "POST", x.srv.URL+"/api/v1/downloaders/"+itoa(did)+"/items/"+sabFailed+"/retry", "", 409)
	if strings.Contains(f.saw(), "retry:") {
		t.Fatal("retry reached the client")
	}
	var u downloads.UsenetView
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/"+itoa(x.dlID)+"/usenet", "", 200), &u)
	if len(u.Stages) != 3 || len(u.JobServers) != 2 {
		t.Fatalf("usenet view %+v", u)
	}
}
