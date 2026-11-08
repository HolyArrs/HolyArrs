package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// noClient is a reader Manager that never reaches a client: the store only
// holds what the test loads.
type noClient struct{ m *downloads.Manager }

func (noClient) Reader(context.Context, domain.Downloader) (downloads.Engine, uint64, error) {
	return nil, 0, errors.New("no client in this test")
}
func (noClient) ReportRead(context.Context, domain.Downloader, error) {}
func (noClient) Generation(int64) uint64                              { return 0 }
func (n noClient) Interval(a int) time.Duration                       { return n.m.Interval(a) }

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

// viewAs reads the Downloads rows as one person, past the sign-in.
func viewAs(t *testing.T, x *dl41, who webserver.Caller) []downloads.DownloadRow {
	t.Helper()
	return viewAt(t, x, who, "/api/v1/downloads/view")
}

// viewAt is viewAs at a given address (?asked=1 for the Requests page).
func viewAt(t *testing.T, x *dl41, who webserver.Caller, url string) []downloads.DownloadRow {
	t.Helper()
	a := New(Server{Store: x.s, Queue: x.q, Live: webserver.NewLiveHub(webserver.LiveOptions{Heartbeat: time.Second}), Metadata: metadata.Lab(), Version: "test"})
	r := httptest.NewRequest("GET", url, nil)
	r = r.WithContext(webserver.ContextWith(r.Context(), who))
	w := httptest.NewRecorder()
	a.dl.DownloadsView(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var out struct {
		Downloads []downloads.DownloadRow `json:"downloads"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out.Downloads
}
