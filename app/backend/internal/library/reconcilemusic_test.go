package library

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// id3File writes a tiny MP3 with an ID3v2.3 tag of text frames (none: no tag).
func id3File(t *testing.T, p string, pad int, frames ...[2]string) {
	t.Helper()
	var body bytes.Buffer
	for _, f := range frames {
		data := append([]byte{3}, f[1]...)
		body.WriteString(f[0])
		_ = binary.Write(&body, binary.BigEndian, uint32(len(data)))
		body.Write([]byte{0, 0})
		body.Write(data)
	}
	var out []byte
	if len(frames) > 0 {
		n := body.Len()
		out = append([]byte("ID3"), 3, 0, 0, byte(n>>21&0x7F), byte(n>>14&0x7F), byte(n>>7&0x7F), byte(n&0x7F))
		out = append(out, body.Bytes()...)
	}
	out = append(append(out, 0xFF, 0xFB), make([]byte, 512+pad)...)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

// reconcileWorld puts an artist with three albums in the Library and no
// file linked: Fifteen (15 tracks), Two Discs (2+2 tracks) and Other (2).
func reconcileWorld(t *testing.T) (*Service, *database.Store, mediafiles.LibraryFolder, map[string][]int64) {
	t.Helper()
	svc, s, _, lf := musicService(t)
	slots := map[string][]int64{}
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules ORDER BY id LIMIT 1").Scan(&pr); err != nil {
			return err
		}
		ti := &mediafiles.Title{Kind: "artist", GateDriver: "none", Name: "Reco Band", SortName: "reco band", Aliases: []string{},
			SourceIDs: map[string]string{"musicbrainz": "f0000000-0000-4000-8000-000000000001"}, MetadataSource: "musicbrainz", NewPartsPolicy: "all",
			ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "running", State: "active", KeepUpToDate: true}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		v := &mediafiles.Version{TitleID: ti.ID, Label: "Main", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Reco Band", IsMain: true, KeepUpToDate: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		album := func(name, rg string, year int, discs ...[]string) error {
			rd := time.Date(year, 3, 1, 0, 0, 0, 0, time.UTC)
			p := &mediafiles.Part{TitleID: ti.ID, Kind: "album", Name: name, ReleaseDate: &rd, KeepUpToDate: true, SourceIDs: map[string]string{"musicbrainz": rg},
				Pressings: []mediafiles.Pressing{}, SecondaryTypes: []string{}, Included: true}
			if _, err := database.Insert(tx, "parts", p, domain.System); err != nil {
				return err
			}
			for d, tracks := range discs {
				for i, tn := range tracks {
					disc, num, n := d+1, i+1, tn
					rec := fmt.Sprintf("%s-0000-4000-8000-%06d%06d", rg[:8], disc, num)
					it := &mediafiles.Item{TitleID: ti.ID, PartID: &p.ID, Kind: "track", Numbers: mediafiles.Numbers{Disc: &disc, Track: &num}, Name: &n,
						KeepUpToDate: true, SourceIDs: map[string]string{}, RecordingID: &rec}
					if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
						return err
					}
					sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotWanted}
					if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
						return err
					}
					slots[name] = append(slots[name], sl.ID)
				}
			}
			return nil
		}
		var fifteen []string
		for i := 1; i <= 15; i++ {
			fifteen = append(fifteen, fmt.Sprintf("Song Number %d", i))
		}
		if err := album("Fifteen", "f1000000-0000-4000-8000-000000000001", 2015, fifteen); err != nil {
			return err
		}
		if err := album("Two Discs", "f2000000-0000-4000-8000-000000000001", 2016, []string{"Side A", "Side B"}, []string{"Side C", "Side D"}); err != nil {
			return err
		}
		return album("Other", "f3000000-0000-4000-8000-000000000001", 2017, []string{"Alpha", "Beta"})
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, s, lf, slots
}

