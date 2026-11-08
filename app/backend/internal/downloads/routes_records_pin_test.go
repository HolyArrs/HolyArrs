package downloads

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// JSON goldens for the records the download routes return. Run with
// PIN_UPDATE_GOLDEN=1 to rewrite one on purpose.

func dlApiGolden(t *testing.T, name string, v any) {
	t.Helper()
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", name)
	if os.Getenv("PIN_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), got) {
		t.Fatalf("%s changed\n got: %s\nwant: %s", name, got, want)
	}
}

func TestDlPinRecordGoldens(t *testing.T) {
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	year, eta, rel, dl := 2020, int64(90), int64(7), int64(5)
	ratio, avail := 1.5, 0.9
	ep := EpisodeLine{SlotID: 11, Season: 1, Episode: 2, Label: "S01E02", Name: "Pilot", State: "downloading"}
	// A full row: every field set.
	full := DownloadRow{DownloadView: DownloadView{ID: 5, State: "downloading", Label: "Downloading", Text: "t", Progress: 0.5, Speed: 100, UpSpeed: 5, Seeds: 4, Peers: 2,
		SeedsTotal: 40, PeersTotal: 20, ETA: &eta, Active: true},
		Phase: "downloading", Name: "Rel.Name", TitleID: 3, Title: "Salt Choir", Year: &year, Version: "HD", Downloader: "Q", Path: "/data/x",
		DownloaderKind: "qbittorrent", Protocol: "torrent", Size: 1000, StartedAt: &at, FinishedAt: &at, UpdatedAt: at, SlotState: "downloading", CanAct: true,
		ClientLocked: true, Replaced: true, Kind: "show", Episodes: []EpisodeLine{ep}, Covers: "S01E02", Album: "Alb", ClientItem: true, Outside: true, Linked: true,
		DownloaderID: 2, Hash: "abcd", Category: "holyarrs", RequestedBy: "ana", SourceName: "Lab", SourceID: 4, PostedAt: &at, Ratio: &ratio,
		Limits: &ShareLimits{}, ClientHint: "checking", QueuePos: 3, Tracker: "t.example", TrackerProblem: "error", Availability: &avail,
		LiveKey: "2:abcd", LimitDown: 10, LimitUp: 20}
	dlApiGolden(t, "pin_download_row_full.golden.json", full)
	// A bare row: every optional field empty (a NULL-partner row).
	dlApiGolden(t, "pin_download_row_bare.golden.json", DownloadRow{DownloadView: DownloadView{ID: 6, State: "sent"}, UpdatedAt: at})
	dlApiGolden(t, "pin_decision_view.golden.json", DecisionView{SlotID: 11, TitleID: 3, Title: "Salt Choir", Year: &year, Version: "HD", Versions: 2, Why: "ran_out",
		Reason: "The download failed.", ReleaseID: &rel, ReleaseName: "Rel", HasCopy: true, Since: at, Kind: "show", Episode: &ep, Album: "Alb", DownloadID: &dl,
		PackSlotIDs: []int64{11, 12}, Pack: []EpisodeLine{ep}})
	dlApiGolden(t, "pin_decision_view_bare.golden.json", DecisionView{SlotID: 11, TitleID: 3, Title: "T", Version: "HD", Versions: 1, Why: "ran_out", Since: at, Kind: "movie"})
	dlApiGolden(t, "pin_download_totals.golden.json", DownloadTotals{Speed: 1, UpSpeed: 2})
}
