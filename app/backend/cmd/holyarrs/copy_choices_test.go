package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestCopyChoiceRoutesAdoptProtectAndKeepAnother(t *testing.T) {
	t.Parallel()
	srv, s := apiServer(t)
	var added struct {
		Title discover.TitleView `json:"title"`
	}
	if err := json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134"}`, 201), &added); err != nil {
		t.Fatal(err)
	}
	base := srv.URL + "/api/v1/titles/" + itoa(added.Title.ID)
	var v discover.VersionView
	if err := json.Unmarshal(do(t, "POST", base+"/versions", `{"label":"Directors HD","edition":"Directors Cut","folder_name":"Existing Directors Cut"}`, 201), &v); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(v.FolderPath, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(v.FolderPath, "Dune.2024.1080p.mkv")
	if err := os.WriteFile(file, []byte("an uninspected lab video"), 0644); err != nil {
		t.Fatal(err)
	}
	do(t, "POST", base+"/versions/"+itoa(v.ID)+"/adopt", `{"relative_path":"../escape.mkv"}`, 400)
	do(t, "POST", base+"/versions/"+itoa(v.ID)+"/adopt", `{"relative_path":"Existing Directors Cut/Dune.2024.1080p.mkv"}`, 400)
	var f mediafiles.File
	if err := json.Unmarshal(do(t, "POST", base+"/versions/"+itoa(v.ID)+"/adopt", `{"relative_path":"Existing Directors Cut/Dune.2024.1080p.mkv","confirmed_edition":"Directors Cut"}`, 201), &f); err != nil {
		t.Fatal(err)
	}
	if f.MediaInfo["edition"] != "Directors Cut" || f.MediaInfo["inspection_status"] != "unsupported" {
		t.Fatalf("facts %+v", f)
	}
	choice := base + "/files/" + itoa(f.ID)
	do(t, "PUT", choice+"/choices", `{"protected":true}`, 200)
	do(t, "PUT", choice+"/choices", `{"protected":false}`, 400)
	do(t, "PUT", choice+"/choices", `{"protected":false,"acknowledge_protection":true}`, 200)
	do(t, "PUT", choice+"/choices", `{"edition":"Extended"}`, 200)
	do(t, "POST", choice+"/keep-and-add", `{"label":"Another"}`, 400)
	do(t, "POST", choice+"/keep-and-add", `{"label":"Directors 4K","edition":"Directors Cut","pick_rules_id":`+itoa(v.PickRulesID)+`,"library_folder_id":`+itoa(v.LibraryFolderID)+`}`, 201)
	var copies struct {
		Files    []mediafiles.File      `json:"files"`
		Versions []discover.VersionView `json:"versions"`
	}
	if err := json.Unmarshal(do(t, "GET", base+"/copies", "", 200), &copies); err != nil {
		t.Fatal(err)
	}
	if len(copies.Files) != 1 || !copies.Files[0].Protected || len(copies.Versions) != 3 {
		t.Fatalf("copies %+v", copies)
	}
	after, err := os.ReadFile(file)
	if err != nil || string(after) != "an uninspected lab video" {
		t.Fatalf("adoption touched bytes %v", err)
	}
	var count int
	if err := s.Read().QueryRow("SELECT count(*) FROM files WHERE protected=1").Scan(&count); err != nil || count != 1 {
		t.Fatalf("protection %d %v", count, err)
	}
}

func TestDownloadEditionConfirmationRetriesOnlyCurrentWaitingMovie(t *testing.T) {
	t.Parallel()
	srv, s := apiServer(t)
	var added struct {
		Title discover.TitleView `json:"title"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134"}`, 201), &added)
	sid := added.Title.Slots[0].ID
	var did int64
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		dr := domain.Downloader{Adapter: "sabnzbd", Name: "Lab", Protocol: "usenet", URL: "http://lab.invalid", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", &dr, domain.System); err != nil {
			return err
		}
		d := domain.Download{DownloaderID: dr.ID, ExternalID: "lab-copy", SlotIDs: []int64{sid}, State: "finished", FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		var err error
		did, err = database.Insert(tx, "downloads", &d, domain.System)
		if err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE slots SET state='needs_a_decision',active_download_id=? WHERE id=?", did, sid)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	url := srv.URL + "/api/v1/downloads/" + itoa(did) + "/edition"
	do(t, "PUT", url, `{"edition":"Theatrical"}`, 400)
	do(t, "PUT", url, `{"edition":"Theatrical","confirm":true,"retry":true}`, 200)
	var state string
	_ = s.Read().QueryRow("SELECT state FROM slots WHERE id=?", sid).Scan(&state)
	if state != "importing" {
		t.Fatalf("retry state %s", state)
	}
	do(t, "PUT", url, `{"edition":"Theatrical","confirm":true,"retry":true}`, 400)
}
