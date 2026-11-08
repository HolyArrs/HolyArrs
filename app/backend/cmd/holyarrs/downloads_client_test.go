package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestDownloadsViewListsEveryTorrent(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	f := &allClient{}
	exec41(t, x.s, "UPDATE downloaders SET url = ?", f.server(t).URL)
	before := countDownloads(t, x.s)
	var out viewRows
	for range 2 { // a second read finds the same rows and still makes no records
		feedStore(t, x.api)
		out = viewRows{}
		_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view", "", 200), &out)
	}
	if len(out.Downloads) != 4 {
		t.Fatalf("rows %d: %+v", len(out.Downloads), out.Downloads)
	}
	by := map[string]downloads.DownloadRow{}
	for _, r := range out.Downloads {
		by[r.Hash] = r
	}
	if own := out.Downloads[0]; own.ID != x.dlID || own.Outside || own.ClientItem {
		t.Fatalf("HolyArrs' own row %+v", own)
	}
	o := by[outHash]
	if !o.Outside || !o.ClientItem || o.ID != 0 || o.Name != "Linux.ISO" || o.Category != "linux" || o.Phase != "downloading" || o.Size != 5000 ||
		o.Seeds != 3 || o.Peers != 4 || o.ETA == nil || *o.ETA != 60 || o.StartedAt == nil || !o.CanAct || o.Kind != "" || o.TitleID != 0 || o.DownloaderID == 0 {
		t.Fatalf("outside row %+v", o)
	}
	if s := by[seedHash]; s.Phase != "done" || s.State != "seeding" || s.Ratio == nil || *s.Ratio != 1.5 || s.Active {
		t.Fatalf("seeding row %+v", s)
	}
	if p := by[pauseHash]; p.Phase != "done" || p.State != "paused" {
		t.Fatalf("stopped row %+v", p)
	}
	if n := countDownloads(t, x.s); n != before {
		t.Fatalf("outside rows made download records: %d -> %d", before, n)
	}
	// One torrents/info per background read, without a category: no call
	// per row, and none from the view itself.
	if n := strings.Count(f.saw(), "torrents/info?"); n != 2 || strings.Contains(f.saw(), "category=") {
		t.Fatalf("client calls %s", f.saw())
	}
}