func linkedPaths(t *testing.T, s *database.Store) map[int64]string {
	t.Helper()
	rows, err := s.Read().Query("SELECT sl.id, f.relative_path FROM slots sl JOIN files f ON f.id = sl.file_id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var p string
		_ = rows.Scan(&id, &p)
		out[id] = p
	}
	return out
}

// Music files a scan recorded as found are matched to the tracks of
// an artist already in the Library, by tags first, then names; nothing
// moves and a second scan changes nothing.
func TestReconcileFoundMusic(t *testing.T) {
	svc, s, lf, slots := reconcileWorld(t)
	ctx := context.Background()
	dir := filepath.Join(lf.Path, "Reco Band", "Fifteen (2015)")
	for i := 1; i <= 15; i++ {
		name := fmt.Sprintf("Song Number %d", i)
		p := filepath.Join(dir, fmt.Sprintf("%02d - %s.mp3", i, name))
		switch {
		case i <= 5: // MusicBrainz ids only, a file name that says nothing
			p = filepath.Join(dir, fmt.Sprintf("track%c.mp3", rune('a'+i)))
			id3File(t, p, 100, [2]string{"TXXX", "MusicBrainz Album Id\x00f1000000-0000-4000-8000-000000000001"},
				[2]string{"TXXX", "MusicBrainz Release Group Id\x00f1000000-0000-4000-8000-000000000001"},
				[2]string{"TXXX", fmt.Sprintf("MUSICBRAINZ_TRACKID\x00f1000000-0000-4000-8000-%06d%06d", 1, i)})
		case i <= 10: // album, disc and track tags
			id3File(t, p, 100, [2]string{"TALB", "Fifteen"}, [2]string{"TRCK", fmt.Sprintf("%d/15", i)}, [2]string{"TIT2", name})
		default: // no tags: the folder and file names
			id3File(t, p, 100)
		}
	}
	// Multi-disc: disc folders, no tags.
	for _, f := range []string{"CD1/01 - Side A.mp3", "CD1/02 - Side B.mp3", "CD2/01 - Side C.mp3", "CD2/02 - Side D.mp3"} {
		id3File(t, filepath.Join(lf.Path, "Reco Band", "Two Discs (2016)", filepath.FromSlash(f)), 100)
	}
	// Wrong album: tagged as Other's first track, but the name is its second.
	id3File(t, filepath.Join(dir, "x.mp3"), 50, [2]string{"TALB", "Other"}, [2]string{"TRCK", "1"}, [2]string{"TIT2", "Beta"})
	// A folder that is no album of the artist.
	id3File(t, filepath.Join(lf.Path, "Reco Band", "Bootlegs", "01 - Alpha.mp3"), 50)
	// Duplicate: a smaller second copy of track 11.
	id3File(t, filepath.Join(dir, "11 - Song Number 11 (copy).mp3"), 0)
	before := snapshot(t, lf.Path)

	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	linked := linkedPaths(t, s)
	got := 0
	for i, sid := range slots["Fifteen"] {
		p, ok := linked[sid]
		if !ok {
			t.Errorf("track %d of Fifteen not linked", i+1)
			continue
		}
		if i >= 10 && !strings.HasSuffix(p, fmt.Sprintf("%02d - Song Number %d.mp3", i+1, i+1)) {
			t.Errorf("track %d linked to %s", i+1, p)
		}
		got++
	}
	if got != 15 {
		t.Fatalf("Fifteen: %d/15 linked", got)
	}
	for i, sid := range slots["Two Discs"] {
		want := []string{"CD1/01 - Side A.mp3", "CD1/02 - Side B.mp3", "CD2/01 - Side C.mp3", "CD2/02 - Side D.mp3"}[i]
		if !strings.HasSuffix(linked[sid], want) {
			t.Errorf("Two Discs track %d: %q want %s", i, linked[sid], want)
		}
	}
	for _, sid := range slots["Other"] {
		if p, ok := linked[sid]; ok {
			t.Errorf("Other got %s", p)
		}
	}
	var wanted int
	_ = s.Read().QueryRow("SELECT count(*) FROM slots WHERE file_id IS NOT NULL AND state = 'wanted'").Scan(&wanted)
	if wanted != 0 {
		t.Fatalf("%d linked slots still wanted", wanted)
	}
	var left int
	var leftPaths string
	_ = s.Read().QueryRow("SELECT count(*), COALESCE(group_concat(relative_path, ' | '), '') FROM files WHERE origin = 'found' AND slot_ids = '[]'").Scan(&left, &leftPaths)
	if left != 3 { // wrong album, bootleg, duplicate
		t.Fatalf("%d found files left (%s), want 3", left, leftPaths)
	}
	if fmt.Sprint(snapshot(t, lf.Path)) != fmt.Sprint(before) {
		t.Fatal("reconcile moved or changed a file")
	}
	// A second scan changes nothing.
	var events int
	_ = s.ReadEvents().QueryRow("SELECT count(*) FROM events WHERE kind = ?", domain.EventAdopted).Scan(&events)
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	if again := linkedPaths(t, s); fmt.Sprint(again) != fmt.Sprint(linked) {
		t.Fatalf("second scan changed links:\n%v\n%v", linked, again)
	}
	var events2 int
	_ = s.ReadEvents().QueryRow("SELECT count(*) FROM events WHERE kind = ?", domain.EventAdopted).Scan(&events2)
	if events != 1 || events2 != events {
		t.Fatalf("events %d then %d", events, events2)
	}
}

