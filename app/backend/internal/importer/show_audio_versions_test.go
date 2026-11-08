package importer

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func replaceShowDownloadWithAudioMP4(t *testing.T, w showWorld, name string, languages ...string) {
	t.Helper()
	oldPath := filepath.Join(w.dl, packName, filepath.FromSlash(name))
	if err := os.Remove(oldPath); err != nil {
		t.Fatal(err)
	}
	tracks := [][]byte{audioMP4Track("vide", "hvc1", "und")}
	for _, language := range languages {
		tracks = append(tracks, audioMP4Track("soun", "mp4a", language))
	}
	data := append(audioMP4Box("ftyp", []byte("isom\x00\x00\x00\x00isom")), audioMP4Box("moov", bytes.Join(tracks, nil))...)
	newName := strings.TrimSuffix(name, filepath.Ext(name)) + ".mp4"
	if err := os.WriteFile(filepath.Join(w.dl, packName, filepath.FromSlash(newName)), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE downloads SET files_seen = json_array(json_object('name', ?, 'size', ?)) WHERE id = ?`,
			packName+"/"+newName, len(data), w.dlID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func setShowVersionAudio(t *testing.T, w showWorld, languages ...string) {
	t.Helper()
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		var version mediafiles.Version
		if err := database.Get(context.Background(), tx, "versions", w.vID, &version); err != nil {
			return err
		}
		version.AudioLanguages = append([]string(nil), languages...)
		return database.Update(tx, "versions", &version, domain.System)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestShowImportRejectsFilenameLanguageWhenInspectedAudioDoesNotMatch(t *testing.T) {
	name := strings.Replace(ep(1), "-HOLYARRS.mkv", ".FRENCH-HOLYARRS.mkv", 1)
	w := showSetup(t, []int{1}, name)
	replaceShowDownloadWithAudioMP4(t, w, name, "en")
	setShowVersionAudio(t, w, "fr")

	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "failed" || len(imp.Plan) != 1 || imp.Plan[0].Method != "refuse" ||
		!strings.Contains(strings.Join(imp.Plan[0].Why, " "), "inspected audio") {
		t.Fatalf("show import did not refuse the audio mismatch: %+v", imp)
	}
	if slot := w.slot(t, 1); slot.State != mediafiles.SlotNeedsADecision || slot.FileID != nil {
		t.Fatalf("show Slot did not wait for a decision: %+v", slot)
	}
	files, err := database.Select[mediafiles.File](context.Background(), w.s.Read(), "files", "")
	if err != nil || len(files) != 0 {
		t.Fatalf("audio mismatch placed files: %+v %v", files, err)
	}
}

func TestShowImportPersistsInspectedDualAudioOnSelectedVersionOnly(t *testing.T) {
	w := showSetup(t, []int{1}, ep(1))
	replaceShowDownloadWithAudioMP4(t, w, ep(1), "en", "fr")
	setShowVersionAudio(t, w, "en", "fr")

	ctx := context.Background()
	var otherSlotID int64
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		var selected mediafiles.Version
		if err := database.Get(ctx, tx, "versions", w.vID, &selected); err != nil {
			return err
		}
		other := &mediafiles.Version{TitleID: selected.TitleID, Label: "French dub", PickRulesID: selected.PickRulesID,
			LibraryFolderID: selected.LibraryFolderID, FolderName: selected.FolderName + " [French]", AudioLanguages: []string{"fr"}}
		if _, err := database.Insert(tx, "versions", other, domain.System); err != nil {
			return err
		}
		var selectedSlot mediafiles.Slot
		if err := database.Get(ctx, tx, "slots", w.slots[1], &selectedSlot); err != nil {
			return err
		}
		otherSlot := &mediafiles.Slot{ItemID: selectedSlot.ItemID, VersionID: other.ID, State: mediafiles.SlotWanted}
		if _, err := database.Insert(tx, "slots", otherSlot, domain.System); err != nil {
			return err
		}
		otherSlotID = otherSlot.ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	imp, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("dual-language show import: %+v %v", imp, err)
	}
	files, err := database.Select[mediafiles.File](ctx, w.s.Read(), "files", "")
	if err != nil || len(files) != 1 {
		t.Fatalf("files: %+v %v", files, err)
	}
	if files[0].VersionID != w.vID || !reflect.DeepEqual(files[0].Languages, []string{"en", "fr"}) ||
		!reflect.DeepEqual(InspectedLanguages(files[0].MediaInfo), []string{"en", "fr"}) {
		t.Fatalf("show persisted claimed audio instead of inspected tracks: %+v", files[0])
	}
	selected := w.slot(t, 1)
	var other mediafiles.Slot
	if err := database.Get(ctx, w.s.Read(), "slots", otherSlotID, &other); err != nil {
		t.Fatal(err)
	}
	if selected.State != mediafiles.SlotBetterWanted || selected.FileID == nil || other.State != mediafiles.SlotWanted || other.FileID != nil {
		t.Fatalf("show import crossed Version boundaries: selected=%+v other=%+v", selected, other)
	}
}