func TestDownloadsViewKnownHash(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	f := &allClient{}
	exec41(t, x.s, "UPDATE downloaders SET url = ?", f.server(t).URL)
	exec41(t, x.s, "UPDATE downloads SET external_id = ?", outHash)
	feedStore(t, x.api)
	var out viewRows
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view?limit=1", "", 200), &out)
	for _, r := range out.Downloads {
		if r.Hash == outHash && r.ClientItem {
			t.Fatalf("shown twice: %+v", r)
		}
	}
	// Push HolyArrs' own row off the list with a newer one.
	err := x.s.Write(context.Background(), func(tx *sql.Tx) error {
		var dr int64
		if err := tx.QueryRow("SELECT downloader_id FROM downloads").Scan(&dr); err != nil {
			return err
		}
		_, err := database.Insert(tx, "downloads", &domain.Download{DownloaderID: dr, SlotIDs: []int64{}, ExternalID: "ffffffffffffffffffffffffffffffffffffffff", State: "cleaned",
			FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	out = viewRows{}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view?limit=1", "", 200), &out)
	found := false
	for _, r := range out.Downloads {
		if r.Hash == outHash {
			found = true
			if r.Outside || r.Title != "Salt Choir" || r.Kind != "movie" || r.TitleID == 0 {
				t.Fatalf("known hash row %+v", r)
			}
		}
	}
	if !found {
		t.Fatalf("known hash missing: %+v", out.Downloads)
	}
}

func TestClientItemControls(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	f := &allClient{}
	exec41(t, x.s, "UPDATE downloaders SET url = ?", f.server(t).URL)
	var did int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders").Scan(&did)
	base := x.srv.URL + "/api/v1/downloaders/" + dlItoa(did) + "/items/" + strings.ToUpper(outHash)
	for _, act := range []string{"pause", "resume", "top", "recheck", "reannounce"} {
		do(t, "POST", base+"/"+act, "", 200)
	}
	do(t, "POST", base+"/remove", `{"delete_files":true}`, 200)
	do(t, "POST", base+"/remove", "", 200)
	var d downloads.DownloadDetailView
	_ = json.Unmarshal(do(t, "GET", base+"/detail", "", 200), &d)
	if d.Torrent == nil || d.Torrent.Downloaded != 2500 || d.Downloader != "Q" || len(d.Log) != 0 || d.RequestedBy != "Added in qBittorrent" {
		t.Fatalf("detail %+v", d)
	}
	var sw struct {
		Trackers []map[string]any `json:"trackers"`
	}
	_ = json.Unmarshal(do(t, "GET", base+"/swarm", "", 200), &sw)
	if len(sw.Trackers) != 1 || strings.Contains(sw.Trackers[0]["url"].(string), "passkey") {
		t.Fatalf("swarm %+v", sw)
	}
	var fs struct {
		Files []map[string]any `json:"files"`
	}
	_ = json.Unmarshal(do(t, "GET", base+"/files", "", 200), &fs)
	if len(fs.Files) != 1 {
		t.Fatalf("files %+v", fs)
	}
	do(t, "PUT", base+"/files", `{"indexes":[0],"priority":3}`, 400)
	do(t, "PUT", base+"/files", `{"indexes":[0],"priority":0}`, 200)
	got := f.saw()
	for _, w := range []string{"torrents/stop?hashes=" + outHash, "torrents/start?hashes=" + outHash, "torrents/topPrio?hashes=" + outHash,
		"torrents/recheck?hashes=" + outHash, "torrents/reannounce?hashes=" + outHash, "torrents/delete?deleteFiles=true&hashes=" + outHash,
		"torrents/delete?deleteFiles=false&hashes=" + outHash, "torrents/filePrio?hash=" + outHash + "&id=0&priority=0"} {
		if !strings.Contains(got, w) {
			t.Fatalf("missing %s in %s", w, got)
		}
	}
	// "all" and lists are not one hash; an unknown Downloader is not found.
	do(t, "POST", x.srv.URL+"/api/v1/downloaders/"+dlItoa(did)+"/items/all/pause", "", 404)
	do(t, "POST", x.srv.URL+"/api/v1/downloaders/999/items/"+outHash+"/pause", "", 404)
	do(t, "POST", x.srv.URL+"/api/v1/downloaders/x/items/"+outHash+"/pause", "", 404)
	if strings.Contains(f.saw(), "hashes=all") {
		t.Fatalf("all reached the client: %s", f.saw())
	}
	// Nothing here touched HolyArrs' own download.
	var st string
	_ = x.s.Read().QueryRow("SELECT state FROM downloads WHERE id = ?", x.dlID).Scan(&st)
	if st != "downloading" {
		t.Fatalf("own download now %s", st)
	}
}

// Members who may not manage the Library see no client items and may not
// steer them.
func TestClientItemsNeedManageLibrary(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	f := &allClient{}
	qb := f.server(t)
	var did int64
	err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		dr := &domain.Downloader{Writable: true, Name: "Q", Adapter: "qbittorrent", Protocol: "torrent", URL: qb.URL, Category: "holyarrs", Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		_, err := database.Insert(tx, "downloaders", dr, domain.System)
		did = dr.ID
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/downloaders/" + dlItoa(did) + "/items/" + outHash
	for _, act := range []string{"pause", "resume", "remove"} {
		r.ana.call("POST", base+"/"+act, "", 403)
	}
	r.ana.call("GET", base+"/detail", "", 403)
	var out viewRows
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/downloads/view", "", 200), &out)
	if len(out.Downloads) != 0 {
		t.Fatalf("a member sees client items: %+v", out.Downloads)
	}
	if strings.Contains(f.saw(), "torrents/stop") || strings.Contains(f.saw(), "torrents/delete") {
		t.Fatalf("a member reached the client: %s", f.saw())
	}
	feedStore(t, r.api)
	out = viewRows{}
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/downloads/view", "", 200), &out)
	if len(out.Downloads) != 4 {
		t.Fatalf("owner rows %d", len(out.Downloads))
	}
	r.owner.call("POST", base+"/pause", "", 200)
}
