package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate"
)

// Helpers other api tests still call; their own tests moved to cmd/holyarrs.

// addTitleFor stores a bare movie and returns its id.
func addTitleFor(t *testing.T, s *database.Store, name string) int64 {
	t.Helper()
	var id int64
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: name, SortName: strings.ToLower(name), MetadataSource: "local", NewPartsPolicy: "all",
			ReleaseGate: "released", WhyAdded: "by_hand", Status: "released", State: "active", SourceIDs: map[string]string{}, Aliases: []string{},
			MusicTypes: []string{}, Artwork: []string{}}
		var err error
		id, err = database.Insert(tx, "titles", ti, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func apiMovieCopy(t *testing.T, s *database.Store, protected bool) (mediafiles.File, mediafiles.Version, mediafiles.Slot) {
	t.Helper()
	var f mediafiles.File
	var v mediafiles.Version
	var slot mediafiles.Slot
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		var folderID int64
		if err := tx.QueryRow("SELECT id FROM library_folders ORDER BY id LIMIT 1").Scan(&folderID); err != nil {
			return err
		}
		rules := &mediafiles.PickRules{Name: "Copy search", Kinds: []string{"movie"}, Revision: 1, Body: mediafiles.PickRulesBody{}}
		if _, err := database.Insert(tx, "pick_rules", rules, domain.System); err != nil {
			return err
		}
		title := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "Arrival", SortName: "arrival", MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", WhyAdded: "by_hand", Status: "released", State: "active", SourceIDs: map[string]string{}, Aliases: []string{}, MusicTypes: []string{}, Artwork: []string{}}
		if _, err := database.Insert(tx, "titles", title, domain.System); err != nil {
			return err
		}
		item := &mediafiles.Item{TitleID: title.ID, Kind: "movie", Numbers: mediafiles.Numbers{}, KeepUpToDate: true, SourceIDs: map[string]string{}}
		if _, err := database.Insert(tx, "items", item, domain.System); err != nil {
			return err
		}
		v = mediafiles.Version{TitleID: title.ID, Label: "4K", PickRulesID: rules.ID, LibraryFolderID: folderID, FolderName: "Arrival", KeepUpToDate: true, IsMain: true, AudioLanguages: []string{"en"}}
		if _, err := database.Insert(tx, "versions", &v, domain.System); err != nil {
			return err
		}
		f = mediafiles.File{Protected: protected, SlotIDs: []int64{}, VersionID: v.ID, RelativePath: "Arrival/Arrival.mkv", Size: 10, AddedAt: time.Now().UTC(), Quality: mediafiles.Quality{Codec: "av1"}, Languages: []string{}, MediaInfo: map[string]any{"inspection_status": "complete", "container": "matroska", "video_codec": "hevc", "audio_languages": []string{"en"}, "audio_tracks": 1}, Origin: "imported", PlacedBy: "move", Role: "main", State: "present"}
		if _, err := database.Insert(tx, "files", &f, domain.System); err != nil {
			return err
		}
		slot = mediafiles.Slot{ItemID: item.ID, VersionID: v.ID, State: mediafiles.SlotInLibrary, FileID: &f.ID}
		if _, err := database.Insert(tx, "slots", &slot, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE files SET slot_ids=json_array(?) WHERE id=?", slot.ID, f.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return f, v, slot
}

// moveSetup serves the API with a running move Job and one kept Radarr
// snapshot whose movie folder is on this machine under another path.
func moveSetup(t *testing.T, safe bool) (*httptest.Server, *database.Store, int64, string) {
	t.Helper()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	films := filepath.Join(t.TempDir(), "films")
	if err := os.MkdirAll(filepath.Join(films, "Dune Part Two (2024)"), 0o755); err != nil {
		t.Fatal(err)
	}
	q := jobs.New(st, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond})
	lib := &library.Service{Store: st, Queue: q, Metadata: metadata.Lab()}
	mv := &migrate.Mover{Store: st, Library: lib, Queue: q}
	mv.Register(q)
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	srv := httptest.NewServer(New(Server{Store: st, Queue: q, Library: lib, Metadata: metadata.Lab(), Version: "test", Mover: mv, SafeMode: safe}).Handler())
	t.Cleanup(srv.Close)
	snap := &migrate.Snapshot{Kind: migrate.Radarr, Via: migrate.ViaBackup, Version: "6.4.4",
		Folders: []migrate.Folder{{Path: "/movies"}},
		Titles: []migrate.Title{{SourceID: 1, Kind: "movie", Name: "Dune: Part Two", Year: 2024, IDs: map[string]string{"tmdb": "693134"},
			Path: "/movies/Dune Part Two (2024)", Monitored: true}},
		Tags:         []migrate.Tag{{ID: 1, Label: "kids"}},
		Indexers:     []migrate.Provider{{Name: "Nyaa", Kind: "Torznab", Enabled: true, Settings: map[string]any{"baseUrl": "http://prowlarr:9696/1/"}, NeedsReentry: []string{"apiKey"}}},
		NeedsReentry: []string{`indexer "Nyaa": apiKey`},
	}
	saved, err := migrate.Save(context.Background(), st, 1, snap)
	if err != nil {
		t.Fatal(err)
	}
	return srv, st, saved.ID, filepath.ToSlash(films)
}

// organiseWorld adds a movie Library folder and Dune with one imported file.
func organiseWorld(t *testing.T) (url string, s *database.Store, lib string, titleID, fileID int64) {
	t.Helper()
	srv, s, _ := libSetup(t)
	lib = t.TempDir()
	do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie"}`, lib), 201)
	var added struct {
		Title TitleView `json:"title"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134"}`, 201), &added)
	titleID = added.Title.ID
	rel := "Dune - Part Two (2024)/Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS.mkv"
	if err := os.MkdirAll(filepath.Join(lib, "Dune - Part Two (2024)"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, filepath.FromSlash(rel)), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	release := "Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS"
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		var vid, sid int64
		if err := tx.QueryRow("SELECT v.id, s.id FROM versions v JOIN slots s ON s.version_id = v.id WHERE v.title_id = ?", titleID).Scan(&vid, &sid); err != nil {
			return err
		}
		f := &mediafiles.File{SlotIDs: []int64{sid}, VersionID: vid, RelativePath: rel, Size: 5, AddedAt: time.Now(), Languages: []string{},
			MediaInfo: map[string]any{}, ReleaseName: &release, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		id, err := database.Insert(tx, "files", f, domain.System)
		fileID = id
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv.URL, s, lib, titleID, fileID
}

func waitJob(t *testing.T, url string, id int64) domain.Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var j domain.Job
		_ = json.Unmarshal(do(t, "GET", url+"/api/v1/jobs/"+itoa(id), "", 200), &j)
		if j.State == "done" || j.State == "failed" || time.Now().After(deadline) {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
}
