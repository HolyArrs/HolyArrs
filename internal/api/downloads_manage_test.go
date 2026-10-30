package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

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
