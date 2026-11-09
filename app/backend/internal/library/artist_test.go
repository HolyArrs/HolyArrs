package library

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// fakeMusic is MusicBrainz over fixed answers.
type fakeMusic struct {
	mu        sync.Mutex
	artists   map[string]metadata.Artist
	pressings map[string][]metadata.Pressing
	err       error
	calls     []string
	// pressErr answers one album's Pressings with an error; cut marks an
	// album's Pressings list as cut short; partial marks every artist's
	// album list as cut short.
	pressErr map[string]error
	cut      map[string]bool
	partial  bool
}

// ArtistIDs: every fake artist links Deezer artist 27 (labDeezerID).
func (f *fakeMusic) ArtistIDs(_ context.Context, mbid string) (metadata.ArtistIDs, error) {
	return metadata.ArtistIDs{Deezer: labDeezerID}, nil
}

// labDeezerID is the Deezer artist the fake MusicBrainz links Lab Band to.
const labDeezerID = 27

func (f *fakeMusic) SearchArtists(_ context.Context, q string) ([]metadata.Artist, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	out := []metadata.Artist{}
	for _, a := range f.artists {
		if strings.Contains(strings.ToLower(a.Name), strings.ToLower(q)) {
			a.Albums = nil
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeMusic) Artist(ctx context.Context, mbid string) (metadata.Artist, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "artist:"+mbid+map[bool]string{true: ":fresh"}[metadata.Fresh(ctx)])
	if f.err != nil {
		return metadata.Artist{}, f.err
	}
	a, ok := f.artists[mbid]
	if !ok {
		return metadata.Artist{}, metadata.ErrArtistNotFound
	}
	a.Albums = append([]metadata.Album(nil), a.Albums...)
	a.Partial = f.partial
	return a, nil
}

func (f *fakeMusic) Pressings(_ context.Context, mbid string) ([]metadata.Pressing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "pressings:"+mbid)
	if f.err != nil {
		return nil, f.err
	}
	if err := f.pressErr[mbid]; err != nil {
		return nil, err
	}
	ps := append([]metadata.Pressing(nil), f.pressings[mbid]...)
	if f.cut[mbid] {
		return ps, metadata.ErrListCut
	}
	return ps, nil
}

const (
	labArtist = "a0000000-0000-0000-0000-000000000001"
	rgOld     = "10000000-0000-0000-0000-000000000001"
	rgMid     = "10000000-0000-0000-0000-000000000002"
	rgEP      = "10000000-0000-0000-0000-000000000003"
	rgSingle  = "10000000-0000-0000-0000-000000000004"
	rgLive    = "10000000-0000-0000-0000-000000000005"
	rgNext    = "10000000-0000-0000-0000-000000000006"
	rgBoot    = "10000000-0000-0000-0000-000000000007"
	rgNew     = "10000000-0000-0000-0000-000000000008"
	relOldUS  = "20000000-0000-0000-0000-000000000001"
	relOldXW  = "20000000-0000-0000-0000-000000000002"
	relOldDx  = "20000000-0000-0000-0000-000000000003"
)

func tracks(prefix string, recs ...string) []metadata.Track {
	out := []metadata.Track{}
	for i, r := range recs {
		disc, n := 1, i+1
		if strings.HasPrefix(r, "2:") {
			disc, n, r = 2, 1, r[2:]
		}
		out = append(out, metadata.Track{ID: prefix + "-" + r, RecordingID: "rec-" + r, Disc: disc, Number: n, Position: i + 1, Name: "Song " + r, LengthMS: 200000})
	}
	return out
}

// labMusic is the lab artist: albums from 2010 to one in 2099, with an EP,
// a single, a live album, a bootleg-only album; Old Days has three
// Pressings (two of three tracks, one deluxe of four on two discs).
func labMusic() *fakeMusic {
	return &fakeMusic{
		artists: map[string]metadata.Artist{labArtist: {MBID: labArtist, Name: "Lab Band", Type: "group", Status: "running", Albums: []metadata.Album{
			{MBID: rgOld, Name: "Old Days", PrimaryType: "album", ReleaseDate: "2010-05-01"},
			{MBID: rgMid, Name: "Middle", PrimaryType: "album", ReleaseDate: "2018"},
			{MBID: rgEP, Name: "Short", PrimaryType: "ep", ReleaseDate: "2019-02"},
			{MBID: rgSingle, Name: "One Song", PrimaryType: "single", ReleaseDate: "2019-06-01"},
			{MBID: rgLive, Name: "Live Night", PrimaryType: "album", SecondaryTypes: []string{"live"}, ReleaseDate: "2020-01-01"},
			{MBID: rgNext, Name: "Next Record", PrimaryType: "album", ReleaseDate: "2099-01-01"},
			{MBID: rgBoot, Name: "Bootleg Only", PrimaryType: "album", ReleaseDate: "2015-01-01"},
		}}},
		pressings: map[string][]metadata.Pressing{
			rgOld: {
				{MBID: relOldUS, Name: "Old Days", Status: "official", Country: "US", Date: "2010-04-20", Formats: []string{"CD"}, Tracks: tracks("us", "1", "2", "3")},
				{MBID: relOldXW, Name: "Old Days", Status: "official", Country: "XW", Date: "2010-05-01", Formats: []string{"Digital Media"}, Tracks: tracks("xw", "1", "2", "3")},
				{MBID: relOldDx, Name: "Old Days", Status: "official", Country: "GB", Date: "2011", Disambiguation: "deluxe", Formats: []string{"CD", "CD"},
					Tracks: tracks("dx", "1", "2", "3", "2:4")},
			},
			rgMid:  {{MBID: "20000000-0000-0000-0000-000000000004", Name: "Middle", Status: "official", Date: "2018", Formats: []string{"CD"}, Tracks: tracks("mid", "m1", "m2")}},
			rgEP:   {{MBID: "20000000-0000-0000-0000-000000000005", Name: "Short", Status: "official", Date: "2019-02", Formats: []string{"Digital Media"}, Tracks: tracks("ep", "e1")}},
			rgBoot: {{MBID: "20000000-0000-0000-0000-000000000006", Name: "Bootleg Only", Status: "bootleg", Date: "2015", Formats: []string{"CD"}, Tracks: tracks("bt", "b1")}},
		},
	}
}

