package downloads

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// mixedTorrents is outside torrents in four qBittorrent states; each name
// carries the stage the page should file it under.
func mixedTorrents(counts map[string]int) string {
	stage := map[string]string{"uploading": "seeding", "downloading": "downloading", "pausedDL": "paused", "queuedDL": "queued"}
	var parts []string
	i := 0
	for _, state := range []string{"uploading", "downloading", "pausedDL", "queuedDL"} {
		progress := "0.5"
		if state == "uploading" {
			progress = "1"
		}
		for range counts[state] {
			i++
			parts = append(parts, fmt.Sprintf(`{"hash":"%040x","name":"Outside.%s.%d","category":"linux","size":100,"progress":%s,"dlspeed":0,"state":"%s","added_on":%d}`,
				i, stage[state], i, progress, state, 1700000000+i))
		}
	}
	return "[" + strings.Join(parts, ",") + "]"
}

type restAnswer struct {
	Downloads    []DownloadRow     `json:"downloads"`
	OutsideTotal int               `json:"outside_total"`
	OutsideRest  map[string]RestOf `json:"outside_rest"`
}

func restView(t *testing.T, url string) restAnswer {
	t.Helper()
	var v restAnswer
	if err := json.Unmarshal(do(t, "GET", url, "", 200), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// The view says, per Downloader and stage, how many outside rows it did not
// send, so the tabs can count every torrent the Downloader holds.
func TestDownloadsViewRestCountsRowsPastThePage(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	seeded := map[string]int{"uploading": 300, "downloading": 100, "pausedDL": 25, "queuedDL": 25}
	pointQB(t, x, qbList(t, mixedTorrents(seeded)).URL)
	feedStore(t, x.dl)
	want := map[string]int{"seeding": 300, "downloading": 100, "paused": 25, "queued": 25}

	v := restView(t, x.srv.URL+"/api/v1/downloads/view")
	if v.OutsideTotal != 450 {
		t.Fatalf("outside_total %d", v.OutsideTotal)
	}
	if len(v.OutsideRest) != 1 {
		t.Fatalf("outside_rest %v, want one Downloader", v.OutsideRest)
	}
	got := map[string]int{}
	sent := 0
	for _, r := range v.Downloads {
		if r.ClientItem {
			sent++
			got[strings.Split(r.Name, ".")[1]]++
		}
	}
	rest := 0
	for _, e := range v.OutsideRest {
		if e.Protocol != "torrent" {
			t.Fatalf("protocol %q, want torrent", e.Protocol)
		}
		for s, n := range e.Stages {
			got[s] += n
			rest += n
		}
	}
	if sent != OutsideCap || rest != 450-OutsideCap {
		t.Fatalf("sent %d rest %d", sent, rest)
	}
	for s, n := range want {
		if got[s] != n {
			t.Fatalf("stage %s: sent + rest = %d, want %d (all %v)", s, got[s], n, got)
		}
	}

	if all := restView(t, x.srv.URL+"/api/v1/downloads/view?outside_limit=2000"); len(all.OutsideRest) != 0 {
		t.Fatalf("every row sent, rest %v", all.OutsideRest)
	}
}

// With ?client= only that Downloader's rest comes back.
func TestDownloadsViewRestFollowsClient(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	pointQB(t, x, qbList(t, outsideTorrents(250, 0)).URL)
	feedStore(t, x.dl)
	var id int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders LIMIT 1").Scan(&id)
	key := strconv.FormatInt(id, 10)
	v := restView(t, x.srv.URL+"/api/v1/downloads/view?client="+key)
	if len(v.OutsideRest) != 1 || v.OutsideRest[key].Stages["seeding"] != 50 {
		t.Fatalf("rest %v, want only %s with 50 seeding", v.OutsideRest, key)
	}
	if other := restView(t, x.srv.URL+"/api/v1/downloads/view?client="+strconv.FormatInt(id+99, 10)); len(other.OutsideRest) != 0 {
		t.Fatalf("another client: rest %v", other.OutsideRest)
	}
}

// stageOf must file a row the way the page's tabs do.
func TestStageOfMatchesTabs(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		row  DownloadRow
		want string
	}{
		{DownloadRow{Phase: "downloading", ClientHint: "checking"}, "checking"},
		{DownloadRow{Phase: "waiting", ClientHint: "moving"}, "checking"},
		{DownloadRow{Phase: "waiting"}, "queued"},
		{DownloadRow{Phase: "paused"}, "paused"},
		{DownloadRow{Phase: "downloading", DownloadView: DownloadView{Progress: 0.4}}, "downloading"},
		{DownloadRow{Phase: "downloading", DownloadView: DownloadView{Progress: 1}}, "importing"},
		{DownloadRow{Phase: "stalled"}, "stalled"},
		{DownloadRow{Phase: "waiting_to_import"}, "importing"},
		{DownloadRow{Phase: "importing"}, "importing"},
		{DownloadRow{Phase: "needs_a_decision"}, "decide"},
		{DownloadRow{Phase: "failed"}, "failed"},
		{DownloadRow{Phase: "vanished", Replaced: true}, "done"},
		{DownloadRow{Phase: "import_refused"}, "failed"},
		{DownloadRow{Phase: "done", DownloadView: DownloadView{State: "seeding"}}, "seeding"},
		{DownloadRow{Phase: "done"}, "done"},
		{DownloadRow{Phase: "removed"}, "done"},
	} {
		if got := stageOf(c.row); got != c.want {
			t.Errorf("%+v: %s, want %s", c.row, got, c.want)
		}
	}
}
