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
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// flacFile writes a tiny FLAC that plays for ms milliseconds, with Vorbis
// comments ("TITLE=x"); no comments means no tags.
func flacFile(t *testing.T, p string, ms, pad int, comments ...string) {
	t.Helper()
	si := make([]byte, 34)
	rate, samples := 44100, int64(ms)*44100/1000
	si[10], si[11], si[12] = byte(rate>>12), byte(rate>>4), byte(rate<<4)
	si[13] = byte(samples>>32) & 0x0F
	binary.BigEndian.PutUint32(si[14:18], uint32(samples))
	out := []byte("fLaC")
	last := byte(0)
	if len(comments) == 0 {
		last = 0x80
	}
	out = append(out, last, 0, 0, 34)
	out = append(out, si...)
	if len(comments) > 0 {
		var vc bytes.Buffer
		_ = binary.Write(&vc, binary.LittleEndian, uint32(4))
		vc.WriteString("test")
		_ = binary.Write(&vc, binary.LittleEndian, uint32(len(comments)))
		for _, c := range comments {
			_ = binary.Write(&vc, binary.LittleEndian, uint32(len(c)))
			vc.WriteString(c)
		}
		n := vc.Len()
		out = append(out, 0x80|4, byte(n>>16), byte(n>>8), byte(n))
		out = append(out, vc.Bytes()...)
	}
	out = append(out, make([]byte, 256+pad)...)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

type uTrack struct {
	name string
	ms   int
}

type uWorld struct {
	svc   *Service
	s     *database.Store
	lf    mediafiles.LibraryFolder
	ids   map[string]int64   // artist name -> title id
	parts map[string]int64   // album name -> part id
	slots map[string][]int64 // album name -> slot ids in disc/track order
	items map[string][]int64 // album name -> item ids in disc/track order
}

func (w *uWorld) artist(t *testing.T, name, mbid string) {
	t.Helper()
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules ORDER BY id LIMIT 1").Scan(&pr); err != nil {
			return err
		}
		ti := &mediafiles.Title{Kind: "artist", GateDriver: "none", Name: name, SortName: strings.ToLower(name), Aliases: []string{},
			SourceIDs: map[string]string{"musicbrainz": mbid}, MetadataSource: "musicbrainz", NewPartsPolicy: "all",
			ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "running", State: "active", KeepUpToDate: true}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		v := &mediafiles.Version{TitleID: ti.ID, Label: "Main", PickRulesID: pr, LibraryFolderID: w.lf.ID, FolderName: name, IsMain: true, KeepUpToDate: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		w.ids[name] = ti.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// album adds an album to artist with its discs of tracks; secondary is "compilation" or "".
func (w *uWorld) album(t *testing.T, artist, name, rg, secondary string, discs ...[]uTrack) {
	t.Helper()
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		tid := w.ids[artist]
		var vid int64
		if err := tx.QueryRow("SELECT id FROM versions WHERE title_id = ?", tid).Scan(&vid); err != nil {
			return err
		}
		rd := time.Date(2010, 3, 1, 0, 0, 0, 0, time.UTC)
		sec := []string{}
		if secondary != "" {
			sec = []string{secondary}
		}
		p := &mediafiles.Part{TitleID: tid, Kind: "album", Name: name, ReleaseDate: &rd, KeepUpToDate: true, SourceIDs: map[string]string{"musicbrainz": rg},
			Pressings: []mediafiles.Pressing{}, SecondaryTypes: sec, Included: true}
		if _, err := database.Insert(tx, "parts", p, domain.System); err != nil {
			return err
		}
		w.parts[name] = p.ID
		for d, tracks := range discs {
			for i, tr := range tracks {
				disc, num, n := d+1, i+1, tr.name
				it := &mediafiles.Item{TitleID: tid, PartID: &p.ID, Kind: "track", Numbers: mediafiles.Numbers{Disc: &disc, Track: &num}, Name: &n,
					KeepUpToDate: true, SourceIDs: map[string]string{}}
				if tr.ms > 0 {
					ms := tr.ms
					it.LengthMS = &ms
				}
				if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
					return err
				}
				sl := &mediafiles.Slot{ItemID: it.ID, VersionID: vid, State: mediafiles.SlotWanted}
				if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
					return err
				}
				w.slots[name] = append(w.slots[name], sl.ID)
				w.items[name] = append(w.items[name], it.ID)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (w *uWorld) pressing(t *testing.T, album, mbid, name string, chosen bool, tracks ...string) {
	t.Helper()
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		ts := make([]domain.PressingTrack, len(tracks))
		for i, n := range tracks {
			ts[i] = domain.PressingTrack{Disc: 1, Number: i + 1, Position: i + 1, Name: n}
		}
		p := &domain.AlbumPressing{PartID: w.parts[album], MBID: mbid, Name: name, Status: "official", Formats: []string{"CD"}, TrackCount: len(tracks), Tracks: ts, Chosen: chosen, ChosenBy: "auto"}
		_, err := database.Insert(tx, "pressings", p, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func newUWorld(t *testing.T) *uWorld {
	svc, s, _, lf := musicService(t)
	w := &uWorld{svc: svc, s: s, lf: lf, ids: map[string]int64{}, parts: map[string]int64{}, slots: map[string][]int64{}, items: map[string][]int64{}}
	w.artist(t, "Main Band", "a0000000-0000-4000-8000-000000000001")
	w.artist(t, "Second Band", "a0000000-0000-4000-8000-000000000002")
	w.artist(t, "Various Artists", "89ad4ac3-39f7-470e-963a-56509c546377")
	w.album(t, "Main Band", "Short", "b1000000-0000-4000-8000-000000000001", "", []uTrack{{"Alpha", 200000}, {"Beta", 180000}})
	w.album(t, "Main Band", "Double", "b2000000-0000-4000-8000-000000000001", "",
		[]uTrack{{"One", 100000}, {"Two", 110000}}, []uTrack{{"Three", 120000}, {"Four", 130000}})
	w.album(t, "Main Band", "Grown", "b3000000-0000-4000-8000-000000000001", "", []uTrack{{"First", 150000}, {"Second", 160000}})
	w.pressing(t, "Grown", "c3000000-0000-4000-8000-000000000001", "Grown", true, "First", "Second")
	w.pressing(t, "Grown", "c3000000-0000-4000-8000-000000000002", "Grown", false, "First", "Second", "Bonus One", "Bonus Two")
	w.album(t, "Main Band", "Hits Collection", "b4000000-0000-4000-8000-000000000001", "compilation", []uTrack{{"Gamma", 0}, {"Delta", 0}})
	w.album(t, "Second Band", "Their Album", "b5000000-0000-4000-8000-000000000001", "", []uTrack{{"Theirs", 0}, {"Also Theirs", 0}})
	return w
}

func reasons(t *testing.T, s *database.Store) map[string]string {
	t.Helper()
	rows, err := s.Read().Query("SELECT relative_path, coalesce(leftover_reason, '') FROM files WHERE origin = 'found' AND slot_ids = '[]'")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var p, r string
		_ = rows.Scan(&p, &r)
		out[p] = r
	}
	return out
}

// Found files of every awkward kind either link, move to the artist
// they belong to, or stay listed with the reason they did not.
func TestReconcileUnmatchedMusic(t *testing.T) {
	w := newUWorld(t)
	ctx := context.Background()
	root := func(p ...string) string { return filepath.Join(append([]string{w.lf.Path}, p...)...) }
	// Featured artist in the artist tag and the title.
	flacFile(t, root("Main Band", "Short", "a.flac"), 200500, 100, "ALBUM=Short", "TRACKNUMBER=1", "TITLE=Alpha (feat. Guest)", "ARTIST=Main Band feat. Guest")
	// A title that reads differently but plays as long as track 2: the length vouches.
	flacFile(t, root("Main Band", "Short", "b.flac"), 181000, 100, "ALBUM=Short", "TRACKNUMBER=2", "TITLE=Betta")
	// A smaller copy of track 2: a duplicate.
	flacFile(t, root("Main Band", "Short", "b copy.flac"), 181000, 10, "ALBUM=Short", "TRACKNUMBER=2", "TITLE=Beta")
	// Multi-disc, no disc tags or disc folders, no titles: counted across the album, the length vouches.
	for i, ms := range []int{100000, 110000, 120000, 130000} {
		flacFile(t, root("Main Band", "Double", fmt.Sprintf("x%d.flac", i)), ms+400, 100, "ALBUM=Double", fmt.Sprintf("TRACKNUMBER=%d", i+1))
	}
	// Untagged, named for Grown's first track but far too short: not sure.
	flacFile(t, root("Main Band", "Grown", "01 - First.flac"), 30000, 100)
	// Untagged and right: links by its file name.
	flacFile(t, root("Main Band", "Grown", "02 - Second.flac"), 160200, 100)
	// The deluxe pressing's bonus tracks.
	flacFile(t, root("Main Band", "Grown", "03 - Bonus One.flac"), 90000, 100, "ALBUM=Grown", "TRACKNUMBER=3", "TITLE=Bonus One")
	flacFile(t, root("Main Band", "Grown", "04 - Bonus Two.flac"), 90000, 100, "ALBUM=Grown", "TRACKNUMBER=4", "TITLE=Bonus Two")
	// Wrong folder: Second Band's song in Main Band's folder.
	flacFile(t, root("Main Band", "Their Album", "01.flac"), 1000, 100, "ALBUMARTIST=Second Band", "ALBUM=Their Album", "TRACKNUMBER=1", "TITLE=Theirs")
	// A compilation under Various Artists, held by Main Band's album list (by release group).
	flacFile(t, root("Various Artists", "Hits Collection", "01.flac"), 1000, 100, "ALBUMARTIST=Various Artists", "ARTIST=Main Band", "ALBUM=Hits Collection",
		"TRACKNUMBER=1", "TITLE=Gamma", "MUSICBRAINZ_RELEASEGROUPID=b4000000-0000-4000-8000-000000000001")
	// An album nobody has.
	flacFile(t, root("Main Band", "Bootlegs", "01 - Nope.flac"), 1000, 100, "ALBUM=Bootlegs", "TRACKNUMBER=1", "TITLE=Nope")
	before := snapshot(t, w.lf.Path)

	if _, err := w.svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	linked := linkedPaths(t, w.s)
	want := map[int64]string{
		w.slots["Short"][0]:           "Main Band/Short/a.flac",
		w.slots["Short"][1]:           "Main Band/Short/b.flac",
		w.slots["Double"][0]:          "Main Band/Double/x0.flac",
		w.slots["Double"][1]:          "Main Band/Double/x1.flac",
		w.slots["Double"][2]:          "Main Band/Double/x2.flac",
		w.slots["Double"][3]:          "Main Band/Double/x3.flac",
		w.slots["Grown"][1]:           "Main Band/Grown/02 - Second.flac",
		w.slots["Their Album"][0]:     "Main Band/Their Album/01.flac",
		w.slots["Hits Collection"][0]: "Various Artists/Hits Collection/01.flac",
	}
	for sid, p := range want {
		if linked[sid] != p {
			t.Errorf("slot %d: %q, want %q", sid, linked[sid], p)
		}
	}
	if len(linked) != len(want) {
		t.Errorf("%d linked, want %d: %v", len(linked), len(want), linked)
	}
	wantWhy := map[string]string{
		"Main Band/Short/b copy.flac":         importer.FoundHasFile, // a duplicate once the bigger copy holds the track
		"Main Band/Grown/01 - First.flac":     importer.FoundUnsure,
		"Main Band/Grown/03 - Bonus One.flac": importer.FoundNoTrack,
		"Main Band/Grown/04 - Bonus Two.flac": importer.FoundNoTrack,
		"Main Band/Bootlegs/01 - Nope.flac":   importer.FoundNoAlbum,
	}
	got := reasons(t, w.s)
	if fmt.Sprint(got) != fmt.Sprint(wantWhy) {
		t.Errorf("reasons\n got %v\nwant %v", got, wantWhy)
	}
	if fmt.Sprint(snapshot(t, w.lf.Path)) != fmt.Sprint(before) {
		t.Fatal("a file moved or changed on disk")
	}
	// The moved files now belong to their artists.
	var owner string
	_ = w.s.Read().QueryRow(`SELECT t.name FROM files f JOIN versions v ON v.id = f.version_id JOIN titles t ON t.id = v.title_id WHERE f.relative_path = ?`,
		"Main Band/Their Album/01.flac").Scan(&owner)
	if owner != "Second Band" {
		t.Errorf("wrong-folder file belongs to %q", owner)
	}

	// The artist page's list: reasons, closest album and track, and the deluxe pressing offered.
	ls, hints, err := w.svc.LeftoverList(ctx, w.ids["Main Band"])
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) != 5 {
		t.Fatalf("%d leftovers: %+v", len(ls), ls)
	}
	byPath := map[string]LeftoverView{}
	for _, l := range ls {
		byPath[l.RelativePath] = l
	}
	first := byPath["Main Band/Grown/01 - First.flac"]
	if first.Reason != importer.FoundUnsure || first.Music == nil || first.Music.AlbumName != "Grown" || first.Music.TrackID != w.items["Grown"][0] {
		t.Errorf("unsure file: %+v %+v", first, first.Music)
	}
	if len(hints) != 1 || hints[0].AlbumID != w.parts["Grown"] || hints[0].Fits != 3 || hints[0].Chosen != 1 || hints[0].TrackCount != 4 {
		t.Errorf("pressing hints: %+v", hints)
	}
	total, artists, err := w.svc.UnmatchedMusic(ctx)
	if err != nil || total != 5 || len(artists) != 1 || artists[0].Name != "Main Band" {
		t.Errorf("library count %d %+v %v", total, artists, err)
	}

	// Linking by hand from the list, reusing the leftover match.
	out, err := w.svc.MatchLeftovers(ctx, w.ids["Main Band"], []LeftoverMatch{{FileID: first.FileID, EpisodeIDs: []int64{w.items["Grown"][0]}}}, false, domain.System)
	if err != nil || len(out) != 1 || !out[0].Linked {
		t.Fatalf("match by hand: %+v %v", out, err)
	}
	if linkedPaths(t, w.s)[w.slots["Grown"][0]] != "Main Band/Grown/01 - First.flac" {
		t.Error("hand match did not fill the track")
	}
	// A track of another artist is refused.
	nope := byPath["Main Band/Bootlegs/01 - Nope.flac"]
	out, _ = w.svc.MatchLeftovers(ctx, w.ids["Main Band"], []LeftoverMatch{{FileID: nope.FileID, EpisodeIDs: []int64{w.items["Their Album"][1]}}}, false, domain.System)
	if len(out) != 1 || out[0].Linked || out[0].Refused != "wrong_title" {
		t.Errorf("other artist's track: %+v", out)
	}

	// A second scan links nothing new and says nothing new.
	count := func() int {
		var n int
		_ = w.s.ReadEvents().QueryRow("SELECT count(*) FROM events WHERE kind = ? AND summary LIKE '%Unmatched files%'", domain.EventFolderScanned).Scan(&n)
		return n
	}
	n1 := count()
	if _, err := w.svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	if n1 != 1 || count() != n1 {
		t.Errorf("unmatched events %d then %d", n1, count())
	}
	if after := reasons(t, w.s); len(after) != 4 {
		t.Errorf("after the second scan: %v", after)
	}
	// The hand-matched track file is named for its folder and format, as a scan names it.
	var rel string
	_ = w.s.Read().QueryRow("SELECT coalesce(release_name, '') FROM files WHERE id = ?", first.FileID).Scan(&rel)
	if rel != "Grown [FLAC]" {
		t.Errorf("hand-matched release name %q", rel)
	}
	// Using the offered pressing, then scanning, links the bonus tracks.
	if _, err := w.svc.SetAlbum(ctx, w.parts["Grown"], nil, hints[0].PressingID, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	if after := reasons(t, w.s); len(after) != 2 {
		t.Errorf("after using the deluxe pressing: %v", after)
	}
	// No duplicate record of the moved file under the folder it sits in.
	var copies int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM files WHERE relative_path = ?", "Main Band/Their Album/01.flac").Scan(&copies)
	if copies != 1 {
		t.Errorf("moved file recorded %d times", copies)
	}
}

// A file tagged for an artist in another Library folder stays listed, saying whose it is.
func TestReconcileOtherArtistElsewhere(t *testing.T) {
	w := newUWorld(t)
	ctx := context.Background()
	other, err := AddFolder(ctx, w.s, FolderInput{Path: t.TempDir(), Kind: "music", Label: "Other"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE versions SET library_folder_id = ? WHERE title_id = ?", other.ID, w.ids["Second Band"])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	flacFile(t, filepath.Join(w.lf.Path, "Main Band", "Their Album", "01.flac"), 1000, 100, "ALBUMARTIST=Second Band", "ALBUM=Their Album", "TRACKNUMBER=1", "TITLE=Theirs")
	if _, err := w.svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	if got := reasons(t, w.s)["Main Band/Their Album/01.flac"]; got != importer.FoundOtherArtist {
		t.Fatalf("reason %q", got)
	}
	ls, _, err := w.svc.LeftoverList(ctx, w.ids["Main Band"])
	if err != nil || len(ls) != 1 || ls[0].Music == nil || ls[0].Music.Owner != "Second Band" {
		t.Fatalf("%+v %v", ls, err)
	}
}

// Edge cases of passing files on and listing them: a file passed on stays
// with its new artist even when it fits no track there; a file naming its own
// artist or an artist not in the Library stays put; a match undone by hand and
// a file left alone are never retried or counted.
func TestReconcileUnmatchedEdges(t *testing.T) {
	w := newUWorld(t)
	ctx := context.Background()
	root := func(p ...string) string { return filepath.Join(append([]string{w.lf.Path}, p...)...) }
	// Second Band's album, but a track it does not have.
	flacFile(t, root("Main Band", "Their Album", "09.flac"), 1000, 100, "ALBUMARTIST=Second Band", "ALBUM=Their Album", "TRACKNUMBER=9", "TITLE=Not On It")
	// Its own artist with a featured guest, and an album nobody has.
	flacFile(t, root("Main Band", "Live", "01.flac"), 1000, 100, "ALBUMARTIST=Main Band feat. Guest", "ALBUM=Live Somewhere", "TRACKNUMBER=1", "TITLE=Alpha")
	// An artist not in the Library.
	flacFile(t, root("Main Band", "VA", "01.flac"), 1000, 100, "ALBUMARTIST=Various Artists", "ARTIST=Nobody Known", "ALBUM=Summer Hits", "TRACKNUMBER=1", "TITLE=Sun")
	// One that links, to be undone by hand.
	flacFile(t, root("Main Band", "Short", "01 - Alpha.flac"), 200000, 100)
	for i := 0; i < 2; i++ {
		if _, err := w.svc.Rescan(ctx, domain.System); err != nil {
			t.Fatal(err)
		}
	}
	got := reasons(t, w.s)
	want := map[string]string{
		"Main Band/Their Album/09.flac": importer.FoundNoTrack,
		"Main Band/Live/01.flac":        importer.FoundNoAlbum,
		"Main Band/VA/01.flac":          importer.FoundNoAlbum,
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("reasons\n got %v\nwant %v", got, want)
	}
	var owner string
	_ = w.s.Read().QueryRow(`SELECT t.name FROM files f JOIN versions v ON v.id = f.version_id JOIN titles t ON t.id = v.title_id WHERE f.relative_path = ?`,
		"Main Band/Their Album/09.flac").Scan(&owner)
	if owner != "Second Band" {
		t.Errorf("passed-on file went back to %q", owner)
	}
	ls, _, err := w.svc.LeftoverList(ctx, w.ids["Second Band"])
	if err != nil || len(ls) != 1 || ls[0].Reason != importer.FoundNoTrack || ls[0].Music == nil || ls[0].Music.AlbumName != "Their Album" {
		t.Fatalf("Second Band's list: %+v %v", ls, err)
	}
	// Undo the link by hand: it stays unmatched after a scan.
	var fid int64
	_ = w.s.Read().QueryRow("SELECT id FROM files WHERE relative_path = ?", "Main Band/Short/01 - Alpha.flac").Scan(&fid)
	if n, err := w.svc.UnmatchLeftovers(ctx, w.ids["Main Band"], []int64{fid}, domain.System); err != nil || n != 1 {
		t.Fatalf("unmatch: %d %v", n, err)
	}
	// Leave one alone.
	var vaID int64
	_ = w.s.Read().QueryRow("SELECT id FROM files WHERE relative_path = ?", "Main Band/VA/01.flac").Scan(&vaID)
	if err := w.svc.DismissLeftover(ctx, w.ids["Main Band"], vaID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	if r := reasons(t, w.s)["Main Band/Short/01 - Alpha.flac"]; r != "unmatched" {
		t.Errorf("undone match came back as %q", r)
	}
	total, artists, err := w.svc.UnmatchedMusic(ctx)
	if err != nil || total != 2 || len(artists) != 2 {
		t.Errorf("library count %d %+v %v", total, artists, err)
	}
}
