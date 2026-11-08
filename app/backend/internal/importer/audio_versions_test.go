package importer

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func audioMP4Box(kind string, payload []byte) []byte {
	box := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(box[:4], uint32(len(box)))
	copy(box[4:8], kind)
	copy(box[8:], payload)
	return box
}

func audioMP4Track(handler, codec, language string) []byte {
	mdhd := make([]byte, 24)
	packed := uint16('u'-0x60)<<10 | uint16('n'-0x60)<<5 | uint16('d'-0x60)
	binary.BigEndian.PutUint16(mdhd[20:22], packed)
	mdia := audioMP4Box("mdhd", mdhd)
	if handler == "soun" && language != "" {
		mdia = append(mdia, audioMP4Box("elng", append([]byte{0, 0, 0, 0}, append([]byte(language), 0)...))...)
	}
	hdlr := append([]byte{0, 0, 0, 0, 0, 0, 0, 0}, []byte(handler)...)
	hdlr = append(hdlr, make([]byte, 12)...)
	mdia = append(mdia, audioMP4Box("hdlr", hdlr)...)
	entry := make([]byte, 16)
	binary.BigEndian.PutUint32(entry[:4], uint32(len(entry)))
	copy(entry[4:8], codec)
	stsd := append([]byte{0, 0, 0, 0, 0, 0, 0, 1}, entry...)
	mdia = append(mdia, audioMP4Box("minf", audioMP4Box("stbl", audioMP4Box("stsd", stsd)))...)
	return audioMP4Box("trak", audioMP4Box("mdia", mdia))
}

func replaceDownloadWithAudioMP4(t *testing.T, w world, base string, languages ...string) {
	t.Helper()
	oldPath := filepath.Join(w.dl, base+".mkv")
	if err := os.Remove(oldPath); err != nil {
		t.Fatal(err)
	}
	tracks := [][]byte{audioMP4Track("vide", "hvc1", "und")}
	for _, language := range languages {
		tracks = append(tracks, audioMP4Track("soun", "mp4a", language))
	}
	data := append(audioMP4Box("ftyp", []byte("isom\x00\x00\x00\x00isom")), audioMP4Box("moov", bytes.Join(tracks, nil))...)
	path := filepath.Join(w.dl, base+".mp4")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE downloads SET files_seen = json_array(json_object('name', ?, 'size', ?)) WHERE id = ?`, base+".mp4", len(data), w.dlID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func setVersionAudio(t *testing.T, w world, languages ...string) {
	t.Helper()
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		v := mediafiles.Version{}
		if err := database.Get(context.Background(), tx, "versions", w.vID, &v); err != nil {
			return err
		}
		v.AudioLanguages = append([]string(nil), languages...)
		return database.Update(tx, "versions", &v, domain.System)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMovieImportRejectsFilenameLanguageWhenInspectedAudioDoesNotMatch(t *testing.T) {
	const base = "Dune.Part.Two.2024.1080p.WEB.H264.FRENCH-HOLYARRS"
	w, releaseID, oldID := upgradeWorld(t, base, base, "Dune.Part.Two.2024.720p.WEB.H264-HOLYARRS")
	replaceDownloadWithAudioMP4(t, w, base, "en")
	setVersionAudio(t, w, "fr")

	oldPath := filepath.Join(w.lib, "Dune - Part Two (2024)", "Dune.Part.Two.2024.720p.WEB.H264-HOLYARRS.mkv")
	before, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "failed" || len(imp.Plan) != 1 || imp.Plan[0].Method != "refuse" ||
		!strings.Contains(strings.Join(imp.Plan[0].Why, " "), "inspected audio") {
		t.Fatalf("import did not refuse the audio mismatch: %+v", imp)
	}
	if after, err := os.ReadFile(oldPath); err != nil || !bytes.Equal(after, before) {
		t.Fatalf("old copy changed: %v", err)
	}

	ctx := context.Background()
	var old mediafiles.File
	if err := database.Get(ctx, w.s.Read(), "files", oldID, &old); err != nil {
		t.Fatal(err)
	}
	slot := slotOf(t, w)
	if old.State != "present" || slot.State != mediafiles.SlotNeedsADecision || slot.FileID == nil || *slot.FileID != oldID {
		t.Fatalf("old copy or Slot changed: file=%+v slot=%+v", old, slot)
	}
	var release domain.Release
	if err := database.Get(ctx, w.s.Read(), "releases", releaseID, &release); err != nil {
		t.Fatal(err)
	}
	if release.State == "turned_down" || release.Verdict["turned_down_reason"] != nil {
		t.Fatalf("copy-specific mismatch globally turned down the Release: %+v", release)
	}
	files, err := database.Select[mediafiles.File](ctx, w.s.Read(), "files", "")
	if err != nil || len(files) != 1 {
		t.Fatalf("audio mismatch placed another file: %+v %v", files, err)
	}
}

func TestMovieImportPersistsInspectedDualAudioOnSelectedVersionOnly(t *testing.T) {
	const base = "Dune.Part.Two.2024.1080p.WEB.H264.FRENCH-HOLYARRS"
	w, _, _ := upgradeWorld(t, base, base, "")
	replaceDownloadWithAudioMP4(t, w, base, "en", "fr")
	setVersionAudio(t, w, "en", "fr")

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
		if err := database.Get(ctx, tx, "slots", w.slotID, &selectedSlot); err != nil {
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
		t.Fatalf("dual-language import: %+v %v", imp, err)
	}
	files, err := database.Select[mediafiles.File](ctx, w.s.Read(), "files", "")
	if err != nil || len(files) != 1 {
		t.Fatalf("files: %+v %v", files, err)
	}
	if files[0].VersionID != w.vID || !reflect.DeepEqual(files[0].Languages, []string{"en", "fr"}) ||
		!reflect.DeepEqual(InspectedLanguages(files[0].MediaInfo), []string{"en", "fr"}) {
		t.Fatalf("persisted languages/version came from the claim instead of inspected tracks: %+v", files[0])
	}
	selected := slotOf(t, w)
	var other mediafiles.Slot
	if err := database.Get(ctx, w.s.Read(), "slots", otherSlotID, &other); err != nil {
		t.Fatal(err)
	}
	if selected.State != mediafiles.SlotInLibrary || selected.FileID == nil || other.State != mediafiles.SlotWanted || other.FileID != nil {
		t.Fatalf("import crossed Version boundaries: selected=%+v other=%+v", selected, other)
	}
}
