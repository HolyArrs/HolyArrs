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
	for _, al := range []string{"Part II (Live)", "Part II [Acoustic]"} {
		if g := got["x/"+al+".flac"].Guess; g.Confidence >= 0.7 {
			t.Errorf("%s: %+v", al, g)
		}
	}
	// A deluxe pressing holds the same recordings, as another pressing of the album does in Lidarr.
	if g := got["x/Part II (Deluxe Edition).flac"].Guess; g.AlbumID != w.album || g.Track != 3 || g.Confidence != 1 {
		t.Errorf("deluxe: %+v", g)
	}
	if g := got["x/year.flac"].Guess; g.AlbumID != w.album || g.Track != 3 || g.Confidence != 1 {
		t.Errorf("year tail: %+v", g)
	}
}

// The same holds for an untagged file whose album folder only matches once its
// "(Live)" is cut off; a year, format or edition tail stays sure.
func TestByHandMusicAlbumFolderTailNotSure(t *testing.T) {
	w := musicSetup(t)
	for _, al := range []string{"Part II (Live)", "Part II (Deluxe Edition)", "Part II (2001) [FLAC]"} {
		mp3Fixture(t, filepath.Join(w.in, "Brad Paisley", al, "03 "+partTwo[2]+".mp3"))
	}
	got := w.scan(t, w.in)
	if g := got["Brad Paisley/Part II (Live)/03 "+partTwo[2]+".mp3"].Guess; g.Confidence >= 0.7 {
		t.Errorf("live: %+v", g)
	}
	if g := got["Brad Paisley/Part II (Deluxe Edition)/03 "+partTwo[2]+".mp3"].Guess; g.AlbumID != w.album || g.Track != 3 || g.Confidence < 0.7 {
		t.Errorf("deluxe: %+v", g)
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
	for name, want := range map[string]string{"miss/Nobody Here/Album/01 Song.mp3": MusicNoArtist, "miss/b.mp3": MusicNoAlbum, "miss/c.mp3": MusicNoTrack, "track.mp3": MusicNoFacts} {
		f := got[name]
		if f.Guess.Reason != want || f.Guess.SlotID != 0 || f.Guess.Confidence != 0 {
			t.Errorf("%s: %+v, want %s", name, f.Guess, want)
		}
	}
	// A file loose in the import folder takes no artist from the folders around it.
	if d := got["track.mp3"].Guess.Detail; d != "" {
		t.Errorf("loose file named an artist from the folder: %q", d)
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
		// A lossless file imported by hand reaches Lossless first's good enough.
		if sl.State != mediafiles.SlotInLibrary {
			t.Fatalf("slot %d state %s", i, sl.State)
		}
		var f mediafiles.File
		if err := database.Get(ctx, w.s.Read(), "files", *sl.FileID, &f); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(f.RelativePath, "Brad Paisley/") || !strings.Contains(f.RelativePath, "Part II") || !strings.HasSuffix(f.RelativePath, ".flac") ||
			f.Quality.Source != "flac" || f.Origin != "imported" || f.ReleaseName == nil || !strings.HasSuffix(*f.ReleaseName, " [FLAC]") {
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
	flacFixture(t, filepath.Join(w.in, "y", "03.flac"), "ARTIST=Brad Paisley", "ALBUM=Part II (Deluxe Edition) [2001]", "TRACKNUMBER=3", "TITLE="+partTwo[2])
	flacFixture(t, filepath.Join(w.in, "Brad Paisley", "Part II (Deluxe Edition) (2001) [FLAC]", "03.flac"))
	got := w.scan(t, w.in)
	for _, k := range []string{"Brad Paisley/Part II (2001)/03.flac", "x/03.flac", "y/03.flac", "Brad Paisley/Part II (Deluxe Edition) (2001) [FLAC]/03.flac"} {
		if g := got[k].Guess; g.AlbumID != deluxe || g.Track != 3 {
			t.Errorf("%s: %+v", k, g)
		}
	}
}

// An album tag that names no album of the artist leaves only the folder name,
// so a folder hit is a guess even when the artist came from the tags.
func TestByHandMusicUnknownAlbumTagFolderGuess(t *testing.T) {
	w := musicSetup(t)
	flacFixture(t, filepath.Join(w.in, "Brad Paisley", "Part II", "03.flac"), "ARTIST=Brad Paisley", "ALBUM=Unplugged", "TRACKNUMBER=3")
	got := w.scan(t, w.in)
	if g := got["Brad Paisley/Part II/03.flac"].Guess; g.Confidence > 0.4 {
		t.Fatalf("%+v", g)
	}
}

// addAlbum gives an artist one more album; tracks are "disc:name", and the Slots come back in order.
func (w musicWorld) addAlbum(t *testing.T, artist int64, name string, year int, rgid string, tracks ...string) (int64, []int64) {
	t.Helper()
	var part int64
	var slots []int64
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		var vid int64
		if err := tx.QueryRow("SELECT id FROM versions WHERE title_id = ?", artist).Scan(&vid); err != nil {
			return err
		}
		rd := time.Date(year, 5, 1, 0, 0, 0, 0, time.UTC)
		p := &mediafiles.Part{TitleID: artist, Kind: "album", Name: name, ReleaseDate: &rd, KeepUpToDate: true,
			SourceIDs: map[string]string{"musicbrainz": rgid}, Pressings: []mediafiles.Pressing{}, SecondaryTypes: []string{}, Included: true}
		if _, err := database.Insert(tx, "parts", p, domain.System); err != nil {
			return err
		}
		part = p.ID
		nums := map[int]int{}
		for _, tn := range tracks {
			d, n, _ := strings.Cut(tn, ":")
			disc := int(d[0] - '0')
			nums[disc]++
			num, name := nums[disc], n
			it := &mediafiles.Item{TitleID: artist, PartID: &p.ID, Kind: "track", Numbers: mediafiles.Numbers{Disc: &disc, Track: &num}, Name: &name,
				KeepUpToDate: true, SourceIDs: map[string]string{}}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			sl := &mediafiles.Slot{ItemID: it.ID, VersionID: vid, State: mediafiles.SlotWanted}
			if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			slots = append(slots, sl.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return part, slots
}

// A two-disc album in CD1/CD2 folders with no disc tags: the folder gives the disc.
func TestByHandMusicDiscFolders(t *testing.T) {
	w := musicSetup(t)
	part, slots := w.addAlbum(t, w.artist, "Hits Live", 2003, "e0000000-0000-4000-8000-0000000000c1", "1:Opener", "1:Second", "2:Encore", "2:Closer")
	root := filepath.Join(w.in, "Brad Paisley", "Hits Live (2003)")
	flacFixture(t, filepath.Join(root, "CD1", "01.flac"), "ARTIST=Brad Paisley", "ALBUM=Hits Live", "TRACKNUMBER=1", "TITLE=Opener")
	flacFixture(t, filepath.Join(root, "CD2", "01.flac"), "ARTIST=Brad Paisley", "ALBUM=Hits Live", "TRACKNUMBER=1", "TITLE=Encore")
	flacFixture(t, filepath.Join(root, "CD2", "02.flac"), "ARTIST=Brad Paisley", "ALBUM=Hits Live", "TRACKNUMBER=2/2", "TITLE=Closer")
	mp3Fixture(t, filepath.Join(root, "Disc 2", "02 Closer.mp3"))
	mp3Fixture(t, filepath.Join(root, "CD2", "x.mp3"), [2]string{"TPE1", "Brad Paisley"}, [2]string{"TALB", "Hits Live"}, [2]string{"TRCK", "1/2"}, [2]string{"TPOS", "2/2"})
	got := w.scan(t, w.in)
	for k, want := range map[string]int64{"CD1/01.flac": slots[0], "CD2/01.flac": slots[2], "CD2/02.flac": slots[3], "Disc 2/02 Closer.mp3": slots[3], "CD2/x.mp3": slots[2]} {
		g := got["Brad Paisley/Hits Live (2003)/"+k].Guess
		if g.AlbumID != part || g.SlotID != want || g.Confidence < 0.7 {
			t.Errorf("%s: %+v", k, g)
		}
	}
}

// Two albums of the same name: the year in the folder picks one; with no year it is left to the user.
func TestByHandMusicSameNameAlbums(t *testing.T) {
	w := musicSetup(t)
	again, slots := w.addAlbum(t, w.artist, "Part II", 2010, "e0000000-0000-4000-8000-0000000000c2", "1:New One", "1:New Two", "1:New Three")
	flacFixture(t, filepath.Join(w.in, "Brad Paisley", "Part II (2001)", "03.flac"), "ARTIST=Brad Paisley", "ALBUM=Part II", "TRACKNUMBER=3", "TITLE="+partTwo[2])
	flacFixture(t, filepath.Join(w.in, "Brad Paisley", "Part II (2010)", "03.flac"), "ARTIST=Brad Paisley", "ALBUM=Part II", "TRACKNUMBER=3", "TITLE=New Three")
	flacFixture(t, filepath.Join(w.in, "Brad Paisley", "Part II", "03.flac"), "ARTIST=Brad Paisley", "ALBUM=Part II", "TRACKNUMBER=3")
	got := w.scan(t, w.in)
	if g := got["Brad Paisley/Part II (2001)/03.flac"].Guess; g.AlbumID != w.album || g.SlotID != w.slots[2] || g.Confidence != 1 {
		t.Errorf("2001: %+v", g)
	}
	if g := got["Brad Paisley/Part II (2010)/03.flac"].Guess; g.AlbumID != again || g.SlotID != slots[2] || g.Confidence != 1 {
		t.Errorf("2010: %+v", g)
	}
	if g := got["Brad Paisley/Part II/03.flac"].Guess; g.Reason != MusicTwoAlbum || g.SlotID != 0 || g.TitleID != w.artist {
		t.Errorf("no year: %+v", g)
	}
}

// A single named like the album, with no tracks listed, never blocks the album.
func TestByHandMusicSameNameEmptySingle(t *testing.T) {
	w := musicSetup(t)
	w.addAlbum(t, w.artist, "Part II", 2002, "e0000000-0000-4000-8000-0000000000c3")
	mp3Fixture(t, filepath.Join(w.in, "Brad Paisley", "Part II", "03 "+partTwo[2]+".mp3"))
	if g := w.scan(t, w.in)["Brad Paisley/Part II/03 "+partTwo[2]+".mp3"].Guess; g.AlbumID != w.album || g.SlotID != w.slots[2] {
		t.Fatalf("%+v", g)
	}
}

// A compilation: the album artist is Various Artists, the track artist one of ours.
func TestByHandMusicCompilation(t *testing.T) {
	w := musicSetup(t)
	flacFixture(t, filepath.Join(w.in, "Various Artists", "Road Songs (2019)", "05.flac"), "ALBUMARTIST=Various Artists", "ARTIST=Lab Band",
		"ALBUM=Road Songs", "TRACKNUMBER=5", "TITLE=Night Train", "COMPILATION=1")
	flacFixture(t, filepath.Join(w.in, "Various Artists", "First Light", "03.flac"), "ALBUMARTIST=Various Artists", "ARTIST=Lab Band",
		"ALBUM=First Light", "TRACKNUMBER=3", "TITLE=Night Train")
	got := w.scan(t, w.in)
	if g := got["Various Artists/Road Songs (2019)/05.flac"].Guess; g.TitleName != "Lab Band" || g.Reason != MusicNoAlbum || g.SlotID != 0 || g.Confidence != 0 {
		t.Errorf("compilation: %+v", g)
	}
	if g := got["Various Artists/First Light/03.flac"].Guess; g.TitleName != "Lab Band" || g.Track != 3 || g.Confidence != 1 {
		t.Errorf("own album under Various Artists: %+v", g)
	}
}

// "feat." in the artist tag, with the MusicBrainz ids of both artists.
func TestByHandMusicFeaturedArtist(t *testing.T) {
	w := musicSetup(t)
	brad, other := "e0000000-0000-4000-8000-000000000001", "e0000000-0000-4000-8000-0000000000ff"
	flacFixture(t, filepath.Join(w.in, "a", "03.flac"), "ARTIST=Brad Paisley feat. Alison Krauss", "ALBUM=Part II", "TRACKNUMBER=3", "TITLE="+partTwo[2],
		"MUSICBRAINZ_ARTISTID="+brad, "MUSICBRAINZ_ARTISTID="+other)
	flacFixture(t, filepath.Join(w.in, "b", "03.flac"), "ARTIST=Alison Krauss & Brad Paisley", "ALBUM=Part II", "TRACKNUMBER=3", "TITLE="+partTwo[2],
		"MUSICBRAINZ_ARTISTID="+other+"; "+brad)
	mp3Fixture(t, filepath.Join(w.in, "c", "03.mp3"), [2]string{"TPE1", "Brad Paisley feat. Alison Krauss"}, [2]string{"TALB", "Part II"}, [2]string{"TRCK", "3/12"},
		[2]string{"TIT2", partTwo[2]})
	got := w.scan(t, w.in)
	for _, k := range []string{"a/03.flac", "b/03.flac", "c/03.mp3"} {
		if g := got[k].Guess; g.TitleID != w.artist || g.SlotID != w.slots[2] || g.Confidence != 1 {
			t.Errorf("%s: %+v", k, g)
		}
	}
}

// Replace puts the new file on a track that has one; the old file waits in Recently deleted.
func TestByHandMusicReplace(t *testing.T) {
	w := musicSetup(t)
	ctx := context.Background()
	first := filepath.Join(w.in, "one", "03.mp3")
	mp3Fixture(t, first, [2]string{"TPE1", "Brad Paisley"}, [2]string{"TALB", "Part II"}, [2]string{"TRCK", "3/12"})
	im := &Importer{Store: w.s, SavePath: w.in}
	if res, err := im.ImportByHand(ctx, []ByHandItem{{Path: first, SlotID: w.slots[2], Mode: "copy"}}, domain.System); err != nil || res[0].Outcome != "placed" {
		t.Fatalf("first: %+v %v", res, err)
	}
	var sl mediafiles.Slot
	_ = database.Get(ctx, w.s.Read(), "slots", w.slots[2], &sl)
	oldID := *sl.FileID
	var old mediafiles.File
	_ = database.Get(ctx, w.s.Read(), "files", oldID, &old)
	better := filepath.Join(w.in, "two", "03.flac")
	flacFixture(t, better, "ARTIST=Brad Paisley", "ALBUM=Part II", "TRACKNUMBER=3", "TITLE="+partTwo[2])
	if g := w.scan(t, filepath.Join(w.in, "two"))["03.flac"].Guess; !g.HasFile || g.SlotID != w.slots[2] {
		t.Fatalf("scan sees no file: %+v", g)
	}
	if res, _ := im.ImportByHand(ctx, []ByHandItem{{Path: better, SlotID: w.slots[2]}}, domain.System); res[0].Outcome != "refused" {
		t.Fatalf("no replace: %+v", res)
	}
	res, err := im.ImportByHand(ctx, []ByHandItem{{Path: better, SlotID: w.slots[2], Replace: true}}, domain.System)
	if err != nil || res[0].Outcome != "placed" {
		t.Fatalf("replace: %+v %v", res, err)
	}
	_ = database.Get(ctx, w.s.Read(), "slots", w.slots[2], &sl)
	var nf, gone mediafiles.File
	_ = database.Get(ctx, w.s.Read(), "files", *sl.FileID, &nf)
	_ = database.Get(ctx, w.s.Read(), "files", oldID, &gone)
	if *sl.FileID == oldID || !strings.HasSuffix(nf.RelativePath, ".flac") || !exists(filepath.Join(w.lib, filepath.FromSlash(nf.RelativePath))) {
		t.Fatalf("new file %+v", nf)
	}
	if gone.State != "gone" || !strings.HasPrefix(gone.RelativePath, HoldingDir+"/") || !exists(filepath.Join(w.lib, filepath.FromSlash(gone.RelativePath))) ||
		exists(filepath.Join(w.lib, filepath.FromSlash(old.RelativePath))) {
		t.Fatalf("old file %+v (was %s)", gone, old.RelativePath)
	}
	if !exists(better) || !exists(first) {
		t.Fatal("a source file went missing")
	}
}

// Titles that differ only by a featured artist or a remaster tail, a
// deluxe pressing that numbers the tracks differently, and multi-disc files
// with no disc tag or folder all find their track.
func TestByHandMusicLooseTrackTitles(t *testing.T) {
	w := musicSetup(t)
	_, slots := w.addAlbum(t, w.artist, "Two Sides", 2005, "e0000000-0000-4000-8000-0000000000c4", "1:Alpha", "1:Beta", "2:Gamma", "2:Delta")
	root := filepath.Join(w.in, "Brad Paisley")
	cases := []struct {
		file string
		tags []string
		slot int64
		sure bool
	}{
		{"Part II/03.flac", []string{"TRACKNUMBER=3", "TITLE=" + partTwo[2] + " (feat. Alison Krauss)"}, w.slots[2], true},
		{"Part II/03r.flac", []string{"TRACKNUMBER=3", "TITLE=" + partTwo[2] + " - 2011 Remastered"}, w.slots[2], true},
		{"Part II/x.flac", []string{"TRACKNUMBER=14", "TITLE=" + partTwo[1]}, w.slots[1], true},
		{"Part II/y.flac", []string{"TRACKNUMBER=3", "TITLE=Some Other Song"}, w.slots[2], false},
		{"Two Sides/a.flac", []string{"TRACKNUMBER=3", "TITLE=Gamma", "ALBUM=Two Sides"}, slots[2], true},
		{"Two Sides/b.flac", []string{"TRACKNUMBER=202", "ALBUM=Two Sides"}, slots[3], true},
		{"Two Sides/c.flac", []string{"TRACKNUMBER=2", "TITLE=Beta", "ALBUM=Two Sides"}, slots[1], true},
		// Numbered per disc with the disc only in the file name: the name wins over counting across the album.
		{"Two Sides/2-01 Gamma.flac", []string{"TRACKNUMBER=1", "ALBUM=Two Sides"}, slots[2], true},
		// Numbered per disc, no disc anywhere, a title that is not the counted track: not a sure match to disc 1.
		{"Two Sides/d.flac", []string{"TRACKNUMBER=2", "TITLE=Delta", "ALBUM=Two Sides"}, slots[3], true},
	}
	for _, c := range cases {
		al := "ALBUM=Part II"
		if strings.HasPrefix(c.file, "Two") {
			al = "ALBUM=Two Sides"
		}
		flacFixture(t, filepath.Join(root, filepath.FromSlash(c.file)), append([]string{"ARTIST=Brad Paisley", al}, c.tags...)...)
	}
	got := w.scan(t, w.in)
	for _, c := range cases {
		g := got["Brad Paisley/"+c.file].Guess
		if g.SlotID != c.slot || (g.Confidence == 1) != c.sure {
			t.Errorf("%s: %+v", c.file, g)
		}
	}
}

func TestSameSongAndEdition(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"Song (feat. X)", "Song", true}, {"Song ft. X", "Song", true}, {"Song [2009 Remaster]", "Song", true},
		{"Song - Remastered 2011", "Song", true}, {"Song (Live)", "Song", false}, {"Song (Remix)", "Song", false}, {"Other", "Song", false},
		{"Song (Live 2011 Remaster)", "Song", false}, {"Song - Live at Wembley / 2011 Remaster", "Song", false},
		{"Song (Acoustic) [Remastered]", "Song", false}, {"Song (Demo) [2009 Remaster]", "Song", false},
		{"Song (Monologue)", "Song", false}, {"Song (Stereophonic)", "Song", false},
	} {
		if got := sameSong(c.a, c.b); got != c.want {
			t.Errorf("sameSong(%q,%q)=%v", c.a, c.b, got)
		}
	}
	for _, c := range []struct {
		a    string
		want bool
	}{
		{"Album (Deluxe Edition)", true}, {"Album [2011 Remaster]", true}, {"Album (20th Anniversary Edition) [FLAC]", true},
		{"Album (Live)", false}, {"Album (Deluxe Live Edition)", false}, {"Album (Remixes)", false}, {"Other (Deluxe)", false},
	} {
		if got := editionOf(c.a, "Album"); got != c.want {
			t.Errorf("editionOf(%q)=%v", c.a, got)
		}
	}
}