func musicService(t *testing.T) (*Service, *database.Store, *fakeMusic, mediafiles.LibraryFolder) {
	t.Helper()
	s := open(t)
	fm := labMusic()
	svc := &Service{Store: s, Queue: jobs.New(s, jobs.Options{}), Artists: fm, Now: func() time.Time { return now }}
	lf, err := AddFolder(context.Background(), s, FolderInput{Path: t.TempDir(), Kind: "music", Label: "Music"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return svc, s, fm, lf
}

type albumRow struct {
	ID       int64
	Name     string
	Keep     bool
	Included bool
	Dropped  bool
}

func albumsOf(t *testing.T, s *database.Store, titleID int64) map[string]albumRow {
	t.Helper()
	ps, err := database.Select[mediafiles.Part](context.Background(), s.Read(), "parts", "title_id = ? AND kind = 'album'", titleID)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]albumRow{}
	for _, p := range ps {
		out[p.SourceIDs["musicbrainz"]] = albumRow{p.ID, p.Name, p.KeepUpToDate, p.Included, p.DroppedAt != nil}
	}
	return out
}

type trackRow struct {
	ItemID  int64
	Name    string
	Rec     string
	Disc    int
	Number  int
	SlotID  int64
	State   mediafiles.SlotState
	Reason  string
	Keep    bool
	Dropped bool
}

func tracksOf(t *testing.T, s *database.Store, partID int64) []trackRow {
	t.Helper()
	rows, err := s.Read().Query(`SELECT i.id, COALESCE(i.name, ''), COALESCE(i.recording_id, ''), json_extract(i.numbers, '$.disc'), json_extract(i.numbers, '$.track'),
		COALESCE(sl.id, 0), COALESCE(sl.state, ''), COALESCE(sl.last_reason, ''), i.keep_up_to_date, i.dropped_at IS NOT NULL
		FROM items i LEFT JOIN slots sl ON sl.item_id = i.id WHERE i.part_id = ? AND i.kind = 'track'
		ORDER BY json_extract(i.numbers, '$.disc'), json_extract(i.numbers, '$.track'), i.id`, partID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []trackRow
	for rows.Next() {
		var r trackRow
		if err := rows.Scan(&r.ItemID, &r.Name, &r.Rec, &r.Disc, &r.Number, &r.SlotID, &r.State, &r.Reason, &r.Keep, &r.Dropped); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func chosenOf(t *testing.T, s *database.Store, partID int64) domain.AlbumPressing {
	t.Helper()
	ps, err := database.Select[domain.AlbumPressing](context.Background(), s.Read(), "pressings", "part_id = ? AND chosen = 1", partID)
	if err != nil || len(ps) != 1 {
		t.Fatalf("chosen pressing of %d: %v %v", partID, ps, err)
	}
	return ps[0]
}

// TestAddArtistAlbumChoices is the table of album choices: which albums
// are kept up to date right after adding. Included by the default kinds
// (Album and EP, no extra kinds): Old Days 2010, Middle 2018, Short 2019,
// Bootleg Only 2015 (until its Pressings show it has no official one) and
// Next Record 2099; the single and the live album are not included.
func TestAddArtistAlbumChoices(t *testing.T) {
	cases := []struct {
		choice string
		kept   []string
	}{
		{AlbumsAll, []string{rgOld, rgMid, rgEP, rgNext, rgBoot}},
		{AlbumsMissing, []string{rgOld, rgMid, rgEP, rgNext, rgBoot}},
		{AlbumsFuture, []string{rgNext}},
		{AlbumsLatest, []string{rgEP}},
		{AlbumsFirst, []string{rgOld}},
		{AlbumsNone, nil},
	}
	for _, c := range cases {
		t.Run(c.choice, func(t *testing.T) {
			svc, s, _, _ := musicService(t)
			id, jobID, err := svc.AddArtist(context.Background(), "mbid:"+strings.ToUpper(labArtist), ArtistAddOptions{Albums: c.choice}, domain.System)
			if err != nil || jobID == 0 {
				t.Fatalf("add: %v (job %d)", err, jobID)
			}
			var ti mediafiles.Title
			if err := database.Get(context.Background(), s.Read(), "titles", id, &ti); err != nil {
				t.Fatal(err)
			}
			if ti.Kind != "artist" || ti.MetadataSource != "musicbrainz" || ti.SourceIDs["musicbrainz"] != labArtist || *ti.AlbumChoice != c.choice ||
				ti.NewPartsPolicy != "all" || strings.Join(ti.MusicTypes, ",") != "album,ep" || ti.MusicSecondary[0] != "studio" || ti.Status != "running" {
				t.Fatalf("artist %+v", ti)
			}
			al := albumsOf(t, s, id)
			if len(al) != 7 {
				t.Fatalf("albums %v", al)
			}
			for _, rg := range []string{rgSingle, rgLive} {
				if al[rg].Included || al[rg].Keep {
					t.Errorf("%s included %+v", al[rg].Name, al[rg])
				}
			}
			for rg, a := range al {
				want := false
				for _, k := range c.kept {
					want = want || k == rg
				}
				if a.Keep != want {
					t.Errorf("%s kept %v, want %v", a.Name, a.Keep, want)
				}
			}
			var vs []mediafiles.Version
			if vs, err = database.Select[mediafiles.Version](context.Background(), s.Read(), "versions", "title_id = ?", id); err != nil || len(vs) != 1 || vs[0].FolderName != "Lab Band" {
				t.Fatalf("version %+v %v", vs, err)
			}
			var n int
			_ = s.Read().QueryRow("SELECT count(*) FROM items WHERE title_id = ?", id).Scan(&n)
			if n != 0 {
				t.Fatalf("tracks before the refresh: %d", n)
			}
		})
	}
}

func TestAddArtistRefusals(t *testing.T) {
	svc, s, fm, lf := musicService(t)
	ctx := context.Background()
	if _, _, err := svc.AddArtist(ctx, "tmdb:1", ArtistAddOptions{}, domain.System); !errors.Is(err, ErrBadArtistID) {
		t.Fatalf("bad id: %v", err)
	}
	for _, o := range []ArtistAddOptions{{Albums: "existing"}, {NewAlbums: "future"}, {AlbumTypes: []string{"mixtape"}}, {AlbumTypes: []string{}},
		{SecondaryTypes: []string{"bonus"}}, {ReleaseStatuses: []string{"leaked"}}} {
		if _, _, err := svc.AddArtist(ctx, labArtist, o, domain.System); err == nil {
			t.Errorf("%+v accepted", o)
		}
	}
	mf, _ := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "movie", Label: "Movies"}, nil, domain.System)
	if _, _, err := svc.AddArtist(ctx, labArtist, ArtistAddOptions{LibraryFolderID: mf.ID}, domain.System); !errors.Is(err, ErrNotMusicFolder) {
		t.Fatalf("movie folder: %v", err)
	}
	if _, _, err := svc.AddArtist(ctx, "mbid:"+rgOld, ArtistAddOptions{}, domain.System); !errors.Is(err, metadata.ErrArtistNotFound) {
		t.Fatalf("unknown artist: %v", err)
	}
	id, _, err := svc.AddArtist(ctx, labArtist, ArtistAddOptions{LibraryFolderID: lf.ID}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var dup *ArtistAlreadyAddedError
	if _, _, err := svc.AddArtist(ctx, "musicbrainz:"+labArtist, ArtistAddOptions{}, domain.System); !errors.As(err, &dup) || dup.TitleID != id {
		t.Fatalf("second add: %v", err)
	}
	other := "a0000000-0000-0000-0000-000000000002"
	fm.artists[other] = metadata.Artist{MBID: other, Name: "Kept Out", Status: "running"}
	_ = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "never_add", &mediafiles.NeverAdd{Kind: "artist", SourceIDs: map[string]string{"musicbrainz": other}, Name: "Kept Out"}, domain.System)
		return err
	})
	if _, _, err := svc.AddArtist(ctx, other, ArtistAddOptions{}, domain.System); !errors.Is(err, ErrArtistNeverAdd) {
		t.Fatalf("never add: %v", err)
	}
	none := &Service{Store: s, Queue: svc.Queue}
	if _, _, err := none.AddArtist(ctx, labArtist, ArtistAddOptions{}, domain.System); !errors.Is(err, ErrNoMusicSource) {
		t.Fatalf("no source: %v", err)
	}
}

