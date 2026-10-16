package importer

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

const albumRel = "Lab Band - First Light (2020) [FLAC]"

// albumWorld is an artist with one album of three tracks (or two discs of
// two when discs is 2) in one Version, and a finished Download filling
// every track.
type albumWorld struct {
	s      *store.Store
	dl     string
	lib    string
	dlID   int64
	relID  int64
	slots  []int64 // in track order
	tracks []int64
}

func albumSetup(t *testing.T, discs int, files ...string) albumWorld {
	t.Helper()
	s, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	// Test files are tiny stand-ins: the size floors are tested on their own.
	setFake(t, s, "import_size_floors", "false")
	w := albumWorld{s: s, dl: t.TempDir(), lib: t.TempDir()}
	var seen []domain.SeenFile
	for _, f := range files {
		write(t, filepath.Join(w.dl, albumRel, filepath.FromSlash(f)), 2048)
		seen = append(seen, domain.SeenFile{Name: albumRel + "/" + f, Size: 2048})
	}
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		_ = tx.QueryRow("SELECT id FROM pick_rules WHERE name = 'Lossless first' ").Scan(&pr)
		lf := &domain.LibraryFolder{Path: w.lib, Kind: "music", Label: "Music", Health: "ok", SameDiskAs: []int64{}}
		if _, err := store.Insert(tx, "library_folders", lf, domain.System); err != nil {
			return err
		}
		ti := &domain.Title{Kind: "artist", GateDriver: "none", Name: "Lab Band", SortName: "lab band", Aliases: []string{},
			SourceIDs: map[string]string{}, MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released",
			MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "running", State: "active", KeepUpToDate: true}
		if _, err := store.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		v := &domain.Version{TitleID: ti.ID, Label: "Main", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Lab Band", IsMain: true, KeepUpToDate: true}
		if _, err := store.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		rd := time.Date(2020, 5, 1, 0, 0, 0, 0, time.UTC)
		p := &domain.Part{TitleID: ti.ID, Kind: "album", Name: "First Light", ReleaseDate: &rd, KeepUpToDate: true, SourceIDs: map[string]string{},
			Pressings: []domain.Pressing{}, SecondaryTypes: []string{}, Included: true}
		if _, err := store.Insert(tx, "parts", p, domain.System); err != nil {
			return err
		}
		names := []string{"Dawn", "Glass River", "Night Train", "Last Call"}
		n := 3
		if discs == 2 {
			n = 4
		}
		for i := 0; i < n; i++ {
			disc, num, name := 1, i+1, names[i]
			if discs == 2 {
				disc, num = i/2+1, i%2+1
			}
			it := &domain.Item{TitleID: ti.ID, PartID: &p.ID, Kind: "track", Numbers: domain.Numbers{Disc: &disc, Track: &num}, Name: &name,
				KeepUpToDate: true, SourceIDs: map[string]string{}}
			if _, err := store.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			sl := &domain.Slot{ItemID: it.ID, VersionID: v.ID, State: domain.SlotDownloading}
			if _, err := store.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			w.slots, w.tracks = append(w.slots, sl.ID), append(w.tracks, it.ID)
		}
		d := &domain.Downloader{Name: "q", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://q", Category: "holyarrs", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := store.Insert(tx, "downloaders", d, domain.System); err != nil {
			return err
		}
		src := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://x", Kinds: []string{}, Categories: map[string][]int{}, State: "enabled", Extra: map[string]string{}}
		if _, err := store.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		r := &domain.Release{SourceID: src.ID, Name: albumRel, LinkRef: "http://x/1.torrent", Protocol: "torrent", Flags: []string{}, Parsed: map[string]any{},
			MatchedSlotIDs: w.slots, Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		if _, err := store.Insert(tx, "releases", r, domain.System); err != nil {
			return err
		}
		w.relID = r.ID
		dl := &domain.Download{DownloaderID: d.ID, ReleaseID: &r.ID, SlotIDs: w.slots, ExternalID: "album", Progress: 1, State: "finished",
			FilesSeen: seen, SavePath: w.dl, StartedBy: domain.System}
		if _, err := store.Insert(tx, "downloads", dl, domain.System); err != nil {
			return err
		}
		w.dlID = dl.ID
		_, err := tx.Exec("UPDATE slots SET active_download_id = ?", dl.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func (w albumWorld) state(t *testing.T, i int) domain.SlotState {
	t.Helper()
	var sl domain.Slot
	if err := store.Get(context.Background(), w.s.Read(), "slots", w.slots[i], &sl); err != nil {
		t.Fatal(err)
	}
	return sl.State
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// A whole album: every track is placed as Artist/Album (Year)/NN - Title,
// the cover, log and cue are left alone, and every track Slot is in the
// library.
func TestAlbumImportPlacesEveryTrack(t *testing.T) {
	w := albumSetup(t, 1, "01 - Dawn.flac", "02 - Glass River.flac", "03 - Night Train.flac", "cover.jpg", "album.log", "album.cue")
	im := &Importer{Store: w.s}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "done" || imp.Kind != "tracks" {
		t.Fatalf("import %s %s: %+v", imp.State, imp.Kind, imp.Plan)
	}
	for _, f := range []string{"01 - Dawn.flac", "02 - Glass River.flac", "03 - Night Train.flac"} {
		if !exists(filepath.Join(w.lib, "Lab Band", "First Light (2020)", f)) {
			t.Fatalf("%s not placed", f)
		}
	}
	if exists(filepath.Join(w.lib, "Lab Band", "First Light (2020)", "cover.jpg")) {
		t.Fatal("an extra file was placed")
	}
	for i := range w.slots {
		if st := w.state(t, i); st != domain.SlotInLibrary {
			t.Fatalf("track %d: %s", i+1, st)
		}
	}
	var n int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM files WHERE state = 'present'").Scan(&n)
	if n != 3 {
		t.Fatalf("%d files", n)
	}
}

// A partial album settles per track: the tracks it holds are placed; the
// missing one turns the Release down for that track only and is looked for
// again.
func TestPartialAlbumSettlesPerTrack(t *testing.T) {
	w := albumSetup(t, 1, "01 - Dawn.flac", "03 - Night Train.flac")
	var again []int64
	im := &Importer{Store: w.s, OnRejected: func(_ context.Context, id int64) { again = append(again, id) }}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "partly_done" {
		t.Fatalf("state %s", imp.State)
	}
	if w.state(t, 0) != domain.SlotInLibrary || w.state(t, 2) != domain.SlotInLibrary || w.state(t, 1) != domain.SlotWanted {
		t.Fatalf("states %s %s %s", w.state(t, 0), w.state(t, 1), w.state(t, 2))
	}
	if len(again) != 1 || again[0] != w.slots[1] {
		t.Fatalf("searched again: %v", again)
	}
	var down string
	_ = w.s.Read().QueryRow("SELECT json_extract(verdict, '$.turned_down_slot_ids') FROM releases WHERE id = ?", w.relID).Scan(&down)
	if down != "["+strconv.FormatInt(w.slots[1], 10)+"]" {
		t.Fatalf("turned down for %s", down)
	}
}

// Two discs in CD1 and CD2 folders with the same track numbers go to their
// own tracks and are named D-NN.
func TestAlbumImportMultiDisc(t *testing.T) {
	w := albumSetup(t, 2, "CD1/01 - Dawn.flac", "CD1/02 - Glass River.flac", "CD2/01 - Night Train.flac", "CD2/02 - Last Call.flac")
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("%v %+v", err, imp)
	}
	if !exists(filepath.Join(w.lib, "Lab Band", "First Light (2020)", "2-01 - Night Train.flac")) {
		t.Fatal("disc 2 track 1 not placed as 2-01")
	}
}

// An album that does not fit whole waits: nothing is placed.
func TestAlbumImportNeedsSpaceForWholeAlbum(t *testing.T) {
	w := albumSetup(t, 1, "01 - Dawn.flac", "02 - Glass River.flac", "03 - Night Train.flac")
	fakeDisk(t, 100*mb+2*2048+1) // room for two of the three tracks
	sameDiskOf = func(string, string) bool { return false }
	defer func() { sameDiskOf = sameDisk }()
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(w.lib, "Lab Band")) {
		t.Fatal("part of the album was placed")
	}
	for i := range w.slots {
		if st := w.state(t, i); st != domain.SlotNeedsADecision {
			t.Fatalf("track %d: %s (%s)", i+1, st, imp.State)
		}
	}
}

// A run cut off after placing resumes and finishes the album.
func TestAlbumImportResumesAfterCrash(t *testing.T) {
	w := albumSetup(t, 1, "01 - Dawn.flac", "02 - Glass River.flac", "03 - Night Train.flac")
	im := &Importer{Store: w.s}
	crashAt = func(stage string) error {
		if stage == "placed" {
			return errors.New("power cut")
		}
		return nil
	}
	if _, err := im.Import(context.Background(), w.dlID, false, domain.System); err == nil {
		t.Fatal("no crash")
	}
	crashAt = nil
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("%v %+v", err, imp)
	}
	var n int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM files").Scan(&n)
	if n != 3 {
		t.Fatalf("%d files after resume", n)
	}
}

// The stranded sweep queues finished album downloads whose tracks wait.
func TestStrandedAlbumDownloadsAreQueued(t *testing.T) {
	w := albumSetup(t, 1, "01 - Dawn.flac")
	var queued []int64
	n, err := (&Importer{Store: w.s}).RequeueStranded(context.Background(), func(_ context.Context, id int64) { queued = append(queued, id) })
	if err != nil || n != 1 || queued[0] != w.dlID {
		t.Fatalf("%d %v %v", n, err, queued)
	}
	defer func(old bool) { canImportAlbums = old }(canImportAlbums)
	canImportAlbums = false
	if n, _ := (&Importer{Store: w.s}).RequeueStranded(context.Background(), func(context.Context, int64) {}); n != 0 {
		t.Fatalf("album import off: %d", n)
	}
	if _, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System); !errors.Is(err, ErrAlbumImport) {
		t.Fatalf("album import off: %v", err)
	}
}

func TestPlanTracksMatching(t *testing.T) {
	at := AlbumTarget{LibraryPath: "/lib", ArtistFolder: "A", Album: "B", Tracks: []Track{
		{ItemID: 1, Disc: 1, Number: 1, Name: "Dawn", SlotID: 11, Wanted: true},
		{ItemID: 2, Disc: 1, Number: 2, Name: "Glass River", SlotID: 12, Wanted: true},
		{ItemID: 3, Disc: 1, Number: 3, Name: "Night Train", SlotID: 13},
	}}
	cands := []Candidate{
		{Path: "/d/1-01 Dawn.mp3", Size: 10},
		{Path: "/d/01 - Dawn.flac", Size: 900},
		{Path: "/d/Lab Band - B - 02 - Glass River.flac", Size: 900},
		{Path: "/d/Night Train.flac", Size: 900},
		{Path: "/d/07 - Hidden Bonus.flac", Size: 900},
		{Path: "/d/folder.jpg", Size: 9},
		{Path: "/d/setup.exe", Size: 9},
		{Path: "/d/Untitled.flac", Size: 9},
	}
	plan := PlanTracks(cands, at)
	want := []string{"skip", "hardlink", "hardlink", "skip", "skip", "skip", "refuse", "skip"}
	for i, st := range plan {
		if st.Method != want[i] {
			t.Errorf("%s: %s, want %s (%v)", st.SourceFile, st.Method, want[i], st.Why)
		}
	}
	if !strings.Contains(strings.Join(plan[4].Why, " "), "bonus") || !strings.Contains(strings.Join(plan[3].Why, " "), "not filled") {
		t.Errorf("why: %v / %v", plan[4].Why, plan[3].Why)
	}
	if !slices.Equal(plan[2].TargetSlotIDs, []int64{12}) || filepath.ToSlash(plan[1].TargetPath) != "/lib/A/B/01 - Dawn.flac" {
		t.Errorf("targets %v %s", plan[2].TargetSlotIDs, plan[1].TargetPath)
	}
}
