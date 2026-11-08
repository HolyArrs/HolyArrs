package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
)

func TestAddTorrentsRoute(t *testing.T) {
	t.Parallel()
	_, f, base := manageBase(t)
	var out struct{ Added, Duplicates, Pending int }
	_ = json.Unmarshal(do(t, "POST", base+"/torrents", `{"links":["magnet:?xt=urn:btih:`+outHash+`","magnet:?xt=urn:btih:`+strings.Repeat("9", 40)+`"],"category":"linux","paused":true}`, 200), &out)
	if out.Added != 1 || out.Duplicates != 1 {
		t.Fatalf("json add %+v (%s)", out, f.saw())
	}
	do(t, "POST", base+"/torrents", `{"links":["ftp://nope"]}`, 400)
	do(t, "POST", base+"/torrents", `{"links":[]}`, 400)
	// A multipart upload with a .torrent file.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("links", "")
	fw, _ := mw.CreateFormFile("torrents", "a.torrent")
	_, _ = fw.Write([]byte("d4:infod6:lengthi1e4:name1:a12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaaee"))
	_ = mw.Close()
	req, _ := http.NewRequest("POST", base+"/torrents", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out = struct{ Added, Duplicates, Pending int }{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != 200 || out.Added != 1 {
		t.Fatalf("upload %d %+v", resp.StatusCode, out)
	}
	if n := strings.Count(f.saw(), "torrents/add?"); n != 2 {
		t.Fatalf("adds %d: %s", n, f.saw())
	}
	do(t, "POST", strings.Replace(base, "/downloaders/", "/downloaders/9", 1)+"/torrents", `{"links":["`+outHash+`"]}`, 404)
}

// route: POST /api/v1/downloaders/{id}/bulk
func TestBulkRoute(t *testing.T) {
	t.Parallel()
	_, f, base := manageBase(t)
	hs := `["` + outHash + `","` + strings.ToUpper(seedHash) + `"]`
	for _, a := range []string{"start", "pause", "recheck", "reannounce", "top", "bottom", "up", "down", "force", "unforce"} {
		do(t, "POST", base+"/bulk", `{"action":"`+a+`","hashes":`+hs+`}`, 200)
	}
	do(t, "POST", base+"/bulk", `{"action":"remove","hashes":`+hs+`,"delete_files":true}`, 200)
	if !strings.Contains(f.saw(), "torrents/delete?deleteFiles=true&hashes="+outHash+"%7C"+seedHash) || !strings.Contains(f.saw(), "torrents/setForceStart?hashes="+outHash+"%7C"+seedHash+"&value=true") {
		t.Fatalf("calls %s", f.saw())
	}
	do(t, "POST", base+"/bulk", `{"action":"explode","hashes":`+hs+`}`, 400)
	do(t, "POST", base+"/bulk", `{"action":"start","hashes":["all"]}`, 400)
	do(t, "POST", base+"/bulk", `{"action":"start","hashes":[]}`, 400)
}

// Without the settings permission a folder is only a subfolder of the
// category's folder and the category must exist in the client.
func TestClientFolderWithoutSettings(t *testing.T) {
	t.Parallel()
	x, _, _ := manageBase(t)
	var did int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders").Scan(&did)
	ctx := webserver.ContextWith(context.Background(), webserver.Caller{UserID: 7, Via: "session", Perms: []webserver.Perm{webserver.PermManageLibrary}})
	if _, _, err := x.api.DownloadsWeb().ClientFolder(ctx, did, "linux", "/etc", true); !errors.Is(err, downloads.ErrFolder) {
		t.Fatalf("absolute: %v", err)
	}
	if _, _, err := x.api.DownloadsWeb().ClientFolder(ctx, did, "linux", "../../root", true); !errors.Is(err, downloads.ErrFolder) {
		t.Fatalf("traversal: %v", err)
	}
	if _, _, err := x.api.DownloadsWeb().ClientFolder(ctx, did, "made-up", "", true); !errors.Is(err, downloads.ErrPickCategory) {
		t.Fatalf("category: %v", err)
	}
	if c, f, err := x.api.DownloadsWeb().ClientFolder(ctx, did, "linux", "iso/new", true); err != nil || c != "linux" || f != "/data/linux/iso/new" {
		t.Fatalf("subfolder %q %q %v", c, f, err)
	}
	if _, f, err := x.api.DownloadsWeb().ClientFolder(ctx, did, "music", "x", true); err != nil || f != "/data/downloads/x" {
		t.Fatalf("default root %q %v", f, err)
	}
	admin := webserver.ContextWith(context.Background(), webserver.Caller{UserID: 1, Via: "session", Perms: []webserver.Perm{webserver.PermManageLibrary, webserver.PermSettings}})
	if _, f, err := x.api.DownloadsWeb().ClientFolder(admin, did, "anything", "/srv/media", true); err != nil || f != "/srv/media" {
		t.Fatalf("admin %q %v", f, err)
	}
}

// The route refuses CR/LF inside a link; a private http link is fetched by
// HolyArrs under the Downloader's policy, refused, and never reaches the client.
func TestAddTorrentsRouteRefusals(t *testing.T) {
	t.Parallel()
	x, f, base := manageBase(t)
	do(t, "POST", base+"/torrents", `{"links":["magnet:?xt=urn:btih:`+outHash+`\r\nhttp://127.0.0.1/x"]}`, 400)
	do(t, "POST", base+"/torrents", `{"links":["magnet:?xt=urn:btih:`+outHash+`\u0001"]}`, 400)
	exec41(t, x.s, "UPDATE downloaders SET allow_local_network = 0")
	var out struct {
		Failed int
		Bad    []string
	}
	_ = json.Unmarshal(do(t, "POST", base+"/torrents", `{"links":["http://192.168.1.10/x.torrent"]}`, 200), &out)
	if out.Failed != 1 || len(out.Bad) != 1 || strings.Contains(f.saw(), "torrents/add") {
		t.Fatalf("private link %+v, calls %s", out, f.saw())
	}
}

// route: POST /api/v1/downloaders/{id}/share-limits
func TestShareLimitsRoute(t *testing.T) {
	t.Parallel()
	_, f, base := manageBase(t)
	hs := `["` + outHash + `","` + seedHash + `"]`
	do(t, "POST", base+"/share-limits", `{"hashes":`+hs+`,"ratio":2,"seeding_minutes":1440,"inactive_minutes":-1,"action":"stop"}`, 200)
	if !strings.Contains(f.saw(), "torrents/setShareLimits?hashes="+outHash+"%7C"+seedHash+"&inactiveSeedingTimeLimit=-1&ratioLimit=2&seedingTimeLimit=1440&shareLimitAction=Stop") {
		t.Fatalf("calls %s", f.saw())
	}
	for _, bad := range []string{
		`{"hashes":` + hs + `,"ratio":-3,"seeding_minutes":-2,"inactive_minutes":-2}`,
		`{"hashes":` + hs + `,"ratio":1,"seeding_minutes":9999999,"inactive_minutes":-2}`,
		`{"hashes":` + hs + `,"ratio":1,"seeding_minutes":-2,"inactive_minutes":-2,"action":"explode"}`,
		`{"hashes":["all"],"ratio":1,"seeding_minutes":-2,"inactive_minutes":-2}`,
	} {
		do(t, "POST", base+"/share-limits", bad, 400)
	}
}

// route: POST /api/v1/downloaders/{id}/slow-mode
func TestSlowModeRoute(t *testing.T) {
	t.Parallel()
	_, f, base := manageBase(t)
	do(t, "POST", base+"/slow-mode", `{"on":true}`, 200)
	if !strings.Contains(f.saw(), "transfer/speedLimitsMode") || !strings.Contains(f.saw(), "transfer/toggleSpeedLimitsMode") {
		t.Fatalf("calls %s", f.saw())
	}
	do(t, "POST", base+"/slow-mode", `{}`, 400)
}

func TestShareLimitsOutsideNeedSettings(t *testing.T) {
	t.Parallel()
	x, f, _ := manageBase(t)
	var did int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders").Scan(&did)
	member := webserver.Caller{UserID: 7, Via: "session", Perms: []webserver.Perm{webserver.PermManageLibrary}, Scopes: []string{webserver.ScopeWrite}}
	owner := webserver.Caller{UserID: 1, Via: "session", Perms: []webserver.Perm{webserver.PermManageLibrary, webserver.PermSettings}, Scopes: []string{webserver.ScopeAdmin, webserver.ScopeWrite}}
	call := func(who webserver.Caller, body string) int {
		req := httptest.NewRequest("POST", "/x", strings.NewReader(body)).WithContext(webserver.ContextWith(context.Background(), who))
		req.SetPathValue("id", dlItoa(did))
		rec := httptest.NewRecorder()
		x.api.DownloadsWeb().ShareLimits(rec, req)
		return rec.Code
	}
	ratio := `{"hashes":["` + outHash + `"],"ratio":2,"seeding_minutes":-2,"inactive_minutes":-2}`
	stop := `{"hashes":["` + outHash + `"],"ratio":2,"seeding_minutes":-2,"inactive_minutes":-2,"action":"stop"}`
	clear := `{"hashes":["` + outHash + `"],"ratio":-1,"seeding_minutes":-1,"inactive_minutes":-1}`
	f.webapi = "2.11.4"
	if c := call(member, ratio); c != 403 {
		t.Fatalf("member ratio on 5.1: %d", c)
	}
	if c := call(member, stop); c != 409 {
		t.Fatalf("member stop on 5.1: %d", c)
	}
	if c := call(member, clear); c != 200 {
		t.Fatalf("member clear: %d", c)
	}
	// A too-old version is kept for a couple of minutes, so the upgraded
	// client is a fresh Downloader here.
	x, f, _ = manageBase(t)
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders").Scan(&did)
	f.webapi = "2.15.1"
	if c := call(member, ratio); c != 403 {
		t.Fatalf("member default action on 5.2: %d", c)
	}
	if c := call(member, stop); c != 200 {
		t.Fatalf("member stop on 5.2: %d", c)
	}
	if c := call(owner, ratio); c != 200 {
		t.Fatalf("owner: %d", c)
	}
}

// route: POST /api/v1/downloaders/{id}/items/{hash}/trackers
// route: PUT /api/v1/downloaders/{id}/items/{hash}/trackers
// route: POST /api/v1/downloaders/{id}/items/{hash}/trackers/remove
func TestTrackerRoutes(t *testing.T) {
	t.Parallel()
	x, f, base := manageBase(t)
	item := base + "/items/" + outHash + "/trackers"
	var sw struct{ Trackers []downloads.Tracker }
	_ = json.Unmarshal(do(t, "GET", base+"/items/"+outHash+"/swarm", "", 200), &sw)
	if len(sw.Trackers) != 1 || strings.Contains(sw.Trackers[0].URL, "passkey") || sw.Trackers[0].Ref == "" {
		t.Fatalf("swarm %+v", sw)
	}
	ref := sw.Trackers[0].Ref
	do(t, "POST", item, `{"urls":["udp://tracker.example:80"," "]}`, 200)
	do(t, "PUT", item, `{"ref":"`+ref+`","tier":4}`, 200)
	do(t, "POST", item+"/remove", `{"refs":["`+ref+`"]}`, 200)
	saw := f.saw()
	if !strings.Contains(saw, "torrents/addTrackers?hash="+outHash+"&urls=udp%3A%2F%2Ftracker.example%3A80") || !strings.Contains(saw, "torrents/editTracker?") || !strings.Contains(saw, "tier=4") ||
		!strings.Contains(saw, "torrents/removeTrackers?hash="+outHash+"&urls=http%3A%2F%2Ft.lab%2Fannounce%3Fpasskey%3Dx") {
		t.Fatalf("calls %s", saw)
	}
	do(t, "POST", item+"/remove", `{"refs":["nope"]}`, 404)
	do(t, "PUT", item, `{"ref":"`+ref+`"}`, 400)
	do(t, "PUT", item, `{"ref":"`+ref+`","tier":300}`, 400)
	do(t, "POST", item, `{"urls":[]}`, 400)
	exec41(t, x.s, "UPDATE downloaders SET allow_local_network = 0")
	do(t, "POST", item, `{"urls":["http://127.1/announce"]}`, 400)
	do(t, "POST", item, `{"urls":["ftp://x.example/"]}`, 400)
}

// route: POST /api/v1/downloaders/{id}/location
// route: POST /api/v1/downloaders/{id}/speed-limits
func TestLocationAndSpeedRoutes(t *testing.T) {
	t.Parallel()
	_, f, base := manageBase(t)
	hs := `["` + outHash + `"]`
	do(t, "POST", base+"/location", `{"hashes":`+hs+`,"folder":"/srv/elsewhere"}`, 200)
	do(t, "POST", base+"/speed-limits", `{"hashes":`+hs+`,"down":2048,"up":512}`, 200)
	saw := f.saw()
	if !strings.Contains(saw, "torrents/setLocation?hashes="+outHash+"&location=%2Fsrv%2Felsewhere") || !strings.Contains(saw, "torrents/setDownloadLimit?hashes="+outHash+"&limit=2048") || !strings.Contains(saw, "torrents/setUploadLimit?hashes="+outHash+"&limit=512") {
		t.Fatalf("calls %s", saw)
	}
	do(t, "POST", base+"/location", `{"hashes":`+hs+`,"folder":""}`, 400)
	do(t, "POST", base+"/location", `{"hashes":`+hs+`,"folder":"a\u0000b"}`, 400)
	do(t, "POST", base+"/speed-limits", `{"hashes":`+hs+`,"down":-1,"up":0}`, 400)
}

func TestLocationWithoutSettings(t *testing.T) {
	t.Parallel()
	x, f, _ := manageBase(t)
	var did int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders").Scan(&did)
	member := webserver.Caller{UserID: 7, Via: "session", Perms: []webserver.Perm{webserver.PermManageLibrary}, Scopes: []string{webserver.ScopeWrite}}
	call := func(body string) int {
		req := httptest.NewRequest("POST", "/x", strings.NewReader(body)).WithContext(webserver.ContextWith(context.Background(), member))
		req.SetPathValue("id", dlItoa(did))
		rec := httptest.NewRecorder()
		x.api.DownloadsWeb().SetLocation(rec, req)
		return rec.Code
	}
	// A torrent HolyArrs did not send needs the settings permission to move at all.
	if c := call(`{"hashes":["` + outHash + `"],"category":"linux","folder":"iso/old"}`); c != 403 {
		t.Fatalf("outside torrent: %d", c)
	}
	if c := call(`{"hashes":["` + ownHash + `"],"folder":"/etc"}`); c != 400 {
		t.Fatalf("absolute: %d", c)
	}
	if c := call(`{"hashes":["` + ownHash + `"],"folder":"../up"}`); c != 400 {
		t.Fatalf("traversal: %d", c)
	}
	if c := call(`{"hashes":["` + ownHash + `"],"category":"linux","folder":"iso/old"}`); c != 200 || !strings.Contains(f.saw(), "location=%2Fdata%2Flinux%2Fiso%2Fold") {
		t.Fatalf("subfolder: %d %s", c, f.saw())
	}
}
