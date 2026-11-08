package library

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func inspectionMP4Track(handler, codec, extendedLanguage, packedLanguage string) []byte {
	mdia := inspectionMP4Box("mdhd", func() []byte {
		payload := make([]byte, 24)
		if len(packedLanguage) == 3 {
			packed := uint16(packedLanguage[0]-0x60)<<10 | uint16(packedLanguage[1]-0x60)<<5 | uint16(packedLanguage[2]-0x60)
			binary.BigEndian.PutUint16(payload[20:22], packed)
		}
		return payload
	}())
	if extendedLanguage != "" {
		mdia = append(mdia, inspectionMP4Box("elng", append([]byte{0, 0, 0, 0}, append([]byte(extendedLanguage), 0)...))...)
	}
	hdlr := append([]byte{0, 0, 0, 0, 0, 0, 0, 0}, []byte(handler)...)
	hdlr = append(hdlr, make([]byte, 12)...)
	mdia = append(mdia, inspectionMP4Box("hdlr", hdlr)...)
	entry := make([]byte, 16)
	binary.BigEndian.PutUint32(entry[:4], uint32(len(entry)))
	copy(entry[4:8], codec)
	stsd := append([]byte{0, 0, 0, 0, 0, 0, 0, 1}, entry...)
	mdia = append(mdia, inspectionMP4Box("minf", inspectionMP4Box("stbl", inspectionMP4Box("stsd", stsd)))...)
	return inspectionMP4Box("trak", inspectionMP4Box("mdia", mdia))
}

func inspectionMP4Box(kind string, payload []byte) []byte {
	box := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(box[:4], uint32(len(box)))
	copy(box[4:8], kind)
	copy(box[8:], payload)
	return box
}

func writeInspectionMP4(t *testing.T, file string) {
	t.Helper()
	ftyp := inspectionMP4Box("ftyp", []byte("isom\x00\x00\x00\x00isom"))
	moov := inspectionMP4Box("moov", bytes.Join([][]byte{
		inspectionMP4Track("vide", "hvc1", "", "und"),
		inspectionMP4Track("soun", "mp4a", "fr-FR", "und"),
	}, nil))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, append(ftyp, moov...), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRescanInspectsMovieFilesAndSkipsUnchanged(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	folder := "Dune - Part Two (2024)"
	adoptedRel := filepath.ToSlash(filepath.Join(folder, "Dune.2024.XviD.English.mkv"))
	foundRel := filepath.ToSlash(filepath.Join(folder, "Dune.2024.Extended.XviD.English.mkv"))
	writeInspectionMP4(t, filepath.Join(lf.Path, filepath.FromSlash(adoptedRel)))
	svc := &Service{Store: s, Metadata: adoptTMDB, Now: func() time.Time { return now }}
	res, err := svc.Adopt(context.Background(), lf.ID, []AdoptChoice{{Folder: folder, TMDB: "693134"}}, domain.System)
	if err != nil || res[0].Error != "" {
		t.Fatalf("adopt: %+v %v", res, err)
	}
	var adopted mediafiles.File
	files, _ := database.Select[mediafiles.File](context.Background(), s.Read(), "files", "origin = 'adopted'")
	if len(files) != 1 {
		t.Fatalf("adopted files: %+v", files)
	}
	adopted = files[0]
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE files SET protected = 1, media_info = json(?) WHERE id = ?`,
			`{"edition":"Director's Cut","edition_source":"confirmed","inspection_size":0,"inspection_modified":"0","video_codec":"xvid"}`, adopted.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	writeInspectionMP4(t, filepath.Join(lf.Path, filepath.FromSlash(foundRel)))

	got, err := svc.Rescan(context.Background(), domain.System)
	if err != nil || got.Found != 1 {
		t.Fatalf("rescan: %+v %v", got, err)
	}
	if err := database.Get(context.Background(), s.Read(), "files", adopted.ID, &adopted); err != nil {
		t.Fatal(err)
	}
	if !adopted.Protected || adopted.MediaInfo["video_codec"] != "hevc" || adopted.MediaInfo["edition"] != "Director's Cut" || adopted.MediaInfo["edition_source"] != "confirmed" {
		t.Fatalf("existing file facts/protection: %+v", adopted)
	}
	if fmt.Sprint(adopted.MediaInfo["audio_languages"]) != "[fr]" || adopted.MediaInfo["inspection_size"] == nil || adopted.MediaInfo["inspection_modified"] == nil {
		t.Fatalf("existing inspection facts: %+v", adopted.MediaInfo)
	}
	var found mediafiles.File
	if err := s.Read().QueryRow("SELECT id FROM files WHERE relative_path = ?", foundRel).Scan(&found.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.Get(context.Background(), s.Read(), "files", found.ID, &found); err != nil {
		t.Fatal(err)
	}
	if found.MediaInfo["video_codec"] != "hevc" || found.MediaInfo["edition"] != "Extended" || found.MediaInfo["edition_source"] != "name" {
		t.Fatalf("found movie facts: %+v", found.MediaInfo)
	}

	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`CREATE TABLE inspection_file_updates (id INTEGER)`); err != nil {
			return err
		}
		_, err := tx.Exec(`CREATE TRIGGER count_inspection_file_updates AFTER UPDATE ON files BEGIN INSERT INTO inspection_file_updates(id) VALUES (new.id); END`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rescan(context.Background(), domain.System); err != nil {
		t.Fatal(err)
	}
	var updates int
	if err := s.Read().QueryRow("SELECT count(*) FROM inspection_file_updates").Scan(&updates); err != nil || updates != 0 {
		t.Fatalf("repeat rescan changed %d unchanged File rows: %v", updates, err)
	}
}
