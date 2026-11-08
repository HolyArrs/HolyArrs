//go:build labms

package mediaservers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

const labRelease = "HolyArrs.Lab.Signal.2026.1080p.WEB.H264-HOLYARRS"

type jellyItem struct {
	ID   string `json:"Id"`
	Name string `json:"Name"`
	Path string `json:"Path"`
}

func labItems(t *testing.T, base, token string) []jellyItem {
	t.Helper()
	u := base + "/Items?Recursive=true&IncludeItemTypes=Movie&Fields=Path&SearchTerm=" + url.QueryEscape("HolyArrs Lab Signal")
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", fmt.Sprintf(`MediaBrowser Client="HolyArrs Lab", Device="Disposable Proof", DeviceId="holyarrs-mslab-20261015", Version="1", Token=%q`, token))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Jellyfin Items answered %d", resp.StatusCode)
	}
	var out struct {
		Items []jellyItem `json:"Items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Items
}

func waitItems(t *testing.T, base, token string, want int, pathPart string) []jellyItem {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for {
		items := labItems(t, base, token)
		if len(items) == want {
			if want == 0 || strings.Contains(items[0].Path, pathPart) {
				return items
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("Jellyfin items did not become count=%d path containing %q; got %+v", want, pathPart, items)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func waitJob(t *testing.T, s *database.Store, id int64) domain.Job {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		var j domain.Job
		if err := database.Get(context.Background(), s.Read(), "jobs", id, &j); err != nil {
			t.Fatal(err)
		}
		if j.State == "done" {
			return j
		}
		if j.State == "failed" || time.Now().After(deadline) {
			t.Fatalf("job %d ended as %s: %s", id, j.State, j.Error)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestLiveJellyfinImporterRenameDelete(t *testing.T) {
	root := os.Getenv("HOLYARRS_MSLAB_ROOT")
	base := os.Getenv("HOLYARRS_MSLAB_URL")
	tokenFile := os.Getenv("HOLYARRS_MSLAB_TOKEN_FILE")
	if root == "" || base == "" || tokenFile == "" {
		t.Fatal("HOLYARRS_MSLAB_ROOT, HOLYARRS_MSLAB_URL and HOLYARRS_MSLAB_TOKEN_FILE are required")
	}
	tokenBytes, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.TrimSpace(strings.TrimPrefix(string(tokenBytes), "\ufeff"))
	if token == "" {
		t.Fatal("empty Jellyfin API key")
	}
	ctx := context.Background()
	dbDir := filepath.Join(root, "holyarrs-db")
	s, err := database.Open(dbDir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	vault, err := secrets.Open(ctx, s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	libraryRoot := filepath.Join(root, "media", "movies")
	downloadRoot := filepath.Join(root, "downloads")
	sourceFile := filepath.Join(downloadRoot, labRelease+".mp4")
	stat, err := os.Stat(sourceFile)
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	m, err := Add(ctx, s, vault, Input{Name: "Disposable Jellyfin 12", Kind: "jellyfin", URL: base, Token: token,
		PathFrom: libraryRoot, PathTo: "/media/movies", AllowLocal: &yes}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	link := &Linker{Store: s, Vault: vault, Delay: 100 * time.Millisecond}
	found, err := link.Test(ctx, m)
	if err != nil || !strings.Contains(found, "12.2.0") {
		t.Fatalf("Test button behavior: %q %v", found, err)
	}
	link.Watch(ctx)
	if got := waitItems(t, base, token, 0, ""); len(got) != 0 {
		t.Fatalf("library was not empty before import: %+v", got)
	}

	var titleID, downloadID int64
	err = s.Write(ctx, func(tx *sql.Tx) error {
		var pickRulesID int64
		if err := tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pickRulesID); err != nil {
			return err
		}
		year := 2026
		folder := &mediafiles.LibraryFolder{Path: libraryRoot, Kind: "movie", Label: "HolyArrs Lab", Health: "ok", SameDiskAs: []int64{}}
		title := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "HolyArrs Lab Signal", SortName: "holyarrs lab signal", Year: &year,
			Aliases: []string{}, SourceIDs: map[string]string{}, MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released",
			MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active"}
		if _, err := database.Insert(tx, "library_folders", folder, domain.System); err != nil {
			return err
		}
		if _, err := database.Insert(tx, "titles", title, domain.System); err != nil {
			return err
		}
		item := &mediafiles.Item{TitleID: title.ID, Kind: "movie"}
		if _, err := database.Insert(tx, "items", item, domain.System); err != nil {
			return err
		}
		version := &mediafiles.Version{TitleID: title.ID, Label: "HD", PickRulesID: pickRulesID, LibraryFolderID: folder.ID,
			FolderName: importer.FolderName(title.Name, year), IsMain: true}
		if _, err := database.Insert(tx, "versions", version, domain.System); err != nil {
			return err
		}
		slot := &mediafiles.Slot{ItemID: item.ID, VersionID: version.ID, State: mediafiles.SlotDownloading}
		if _, err := database.Insert(tx, "slots", slot, domain.System); err != nil {
			return err
		}
		downloader := &domain.Downloader{Name: "Disposable", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://lab.invalid",
			Category: "holyarrs", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", downloader, domain.System); err != nil {
			return err
		}
		download := &domain.Download{DownloaderID: downloader.ID, SlotIDs: []int64{slot.ID}, ExternalID: "media-server-lab",
			Progress: 1, State: "finished", FilesSeen: []domain.SeenFile{{Name: filepath.Base(sourceFile), Size: stat.Size()}},
			SavePath: downloadRoot, StartedBy: domain.System}
		if _, err := database.Insert(tx, "downloads", download, domain.System); err != nil {
			return err
		}
		titleID, downloadID = title.ID, download.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	imp, err := (&importer.Importer{Store: s, NoSizeFloors: true}).Import(ctx, downloadID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("import state %s: %v", imp.State, err)
	}
	link.Wait()
	imported := waitItems(t, base, token, 1, "HolyArrs Lab Signal (2026)")
	t.Logf("import visible: name=%q path=%q", imported[0].Name, imported[0].Path)

	settings := mediafiles.NamingDefaults()
	settings.FilePattern = "{title} ({year}) [{quality}] {group} renamed"
	if err := mediafiles.PutNamingSettings(ctx, s, settings, domain.System); err != nil {
		t.Fatal(err)
	}
	plan, hash, err := library.PlanRenameHashed(ctx, s.Read(), []int64{titleID})
	if err != nil || len(plan) != 1 || plan[0].From == plan[0].To {
		t.Fatalf("rename plan %+v: %v", plan, err)
	}
	queue := jobs.New(s, jobs.Options{Workers: 1, PollInterval: 10 * time.Millisecond, BaseBackoff: 10 * time.Millisecond})
	service := &library.Service{Store: s, Queue: queue}
	service.Register(queue)
	if err := queue.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer queue.Stop()
	jobID, err := service.Rename(ctx, []int64{titleID}, nil, hash, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	waitJob(t, s, jobID)
	link.Wait()
	renamed := waitItems(t, base, token, 1, "renamed.mp4")
	t.Logf("rename visible: name=%q path=%q", renamed[0].Name, renamed[0].Path)

	removed, err := service.Remove(ctx, titleID, library.RemoveChoice{DeleteFiles: true}, true, domain.System)
	if err != nil || !removed.Applied || len(removed.Held) != 1 {
		t.Fatalf("remove applied=%v held=%d: %v", removed.Applied, len(removed.Held), err)
	}
	link.Wait()
	waitItems(t, base, token, 0, "")
	t.Log("delete visible: matching Jellyfin item count=0")

	events, _, err := s.Events(ctx, database.EventFilter{Kinds: []string{domain.EventMediaRefreshed}})
	if err != nil || len(events) != 3 {
		t.Fatalf("media refresh events=%d: %v", len(events), err)
	}
	changes := make([]string, 0, len(events))
	for _, event := range events {
		changes = append(changes, fmt.Sprint(event.Data["change"]))
	}
	t.Logf("media refresh events=%d changes=%v", len(events), changes)
}
