package importer

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestTrackFileCutsLongNames(t *testing.T) {
	long := strings.Repeat("é", 200)
	at := AlbumTarget{ArtistFolder: "A", Album: long, Year: 2020}
	p := at.trackFile(Track{Disc: 1, Number: 1, Name: long}, ".FLAC", 1, "")
	parts := strings.Split(p, "/")
	if len(parts) != 3 {
		t.Fatalf("path %q", p)
	}
	for _, c := range parts[1:] {
		if len(c) > maxNameBytes || !utf8.ValidString(c) {
			t.Fatalf("component of %d bytes (valid %v): %q", len(c), utf8.ValidString(c), c)
		}
	}
	if !strings.HasSuffix(parts[1], " (2020)") {
		t.Fatalf("album folder lost its year: %q", parts[1])
	}
	if !strings.HasSuffix(parts[2], ".flac") || !strings.HasPrefix(parts[2], "01 - ") {
		t.Fatalf("track file %q", parts[2])
	}
}

func TestTrackFileUnknownAlbum(t *testing.T) {
	for _, name := range []string{"..", "...", "", " . "} {
		at := AlbumTarget{ArtistFolder: "A", Album: name, Year: 2020}
		if p := at.trackFile(Track{Disc: 1, Number: 2, Name: "Song"}, ".mp3", 1, ""); p != "A/Unknown album (2020)/02 - Song.mp3" {
			t.Errorf("album %q: %q", name, p)
		}
	}
	at := AlbumTarget{ArtistFolder: "A", Album: ".."}
	if p := at.trackFile(Track{Disc: 1, Number: 2, Name: "Song"}, ".mp3", 1, ""); p != "A/Unknown album/02 - Song.mp3" {
		t.Errorf("no year: %q", p)
	}
}

func TestPlanTracksSameNameOnTwoDiscsNeedsADecision(t *testing.T) {
	at := AlbumTarget{LibraryPath: "/lib", ArtistFolder: "A", Album: "B", Tracks: []Track{
		{ItemID: 1, Disc: 1, Number: 1, Name: "Intro", SlotID: 11, Wanted: true},
		{ItemID: 2, Disc: 1, Number: 2, Name: "Dawn", SlotID: 12, Wanted: true},
		{ItemID: 3, Disc: 2, Number: 1, Name: "Intro", SlotID: 13, Wanted: true},
	}}
	plan := PlanTracks([]Candidate{{Path: "/dl/Intro.flac", Size: 10}, {Path: "/dl/Dawn.flac", Size: 10}}, at)
	if plan[0].Method == "hardlink" {
		t.Fatalf("a shared name was given to one track: %+v", plan[0])
	}
	if len(plan[0].TargetSlotIDs) != 2 || !strings.Contains(strings.Join(plan[0].Why, " "), "more than one track") {
		t.Fatalf("the step does not name both tracks: %+v", plan[0])
	}
	if plan[1].Method != "hardlink" {
		t.Fatalf("a unique name was not placed: %+v", plan[1])
	}
}

func TestAlbumImportRefusesTrackWithoutAlbum(t *testing.T) {
	w := albumSetup(t, 1, "01 - Dawn.flac", "02 - Glass River.flac", "03 - Night Train.flac")
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE items SET part_id = NULL")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System); err == nil || !strings.Contains(err.Error(), "no album") {
		t.Fatalf("a track without an album was imported: %v", err)
	}
	if exists(filepath.Join(w.lib, "Lab Band")) {
		t.Fatal("files were placed")
	}
	for i := range w.slots {
		if st := w.state(t, i); st != mediafiles.SlotDownloading {
			t.Fatalf("track %d moved to %s", i+1, st)
		}
	}
}

