package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
)

// qbList is a fake qBittorrent that lists the given torrents.
func qbList(t *testing.T, torrents string) *httptest.Server {
	t.Helper()
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch strings.TrimPrefix(r.URL.Path, "/api/v2/") {
		case "torrents/info":
			if r.Form.Get("category") != "" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(torrents))
		case "app/webapiVersion":
			_, _ = w.Write([]byte("2.15.1"))
		}
	}))
	t.Cleanup(qb.Close)
	return qb
}

// pointQB moves the seeded Downloader to url and drops its cached engine.
func pointQB(t *testing.T, x *dl41, url string) {
	t.Helper()
	exec41(t, x.s, "UPDATE downloaders SET url = ?", url)
	var id int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders LIMIT 1").Scan(&id)
	x.api.dl.Manager().Forget(id)
}

type viewAnswer struct {
	Downloads    []downloads.DownloadRow  `json:"downloads"`
	Totals       downloads.DownloadTotals `json:"totals"`
	Clients      []downloads.ClientHealth `json:"clients"`
	OutsideTotal int                      `json:"outside_total"`
	NextCursor   string                   `json:"next_cursor"`
}

func view(t *testing.T, url string) viewAnswer {
	t.Helper()
	var v viewAnswer
	if err := json.Unmarshal(do(t, "GET", url, "", 200), &v); err != nil {
		t.Fatal(err)
	}
	return v
}
