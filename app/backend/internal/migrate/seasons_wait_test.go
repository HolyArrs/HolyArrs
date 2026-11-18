package migrate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Once the last waiting file is matched or left alone, the show stops
// waiting: a file found later is put on its episode by number again, while
// a file left alone keeps the old app's numbers and is never linked by them.
func TestSeasonsDifferWaitEnds(t *testing.T) {
	l, _ := cunkSetup(t, cunkSonarr, []metadata.Show{cunkOn()}, cunkFiles...)
	l.move(t, Choices{})
	ctx := context.Background()
	var id int64
	if err := l.st.Read().QueryRow(`SELECT id FROM titles WHERE seasons_differ = 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	ls, err := l.mv.Library.Leftovers(ctx, id)
	if err != nil || len(ls) != 2 {
		t.Fatalf("%v %+v", err, ls)
	}
	var ep int64 // Earth's first episode here is season 2, episode 1
	if err := l.st.Read().QueryRow(`SELECT id FROM items WHERE title_id = ? AND json_extract(numbers, '$.season') = 2 AND json_extract(numbers, '$.episode') = 1`, id).Scan(&ep); err != nil {
		t.Fatal(err)
	}
	if out, err := l.mv.Library.MatchLeftovers(ctx, id, []library.LeftoverMatch{{FileID: ls[0].FileID, EpisodeIDs: []int64{ep}}}, false, domain.System); err != nil || !out[0].Linked {
		t.Fatalf("match %v %+v", err, out)
	}
	mark := `SELECT seasons_differ FROM titles WHERE id = ?`
	if n := l.count(t, mark, id); n != 1 {
		t.Fatalf("one file still waits, mark %d", n)
	}
	if err := l.mv.Library.DismissLeftover(ctx, id, ls[1].FileID, true); err != nil {
		t.Fatal(err)
	}
	if n := l.count(t, mark, id); n != 2 {
		t.Fatalf("nothing waits, mark %d", n)
	}
	var folder string
	if err := l.st.Read().QueryRow(`SELECT lf.path || '/' || v.folder_name FROM versions v JOIN library_folders lf ON lf.id = v.library_folder_id WHERE v.title_id = ?`, id).Scan(&folder); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(filepath.FromSlash(folder), "Season 02", "Cunk on... - S02E02 - Faith+Off.mkv")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("later"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := l.mv.Library.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	if n := l.count(t, showFilled); n != 2 {
		t.Fatalf("%d episodes have a file; want the matched one and the one found later", n)
	}
	if n := l.count(t, `SELECT count(*) FROM files WHERE origin = 'found' AND leftover_reason = 'seasons_differ' AND slot_ids = '[]' AND leftover_dismissed = 1`); n != 1 {
		t.Fatalf("the file left alone was linked: %d", n)
	}
	// Bringing it back does not make the show wait again by itself, and it
	// is still never linked by number.
	if err := l.mv.Library.DismissLeftover(ctx, id, ls[1].FileID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := l.mv.Library.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	if n := l.count(t, showFilled); n != 2 {
		t.Fatalf("rescan linked the old file: %d", n)
	}
}
