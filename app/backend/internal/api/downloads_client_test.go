package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

const (
	ownHash   = "abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd"
	outHash   = "1111111111111111111111111111111111111111"
	seedHash  = "2222222222222222222222222222222222222222"
	pauseHash = "3333333333333333333333333333333333333333"
)

// allClient is a qBittorrent holding HolyArrs' torrent plus three torrents
// in other categories; it records every call.
type allClient struct {
	webapi string // app/webapiVersion; "" answers 2.15.1
	mu     sync.Mutex
	calls  []string
}

func (f *allClient) saw() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.calls, " ")
}

func (f *allClient) server(t *testing.T) *httptest.Server {
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		path := strings.TrimPrefix(r.URL.Path, "/api/v2/")
		f.mu.Lock()
		f.calls = append(f.calls, path+"?"+r.Form.Encode())
		f.mu.Unlock()
		switch path {
		case "torrents/info":
			if r.Form.Get("category") != "" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[
{"hash":"` + strings.ToUpper(ownHash) + `","name":"Salt.Choir","category":"holyarrs","size":1000,"progress":0.3,"state":"downloading","added_on":1700000000},
{"hash":"` + outHash + `","name":"Linux.ISO","category":"linux","size":5000,"progress":0.5,"dlspeed":100,"upspeed":5,"num_seeds":3,"num_leechs":4,"eta":60,"state":"downloading","added_on":1700000100},
{"hash":"` + seedHash + `","name":"Old.Album","category":"music","size":300,"progress":1,"upspeed":9,"state":"uploading","ratio":1.5},
{"hash":"` + pauseHash + `","name":"Done.Thing","category":"","size":200,"progress":1,"state":"stoppedUP"}]`))
		case "torrents/properties":
			_, _ = w.Write([]byte(`{"total_size":5000,"total_downloaded":2500,"seeds":3,"seeds_total":9,"save_path":"/data/linux"}`))
		case "torrents/files":
			_, _ = w.Write([]byte(`[{"index":0,"name":"linux.iso","size":5000,"progress":0.5,"priority":1}]`))
		case "torrents/trackers":
			_, _ = w.Write([]byte(`[{"url":"http://t.lab/announce?passkey=x","tier":0,"status":2}]`))
		case "app/preferences":
			_, _ = w.Write([]byte(`{"save_path":"/data/downloads"}`))
		case "torrents/categories":
			_, _ = w.Write([]byte(`{"linux":{"name":"linux","savePath":"/data/linux"},"music":{"name":"music","savePath":""}}`))
		case "sync/torrentPeers":
			_, _ = w.Write([]byte(`{"peers":{}}`))
		case "app/webapiVersion":
			v := f.webapi
			if v == "" {
				v = "2.15.1"
			}
			_, _ = w.Write([]byte(v))
		}
	}))
	t.Cleanup(qb.Close)
	return qb
}

type viewRows struct {
	Downloads []DownloadRow `json:"downloads"`
}

func countDownloads(t *testing.T, s *database.Store) int {
	t.Helper()
	var n int
	if err := s.Read().QueryRow("SELECT COUNT(*) FROM downloads").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The Downloads view lists every torrent in the client, HolyArrs' own once.
// The rest are read-only outside rows: no download records appear for them.
func TestDownloadsViewListsEveryTorrent(t *testing.T) {
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
	by := map[string]DownloadRow{}
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

// A torrent HolyArrs knows by hash but that is off the list keeps its Title
// and loses the outside mark.
func TestDownloadsViewKnownHash(t *testing.T) {
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

// route: POST /api/v1/downloaders/{id}/items/{hash}/pause
// route: POST /api/v1/downloaders/{id}/items/{hash}/resume
// route: POST /api/v1/downloaders/{id}/items/{hash}/top
// route: POST /api/v1/downloaders/{id}/items/{hash}/recheck
// route: POST /api/v1/downloaders/{id}/items/{hash}/reannounce
// route: POST /api/v1/downloaders/{id}/items/{hash}/remove
// route: GET /api/v1/downloaders/{id}/items/{hash}/detail
// route: GET /api/v1/downloaders/{id}/items/{hash}/swarm
// route: GET /api/v1/downloaders/{id}/items/{hash}/files
// route: PUT /api/v1/downloaders/{id}/items/{hash}/files
func TestClientItemControls(t *testing.T) {
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	f := &allClient{}
	exec41(t, x.s, "UPDATE downloaders SET url = ?", f.server(t).URL)
	var did int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders").Scan(&did)
	base := x.srv.URL + "/api/v1/downloaders/" + itoa(did) + "/items/" + strings.ToUpper(outHash)
	for _, act := range []string{"pause", "resume", "top", "recheck", "reannounce"} {
		do(t, "POST", base+"/"+act, "", 200)
	}
	do(t, "POST", base+"/remove", `{"delete_files":true}`, 200)
	do(t, "POST", base+"/remove", "", 200)
	var d DownloadDetailView
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
	do(t, "POST", x.srv.URL+"/api/v1/downloaders/"+itoa(did)+"/items/all/pause", "", 404)
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
	base := "/api/v1/downloaders/" + itoa(did) + "/items/" + outHash
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
