package api

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

	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/downloader"
)

func manageBase(t *testing.T) (x *dl41, f *allClient, base string) {
	t.Helper()
	x = seed41(t, domain.SlotDownloading, "downloading")
	f = &allClient{}
	exec41(t, x.s, "UPDATE downloaders SET url = ?", f.server(t).URL)
	var did int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders").Scan(&did)
	return x, f, x.srv.URL + "/api/v1/downloaders/" + itoa(did)
}

// route: POST /api/v1/downloaders/{id}/torrents
func TestAddTorrentsRoute(t *testing.T) {
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

func TestSubfolderAndLinkGuards(t *testing.T) {
	for _, bad := range []string{"/etc", "../up", "a/../../b", `C:\x`, `\host\share`, "c:x", "a\x00b", "..", "."} {
		if _, err := subfolder(bad); err == nil {
			t.Errorf("subfolder %q passed", bad)
		}
	}
	if s, err := subfolder("tv/new//"); err != nil || s != "tv/new" {
		t.Fatalf("good subfolder %q %v", s, err)
	}
	if joinFolder(`D:\dl`, "tv/new") != `D:\dl\tv\new` || joinFolder("/data/", "tv") != "/data/tv" {
		t.Fatal("join")
	}
	for _, bad := range []string{"magnet:?xt=urn:btih:" + outHash + "\r\nx", "https://a.test/x\n", "ftp://a.test/x", "javascript:alert(1)", "https://user@1.1.1.1/x", "magnet:", "https://a.test/a b"} {
		if err := checkLink(bad); err == nil {
			t.Errorf("link %q passed", bad)
		}
	}
	for _, ok := range []string{outHash, strings.Repeat("a", 64), "magnet:?xt=urn:btih:" + outHash, "https://1.1.1.1/x.torrent"} {
		if err := checkLink(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
}

// Without the settings permission a folder is only a subfolder of the
// category's folder and the category must exist in the client.
func TestClientFolderWithoutSettings(t *testing.T) {
	x, _, _ := manageBase(t)
	var did int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders").Scan(&did)
	ctx := auth.ContextWith(context.Background(), auth.Who{UserID: 7, Via: "session", Perms: []auth.Perm{auth.PermManageLibrary}})
	if _, _, err := x.api.clientFolder(ctx, did, "linux", "/etc", true); !errors.Is(err, errFolder) {
		t.Fatalf("absolute: %v", err)
	}
	if _, _, err := x.api.clientFolder(ctx, did, "linux", "../../root", true); !errors.Is(err, errFolder) {
		t.Fatalf("traversal: %v", err)
	}
	if _, _, err := x.api.clientFolder(ctx, did, "made-up", "", true); !errors.Is(err, errCategory) {
		t.Fatalf("category: %v", err)
	}
	if c, f, err := x.api.clientFolder(ctx, did, "linux", "iso/new", true); err != nil || c != "linux" || f != "/data/linux/iso/new" {
		t.Fatalf("subfolder %q %q %v", c, f, err)
	}
	if _, f, err := x.api.clientFolder(ctx, did, "music", "x", true); err != nil || f != "/data/downloads/x" {
		t.Fatalf("default root %q %v", f, err)
	}
	admin := auth.ContextWith(context.Background(), auth.Who{UserID: 1, Via: "session", Perms: []auth.Perm{auth.PermManageLibrary, auth.PermSettings}})
	if _, f, err := x.api.clientFolder(admin, did, "anything", "/srv/media", true); err != nil || f != "/srv/media" {
		t.Fatalf("admin %q %v", f, err)
	}
}

// The route refuses CR/LF inside a link; a private http link is fetched by
// HolyArrs under the Downloader's policy, refused, and never reaches the client.
func TestAddTorrentsRouteRefusals(t *testing.T) {
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
	_, f, base := manageBase(t)
	do(t, "POST", base+"/slow-mode", `{"on":true}`, 200)
	if !strings.Contains(f.saw(), "transfer/speedLimitsMode") || !strings.Contains(f.saw(), "transfer/toggleSpeedLimitsMode") {
		t.Fatalf("calls %s", f.saw())
	}
	do(t, "POST", base+"/slow-mode", `{}`, 400)
}

// Seeding rules on an outside torrent that may end in a remove (the
// torrent's own action, or the client's global one) need the settings
// permission; an explicit stop on 5.2, or clearing every limit, does not.
func TestShareLimitsOutsideNeedSettings(t *testing.T) {
	x, f, _ := manageBase(t)
	var did int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders").Scan(&did)
	member := auth.Who{UserID: 7, Via: "session", Perms: []auth.Perm{auth.PermManageLibrary}, Scopes: []string{auth.ScopeWrite}}
	owner := auth.Who{UserID: 1, Via: "session", Perms: []auth.Perm{auth.PermManageLibrary, auth.PermSettings}, Scopes: []string{auth.ScopeAdmin, auth.ScopeWrite}}
	call := func(who auth.Who, body string) int {
		req := httptest.NewRequest("POST", "/x", strings.NewReader(body)).WithContext(auth.ContextWith(context.Background(), who))
		req.SetPathValue("id", itoa(did))
		rec := httptest.NewRecorder()
		x.api.shareLimits(rec, req)
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

func TestClientHints(t *testing.T) {
	for in, want := range map[string]string{"checkingDL": "checking", "checkingUP": "checking", "checkingResumeData": "checking", "moving": "moving", "error": "errored", "missingFiles": "missing", "stalledUP": "idle_seed", "stalledDL": "", "uploading": ""} {
		if got := clientHint(in); got != want {
			t.Errorf("%s: %q", in, got)
		}
	}
}

// route: POST /api/v1/downloaders/{id}/items/{hash}/trackers
// route: PUT /api/v1/downloaders/{id}/items/{hash}/trackers
// route: POST /api/v1/downloaders/{id}/items/{hash}/trackers/remove
func TestTrackerRoutes(t *testing.T) {
	x, f, base := manageBase(t)
	item := base + "/items/" + outHash + "/trackers"
	var sw struct{ Trackers []downloader.Tracker }
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

// Without the settings permission a move stays inside the Downloader's folders.
func TestLocationWithoutSettings(t *testing.T) {
	x, f, _ := manageBase(t)
	var did int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders").Scan(&did)
	member := auth.Who{UserID: 7, Via: "session", Perms: []auth.Perm{auth.PermManageLibrary}, Scopes: []string{auth.ScopeWrite}}
	call := func(body string) int {
		req := httptest.NewRequest("POST", "/x", strings.NewReader(body)).WithContext(auth.ContextWith(context.Background(), member))
		req.SetPathValue("id", itoa(did))
		rec := httptest.NewRecorder()
		x.api.setLocation(rec, req)
		return rec.Code
	}
	if c := call(`{"hashes":["` + outHash + `"],"folder":"/etc"}`); c != 400 {
		t.Fatalf("absolute: %d", c)
	}
	if c := call(`{"hashes":["` + outHash + `"],"folder":"../up"}`); c != 400 {
		t.Fatalf("traversal: %d", c)
	}
	if c := call(`{"hashes":["` + outHash + `"],"category":"linux","folder":"iso/old"}`); c != 200 || !strings.Contains(f.saw(), "location=%2Fdata%2Flinux%2Fiso%2Fold") {
		t.Fatalf("subfolder: %d %s", c, f.saw())
	}
}
