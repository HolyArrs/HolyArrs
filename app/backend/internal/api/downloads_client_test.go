package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
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
	Downloads []downloads.DownloadRow `json:"downloads"`
}