// A deluxe folder, titles with a featured artist or a remaster
// tail, and a multi-disc album tagged with no disc number all link.
func TestReconcileFoundMusicLooseFiles(t *testing.T) {
	svc, s, lf, slots := reconcileWorld(t)
	other := filepath.Join(lf.Path, "Reco Band", "Other (Deluxe Edition)")
	id3File(t, filepath.Join(other, "a.mp3"), 100, [2]string{"TALB", "Other (Deluxe Edition)"}, [2]string{"TRCK", "1"}, [2]string{"TIT2", "Alpha (feat. Guest)"})
	id3File(t, filepath.Join(other, "b.mp3"), 100, [2]string{"TALB", "Other (Deluxe Edition)"}, [2]string{"TRCK", "2"}, [2]string{"TIT2", "Beta - 2011 Remastered"})
	id3File(t, filepath.Join(other, "bonus.mp3"), 100, [2]string{"TALB", "Other (Deluxe Edition)"}, [2]string{"TRCK", "3"}, [2]string{"TIT2", "Bonus Song"})
	two := filepath.Join(lf.Path, "Reco Band", "Two Discs (2016)")
	for i, n := range []string{"Side A", "Side B", "Side C", "Side D"} {
		id3File(t, filepath.Join(two, fmt.Sprintf("t%d.mp3", i)), 100, [2]string{"TALB", "Two Discs"}, [2]string{"TRCK", fmt.Sprint(i + 1)}, [2]string{"TIT2", n})
	}
	if _, err := svc.Rescan(context.Background(), domain.System); err != nil {
		t.Fatal(err)
	}
	linked := linkedPaths(t, s)
	for _, name := range []string{"Other", "Two Discs"} {
		for i, sid := range slots[name] {
			if _, ok := linked[sid]; !ok {
				t.Errorf("%s track %d not linked", name, i+1)
			}
		}
	}
	if !strings.HasSuffix(linked[slots["Two Discs"][2]], "t2.mp3") {
		t.Errorf("disc 2 track 1: %q", linked[slots["Two Discs"][2]])
	}
	var left int
	_ = s.Read().QueryRow("SELECT count(*) FROM files WHERE origin = 'found' AND slot_ids = '[]'").Scan(&left)
	if left != 1 { // the bonus track has no track to fill
		t.Fatalf("%d found files left, want 1", left)
	}
}
