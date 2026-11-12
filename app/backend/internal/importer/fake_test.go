package importer

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func setFake(t *testing.T, s *database.Store, key, value string) {
	t.Helper()
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('what_to_pick', ?, ?, '2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}')
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value`, key, value)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func releaseState(t *testing.T, s *database.Store, id int64) (string, string) {
	t.Helper()
	var r domain.Release
	if err := database.Get(context.Background(), s.Read(), "releases", id, &r); err != nil {
		t.Fatal(err)
	}
	why, _ := r.Verdict["turned_down_reason"].(string)
	return r.State, why
}

func TestImportRefusesEmptyMovieFile(t *testing.T) {
	for _, on := range []bool{true, false} {
		w, relID, _ := upgradeWorld(t, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", "")
		if err := os.WriteFile(filepath.Join(w.dl, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if !on {
			setFake(t, w.s, "refuse_tiny_releases", "false")
		}
		var again []int64
		im := &Importer{Store: w.s, OnRejected: func(_ context.Context, id int64) { again = append(again, id) }}
		imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		state, why := releaseState(t, w.s, relID)
		if on && (imp.State != "rejected" || state != "turned_down" || !strings.Contains(why, "empty") || len(again) != 1) {
			t.Fatalf("on: import %s, release %s %q, again %v", imp.State, state, why, again)
		}
		if !on && imp.State != "done" {
			t.Fatalf("off: import %s %+v", imp.State, imp.Plan)
		}
	}
}

func TestImportRefusesFakeEpisodes(t *testing.T) {
	wmv := strings.Replace(ep(1), ".mkv", ".wmv", 1)
	w := showSetup(t, []int{1, 2}, wmv, ep(2))
	write(t, filepath.Join(w.dl, packName, wmv), 2<<20)
	setFake(t, w.s, "import_min_video_mb", "1")
	im := &Importer{Store: w.s}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State == "done" {
		t.Fatalf("import %s", imp.State)
	}
	var whys []string
	for _, st := range imp.Plan {
		whys = append(whys, strings.Join(st.Why, "; "))
	}
	all := strings.Join(whys, " | ")
	if !strings.Contains(all, "a .wmv file where the Release said 1080p") || !strings.Contains(all, "too small to be the full video") {
		t.Fatalf("plan %s", all)
	}
	for _, n := range []int{1, 2} {
		if sl := w.slot(t, n); sl.FileID != nil || sl.State == mediafiles.SlotInLibrary {
			t.Fatalf("E%d placed: %+v", n, sl)
		}
	}
}

func TestImportRefusesEmptyTrack(t *testing.T) {
	w := albumSetup(t, 1, "01 - Dawn.flac", "02 - Glass River.flac", "03 - Night Train.flac")
	if err := os.WriteFile(filepath.Join(w.dl, albumRel, "02 - Glass River.flac"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	im := &Importer{Store: w.s}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if w.state(t, 0) != mediafiles.SlotInLibrary || w.state(t, 2) != mediafiles.SlotInLibrary || w.state(t, 1) == mediafiles.SlotInLibrary {
		t.Fatalf("states %s %s %s; import %s %+v", w.state(t, 0), w.state(t, 1), w.state(t, 2), imp.State, imp.Plan)
	}
	if exists(filepath.Join(w.lib, "Lab Band", "First Light (2020)", "02 - Glass River.flac")) {
		t.Fatal("the empty track was placed")
	}
}

func floorWorld(t *testing.T, size, claimed int64, minutes int, extra ...string) (world, int64) {
	t.Helper()
	w, relID, _ := upgradeWorld(t, relName, relName, "")
	setFake(t, w.s, "import_size_floors", "true")
	write(t, filepath.Join(w.dl, relName+".mkv"), int(size))
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		seen := `json_array(json_object('name', ?, 'size', ?))`
		args := []any{relName + ".mkv", size}
		for _, x := range extra {
			write(t, filepath.Join(w.dl, x), 100)
			seen = strings.TrimSuffix(seen, ")") + `, json_object('name', ?, 'size', 100))`
			args = append(args, x)
		}
		if _, err := tx.Exec("UPDATE downloads SET files_seen = "+seen+" WHERE id = ?", append(args, w.dlID)...); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE releases SET size = ? WHERE id = ?", claimed, relID); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE items SET runtime = ?", minutes)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return w, relID
}

func TestImportSizeFloors(t *testing.T) {
	cases := []struct {
		name, key, value string
		placed           bool
	}{
		{"fixed floor", "", "", false},
		{"lowered", "import_min_video_mb", "2", true},
		{"raised", "import_min_video_mb", "5000", false},
		{"off", "import_size_floors", "false", true},
	}
	for _, c := range cases {
		w, relID := floorWorld(t, 3<<20, 0, 166)
		if c.key != "" {
			setFake(t, w.s, c.key, c.value)
		}
		imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		state, why := releaseState(t, w.s, relID)
		if c.placed != (imp.State == "done") {
			t.Fatalf("%s: import %s %+v", c.name, imp.State, imp.Plan)
		}
		if !c.placed && (state != "turned_down" || !strings.Contains(why, "too small to be the full video")) {
			t.Fatalf("%s: release %s %q", c.name, state, why)
		}
	}
	w, _ := floorWorld(t, 3<<20, 0, 0)
	if imp, _ := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System); imp.State == "done" {
		t.Fatal("a 3 MB video of unknown length was placed")
	}
}

func TestImportRefusesClaimMismatch(t *testing.T) {
	w, relID := floorWorld(t, 3<<20, 4<<30, 166)
	setFake(t, w.s, "import_min_video_mb", "1")
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	state, why := releaseState(t, w.s, relID)
	if imp.State == "done" || state != "turned_down" || !strings.Contains(why, "the Release said 4.0 GB but holds 3 MB") {
		t.Fatalf("import %s, release %s %q", imp.State, state, why)
	}
}

func TestImportWithShortcutBeside(t *testing.T) {
	fakeDisk(t, 1<<40)
	w, _ := floorWorld(t, 200<<20, 210<<20, 166, "Downloaded from X.url")
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("import %v %+v", err, imp)
	}
}

func TestImportUnreadableFileIsNotFake(t *testing.T) {
	w, relID := floorWorld(t, 3<<20, 0, 166)
	if err := os.Remove(filepath.Join(w.dl, relName+".mkv")); err != nil {
		t.Fatal(err)
	}
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	state, _ := releaseState(t, w.s, relID)
	if imp.State == "done" || state == "turned_down" {
		t.Fatalf("import %s, release %s", imp.State, state)
	}
	// A file gone from the download is let go, not called fake: the movie
	// stays wanted.
	if sl := slotOf(t, w); sl.State != mediafiles.SlotWanted {
		t.Fatalf("slot %s", sl.State)
	}
	if !strings.Contains(strings.Join(imp.Plan[0].Why, ";"), "not in the download folder") {
		t.Fatalf("plan %+v", imp.Plan)
	}
}
