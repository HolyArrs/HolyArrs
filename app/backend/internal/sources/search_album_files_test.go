package sources

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestTrackFiles_IgnoreExtras checks that log, cue, playlist, cover and text files are not counted as tracks.
func TestTrackFiles_IgnoreExtras(t *testing.T) {
	names := []string{"01 Song.flac", "02 Song.flac", "Album.log", "Album.cue", "Album.m3u", "folder.jpg", "back.PNG", "info.nfo", "notes.txt."}
	if n := trackFiles(names, 99, 5); n != 2 {
		t.Fatalf("file list: %d tracks, want 2", n)
	}
	for _, c := range [][3]int{{12, 0, 12}, {12, 2, 10}, {1, 3, 0}} {
		if n := trackFiles(nil, c[0], c[1]); n != c[2] {
			t.Errorf("count %d less %d: %d, want %d", c[0], c[1], n, c[2])
		}
	}
}

// TestAlbumExtraFiles_Setting checks that with album_extra_files on, a bare file count padded with extras no longer passes for the whole album.
func TestAlbumExtraFiles_Setting(t *testing.T) {
	name := "Velvet Cartography - Low Tide Atlas (2019) [FLAC] [CD]"
	ms := &musicServer{answer: func(url.Values) []string { return []string{name + "#6"} }}
	f := seedArtist(t, ms.start(t))
	j := f.albumJob(t, &Searcher{Store: f.s}, lta, nil)
	if j.State != "done" || hasCode(relByName(t, f.fixture, name), "match.track_count") {
		t.Fatalf("6 files for 5 tracks refused without the setting: %v", reasonCodes(relByName(t, f.fixture, name)))
	}
	ms2 := &musicServer{answer: ms.answer}
	f = seedArtist(t, ms2.start(t))
	exec(t, f.fixture, `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
		VALUES ('what_to_pick', 'album_extra_files', '3', '2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}')`)
	_ = f.albumJob(t, &Searcher{Store: f.s}, lta, nil)
	if r := relByName(t, f.fixture, name); !hasCode(r, "match.track_count") {
		t.Fatalf("3 extras allowed, 6 files for 5 tracks passed: %v", reasonCodes(r))
	}
}

// TestChooseAlbum_MixNeedsConfirm checks that a hand choice that would leave tracks with different copies needs the person to confirm, and the refusal names the tracks left out.
func TestChooseAlbum_MixNeedsConfirm(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{flac} }}
	f := seedArtist(t, ms.start(t))
	sr := &Searcher{Store: f.s}
	f.albumJob(t, sr, lta, map[string]any{"choose": false, "by_hand": true})
	r := relByName(t, f.fixture, flac)
	var err error
	err = f.s.Write(context.Background(), func(tx *sql.Tx) error {
		name := "Velvet Cartography - Low Tide Atlas (2019) [FLAC 24-96]"
		fl := &mediafiles.File{SlotIDs: []int64{f.slots[ltaTracks[0]]}, VersionID: f.verID, RelativePath: "1-1.flac", Size: 1, AddedAt: time.Now(),
			Languages: []string{}, MediaInfo: map[string]any{}, ReleaseName: &name, Origin: "imported", PlacedBy: "copy", Role: "main", State: "present"}
		if _, err := database.Insert(tx, "files", fl, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE slots SET state = 'better_wanted', file_id = ? WHERE id = ?", fl.ID, f.slots[ltaTracks[0]])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sr.Choose(context.Background(), r.ID, 0, false, domain.System)
	var nf *NeedsForceError
	if !errors.As(err, &nf) || !strings.Contains(err.Error(), "mix") || !strings.Contains(err.Error(), "already has a copy as good") {
		t.Fatalf("mixed album not refused: %v", err)
	}
	if st := f.state(t, ltaTracks[1]); st == mediafiles.SlotChosen {
		t.Fatal("chosen without confirming")
	}
	ch, err := sr.Choose(context.Background(), r.ID, 0, true, domain.System)
	if err != nil || len(ch.SlotIDs) != 4 || !ch.Forced {
		t.Fatalf("confirmed: %+v %v", ch, err)
	}
}
