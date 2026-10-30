package api

import (
	"bytes"
	"context"
	"errors"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/domain"
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
	ctx := context.Background()
	for _, bad := range []string{"magnet:?xt=urn:btih:" + outHash + "\r\nx", "https://a.test/x\n", "ftp://a.test/x", "javascript:alert(1)", "https://user@1.1.1.1/x", "magnet:"} {
		if err := checkLink(ctx, bad, true); err == nil {
			t.Errorf("link %q passed", bad)
		}
	}
	for _, local := range []string{"http://127.0.0.1/x.torrent", "http://10.10.25.25:8080/x.torrent", "http://[::1]/x", "http://169.254.169.254/latest"} {
		if err := checkLink(ctx, local, false); err == nil {
			t.Errorf("private link %q passed", local)
		}
	}
	if err := checkLink(ctx, "http://10.10.25.25/x.torrent", true); err != nil {
		t.Fatalf("local allowed: %v", err)
	}
	if err := checkLink(ctx, "http://169.254.169.254/latest", true); err == nil {
		t.Fatal("metadata address passed with local network on")
	}
	for _, ok := range []string{outHash, strings.Repeat("a", 64), "magnet:?xt=urn:btih:" + outHash, "https://1.1.1.1/x.torrent"} {
		if err := checkLink(ctx, ok, false); err != nil {
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

// The route refuses CR/LF inside a link, private http links and traversal.
func TestAddTorrentsRouteRefusals(t *testing.T) {
	x, f, base := manageBase(t)
	do(t, "POST", base+"/torrents", `{"links":["magnet:?xt=urn:btih:`+outHash+`\r\nhttp://127.0.0.1/x"]}`, 400)
	// The seeded Downloader may use the local network; with that off a private link is refused.
	exec41(t, x.s, "UPDATE downloaders SET allow_local_network = 0")
	do(t, "POST", base+"/torrents", `{"links":["http://192.168.1.10/x.torrent"]}`, 400)
	if strings.Contains(f.saw(), "torrents/add") {
		t.Fatalf("refused links reached the client: %s", f.saw())
	}
}
