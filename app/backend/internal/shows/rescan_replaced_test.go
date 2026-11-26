package shows

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// writeSizedMP4 writes an MP4 whose video track header says width x
// height; pad bytes after the movie header change its size.
func writeSizedMP4(t *testing.T, file string, width, height, pad int, mod time.Time) {
	t.Helper()
	entry := make([]byte, 86)
	binary.BigEndian.PutUint32(entry[:4], uint32(len(entry)))
	copy(entry[4:8], "hvc1")
	binary.BigEndian.PutUint16(entry[32:34], uint16(width))
	binary.BigEndian.PutUint16(entry[34:36], uint16(height))
	hdlr := append([]byte{0, 0, 0, 0, 0, 0, 0, 0}, []byte("vide")...)
	mdia := append(inspectionMP4Box("mdhd", make([]byte, 24)), inspectionMP4Box("hdlr", append(hdlr, make([]byte, 12)...))...)
	stsd := append([]byte{0, 0, 0, 0, 0, 0, 0, 1}, entry...)
	mdia = append(mdia, inspectionMP4Box("minf", inspectionMP4Box("stbl", inspectionMP4Box("stsd", stsd)))...)
	moov := inspectionMP4Box("moov", bytes.Join([][]byte{
		inspectionMP4Box("trak", inspectionMP4Box("mdia", mdia)),
		inspectionMP4Track("soun", "mp4a", "en", "eng"),
	}, nil))
	b := append(inspectionMP4Box("ftyp", []byte("isom\x00\x00\x00\x00isom")), moov...)
	b = append(b, inspectionMP4Box("free", make([]byte, pad))...)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(file, mod, mod); err != nil {
		t.Fatal(err)
	}
}

// replacedEpisode adopts the lab show with S01E02 as a 1080p file the old
// app recorded as WEBDL-1080p from a 1080p web release, judged by the 4K rules so the 1080p copy is
// not good enough. It returns the file's path and id and the slot's id.
func replacedEpisode(t *testing.T) (*Service, *database.Store, string, int64, int64) {
	t.Helper()
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	showTree(t, lf.Path)
	rel := filepath.ToSlash(filepath.Join("Lab Show (2025)", "Season 1", "Lab Show - S01E02 - Second.mkv"))
	full := filepath.Join(lf.Path, filepath.FromSlash(rel))
	writeSizedMP4(t, full, 1920, 1080, 10, now.Add(-48*time.Hour))
	res, err := svc.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: "Lab Show (2025)", ID: "tmdb:100"}}, domain.System)
	if err != nil || res[0].Error != "" {
		t.Fatalf("adopt: %+v %v", res, err)
	}
	var fid int64
	if err := s.Read().QueryRow("SELECT id FROM files WHERE relative_path = ?", rel).Scan(&fid); err != nil {
		t.Fatal(err)
	}
	ep := episodes(t, s, res[0].TitleID)["S01E02"]
	fourK, _ := json.Marshal(mediafiles.FourK())
	var prID int64
	err = s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE files SET media_info = json_set(media_info, '$.recorded_quality', 'WEBDL-1080p'),
			release_name = 'Lab.Show.S01E02.1080p.WEB-DL.H264-GRP', release_group = 'GRP' WHERE id = ?`, fid); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT pick_rules_id FROM versions WHERE id = (SELECT version_id FROM files WHERE id = ?)", fid).Scan(&prID); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE pick_rules SET body = json(?) WHERE id = ?", string(fourK), prID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// Moving the old app's qualities over reads each file's quality again.
	if _, err := svc.RejudgeAdopted(ctx); err != nil {
		t.Fatal(err)
	}
	// The first rescan only records what adopting left alone.
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	if sl := slotRow(t, s, ep.SlotID); sl.GoodEnough || sl.State != mediafiles.SlotBetterWanted {
		t.Fatalf("before: slot %+v, want a better copy wanted", sl)
	}
	return svc, s, full, fid, ep.SlotID
}

func slotRow(t *testing.T, s *database.Store, id int64) mediafiles.Slot {
	t.Helper()
	var sl mediafiles.Slot
	if err := database.Get(context.Background(), s.Read(), "slots", id, &sl); err != nil {
		t.Fatal(err)
	}
	return sl
}

// TestRescanRereadsReplacedFile: an adopted 1080p episode replaced
// in place by a 2160p file is read again on the next rescan. The quality
// the old app recorded for the old file, and the release it came from, no
// longer describe it and are dropped; the quality follows the new header
// and the episode is judged good enough again.
func TestRescanRereadsReplacedFile(t *testing.T) {
	svc, s, full, fid, slotID := replacedEpisode(t)
	before := fileRow(t, s, fid)
	if before.Quality.Resolution != 1080 {
		t.Fatalf("before: quality %+v", before.Quality)
	}
	writeSizedMP4(t, full, 3840, 2160, 500, now.Add(-time.Hour))
	if _, err := svc.Rescan(context.Background(), domain.System); err != nil {
		t.Fatal(err)
	}
	f := fileRow(t, s, fid)
	fi, _ := os.Stat(full)
	if f.Quality.Resolution != 2160 || f.MediaInfo["resolution"] != "2160p" || f.Size != fi.Size() {
		t.Fatalf("after: quality %+v, resolution %v, size %d (file %d)", f.Quality, f.MediaInfo["resolution"], f.Size, fi.Size())
	}
	if _, kept := f.MediaInfo[mediafiles.RecordedQualityKey]; kept {
		t.Fatalf("the old file's recorded quality was kept: %v", f.MediaInfo)
	}
	if *f.ReleaseName != "Lab Show - S01E02 - Second" || f.Group != nil {
		t.Fatalf("the old file's release was kept: %q group %v", *f.ReleaseName, f.Group)
	}
	if sl := slotRow(t, s, slotID); !sl.GoodEnough || sl.State != mediafiles.SlotInLibrary {
		t.Fatalf("after: slot %+v, want good enough in the Library", sl)
	}
}

// An unchanged file, and a file replaced by one of the same resolution,
// keep the quality the old app recorded and their slot's state.
func TestRescanKeepsFactsOfSameQualityFile(t *testing.T) {
	for _, replace := range []bool{false, true} {
		svc, s, full, fid, slotID := replacedEpisode(t)
		before, beforeSlot := fileRow(t, s, fid), slotRow(t, s, slotID)
		if replace {
			writeSizedMP4(t, full, 1920, 1080, 900, now.Add(-time.Hour))
		}
		if _, err := svc.Rescan(context.Background(), domain.System); err != nil {
			t.Fatal(err)
		}
		f, sl := fileRow(t, s, fid), slotRow(t, s, slotID)
		if f.MediaInfo[mediafiles.RecordedQualityKey] != "WEBDL-1080p" || !reflect.DeepEqual(f.Quality, before.Quality) || *f.ReleaseName != *before.ReleaseName {
			t.Fatalf("replace=%v: file %+v, was %+v", replace, f, before)
		}
		if sl.State != beforeSlot.State || sl.GoodEnough != beforeSlot.GoodEnough {
			t.Fatalf("replace=%v: slot %+v, was %+v", replace, sl, beforeSlot)
		}
		if !replace && (f.Size != before.Size || !reflect.DeepEqual(f.MediaInfo, before.MediaInfo)) {
			t.Fatalf("unchanged file was written: %+v, was %+v", f, before)
		}
	}
}
