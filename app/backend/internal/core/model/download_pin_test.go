package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Golden pins for the download records (R4): the JSON they marshal to and
// the field order with db tags, frozen before they move to the downloads
// module. Run with PIN_UPDATE_GOLDEN=1 to rewrite a golden on purpose.

func dlPinGolden(t *testing.T, name string, v any) {
	t.Helper()
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", name)
	if os.Getenv("PIN_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
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

// dlPinFields lists a struct's fields in order with their db and json tags.
func dlPinFields(v any) string {
	var b strings.Builder
	var walk func(rt reflect.Type)
	walk = func(rt reflect.Type) {
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			if f.Anonymous {
				fmt.Fprintf(&b, "embed %s\n", f.Type.Name())
				continue
			}
			fmt.Fprintf(&b, "%s %s db=%q json=%q\n", f.Name, f.Type, f.Tag.Get("db"), f.Tag.Get("json"))
		}
	}
	walk(reflect.TypeOf(v))
	return b.String()
}

func TestDlPinDownloadRecordGolden(t *testing.T) {
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	rel, eta := int64(7), int64(90)
	reason := "the client dropped it"
	meta := Meta{ID: 5, CreatedAt: at, CreatedBy: Actor{Kind: "user", ID: 2}, UpdatedAt: at, UpdatedBy: System}
	dlPinGolden(t, "pin_download.golden.json", Download{Meta: meta, DownloaderID: 3, ReleaseID: &rel, SlotIDs: []int64{11, 12}, ExternalID: "abcd", Progress: 0.5,
		Speed: 100, UpSpeed: 5, Seeds: 4, Peers: 2, ETA: &eta, State: "failed", StateReason: &reason,
		FilesSeen: []SeenFile{{Name: "a.mkv", Size: 1234}}, SavePath: "/data/x", StartedAt: &at, FinishedAt: &at, StartedBy: System, ProgressAt: &at, ConfirmedEdition: "Director's Cut"})
	dlPinGolden(t, "pin_download_nulls.golden.json", Download{Meta: meta, SlotIDs: []int64{}, FilesSeen: []SeenFile{}, StartedBy: System})
	ratio, mins := 1.5, 30
	sec := int64(9)
	dlPinGolden(t, "pin_downloader.golden.json", Downloader{Meta: meta, Name: "Q", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://q:8080", SecretRef: &sec,
		Category: "holyarrs", Priority: 1, PathMap: []PathMap{{Theirs: "/downloads", Ours: "/data/downloads"}}, AfterImport: "leave",
		SeedGoal: SeedGoal{Ratio: &ratio, Minutes: &mins}, State: "ok", Username: "admin", AllowLocal: true, BackoffUntil: &at, FailCount: 2, LastError: "boom",
		CheckedAt: &at, Removed: false, Writable: true, ClearFinished: true, Tags: []string{"4k"}})
	dlPinGolden(t, "pin_downloader_nulls.golden.json", Downloader{Meta: meta, PathMap: []PathMap{}, Tags: []string{}})
}

func TestDlPinDownloadRecordFields(t *testing.T) {
	got := dlPinFields(Download{}) + "--\n" + dlPinFields(Downloader{}) + "--\n" + dlPinFields(SeenFile{}) + "--\n" + dlPinFields(PathMap{}) + "--\n" + dlPinFields(SeedGoal{})
	path := filepath.Join("testdata", "pin_download_fields.golden.txt")
	if os.Getenv("PIN_UPDATE_GOLDEN") == "1" {
		_ = os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != got {
		t.Fatalf("download record fields changed\n got:\n%s\nwant:\n%s", got, want)
	}
}
