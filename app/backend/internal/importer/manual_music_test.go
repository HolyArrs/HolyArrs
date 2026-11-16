package importer

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

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// flacFixture is a tiny FLAC file: the header, a comment block, a few bytes of audio.
func flacFixture(t *testing.T, path string, comments ...string) {
	t.Helper()
	var vc bytes.Buffer
	_ = binary.Write(&vc, binary.LittleEndian, uint32(4))
	vc.WriteString("test")
	_ = binary.Write(&vc, binary.LittleEndian, uint32(len(comments)))
	for _, c := range comments {
		_ = binary.Write(&vc, binary.LittleEndian, uint32(len(c)))
		vc.WriteString(c)
	}
	out := append([]byte("fLaC"), 0, 0, 0, 34)
	out = append(out, make([]byte, 34)...)
	n := vc.Len()
	out = append(out, 0x80|4, byte(n>>16), byte(n>>8), byte(n))
	out = append(append(out, vc.Bytes()...), make([]byte, 512)...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

// mp3Fixture is a tiny MP3 file with an ID3v2.3 tag of text frames, or none.
func mp3Fixture(t *testing.T, path string, frames ...[2]string) {
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
	out = append(append(out, 0xFF, 0xFB), make([]byte, 512)...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

var partTwo = []string{"Two People Fell in Love", "Wrapped Around", "I'm Gonna Miss Her", "The Old Rugged Cross", "I Wish You'd Stay",
	"Too Country", "You Have the Right to Remain Silent", "Come on Over Tonight", "Part II", "Hoe Down", "Munster Rag", "Waitin' on a Woman"}

type musicWorld struct {
	s      *database.Store
	lib    string
	in     string
	artist int64
	album  int64
	slots  []int64
	other  int64 // a second album's first track Slot
}

// musicSetup puts two artists in the Library: Brad Paisley with Part II
// (12 tracks) and Lab Band with First Light (3 tracks).
func musicSetup(t *testing.T) musicWorld {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	w := musicWorld{s: s, lib: t.TempDir(), in: t.TempDir()}
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		_ = tx.QueryRow("SELECT id FROM pick_rules WHERE name = 'Lossless first' ").Scan(&pr)
		lf := &mediafiles.LibraryFolder{Path: w.lib, Kind: "music", Label: "Music", Health: "ok", SameDiskAs: []int64{}}
		if _, err := database.Insert(tx, "library_folders", lf, domain.System); err != nil {
			return err
		}
		add := func(name, mbid, album, rgid string, year int, tracks []string) (int64, int64, []int64, error) {
			ti := &mediafiles.Title{Kind: "artist", GateDriver: "none", Name: name, SortName: strings.ToLower(name), Aliases: []string{},
				SourceIDs: map[string]string{"musicbrainz": mbid}, MetadataSource: "musicbrainz", NewPartsPolicy: "all", ReleaseGate: "released",
				MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "running", State: "active", KeepUpToDate: true}
			if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
				return 0, 0, nil, err
			}
			v := &mediafiles.Version{TitleID: ti.ID, Label: "Main", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: name, IsMain: true, KeepUpToDate: true}
			if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
				return 0, 0, nil, err
			}
			rd := time.Date(year, 5, 1, 0, 0, 0, 0, time.UTC)
			p := &mediafiles.Part{TitleID: ti.ID, Kind: "album", Name: album, ReleaseDate: &rd, KeepUpToDate: true, SourceIDs: map[string]string{"musicbrainz": rgid},
				Pressings: []mediafiles.Pressing{}, SecondaryTypes: []string{}, Included: true}
			if _, err := database.Insert(tx, "parts", p, domain.System); err != nil {
				return 0, 0, nil, err
			}
			var slots []int64
			for i, tn := range tracks {
				disc, num, name := 1, i+1, tn
				rec := fmt.Sprintf("0000000%d-0000-4000-8000-%012d", len(tracks)%10, i+1)
				it := &mediafiles.Item{TitleID: ti.ID, PartID: &p.ID, Kind: "track", Numbers: mediafiles.Numbers{Disc: &disc, Track: &num}, Name: &name,
					KeepUpToDate: true, SourceIDs: map[string]string{}, RecordingID: &rec}
				if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
					return 0, 0, nil, err
				}
				sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotWanted}
				if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
					return 0, 0, nil, err
				}
				slots = append(slots, sl.ID)
			}
			return ti.ID, p.ID, slots, nil
		}
		var err error
		if w.artist, w.album, w.slots, err = add("Brad Paisley", "e0000000-0000-4000-8000-000000000001", "Part II", "e0000000-0000-4000-8000-0000000000a2", 2001, partTwo); err != nil {
			return err
		}
		_, _, other, err := add("Lab Band", "e0000000-0000-4000-8000-000000000002", "First Light", "e0000000-0000-4000-8000-0000000000b1", 2020, []string{"Dawn", "Glass River", "Night Train"})
		if len(other) > 0 {
			w.other = other[0]
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func (w musicWorld) scan(t *testing.T, root string) map[string]ByHandFile {
	t.Helper()
	im := &Importer{Store: w.s, SavePath: w.in}
	sc, err := im.ScanByHand(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]ByHandFile{}
	for _, f := range sc.Files {
		out[f.Name] = f
	}
	return out
}

func TestByHandMusicTaggedAlbum(t *testing.T) {
	w := musicSetup(t)
	album := filepath.Join(w.in, "Brad Paisley", "Part II (2001)")
	for i, n := range partTwo {
		flacFixture(t, filepath.Join(album, fmt.Sprintf("%02d - %s.flac", i+1, n)),
			"ARTIST=Brad Paisley", "ALBUM=Part II", "TITLE="+n, fmt.Sprintf("TRACKNUMBER=%d/12", i+1))
	}
	got := w.scan(t, album)
	if len(got) != 12 {
		t.Fatalf("scan found %d files", len(got))
	}
	for i, n := range partTwo {
		f := got[fmt.Sprintf("%02d - %s.flac", i+1, n)]
		g := f.Guess
		if g.TitleID != w.artist || g.TitleKind != "artist" || g.AlbumID != w.album || g.AlbumName != "Part II" || g.AlbumYear != 2001 ||
			g.Track != i+1 || g.TrackName != n || g.SlotID != w.slots[i] || g.Confidence != 1 || f.Note != "" || g.Reason != "" {
			t.Fatalf("track %d: %+v", i+1, f)
		}
	}
}

// A live or deluxe album tag that only matches once its "(...)" is cut off is a
// guess, never sure: it may be a different recording from the studio track.
func TestByHandMusicAlbumTagTailNotSure(t *testing.T) {
	w := musicSetup(t)
	for _, al := range []string{"Part II (Live)", "Part II (Deluxe Edition)", "Part II [Acoustic]"} {
		flacFixture(t, filepath.Join(w.in, "x", al+".flac"), "ARTIST=Brad Paisley", "ALBUM="+al, "TRACKNUMBER=3", "TITLE="+partTwo[2])
	}
	flacFixture(t, filepath.Join(w.in, "x", "year.flac"), "ARTIST=Brad Paisley", "ALBUM=Part II (2001)", "TRACKNUMBER=3", "TITLE="+partTwo[2])
	got := w.scan(t, w.in)
	for _, al := range []string{"Part II (Live)", "Part II (Deluxe Edition)", "Part II [Acoustic]"} {
		if g := got["x/"+al+".flac"].Guess; g.Confidence >= 0.7 {
			t.Errorf("%s: %+v", al, g)
		}
	}
	if g := got["x/year.flac"].Guess; g.AlbumID != w.album || g.Track != 3 || g.Confidence != 1 {
		t.Errorf("year tail: %+v", g)
	}
}

// The same holds for an untagged file whose album folder only matches once its
// "(Live)" or "(Deluxe Edition)" is cut off; a year or format tail stays sure.
func TestByHandMusicAlbumFolderTailNotSure(t *testing.T) {
	w := musicSetup(t)
	for _, al := range []string{"Part II (Live)", "Part II (Deluxe Edition)", "Part II (2001) [FLAC]"} {
		mp3Fixture(t, filepath.Join(w.in, "Brad Paisley", al, "03 "+partTwo[2]+".mp3"))
	}
	got := w.scan(t, w.in)
	for _, al := range []string{"Part II (Live)", "Part II (Deluxe Edition)"} {
		if g := got["Brad Paisley/"+al+"/03 "+partTwo[2]+".mp3"].Guess; g.Confidence >= 0.7 {
			t.Errorf("%s: %+v", al, g)
		}
	}
	if g := got["Brad Paisley/Part II (2001) [FLAC]/03 "+partTwo[2]+".mp3"].Guess; g.AlbumID != w.album || g.Track != 3 || g.Confidence < 0.7 {
		t.Errorf("year tail: %+v", g)
	}
}

func TestByHandMusicFolderNamesAndIDs(t *testing.T) {
	w := musicSetup(t)
	// No tags: the folders name the artist and album, the file the track.
	mp3Fixture(t, filepath.Join(w.in, "untagged", "Lab Band", "First Light [MP3]", "CD1", "02 Glass River.mp3"))
	// Wrong words in the tags, but MusicBrainz ids that settle it.
	flacFixture(t, filepath.Join(w.in, "ids", "x.flac"), "ARTIST=Somebody", "ALBUM=Something",
		"MUSICBRAINZ_ARTISTID=e0000000-0000-4000-8000-000000000001", "MUSICBRAINZ_RELEASEGROUPID=e0000000-0000-4000-8000-0000000000a2",
		"MUSICBRAINZ_TRACKID=00000002-0000-4000-8000-000000000003")
	// The tags say one track and the title another: matched, not sure.
	flacFixture(t, filepath.Join(w.in, "odd", "y.flac"), "ARTIST=Brad Paisley", "ALBUM=Part II", "TRACKNUMBER=4", "TITLE=Hoe Down")
	// Misses, each with its reason.
	mp3Fixture(t, filepath.Join(w.in, "miss", "Nobody Here", "Album", "01 Song.mp3"), [2]string{"TPE1", "Nobody Here"}, [2]string{"TALB", "Album"}, [2]string{"TRCK", "1"})
	mp3Fixture(t, filepath.Join(w.in, "miss", "b.mp3"), [2]string{"TPE1", "Brad Paisley"}, [2]string{"TALB", "Mud on the Tires"}, [2]string{"TRCK", "1"})
	mp3Fixture(t, filepath.Join(w.in, "miss", "c.mp3"), [2]string{"TPE1", "Brad Paisley"}, [2]string{"TALB", "Part II"}, [2]string{"TRCK", "13"})
	mp3Fixture(t, filepath.Join(w.in, "track.mp3"))
	got := w.scan(t, w.in)
	if g := got["untagged/Lab Band/First Light [MP3]/CD1/02 Glass River.mp3"].Guess; g.TitleName != "Lab Band" || g.AlbumName != "First Light" || g.Track != 2 || g.Confidence != 0.7 {
		t.Fatalf("folder names: %+v", g)
	}
	if g := got["ids/x.flac"].Guess; g.TitleID != w.artist || g.AlbumID != w.album || g.Track != 3 || g.SlotID != w.slots[2] || g.Confidence != 1 {
		t.Fatalf("ids: %+v", g)
	}
	if g := got["odd/y.flac"].Guess; g.Track != 4 || g.Confidence >= 0.7 {
		t.Fatalf("number and title disagree: %+v", g)
	}
	for name, want := range map[string]string{"miss/Nobody Here/Album/01 Song.mp3": MusicNoArtist, "miss/b.mp3": MusicNoAlbum, "miss/c.mp3": MusicNoTrack, "track.mp3": MusicNoArtist} {
		f := got[name]
		if f.Guess.Reason != want || f.Guess.SlotID != 0 || f.Guess.Confidence != 0 {
			t.Errorf("%s: %+v, want %s", name, f.Guess, want)
		}
	}
	if d := got["miss/b.mp3"].Guess.Detail; d != "Mud on the Tires" {
		t.Errorf("album detail %q", d)
	}
}

func TestByHandMusicImport(t *testing.T) {
	w := musicSetup(t)
	ctx := context.Background()
	album := filepath.Join(w.in, "Brad Paisley", "Part II (2001)")
	src := filepath.Join(album, "03 - I'm Gonna Miss Her.flac")
	flacFixture(t, src, "ARTIST=Brad Paisley", "ALBUM=Part II", "TITLE=I'm Gonna Miss Her", "TRACKNUMBER=3")
	moved := filepath.Join(album, "09 - Part II.flac")
	flacFixture(t, moved, "ARTIST=Brad Paisley", "ALBUM=Part II", "TITLE=Part II", "TRACKNUMBER=9")
	video := filepath.Join(w.in, "clip.mkv")
	write(t, video, 2048)
	im := &Importer{Store: w.s, SavePath: w.in}
	res, err := im.ImportByHand(ctx, []ByHandItem{
		{Path: src, TitleID: w.artist, SlotID: w.slots[2]},
		{Path: moved, SlotID: w.slots[8], Mode: "move"},
		{Path: src, SlotID: w.slots[2]},
		{Path: video, SlotID: w.slots[4]},
	}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"placed", "placed", "refused", "refused"}
	for i, r := range res {
		if r.Outcome != want[i] {
			t.Fatalf("item %d: %+v, want %s", i, r, want[i])
		}
	}
	if !strings.Contains(res[2].Error, "already") || !strings.Contains(res[3].Error, "music file") {
		t.Fatalf("refusals %q / %q", res[2].Error, res[3].Error)
	}
	if res[0].PlacedBy != "hardlink" || res[1].PlacedBy != "move" {
		t.Fatalf("placed by %+v %+v", res[0], res[1])
	}
	if !exists(src) || exists(moved) {
		t.Fatal("copy removed the original, or move kept it")
	}
	for _, i := range []int{2, 8} {
		var sl mediafiles.Slot
		if err := database.Get(ctx, w.s.Read(), "slots", w.slots[i], &sl); err != nil || sl.FileID == nil {
			t.Fatalf("slot %d: %+v %v", i, sl, err)
		}
		if sl.State != mediafiles.SlotInLibrary && sl.State != mediafiles.SlotBetterWanted {
			t.Fatalf("slot %d state %s", i, sl.State)
		}
		var f mediafiles.File
		if err := database.Get(ctx, w.s.Read(), "files", *sl.FileID, &f); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(f.RelativePath, "Brad Paisley/") || !strings.Contains(f.RelativePath, "Part II") || !strings.HasSuffix(f.RelativePath, ".flac") ||
			f.Quality.Source != "flac" || f.Origin != "imported" {
			t.Fatalf("file %d: %+v", i, f)
		}
		if !exists(filepath.Join(w.lib, filepath.FromSlash(f.RelativePath))) {
			t.Fatalf("file %d not on disk", i)
		}
	}
	imps, _ := database.Select[mediafiles.Import](ctx, w.s.Read(), "imports", "mode = 'by_hand'")
	if len(imps) != 2 || imps[0].Kind != "tracks" {
		t.Fatalf("imports %+v", imps)
	}
	// A rescan sees the copied file's track as filled.
	if g := w.scan(t, album)["03 - I'm Gonna Miss Her.flac"].Guess; !g.HasFile {
		t.Fatalf("rescan %+v", g)
	}
}

// When the Library has both "Part II" and "Part II (Deluxe Edition)", a file
// tagged with the deluxe name goes on the deluxe album, never the base one.
func TestByHandMusicExactAlbumNameWins(t *testing.T) {
	w := musicSetup(t)
	var deluxe int64
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		var vid int64
		if err := tx.QueryRow("SELECT id FROM versions WHERE title_id = ?", w.artist).Scan(&vid); err != nil {
			return err
		}
		rd := time.Date(2001, 5, 1, 0, 0, 0, 0, time.UTC)
		p := &mediafiles.Part{TitleID: w.artist, Kind: "album", Name: "Part II (Deluxe Edition)", ReleaseDate: &rd, KeepUpToDate: true,
			SourceIDs: map[string]string{"musicbrainz": "e0000000-0000-4000-8000-0000000000a3"}, Pressings: []mediafiles.Pressing{}, SecondaryTypes: []string{}, Included: true}
		if _, err := database.Insert(tx, "parts", p, domain.System); err != nil {
			return err
		}
		deluxe = p.ID
		for i, tn := range partTwo {
			disc, num, name := 1, i+1, tn
			it := &mediafiles.Item{TitleID: w.artist, PartID: &p.ID, Kind: "track", Numbers: mediafiles.Numbers{Disc: &disc, Track: &num}, Name: &name,
				KeepUpToDate: true, SourceIDs: map[string]string{}}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			if _, err := database.Insert(tx, "slots", &mediafiles.Slot{ItemID: it.ID, VersionID: vid, State: mediafiles.SlotWanted}, domain.System); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	flacFixture(t, filepath.Join(w.in, "Brad Paisley", "Part II (2001)", "03.flac"), "ARTIST=Brad Paisley", "ALBUM=Part II (Deluxe Edition)", "TRACKNUMBER=3", "TITLE="+partTwo[2])
	flacFixture(t, filepath.Join(w.in, "x", "03.flac"), "ARTIST=Brad Paisley", "ALBUM=Part II (Deluxe Edition)", "TRACKNUMBER=3", "TITLE="+partTwo[2])
	got := w.scan(t, w.in)
	for _, k := range []string{"Brad Paisley/Part II (2001)/03.flac", "x/03.flac"} {
		if g := got[k].Guess; g.AlbumID != deluxe || g.Track != 3 {
			t.Errorf("%s: %+v", k, g)
		}
	}
}