func TestStrandedSweepBacksOffAfterFailedImports(t *testing.T) {
	w := albumSetup(t, 1, "01 - Dawn.flac")
	im := &Importer{Store: w.s}
	sweep := func() int {
		n, err := im.RequeueStranded(context.Background(), func(context.Context, int64) {})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	failed := func(ago time.Duration) {
		t.Helper()
		end := time.Now().Add(-ago)
		if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
			_, err := database.Insert(tx, "jobs", &domain.Job{Kind: KindImport, Subject: &domain.Subject{Type: "download", ID: w.dlID}, Params: map[string]any{},
				Priority: "normal", State: "failed", MaxAttempts: 3, Attempt: 3, RunAfter: end, EndedAt: &end, AskedBy: domain.System}, domain.System)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if sweep() != 1 {
		t.Fatal("never tried: not queued")
	}
	failed(time.Minute)
	if sweep() != 0 {
		t.Fatal("queued again a minute after a failure")
	}
	failed(15 * time.Minute)
	if sweep() != 0 {
		t.Fatal("queued again right after the second failure")
	}
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE jobs SET ended_at = ? WHERE kind = ?", database.FormatTime(time.Now().Add(-25*time.Minute)), KindImport)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if sweep() != 1 {
		t.Fatal("two failures 25 minutes ago (wait 20 minutes): not queued")
	}
	for i := 0; i < 20; i++ {
		failed(23 * time.Hour)
	}
	if sweep() != 0 {
		t.Fatal("many failures: the wait is at most a day, and a day has not passed")
	}
}

func TestAlbumTrackUpgradeSurvivesCrash(t *testing.T) {
	for _, stage := range []string{"held", "placed", "held_late"} {
		t.Run(stage, func(t *testing.T) {
			w := albumSetup(t, 1, "01 - Dawn.flac", "02 - Glass River.flac", "03 - Night Train.flac")
			ctx := context.Background()
			rp := "Lab Band/First Light (2020)/01 - Dawn.mp3"
			if stage == "held" {
				rp = "Lab Band/First Light (2020)/01 - Dawn.flac"
			}
			write(t, filepath.Join(w.lib, filepath.FromSlash(rp)), 100)
			var oldID int64
			if err := w.s.Write(ctx, func(tx *sql.Tx) error {
				var vID int64
				if err := tx.QueryRow("SELECT version_id FROM slots WHERE id = ?", w.slots[0]).Scan(&vID); err != nil {
					return err
				}
				old := "Lab Band - First Light (2020) [MP3 V0]"
				f := &mediafiles.File{SlotIDs: []int64{w.slots[0]}, VersionID: vID, RelativePath: rp, Size: 100, AddedAt: time.Now().Add(-time.Hour),
					Languages: []string{}, MediaInfo: map[string]any{}, ReleaseName: &old, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
				var err error
				if oldID, err = database.Insert(tx, "files", f, domain.System); err != nil {
					return err
				}
				_, err = tx.Exec("UPDATE slots SET file_id = ? WHERE id = ?", oldID, w.slots[0])
				return err
			}); err != nil {
				t.Fatal(err)
			}
			crashAt = func(s string) error {
				if s == stage {
					return os.ErrDeadlineExceeded
				}
				return nil
			}
			_, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
			crashAt = nil
			if err == nil {
				t.Fatal("no crash")
			}
			imp, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
			if err != nil || imp.State != "done" {
				t.Fatalf("restart %+v %v", imp, err)
			}
			for i, f := range []string{"01 - Dawn.flac", "02 - Glass River.flac", "03 - Night Train.flac"} {
				if fi, err := os.Stat(filepath.Join(w.lib, "Lab Band", "First Light (2020)", f)); err != nil || fi.Size() != 2048 {
					t.Fatalf("%s not in place: %v", f, err)
				}
				if st := w.state(t, i); st != mediafiles.SlotInLibrary {
					t.Fatalf("track %d: %s", i+1, st)
				}
			}
			held, _ := filepath.Glob(filepath.Join(w.lib, HoldingDir, "*", "Lab Band", "First Light (2020)", "*"))
			if len(held) != 1 {
				t.Fatalf("held %v", held)
			}
			if fi, _ := os.Stat(held[0]); fi == nil || fi.Size() != 100 {
				t.Fatal("the held copy is not the old one")
			}
			var old mediafiles.File
			_ = database.Get(ctx, w.s.Read(), "files", oldID, &old)
			rel, _ := filepath.Rel(w.lib, held[0])
			if old.State != "replaced" || old.RelativePath != filepath.ToSlash(rel) {
				t.Fatalf("old %s %q, held at %s", old.State, old.RelativePath, rel)
			}
			var n int
			_ = w.s.Read().QueryRow("SELECT count(*) FROM files WHERE state = 'present'").Scan(&n)
			if n != 3 {
				t.Fatalf("%d present files", n)
			}
		})
	}
}

func TestHardlinkedAlbumSurvivesDeletedDownload(t *testing.T) {
	w := albumSetup(t, 1, "01 - Dawn.flac", "02 - Glass River.flac", "03 - Night Train.flac")
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("%v %+v", err, imp)
	}
	var links int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM files WHERE placed_by = 'hardlink'").Scan(&links)
	if links != 3 {
		t.Skipf("tracks were not hardlinked here (%d)", links)
	}
	if err := os.RemoveAll(filepath.Join(w.dl, albumRel)); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"01 - Dawn.flac", "02 - Glass River.flac", "03 - Night Train.flac"} {
		if fi, err := os.Stat(filepath.Join(w.lib, "Lab Band", "First Light (2020)", f)); err != nil || fi.Size() != 2048 {
			t.Fatalf("%s lost with the download: %v", f, err)
		}
	}
}