func addLabArtist(t *testing.T, svc *Service, choice string) int64 {
	t.Helper()
	id, _, err := svc.AddArtist(context.Background(), labArtist, ArtistAddOptions{Albums: choice}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RefreshArtist(useCache(context.Background()), id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestRefreshArtistMakesTracks: the refresh after an add fetches the
// Pressings of the included albums only, chooses one per album (Old Days:
// the digital one of the two three-track Pressings), makes one track row
// and one Slot per track of the chosen Pressing (wanted: out since 2010),
// and stops including the bootleg-only album.
func TestRefreshArtistMakesTracks(t *testing.T) {
	svc, s, fm, _ := musicService(t)
	id := addLabArtist(t, svc, AlbumsAll)
	for _, c := range fm.calls {
		if strings.Contains(c, rgSingle) || strings.Contains(c, rgLive) {
			t.Fatalf("pressings fetched for an album not included: %v", fm.calls)
		}
		if strings.HasSuffix(c, ":fresh") {
			t.Fatalf("the refresh right after an add must use the cache: %v", fm.calls)
		}
	}
	al := albumsOf(t, s, id)
	if p := chosenOf(t, s, al[rgOld].ID); p.MBID != relOldXW || p.ChosenBy != "auto" || p.TrackCount != 3 {
		t.Fatalf("Old Days chose %+v", p)
	}
	var n int
	_ = s.Read().QueryRow("SELECT count(*) FROM pressings WHERE part_id = ?", al[rgOld].ID).Scan(&n)
	if n != 3 {
		t.Fatalf("Old Days pressings %d", n)
	}
	tr := tracksOf(t, s, al[rgOld].ID)
	if len(tr) != 3 || tr[0].Name != "Song 1" || tr[2].Rec != "rec-3" || tr[0].State != mediafiles.SlotWanted || tr[0].Reason != "out since 2010-05-01" || !tr[0].Keep {
		t.Fatalf("Old Days tracks %+v", tr)
	}
	if b := al[rgBoot]; b.Included || b.Keep {
		t.Fatalf("bootleg-only album %+v", b)
	}
	if tr := tracksOf(t, s, al[rgBoot].ID); len(tr) != 0 {
		t.Fatalf("bootleg tracks %+v", tr)
	}
	if tr := tracksOf(t, s, al[rgNext].ID); len(tr) != 0 || !al[rgNext].Included {
		t.Fatalf("Next Record has no Pressings yet: %+v", tr)
	}
	var ti mediafiles.Title
	_ = database.Get(context.Background(), s.Read(), "titles", id, &ti)
	if ti.RefreshedAt == nil || !ti.RefreshedAt.Equal(now) {
		t.Fatalf("refreshed_at %v", ti.RefreshedAt)
	}
	sums, err := Albums(context.Background(), s.Read(), id)
	if err != nil || len(sums) != 7 || sums[0].Name != "Next Record" {
		t.Fatalf("albums %+v %v", sums, err)
	}
	for _, a := range sums {
		switch a.MBID {
		case rgOld:
			if a.State != "missing" || a.Tracks != 3 || a.Missing != 3 || a.Pressings != 3 || !strings.Contains(a.Pressing, "XW, Digital Media, 3 tracks") {
				t.Errorf("Old Days summary %+v", a)
			}
		case rgSingle:
			if a.State != "not_included" {
				t.Errorf("single %+v", a)
			}
		case rgNext:
			if a.State != "no_tracks" {
				t.Errorf("Next Record %+v", a)
			}
		}
	}
	// A plain refresh asks MusicBrainz again, past the cache.
	fm.calls = nil
	if _, err := svc.RefreshArtist(context.Background(), id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	if len(fm.calls) == 0 || !strings.HasSuffix(fm.calls[0], ":fresh") {
		t.Fatalf("plain refresh calls %v", fm.calls)
	}
}

func TestChoosePressing(t *testing.T) {
	ps := []domain.AlbumPressing{
		{MBID: "c", Status: "official", Date: "2010-04-20", Formats: []string{"CD"}, TrackCount: 3},
		{MBID: "b", Status: "official", Date: "2010-05-01", Formats: []string{"Digital Media"}, TrackCount: 3},
		{MBID: "a", Status: "official", Date: "2011", Formats: []string{"CD", "CD"}, TrackCount: 4},
		{MBID: "z", Status: "bootleg", Date: "2001", Formats: []string{"Digital Media"}, TrackCount: 9},
	}
	for rule, want := range map[string]string{PressingCommon: "b", PressingMostTracks: "a", PressingEarliest: "c"} {
		if i := ChoosePressing(ps, []string{"official"}, rule); ps[i].MBID != want {
			t.Errorf("%s chose %s, want %s", rule, ps[i].MBID, want)
		}
	}
	// With bootlegs counted, the earliest is the bootleg.
	if i := ChoosePressing(ps, []string{"official", "bootleg"}, PressingEarliest); ps[i].MBID != "z" {
		t.Errorf("earliest with bootlegs %s", ps[i].MBID)
	}
	// None with a counted status: all are considered.
	if i := ChoosePressing(ps[3:], []string{"official"}, PressingCommon); i != 0 {
		t.Errorf("fallback %d", i)
	}
	if ChoosePressing(nil, []string{"official"}, PressingCommon) != -1 {
		t.Error("empty")
	}
}

// TestRefreshArtistKeepsRowsSlotsAndFiles: a later refresh keeps track
// rows, Slots and Files when MusicBrainz renames tracks and gives new
// track ids (matched by recording), removes an album nothing holds, keeps
// and flags an album a File holds, and adds a new album kept up to date
// (new albums: all).
func TestRefreshArtistKeepsRowsSlotsAndFiles(t *testing.T) {
	svc, s, fm, _ := musicService(t)
	ctx := context.Background()
	id := addLabArtist(t, svc, AlbumsAll)
	al := albumsOf(t, s, id)
	before := tracksOf(t, s, al[rgOld].ID)
	// A File holds the first track of Old Days.
	var fileID int64
	err := s.Write(ctx, func(tx *sql.Tx) error {
		var vid int64
		if err := tx.QueryRow("SELECT id FROM versions WHERE title_id = ?", id).Scan(&vid); err != nil {
			return err
		}
		f := mediafiles.File{SlotIDs: []int64{before[0].SlotID}, VersionID: vid, RelativePath: "Lab Band/Old Days/01 Song 1.flac", Languages: []string{},
			Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		if _, err := database.Insert(tx, "files", &f, domain.System); err != nil {
			return err
		}
		fileID = f.ID
		_, err := tx.Exec("UPDATE slots SET file_id = ?, state = 'in_library' WHERE id = ?", f.ID, before[0].SlotID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// MusicBrainz now: new track ids and a new name on Old Days (same
	// recordings), Middle gone, a new album.
	fm.mu.Lock()
	ps := fm.pressings[rgOld]
	ps[1].Tracks = tracks("xw2", "1", "2", "3")
	ps[1].Tracks[1].Name = "Song Two (Remastered)"
	a := fm.artists[labArtist]
	var albums []metadata.Album
	for _, x := range a.Albums {
		if x.MBID != rgMid {
			albums = append(albums, x)
		}
	}
	a.Albums = append(albums, metadata.Album{MBID: rgNew, Name: "Brand New", PrimaryType: "album", ReleaseDate: "2026-09-01"})
	fm.artists[labArtist] = a
	fm.pressings[rgNew] = []metadata.Pressing{{MBID: "20000000-0000-0000-0000-000000000009", Name: "Brand New", Status: "official", Date: "2026-09-01",
		Formats: []string{"Digital Media"}, Tracks: tracks("bn", "n1")}}
	fm.mu.Unlock()
	out, err := svc.RefreshArtist(ctx, id, domain.System, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.AlbumsAdded != 1 || out.AlbumsRemoved != 1 || out.TracksAdded != 1 {
		t.Fatalf("refresh %+v", out.ArtistSync)
	}
	after := tracksOf(t, s, al[rgOld].ID)
	for i := range before {
		if after[i].ItemID != before[i].ItemID || after[i].SlotID != before[i].SlotID {
			t.Fatalf("track %d row changed: %+v -> %+v", i, before[i], after[i])
		}
	}
	if after[1].Name != "Song Two (Remastered)" || after[0].State != mediafiles.SlotInLibrary {
		t.Fatalf("after %+v", after)
	}
	var sid int64
	_ = s.Read().QueryRow("SELECT COALESCE(file_id, 0) FROM slots WHERE id = ?", before[0].SlotID).Scan(&sid)
	if sid != fileID {
		t.Fatalf("file link lost: %d", sid)
	}
	now2 := albumsOf(t, s, id)
	if _, ok := now2[rgMid]; ok {
		t.Fatal("Middle is no longer listed and nothing holds it; it should be gone")
	}
	if nw := now2[rgNew]; !nw.Keep || !nw.Included {
		t.Fatalf("new album %+v", nw)
	}
	// Old Days leaves MusicBrainz: a File holds it, so it stays, flagged.
	fm.mu.Lock()
	a = fm.artists[labArtist]
	albums = nil
	for _, x := range a.Albums {
		if x.MBID != rgOld {
			albums = append(albums, x)
		}
	}
	a.Albums = albums
	fm.artists[labArtist] = a
	fm.mu.Unlock()
	out, err = svc.RefreshArtist(ctx, id, domain.System, nil)
	if err != nil || out.AlbumsDropped != 1 {
		t.Fatalf("drop %+v %v", out.ArtistSync, err)
	}
	if o := albumsOf(t, s, id)[rgOld]; !o.Dropped || o.ID != al[rgOld].ID {
		t.Fatalf("Old Days %+v", o)
	}
	if tr := tracksOf(t, s, al[rgOld].ID); len(tr) != 3 || tr[0].SlotID != before[0].SlotID {
		t.Fatalf("held album tracks %+v", tr)
	}
}

// TestNewAlbumsPolicy: with new albums "none" a refresh adds a new album
// not kept up to date; "new" keeps one released after the artist was added.
func TestNewAlbumsPolicy(t *testing.T) {
	for _, c := range []struct {
		policy, date string
		keep         bool
	}{{"none", "2099-01-01", false}, {"new", "2099-01-01", true}, {"new", "2001-01-01", false}, {"all", "2001-01-01", true}} {
		svc, s, fm, _ := musicService(t)
		id, _, err := svc.AddArtist(context.Background(), labArtist, ArtistAddOptions{NewAlbums: c.policy}, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		fm.mu.Lock()
		a := fm.artists[labArtist]
		a.Albums = append(a.Albums, metadata.Album{MBID: rgNew, Name: "Brand New", PrimaryType: "album", ReleaseDate: c.date})
		fm.artists[labArtist] = a
		fm.mu.Unlock()
		if _, err := svc.RefreshArtist(context.Background(), id, domain.System, nil); err != nil {
			t.Fatal(err)
		}
		if got := albumsOf(t, s, id)[rgNew]; got.Keep != c.keep {
			t.Errorf("new_albums %s, album of %s: kept %v, want %v", c.policy, c.date, got.Keep, c.keep)
		}
	}
}

// TestSetAlbumPressing: choosing the deluxe Pressing re-maps the tracks
// (three kept by recording, one new with its Slot), survives a refresh,
// and switching Keep up to date reaches the tracks; an album that is not
// included cannot be kept up to date.
func TestSetAlbumPressing(t *testing.T) {
	svc, s, _, _ := musicService(t)
	ctx := context.Background()
	id := addLabArtist(t, svc, AlbumsAll)
	al := albumsOf(t, s, id)
	before := tracksOf(t, s, al[rgOld].ID)
	ps, _ := database.Select[domain.AlbumPressing](ctx, s.Read(), "pressings", "part_id = ? AND mbid = ?", al[rgOld].ID, relOldDx)
	if _, err := svc.SetAlbum(ctx, al[rgOld].ID, nil, ps[0].ID, domain.System); err != nil {
		t.Fatal(err)
	}
	after := tracksOf(t, s, al[rgOld].ID)
	if len(after) != 4 || after[3].Disc != 2 || after[3].State != mediafiles.SlotWanted {
		t.Fatalf("deluxe tracks %+v", after)
	}
	for i := 0; i < 3; i++ {
		if after[i].ItemID != before[i].ItemID {
			t.Fatalf("track %d lost its row", i)
		}
	}
	if _, err := svc.RefreshArtist(ctx, id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	if p := chosenOf(t, s, al[rgOld].ID); p.MBID != relOldDx || p.ChosenBy != "person" {
		t.Fatalf("a person's Pressing after a refresh: %+v", p)
	}
	off := false
	if _, err := svc.SetAlbum(ctx, al[rgOld].ID, &off, 0, domain.System); err != nil {
		t.Fatal(err)
	}
	for _, tr := range tracksOf(t, s, al[rgOld].ID) {
		if tr.Keep {
			t.Fatalf("track still kept %+v", tr)
		}
	}
	on := true
	if _, err := svc.SetAlbum(ctx, al[rgLive].ID, &on, 0, domain.System); !errors.Is(err, ErrAlbumNotIncluded) {
		t.Fatalf("live album: %v", err)
	}
	other, _ := database.Select[domain.AlbumPressing](ctx, s.Read(), "pressings", "part_id = ?", al[rgMid].ID)
	if _, err := svc.SetAlbum(ctx, al[rgOld].ID, nil, other[0].ID, domain.System); !errors.Is(err, ErrNoSuchPressing) {
		t.Fatalf("another album's pressing: %v", err)
	}
}

// TestAlbumGate: an album whose release date is ahead keeps its track
// Slots not out yet; the gate watcher opens them once the date passes, and
// a refresh moving the date later closes them again.
func TestAlbumGate(t *testing.T) {
	svc, s, fm, _ := musicService(t)
	ctx := context.Background()
	fm.mu.Lock()
	a := fm.artists[labArtist]
	for i := range a.Albums {
		if a.Albums[i].MBID == rgNext {
			a.Albums[i].ReleaseDate = "2026-10-14"
		}
	}
	fm.artists[labArtist] = a
	fm.pressings[rgNext] = []metadata.Pressing{{MBID: "20000000-0000-0000-0000-00000000000a", Name: "Next Record", Status: "official", Date: "2026-10-14",
		Formats: []string{"Digital Media"}, Tracks: tracks("nx", "x1", "x2")}}
	fm.mu.Unlock()
	id := addLabArtist(t, svc, AlbumsAll)
	part := albumsOf(t, s, id)[rgNext].ID
	if tr := tracksOf(t, s, part); len(tr) != 2 || tr[0].State != mediafiles.SlotNotOutYet || tr[0].Reason != "out on 2026-10-14" {
		t.Fatalf("before the date %+v", tr)
	}
	if n, err := svc.OpenTrackGates(ctx); err != nil || n != 0 {
		t.Fatalf("too early: %d %v", n, err)
	}
	later := now.Add(13 * time.Hour)
	svc.Now = func() time.Time { return later }
	if n, err := svc.OpenTrackGates(ctx); err != nil || n != 2 {
		t.Fatalf("opened %d %v", n, err)
	}
	if tr := tracksOf(t, s, part); tr[0].State != mediafiles.SlotWanted {
		t.Fatalf("after the date %+v", tr)
	}
	fm.mu.Lock()
	a = fm.artists[labArtist]
	for i := range a.Albums {
		if a.Albums[i].MBID == rgNext {
			a.Albums[i].ReleaseDate = "2027-01-01"
		}
	}
	fm.artists[labArtist] = a
	fm.mu.Unlock()
	out, err := svc.RefreshArtist(ctx, id, domain.System, nil)
	if err != nil || len(out.Closed) != 2 {
		t.Fatalf("moved later: %+v %v", out, err)
	}
	if tr := tracksOf(t, s, part); tr[0].State != mediafiles.SlotNotOutYet {
		t.Fatalf("moved later %+v", tr)
	}
}

func TestArtistDroppedByMusicBrainz(t *testing.T) {
	svc, s, fm, _ := musicService(t)
	id := addLabArtist(t, svc, AlbumsAll)
	fm.mu.Lock()
	delete(fm.artists, labArtist)
	fm.mu.Unlock()
	out, err := svc.RefreshArtist(context.Background(), id, domain.System, nil)
	if err != nil || !out.Dropped {
		t.Fatalf("%+v %v", out, err)
	}
	var ti mediafiles.Title
	_ = database.Get(context.Background(), s.Read(), "titles", id, &ti)
	if ti.DroppedAt == nil || ti.State != "active" || len(albumsOf(t, s, id)) != 7 {
		t.Fatalf("dropped artist %+v", ti)
	}
}

func TestShouldRefreshArtist(t *testing.T) {
	ago := func(d time.Duration) *time.Time { v := now.Add(-d); return &v }
	day := 24 * time.Hour
	cases := []struct {
		refreshed *time.Time
		status    string
		newest    *time.Time
		want      bool
	}{
		{nil, "running", nil, true},
		{ago(31 * day), "ended", nil, true},
		{ago(11 * time.Hour), "running", ago(day), false},
		{ago(3 * day), "running", nil, true},
		{ago(day), "running", nil, false},
		{ago(3 * day), "ended", ago(10 * day), true},
		{ago(3 * day), "ended", ago(40 * day), false},
	}
	for i, c := range cases {
		ti := mediafiles.Title{RefreshedAt: c.refreshed, Status: c.status}
		if got, why := ShouldRefreshArtist(ti, c.newest, now); got != c.want {
			t.Errorf("case %d: %v (%s), want %v", i, got, why, c.want)
		}
	}
}

func TestMusicSettings(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	d, err := ReadMusicDefaults(ctx, s.Read())
	if err != nil || d.AlbumChoice != "all" || d.NewAlbums != "all" || strings.Join(d.AlbumTypes, ",") != "album,ep" || d.PressingRule != "common" ||
		d.MusicBrainzURL != "" || d.CacheHours != 24 || d.ReleaseStatuses[0] != "official" || d.SecondaryTypes[0] != "studio" {
		t.Fatalf("defaults %+v %v", d, err)
	}
	d.AlbumChoice, d.NewAlbums, d.AlbumTypes, d.SecondaryTypes = "latest", "new", []string{"Album", "single", "album"}, []string{"studio", "Live"}
	d.PressingRule, d.MusicBrainzURL, d.CacheHours = "most_tracks", "http://mirror.lan:5000/", 72
	if err := SetMusicDefaults(ctx, s, d, domain.System); err != nil {
		t.Fatal(err)
	}
	back, _ := ReadMusicDefaults(ctx, s.Read())
	if back.AlbumChoice != "latest" || back.NewAlbums != "new" || strings.Join(back.AlbumTypes, ",") != "album,single" || back.SecondaryTypes[1] != "live" ||
		back.PressingRule != "most_tracks" || back.MusicBrainzURL != "http://mirror.lan:5000" || back.CacheHours != 72 {
		t.Fatalf("saved %+v", back)
	}
	for _, bad := range []func(*MusicDefaults){
		func(d *MusicDefaults) { d.AlbumChoice = "existing" },
		func(d *MusicDefaults) { d.NewAlbums = "future" },
		func(d *MusicDefaults) { d.AlbumTypes = []string{} },
		func(d *MusicDefaults) { d.SecondaryTypes = []string{"bonus"} },
		func(d *MusicDefaults) { d.ReleaseStatuses = nil },
		func(d *MusicDefaults) { d.PressingRule = "newest" },
		func(d *MusicDefaults) { d.MusicBrainzURL = "ftp://x" },
		func(d *MusicDefaults) { d.MusicBrainzURL = "https://user:pw@mb.example" },
		func(d *MusicDefaults) { d.CacheHours = 0 },
	} {
		x := back
		x.AlbumTypes = append([]string(nil), back.AlbumTypes...)
		bad(&x)
		if err := SetMusicDefaults(ctx, s, x, domain.System); err == nil {
			t.Errorf("accepted %+v", x)
		}
	}
}

// TestArtistRefreshPlan: the 12-hour run skips an artist fetched just now
// and takes one fetched three days ago (still active); by hand takes all.
func TestArtistRefreshPlan(t *testing.T) {
	svc, s, _, _ := musicService(t)
	id := addLabArtist(t, svc, AlbumsAll)
	ctx := context.Background()
	if ids, _, err := svc.ArtistRefreshPlan(ctx, false); err != nil || len(ids) != 0 {
		t.Fatalf("fresh artist planned: %v %v", ids, err)
	}
	if ids, why, _ := svc.ArtistRefreshPlan(ctx, true); len(ids) != 1 || why[id] != "by hand" {
		t.Fatalf("by hand %v %v", ids, why)
	}
	_ = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET refreshed_at = ? WHERE id = ?", database.FormatTime(now.Add(-72*time.Hour)), id)
		return err
	})
	if ids, why, _ := svc.ArtistRefreshPlan(ctx, false); len(ids) != 1 || why[id] != "still active" {
		t.Fatalf("three days old %v %v", ids, why)
	}
}

// TestRefreshLeavesAlbumWhenPressingsFail: an album whose Pressings lookup
// fails (not found, a plain error) or comes back empty keeps its Pressings,
// chosen Pressing, tracks and Slots untouched; a plain error stops the
// refresh.
func TestRefreshLeavesAlbumWhenPressingsFail(t *testing.T) {
	for _, c := range []struct {
		name  string
		err   error
		empty bool
		fails bool
	}{{"not found", metadata.ErrArtistNotFound, false, false}, {"empty", nil, true, false}, {"plain error", errors.New("MusicBrainz could not answer"), false, true}} {
		t.Run(c.name, func(t *testing.T) {
			svc, s, fm, _ := musicService(t)
			ctx := context.Background()
			id := addLabArtist(t, svc, AlbumsAll)
			al := albumsOf(t, s, id)
			ps, _ := database.Select[domain.AlbumPressing](ctx, s.Read(), "pressings", "part_id = ? AND mbid = ?", al[rgOld].ID, relOldDx)
			if _, err := svc.SetAlbum(ctx, al[rgOld].ID, nil, ps[0].ID, domain.System); err != nil {
				t.Fatal(err)
			}
			before := tracksOf(t, s, al[rgOld].ID)
			fm.mu.Lock()
			if c.empty {
				fm.pressings[rgOld] = nil
			} else {
				fm.pressErr = map[string]error{rgOld: c.err}
			}
			fm.mu.Unlock()
			_, err := svc.RefreshArtist(ctx, id, domain.System, nil)
			if c.fails != (err != nil) {
				t.Fatalf("refresh: %v", err)
			}
			after := tracksOf(t, s, al[rgOld].ID)
			if len(after) != len(before) || len(after) != 4 {
				t.Fatalf("tracks %+v -> %+v", before, after)
			}
			for i := range before {
				if after[i].ItemID != before[i].ItemID || after[i].SlotID != before[i].SlotID || after[i].Dropped {
					t.Fatalf("track %d changed: %+v -> %+v", i, before[i], after[i])
				}
			}
			if p := chosenOf(t, s, al[rgOld].ID); p.MBID != relOldDx || p.ChosenBy != "person" {
				t.Fatalf("chosen %+v", p)
			}
			var n int
			_ = s.Read().QueryRow("SELECT count(*) FROM pressings WHERE part_id = ?", al[rgOld].ID).Scan(&n)
			if n != 3 {
				t.Fatalf("pressings %d", n)
			}
		})
	}
}

// TestRefreshAddsOnlyWhenListsAreCut: an album list or Pressings list the
// source cut short adds and updates rows but removes nothing.
func TestRefreshAddsOnlyWhenListsAreCut(t *testing.T) {
	svc, s, fm, _ := musicService(t)
	ctx := context.Background()
	id := addLabArtist(t, svc, AlbumsAll)
	al := albumsOf(t, s, id)
	fm.mu.Lock()
	a := fm.artists[labArtist]
	var albums []metadata.Album
	for _, x := range a.Albums {
		if x.MBID != rgMid {
			albums = append(albums, x)
		}
	}
	a.Albums = append(albums, metadata.Album{MBID: rgNew, Name: "Brand New", PrimaryType: "album", ReleaseDate: "2026-09-01"})
	fm.artists[labArtist] = a
	fm.partial = true
	fm.pressings[rgOld] = fm.pressings[rgOld][1:2] // only the digital one, renamed
	fm.pressings[rgOld][0].Name = "Old Days (digital)"
	fm.cut = map[string]bool{rgOld: true}
	fm.mu.Unlock()
	out, err := svc.RefreshArtist(ctx, id, domain.System, nil)
	if err != nil {
		t.Fatal(err)
	}
	now2 := albumsOf(t, s, id)
	if _, ok := now2[rgMid]; !ok || out.AlbumsRemoved != 0 || out.AlbumsDropped != 0 {
		t.Fatalf("a cut album list removed Middle: %+v", out.ArtistSync)
	}
	if _, ok := now2[rgNew]; !ok || out.AlbumsAdded != 1 {
		t.Fatalf("new album not added: %+v", out.ArtistSync)
	}
	var n int
	_ = s.Read().QueryRow("SELECT count(*) FROM pressings WHERE part_id = ?", al[rgOld].ID).Scan(&n)
	if n != 3 {
		t.Fatalf("a cut Pressings list removed Pressings: %d left", n)
	}
	if p := chosenOf(t, s, al[rgOld].ID); p.Name != "Old Days (digital)" {
		t.Fatalf("listed Pressing not updated: %+v", p)
	}
}

// TestPartialDatesOpenAtTheEnd: an album dated only by year opens at the
// end of that year, one dated by month at the end of that month.
func TestPartialDatesOpenAtTheEnd(t *testing.T) {
	at := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		date, want string
		open       bool
	}{{"2026", "2026-12-31", false}, {"2026-11", "2026-11-30", false}, {"2026-10", "2026-10-31", false}, {"2026-02", "2026-02-28", true},
		{"2025", "2025-12-31", true}, {"2026-10-13", "2026-10-13", true}, {"2026-10-14", "2026-10-14", false}} {
		d := musicDate(c.date)
		if d == nil || d.Format("2006-01-02") != c.want {
			t.Errorf("%s read as %v, want %s", c.date, d, c.want)
			continue
		}
		if open, _ := AlbumGate(d, at); open != c.open {
			t.Errorf("%s open %v, want %v", c.date, open, c.open)
		}
	}
}
