package sources

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// A feed with its passkey in the path (TorrentLeech style) is shown masked,
// keeps working, and keeps its passkey when the masked address is sent back.
func TestSourcePathPasskeyMasked(t *testing.T) {
	t.Parallel()
	srv, _ := sourceServer(t)
	passkey := "0f3a9c2b7d1e4f5a8b6c0d9e2f1a3b4c"
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+passkey {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>t</title>
<item><title>Some.Show.S01E01.1080p</title><link>` + "http://dl.example/rss/download/1/" + passkey + "/a.torrent" + `</link></item></channel></rss>`))
	}))
	t.Cleanup(feed.Close)

	raw, _ := json.Marshal(map[string]any{"name": "TL", "format": "rss", "url": feed.URL + "/" + passkey, "allow_local_network": true})
	var x SourceView
	b := do(t, "POST", srv.URL+"/api/v1/sources", string(raw), 201)
	_ = json.Unmarshal(b, &x)
	if strings.Contains(string(b), passkey) || x.URL != feed.URL+"/"+secrets.Masked {
		t.Fatalf("added %s", b)
	}
	if list := string(do(t, "GET", srv.URL+"/api/v1/sources", "", 200)); strings.Contains(list, passkey) {
		t.Fatalf("list %s", list)
	}
	id := fmt.Sprint(x.ID)
	raw, _ = json.Marshal(map[string]any{"name": "TL feed", "url": x.URL})
	if b := do(t, "PUT", srv.URL+"/api/v1/sources/"+id, string(raw), 200); strings.Contains(string(b), passkey) {
		t.Fatalf("changed %s", b)
	}
	var res struct {
		OK   bool   `json:"ok"`
		Text string `json:"text"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/sources/"+id+"/test", "", 200), &res)
	if !res.OK {
		t.Fatalf("test after the round trip: %+v", res)
	}
}
