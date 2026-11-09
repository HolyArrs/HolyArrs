//go:build lab

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Lossless protection on the lab (labclients.py run beta, as the 4.14
// album tests): an album already in the Library as FLAC is never replaced
// by the lab's MP3 320 copy, and the same album held as MP3 320 is upgraded
// to the FLAC copy, downloaded and imported over it.

// haveAlbum puts the three tracks in the Library as files from listing,
// each Slot looking for a better copy.
func (x labAlbum) haveAlbum(t *testing.T, listing, ext string, files []labFile) []int64 {
	t.Helper()
	var ids []int64
	var verID int64
	_ = x.s.Read().QueryRow("SELECT version_id FROM slots WHERE id = ?", x.slots[0]).Scan(&verID)
	err := x.s.Write(context.Background(), func(tx *sql.Tx) error {
		for i, s := range smallSongs {
			rel := fmt.Sprintf("Lab Lanterns/Small Hours (2025)/%02d - %s.%s", i+1, s, ext)
			p := filepath.Join(x.music, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(p), 0o777); err != nil {
				return err
			}
			if err := os.WriteFile(p, files[i].data, 0o666); err != nil {
				return err
			}
			name := listing
			fl := &mediafiles.File{SlotIDs: []int64{x.slots[i]}, VersionID: verID, RelativePath: rel, Size: int64(len(files[i].data)), AddedAt: time.Now(),
				Languages: []string{}, MediaInfo: map[string]any{}, ReleaseName: &name, Origin: "imported", PlacedBy: "copy", Role: "main", State: "present"}
			if _, err := database.Insert(tx, "files", fl, domain.System); err != nil {
				return err
			}
			if _, err := tx.Exec("UPDATE slots SET state = 'better_wanted', file_id = ? WHERE id = ?", fl.ID, x.slots[i]); err != nil {
				return err
			}
			ids = append(ids, fl.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

// albumDone waits for the album searches to end.
func (x labAlbum) albumDone(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		var open int
		_ = x.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ? AND state NOT IN ('done', 'failed', 'dead')", jobs.KindSearchAlbum).Scan(&open)
		var all int
		_ = x.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ?", jobs.KindSearchAlbum).Scan(&all)
		if all > 0 && open == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("album search still running")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (x labAlbum) release(t *testing.T, name string) (state, verdict string) {
	t.Helper()
	_ = x.s.Read().QueryRow("SELECT state, COALESCE(verdict, '') FROM releases WHERE name = ? ORDER BY id DESC LIMIT 1", name).Scan(&state, &verdict)
	return
}

func TestLabBeta43LosslessKept(t *testing.T) {
	x := newLabAlbum(t)
	flac := albumFiles(11, "flac")
	mp3 := albumFiles(71, "mp3")
	fh := serveAlbum(t, "lab-small-hours-flac", smallFLAC, smallFLACHash, flac, false)
	mh := serveAlbum(t, "lab-small-hours-mp3", smallMP3, smallMP3Hash, mp3, true)
	labRemove(t, x.m, labQbit(), fh, mh)
	have := x.haveAlbum(t, smallFLAC, "flac", flac)
	x.albumSearch(t)
	x.albumDone(t)
	for i := 0; i < 3; i++ {
		_, _ = x.m.SyncOnce(context.Background())
		time.Sleep(time.Second)
	}
	st, v := x.release(t, smallMP3)
	if st == "chosen" || !strings.Contains(v, `"rule":"album_gain"`) {
		t.Fatalf("MP3 320 over FLAC: %s %s", st, v)
	}
	if fst, _ := x.release(t, smallFLAC); fst == "chosen" {
		t.Fatal("the same FLAC copy was chosen again")
	}
	if rows := labRows(t, x.srv.URL); len(rows) != 0 {
		t.Fatalf("sent %+v", rows)
	}
	for i, id := range x.slots {
		var fid int64
		_ = x.s.Read().QueryRow("SELECT COALESCE(file_id, 0) FROM slots WHERE id = ?", id).Scan(&fid)
		b, err := os.ReadFile(filepath.Join(x.music, fmt.Sprintf("Lab Lanterns/Small Hours (2025)/%02d - %s.flac", i+1, smallSongs[i])))
		if fid != have[i] || err != nil || string(b) != string(flac[i].data) {
			t.Fatalf("track %d: file %d (want %d), %v", i+1, fid, have[i], err)
		}
	}
	t.Logf("#43: FLAC album kept; MP3 320 %s (match.album_gain), nothing sent, slots %v", st, x.states())
}

func TestLabBeta43LossyUpgradedToLossless(t *testing.T) {
	x := newLabAlbum(t)
	flac := albumFiles(11, "flac")
	mp3 := albumFiles(71, "mp3")
	fh := serveAlbum(t, "lab-small-hours-flac", smallFLAC, smallFLACHash, flac, true)
	mh := serveAlbum(t, "lab-small-hours-mp3", smallMP3, smallMP3Hash, mp3, false)
	labRemove(t, x.m, labQbit(), fh, mh)
	x.haveAlbum(t, smallMP3, "mp3", mp3)
	x.albumSearch(t)
	x.until(t, "the FLAC copy to replace the MP3 copy", func() bool { return x.placed(t, "flac", flac) })
	if st, _ := x.release(t, smallFLAC); st != "chosen" {
		t.Fatalf("FLAC %s", st)
	}
	rows := labRows(t, x.srv.URL)
	if len(rows) != 1 || rows[0].Name != smallFLAC {
		t.Fatalf("sent %+v", rows)
	}
	var replaced int
	_ = x.s.Read().QueryRow("SELECT count(*) FROM files WHERE release_name = ? AND state != 'present'", smallMP3).Scan(&replaced)
	t.Logf("#43: MP3 320 album upgraded to FLAC; %d MP3 files no longer present; slots %v", replaced, x.states())
}

// , the two nets for a person: picking the lab MP3 320 copy by hand over
// an album held as FLAC needs force; forced, it is downloaded, and the
// import still refuses every MP3 over its FLAC copy, which stays.
func TestLabBeta43ForcedLossyRefusedAtImport(t *testing.T) {
	x := newLabAlbum(t)
	flac := albumFiles(11, "flac")
	mp3 := albumFiles(71, "mp3")
	mh := serveAlbum(t, "lab-small-hours-mp3", smallMP3, smallMP3Hash, mp3, true)
	fh := serveAlbum(t, "lab-small-hours-flac", smallFLAC, smallFLACHash, flac, false)
	labRemove(t, x.m, labQbit(), fh, mh)
	x.haveAlbum(t, smallFLAC, "flac", flac)
	x.albumSearch(t)
	x.albumDone(t)
	var id int64
	_ = x.s.Read().QueryRow("SELECT id FROM releases WHERE name = ? ORDER BY id DESC LIMIT 1", smallMP3).Scan(&id)
	body := fmt.Sprintf(`{"slot_id":%d}`, x.slots[0])
	do(t, "POST", x.srv.URL+"/api/v1/releases/"+labItoa(id)+"/choose", body, 409)
	do(t, "POST", x.srv.URL+"/api/v1/releases/"+labItoa(id)+"/choose", fmt.Sprintf(`{"slot_id":%d,"force":true}`, x.slots[0]), 200)
	var plan string
	x.until(t, "the forced MP3 copy's import to end", func() bool {
		var st string
		_ = x.s.Read().QueryRow("SELECT state, COALESCE(plan, '') FROM imports ORDER BY id DESC LIMIT 1").Scan(&st, &plan)
		return st != "" && st != "running" && st != "planned"
	})
	if !strings.Contains(plan, "lower than the lossless copy in place") {
		t.Fatalf("import plan %s", plan)
	}
	for i, s := range smallSongs {
		dir := filepath.Join(x.music, "Lab Lanterns", "Small Hours (2025)")
		b, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("%02d - %s.flac", i+1, s)))
		if err != nil || string(b) != string(flac[i].data) {
			t.Fatalf("FLAC track %d changed: %v", i+1, err)
		}
		if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("%02d - %s.mp3", i+1, s))); err == nil {
			t.Fatalf("MP3 track %d placed over FLAC", i+1)
		}
	}
	t.Logf("#43: hand pick of MP3 320 over FLAC needed force; forced, downloaded, and the import refused it: FLAC kept; slots %v", x.states())
}
