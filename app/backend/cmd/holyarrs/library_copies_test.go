package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

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

func TestLibraryCopiesListAndReviewedSearch(t *testing.T) {
	t.Parallel()
	srv, s, _ := lbSetup(t)
	f, v, slot := apiMovieCopy(t, s, false)

	var page mediafiles.LibraryCopyPage
	_ = json.Unmarshal(lbDo(t, "GET", srv.URL+"/api/v1/library/copies?codec=hevc&sort=codec", "", 200), &page)
	if page.Total != 1 || len(page.Copies) != 1 || page.Copies[0].Codec != "hevc" {
		t.Fatalf("page %+v", page)
	}

	body := `{"selections":[{"file_id":` + lbItoa(f.ID) + `,"version_id":` + lbItoa(v.ID) + `}]}`
	var preview mediafiles.LibraryCopySearchPreview
	_ = json.Unmarshal(lbDo(t, "POST", srv.URL+"/api/v1/library/copies/search/preview", body, 200), &preview)
	if preview.ReviewToken == "" || preview.SlotCount != 1 || len(preview.Copies) != 1 {
		t.Fatalf("preview %+v", preview)
	}

	searchBody := `{"selections":[{"file_id":` + lbItoa(f.ID) + `,"version_id":` + lbItoa(v.ID) + `}],"review_token":"` + preview.ReviewToken + `"}`
	var searched mediafiles.LibraryCopySearchResult
	_ = json.Unmarshal(lbDo(t, "POST", srv.URL+"/api/v1/library/copies/search", searchBody, 202), &searched)
	if len(searched.JobIDs) != 1 || len(searched.Items) != 1 || searched.Items[0].SlotIDs[0] != slot.ID {
		t.Fatalf("search %+v", searched)
	}
	var kind, params string
	var subject string
	if err := s.Read().QueryRow("SELECT kind, params, subject FROM jobs WHERE id=?", searched.JobIDs[0]).Scan(&kind, &params, &subject); err != nil {
		t.Fatal(err)
	}
	if kind != "search_slot" || !strings.Contains(params, `"by_hand":true`) || !strings.Contains(params, `"choose":false`) || !strings.Contains(subject, `"id":`+lbItoa(slot.ID)) {
		t.Fatalf("job kind=%s params=%s subject=%s", kind, params, subject)
	}
}

func TestLibraryCopySearchRevalidatesStaleAndProtected(t *testing.T) {
	t.Parallel()
	srv, s, _ := lbSetup(t)
	f, v, _ := apiMovieCopy(t, s, false)
	body := `{"selections":[{"file_id":` + lbItoa(f.ID) + `,"version_id":` + lbItoa(v.ID) + `}]}`
	var preview mediafiles.LibraryCopySearchPreview
	_ = json.Unmarshal(lbDo(t, "POST", srv.URL+"/api/v1/library/copies/search/preview", body, 200), &preview)
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE files SET protected=1 WHERE id=?", f.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	searchBody := `{"selections":[{"file_id":` + lbItoa(f.ID) + `,"version_id":` + lbItoa(v.ID) + `}],"review_token":"` + preview.ReviewToken + `"}`
	lbDo(t, "POST", srv.URL+"/api/v1/library/copies/search", searchBody, 409)
	lbDo(t, "POST", srv.URL+"/api/v1/library/copies/search/preview", body, 409)

	bad := `{"selections":[{"file_id":` + lbItoa(f.ID) + `,"version_id":999999}]}`
	lbDo(t, "POST", srv.URL+"/api/v1/library/copies/search/preview", bad, 409)
	lbDo(t, "POST", srv.URL+"/api/v1/library/copies/search/preview", `{"selections":[]}`, 400)
}
