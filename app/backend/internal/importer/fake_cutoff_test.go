package importer

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// fixture reads a real ffmpeg-made file kept next to the media reader's tests.
func fixture(t *testing.T, name string, cut int) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "mediafiles", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if cut > 0 {
		b = b[:len(b)*cut/100]
	}
	return b
}

func put(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestImportRefusesCutOffMovie(t *testing.T) {
	for _, c := range []struct {
		name, ext string
		cut       int
		off       bool
		refused   bool
	}{
		{"intact.mkv", ".mkv", 0, false, false},
		{"intact.mkv", ".mkv", 60, false, true},
		{"intact.mkv", ".mkv", 60, true, false},
		{"intact.mp4", ".mp4", 0, false, false},
		{"intact.mp4", ".mp4", 60, false, true},
		{"intact-faststart.mp4", ".mp4", 70, false, true},
		{"intact-faststart.mp4", ".mp4", 0, false, false},
	} {
		w, relID, _ := upgradeWorld(t, relName, relName, "")
		setFake(t, w.s, "import_size_floors", "false")
		_ = os.Remove(filepath.Join(w.dl, relName+".mkv"))
		put(t, filepath.Join(w.dl, relName+c.ext), fixture(t, c.name, c.cut))
		if c.ext != ".mkv" {
			seenFile(t, w, relName+c.ext)
		}
		if c.off {
			setFake(t, w.s, "refuse_tiny_releases", "false")
		}
		var again []int64
		im := &Importer{Store: w.s, OnRejected: func(_ context.Context, id int64) { again = append(again, id) }}
		imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		state, why := releaseState(t, w.s, relID)
		if c.refused {
			if imp.State != "rejected" || state != "turned_down" || why != mediafiles.CutOff && why != mediafiles.NoIndex || len(again) != 1 {
				t.Fatalf("%+v: import %s, release %s %q, again %v", c, imp.State, state, why, again)
			}
		} else if imp.State != "done" {
			t.Fatalf("%+v: import %s %+v", c, imp.State, imp.Plan)
		}
	}
}

func TestImportRefusesCutOffEpisode(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2))
	put(t, filepath.Join(w.dl, packName, ep(1)), fixture(t, "intact.mkv", 0))
	put(t, filepath.Join(w.dl, packName, ep(2)), fixture(t, "intact.mkv", 50))
	im := &Importer{Store: w.s}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var whys []string
	for _, st := range imp.Plan {
		whys = append(whys, strings.Join(st.Why, "; "))
	}
	all := strings.Join(whys, " | ")
	if !strings.Contains(all, mediafiles.CutOff) && !strings.Contains(all, mediafiles.NoIndex) {
		t.Fatalf("plan %s", all)
	}
	if sl := w.slot(t, 1); sl.FileID == nil {
		t.Fatalf("the whole episode was not placed: %+v", sl)
	}
	if sl := w.slot(t, 2); sl.FileID != nil {
		t.Fatalf("the cut-off episode was placed: %+v", sl)
	}
}

func seenFile(t *testing.T, w world, name string) {
	t.Helper()
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE downloads SET files_seen = json_array(json_object('name', ?, 'size', 2048)) WHERE id = ?`, name, w.dlID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
