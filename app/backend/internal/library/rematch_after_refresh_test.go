package library

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// rematchWorld adds the lab artist with a found file for a third track of
// Middle that MusicBrainz does not list yet: a scan records it unmatched.
func rematchWorld(t *testing.T) (*Service, *database.Store, *fakeMusic, int64) {
	t.Helper()
	svc, s, fm, lf := musicService(t)
	id := addLabArtist(t, svc, AlbumsAll)
	var folder string
	if err := s.Read().QueryRow("SELECT folder_name FROM versions WHERE title_id = ?", id).Scan(&folder); err != nil || folder == "" {
		t.Fatalf("artist folder %q: %v", folder, err)
	}
	id3File(t, filepath.Join(lf.Path, folder, "Middle (2018)", "03 - Song m3.mp3"), 100,
		[2]string{"TALB", "Middle"}, [2]string{"TRCK", "3"}, [2]string{"TIT2", "Song m3"})
	if _, err := svc.Rescan(context.Background(), domain.System); err != nil {
		t.Fatal(err)
	}
	if n := linkedM3(t, s); n != 0 {
		t.Fatalf("m3 linked before MusicBrainz lists it: %d", n)
	}
	return svc, s, fm, id
}

func linkedM3(t *testing.T, s *database.Store) int {
	t.Helper()
	var n int
	if err := s.Read().QueryRow(`SELECT count(*) FROM slots sl JOIN files f ON f.id = sl.file_id
		JOIN items i ON i.id = sl.item_id WHERE i.recording_id = 'rec-m3' AND f.relative_path LIKE '%Song m3.mp3'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// an artist refresh that adds the track a found file holds links
// that file right away, without a full Library rescan.
func TestRefreshArtistMatchesFoundFiles(t *testing.T) {
	svc, s, fm, id := rematchWorld(t)
	fm.pressings[rgMid][0].Tracks = tracks("mid", "m1", "m2", "m3")
	if _, err := svc.RefreshArtist(useCache(context.Background()), id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	if n := linkedM3(t, s); n != 1 {
		t.Fatalf("m3 not linked after the refresh added it: %d", n)
	}
	// A second refresh changes nothing.
	if _, err := svc.RefreshArtist(useCache(context.Background()), id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	if n := linkedM3(t, s); n != 1 {
		t.Fatalf("second refresh: %d links", n)
	}
}

// picking a Pressing by hand links a found file that only that
// Pressing has a track for.
func TestSetAlbumPressingMatchesFoundFiles(t *testing.T) {
	svc, s, _, id := rematchWorld(t)
	var folder string
	_ = s.Read().QueryRow("SELECT folder_name FROM versions WHERE title_id = ?", id).Scan(&folder)
	var root string
	_ = s.Read().QueryRow("SELECT path FROM library_folders WHERE kind = 'music'").Scan(&root)
	id3File(t, filepath.Join(root, folder, "Old Days (2010)", "CD2", "01 - Song 4.mp3"), 100,
		[2]string{"TALB", "Old Days"}, [2]string{"TPOS", "2"}, [2]string{"TRCK", "1"}, [2]string{"TIT2", "Song 4"})
	if _, err := svc.Rescan(context.Background(), domain.System); err != nil {
		t.Fatal(err)
	}
	count := func() int {
		var n int
		_ = s.Read().QueryRow(`SELECT count(*) FROM slots sl JOIN files f ON f.id = sl.file_id
			JOIN items i ON i.id = sl.item_id WHERE i.recording_id = 'rec-4'`).Scan(&n)
		return n
	}
	if n := count(); n != 0 {
		t.Fatalf("deluxe track linked before its Pressing was picked: %d", n)
	}
	var partID, pressingID int64
	if err := s.Read().QueryRow("SELECT p.part_id, p.id FROM pressings p WHERE p.mbid = ?", relOldDx).Scan(&partID, &pressingID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetAlbum(context.Background(), partID, nil, pressingID, domain.System); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 1 {
		t.Fatalf("deluxe track not linked after picking its Pressing: %d", n)
	}
}
