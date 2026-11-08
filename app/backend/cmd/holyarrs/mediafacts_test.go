package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// The title page reads the stored header facts (resolution, audio codec,
// subtitle languages) from each Slot's file; the API passes them through.
func TestTitleViewCarriesHeaderFacts(t *testing.T) {
	t.Parallel()
	srv, s, _ := lbSetup(t)
	var resp struct {
		Title TitleView `json:"title"`
	}
	_ = json.Unmarshal(lbDo(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134"}`, 201), &resp)
	slot := resp.Title.Slots[0]
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		f := &mediafiles.File{SlotIDs: []int64{slot.ID}, VersionID: slot.VersionID, RelativePath: "Dune/Dune.mkv", Size: 10, AddedAt: time.Now(),
			Languages: []string{"en"}, Origin: "imported", PlacedBy: "copy", Role: "main", State: "present",
			MediaInfo: map[string]any{"resolution": "1080p", "width": 1920, "height": 800, "audio_codec": "eac3",
				"audio_channels": "5.1", "subtitle_languages": []string{"en", "fr"}, "runtime_seconds": 9960}}
		if _, err := database.Insert(tx, "files", f, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE slots SET state = 'in_library', file_id = ? WHERE id = ?", f.ID, slot.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var got TitleView
	_ = json.Unmarshal(lbDo(t, "GET", srv.URL+"/api/v1/titles/"+lbItoa(resp.Title.ID), "", 200), &got)
	if len(got.Slots) == 0 || got.Slots[0].File == nil {
		t.Fatalf("no file on the slot: %+v", got.Slots)
	}
	m := got.Slots[0].File.MediaInfo
	subs, _ := m["subtitle_languages"].([]any)
	if m["resolution"] != "1080p" || m["audio_codec"] != "eac3" || m["audio_channels"] != "5.1" || len(subs) != 2 {
		t.Fatalf("facts %+v", m)
	}
}
