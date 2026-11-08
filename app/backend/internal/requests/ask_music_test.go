package requests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// fakeArtists answers one artist with its albums.
type fakeArtists struct {
	artists   map[string]metadata.Artist
	pressings map[string][]metadata.Pressing
}

func (f *fakeArtists) SearchArtists(context.Context, string) ([]metadata.Artist, error) {
	return nil, nil
}

func (f *fakeArtists) Artist(_ context.Context, mbid string) (metadata.Artist, error) {
	if a, ok := f.artists[mbid]; ok {
		return a, nil
	}
	return metadata.Artist{}, metadata.ErrArtistNotFound
}

func (f *fakeArtists) Pressings(_ context.Context, mbid string) ([]metadata.Pressing, error) {
	return f.pressings[mbid], nil
}

const (
	mArtist = "a0000000-0000-0000-0000-0000000000aa"
	mFirst  = "10000000-0000-0000-0000-0000000000aa"
	mSecond = "10000000-0000-0000-0000-0000000000ab"
	mLive   = "10000000-0000-0000-0000-0000000000ac"
	mSoon   = "10000000-0000-0000-0000-0000000000ad"
)

// musicRig is the asks rig with a music Library folder and an artist with
// two albums, a live album (an extra kind the defaults leave out) and an
// EP far ahead.
func musicRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t)
	r.svc.Library.Artists = &fakeArtists{artists: map[string]metadata.Artist{mArtist: {MBID: mArtist, Name: "Lab Band", Status: "running", Albums: []metadata.Album{
		{MBID: mSecond, Name: "Second Wind", PrimaryType: "album", ReleaseDate: "2022-05-01"},
		{MBID: mFirst, Name: "First Light", PrimaryType: "album", ReleaseDate: "2020-03-06"},
		{MBID: mLive, Name: "Live Night", PrimaryType: "album", SecondaryTypes: []string{"live"}, ReleaseDate: "2021"},
		{MBID: mSoon, Name: "Soon", PrimaryType: "ep", ReleaseDate: "2099-01-01"},
	}}}}
	if _, err := library.AddFolder(context.Background(), r.st, library.FolderInput{Path: t.TempDir(), Kind: "music", Label: "Music"}, nil, domain.System); err != nil {
		t.Fatal(err)
	}
	return r
}

func albumKeeps(t *testing.T, st *database.Store, titleID int64) map[string]bool {
	t.Helper()
	ps, err := database.Select[mediafiles.Part](context.Background(), st.Read(), "parts", "title_id = ? AND kind = 'album'", titleID)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, p := range ps {
		out[p.Name] = p.KeepUpToDate
	}
	return out
}

func TestArtistAskAlbumsJoinApproveAndAskAgain(t *testing.T) {
	r := musicRig(t)
	ctx := context.Background()

	res, err := r.svc.AskArtist(ctx, r.ana, "mbid:"+mArtist, []string{mLive})
	if err != nil {
		t.Fatal(err)
	}
	a := res.Ask
	if a.Kind != "artist" || a.State != domain.AskAsked || a.Name != "Lab Band" || len(a.Albums) != 1 || a.Albums[0].Name != "Live Night" || a.Albums[0].Year != 2021 || a.Ref != "mbid:"+mArtist {
		t.Fatalf("ask %+v", a)
	}
	if e := hasEvent(r.events(t), domain.EventAskMade); e == nil || e.Summary != "ana asked for Lab Band, Live Night" {
		t.Fatalf("event %+v", e)
	}
	if _, err := r.svc.AskArtist(ctx, r.bo, "mbid:"+mArtist, []string{"10000000-0000-0000-0000-0000000000ff"}); !errors.Is(err, ErrNoSuchAlbum) {
		t.Fatalf("unknown album: %v", err)
	}
	if _, err := r.svc.AskArtist(ctx, r.bo, "Lab Band", nil); !errors.Is(err, library.ErrBadArtistID) {
		t.Fatalf("a name is not an id: %v", err)
	}
	// bo asks for the same album: he joins ana's ask.
	res, err = r.svc.AskArtist(ctx, r.bo, mArtist, []string{mLive})
	if err != nil || !res.Joined || res.Ask.ID != a.ID {
		t.Fatalf("join %+v %v", res, err)
	}
	// bo also wants First Light while the ask waits: the ask grows.
	res, err = r.svc.AskArtist(ctx, r.bo, "mbid:"+mArtist, []string{mFirst})
	if err != nil || res.Ask.ID != a.ID || len(res.Ask.Albums) != 2 || !res.Already {
		t.Fatalf("wider %+v %v", res, err)
	}
	vs, _ := r.svc.Views(ctx, "a.id = ?", a.ID)
	if vs[0].AlbumWords != "Live Night and First Light" || len(vs[0].People) != 2 {
		t.Fatalf("view %+v", vs[0])
	}

	got, err := r.svc.Approve(ctx, a.ID, r.luis, ApproveOptions{Note: "enjoy"})
	if err != nil {
		t.Fatal(err)
	}
	if got.State != domain.AskApproved || got.TitleID == nil {
		t.Fatalf("approved %+v", got)
	}
	var tt mediafiles.Title
	if err := database.Get(ctx, r.st.Read(), "titles", *got.TitleID, &tt); err != nil {
		t.Fatal(err)
	}
	if tt.Kind != "artist" || tt.WhyAdded != "ask" || !tt.KeepUpToDate {
		t.Fatalf("title %+v", tt)
	}
	// Only the asked-for albums are kept up to date; the live album is
	// included because it was asked for.
	keeps := albumKeeps(t, r.st, tt.ID)
	if !keeps["Live Night"] || !keeps["First Light"] || keeps["Second Wind"] || keeps["Soon"] || len(keeps) != 4 {
		t.Fatalf("album keeps %v", keeps)
	}
	r.svc.Wait()
	if last := r.fake.last(); last.Moment != alerts.MomentApproved || len(last.People) != 2 || !strings.Contains(last.Text, "albums you asked for") {
		t.Fatalf("alert %+v", last)
	}
	e := hasEvent(r.events(t), domain.EventAskApproved)
	if e == nil || e.Data["approved_by"] != "luis" || len(anyStrings(e.Data["asked_by"])) != 2 {
		t.Fatalf("approved event %+v", e)
	}

	// Asking for an album of the approved ask joins it; the whole artist is
	// a new ask for the albums not kept yet.
	if j, err := r.svc.AskArtist(ctx, r.ana, "mbid:"+mArtist, []string{mFirst}); err != nil || !j.Already || j.Ask.ID != a.ID {
		t.Fatalf("ask kept album: %+v %v", j, err)
	}
	res, err = r.svc.AskArtist(ctx, r.ana, "mbid:"+mArtist, nil)
	if err != nil || res.Ask.ID == a.ID || len(res.Ask.Albums) != 2 {
		t.Fatalf("whole artist in the Library: %+v %v", res, err)
	}
	if names := albumWords(res.Ask.Albums); names != "Second Wind and Soon" {
		t.Fatalf("missing albums %q", names)
	}
}

func TestArtistAskWholeArtistAndKindsLeftOut(t *testing.T) {
	r := musicRig(t)
	ctx := context.Background()
	res, err := r.svc.AskArtist(ctx, r.ana, "mbid:"+mArtist, nil)
	if err != nil {
		t.Fatal(err)
	}
	if label(res.Ask) != "Lab Band, every album" {
		t.Fatalf("label %q", label(res.Ask))
	}
	// A narrower ask joins the whole-artist ask.
	if j, err := r.svc.AskArtist(ctx, r.bo, "mbid:"+mArtist, []string{mFirst}); err != nil || !j.Joined || j.Ask.ID != res.Ask.ID {
		t.Fatalf("join whole %+v %v", j, err)
	}
	got, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	keeps := albumKeeps(t, r.st, *got.TitleID)
	if !keeps["First Light"] || !keeps["Second Wind"] || !keeps["Soon"] || keeps["Live Night"] {
		t.Fatalf("album keeps %v", keeps)
	}
	// The live album is left out by the artist's kinds: asking for it is
	// refused until staff change the kinds.
	if _, err := r.svc.AskArtist(ctx, r.bo, "mbid:"+mArtist, []string{mLive}); !errors.Is(err, ErrAlbumNotIncluded) {
		t.Fatalf("left-out album: %v", err)
	}
	if j, err := r.svc.AskArtist(ctx, r.bo, "mbid:"+mArtist, nil); err != nil || !j.Already {
		t.Fatalf("whole artist again: %+v %v", j, err)
	}
	in := &InLibraryError{Music: true}
	if in.Error() != "this artist (the albums you asked for) is already in the Library; HolyArrs is looking for it" {
		t.Fatalf("words %q", in.Error())
	}
}

// addTracks gives an album n tracks with Slots in the main Version.
func addTracks(t *testing.T, st *database.Store, titleID int64, album string, n int, state mediafiles.SlotState) {
	t.Helper()
	ctx := context.Background()
	err := st.Write(ctx, func(tx *sql.Tx) error {
		var partID, versionID int64
		if err := tx.QueryRow("SELECT id FROM parts WHERE title_id = ? AND name = ?", titleID, album).Scan(&partID); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT id FROM versions WHERE title_id = ? AND is_main = 1", titleID).Scan(&versionID); err != nil {
			return err
		}
		for i := 1; i <= n; i++ {
			disc, num := 1, i
			it := &mediafiles.Item{TitleID: titleID, PartID: &partID, Kind: "track", Numbers: mediafiles.Numbers{Disc: &disc, Track: &num}, KeepUpToDate: true, SourceIDs: map[string]string{}}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			if _, err := database.Insert(tx, "slots", &mediafiles.Slot{ItemID: it.ID, VersionID: versionID, State: state}, domain.System); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestArtistAskReadyWhenEveryTrackIsIn(t *testing.T) {
	r := musicRig(t)
	ctx := context.Background()
	res, _ := r.svc.AskArtist(ctx, r.ana, "mbid:"+mArtist, []string{mFirst})
	a, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// No tracks yet (the Pressings come with the refresh): not ready.
	if n, _ := r.svc.CheckReady(ctx); n != 0 {
		t.Fatalf("ready with no tracks: %d", n)
	}
	addTracks(t, r.st, *a.TitleID, "First Light", 2, mediafiles.SlotWanted)
	if n, _ := r.svc.CheckReady(ctx); n != 0 {
		t.Fatalf("ready with wanted tracks: %d", n)
	}
	// The import placed every file.
	_ = r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET state = 'in_library' WHERE item_id IN (SELECT id FROM items WHERE title_id = ?)", *a.TitleID)
		return err
	})
	if n, err := r.svc.CheckReady(ctx); n != 1 || err != nil {
		t.Fatalf("ready %d %v", n, err)
	}
	vs, _ := r.svc.Views(ctx, "a.id = ?", a.ID)
	if vs[0].State != domain.AskAvailable || vs[0].StateLabel != "Ready to listen" {
		t.Fatalf("view %+v", vs[0])
	}
	r.svc.Wait()
	if last := r.fake.last(); last.Moment != alerts.MomentReady || last.Subject != "Ready to listen: Lab Band, First Light" {
		t.Fatalf("alert %+v", last)
	}
	if e := hasEvent(r.events(t), domain.EventAskAvailable); e == nil || e.Summary != "Lab Band, First Light is ready for ana" {
		t.Fatalf("event %+v", e)
	}
	ms, _, _ := Messages(ctx, r.st.Read(), r.ana.UserID, 10)
	if len(ms) == 0 || ms[0].Kind != "ready" || !strings.Contains(ms[0].Text, "ready to listen to") {
		t.Fatalf("messages %+v", ms)
	}
	var in *InLibraryError
	if _, err := r.svc.AskArtist(ctx, r.bo, "mbid:"+mArtist, []string{mFirst}); !errors.As(err, &in) || !in.Ready {
		t.Fatalf("ask ready album: %v", err)
	}
}

func TestArtistAsksCountAgainstTheMusicLimit(t *testing.T) {
	r := musicRig(t)
	ctx := context.Background()
	set, _ := ReadSettings(ctx, r.st.Read())
	if set.MusicPerWeek != DefaultPerWeek || set.MoviesPerWeek != DefaultPerWeek || set.ShowsPerWeek != DefaultPerWeek {
		t.Fatalf("defaults %+v", set)
	}
	set.MusicPerWeek = 1
	if err := SaveSettings(ctx, r.st, set, domain.System); err != nil {
		t.Fatal(err)
	}
	// An album ask is one against the music limit.
	if _, err := r.svc.AskArtist(ctx, r.ana, "mbid:"+mArtist, []string{mFirst}); err != nil {
		t.Fatal(err)
	}
	// The movie limit is separate.
	if _, err := r.svc.Ask(ctx, r.ana, "900002"); err != nil {
		t.Fatalf("movie after music at the music limit: %v", err)
	}
	// Another music ask is refused, and says which limit.
	useAsk(t, r, r.bo.UserID, "artist")
	var le *LimitError
	if _, err := r.svc.AskArtist(ctx, r.bo, "mbid:"+mArtist, []string{mSecond}); !errors.As(err, &le) || le.Kind != KindMusic || !strings.Contains(err.Error(), "1 music ask in the last 7 days, which is your music limit") {
		t.Fatalf("music over the music limit: %v", err)
	}
	if l, _ := r.svc.LimitOf(ctx, r.ana); l.Music.Used != 1 || l.Music.Left != 0 || l.Movies.Used != 1 {
		t.Fatalf("limit %+v", l)
	}
	// Her own music limit wins over the default.
	five := 5
	if err := SetPersonLimit(ctx, r.st, r.ana.UserID, PersonLimit{Music: &five}); err != nil {
		t.Fatal(err)
	}
	if l, _ := r.svc.LimitOf(ctx, r.ana); l.Music.PerWeek != 5 || l.Music.Left != 4 || l.Movies.PerWeek != DefaultPerWeek {
		t.Fatalf("own music limit %+v", l)
	}
	// The music Library folder setting must be a music folder.
	set.MusicLibraryFolderID = 1
	if err := SaveSettings(ctx, r.st, set, domain.System); !errors.Is(err, library.ErrNotMusicFolder) {
		t.Fatalf("movie folder for music: %v", err)
	}
}

func TestArtistAskTurnedDownAndAutoApproved(t *testing.T) {
	r := musicRig(t)
	ctx := context.Background()
	res, _ := r.svc.AskArtist(ctx, r.ana, "mbid:"+mArtist, []string{mSecond})
	if _, err := r.svc.TurnDown(ctx, res.Ask.ID, r.luis, "not this one"); err != nil {
		t.Fatal(err)
	}
	r.svc.Wait()
	if last := r.fake.last(); last.Moment != alerts.MomentTurnedDown || last.Subject != "Turned down: Lab Band, Second Wind" {
		t.Fatalf("alert %+v", last)
	}
	// Someone who may auto-approve: added straight away.
	auto := r.bo
	auto.AutoApprove = true
	got, err := r.svc.AskArtist(ctx, auto, "mbid:"+mArtist, []string{mFirst})
	if err != nil || got.Ask.State != domain.AskApproved || !got.Ask.AutoApproved || got.Ask.TitleID == nil {
		t.Fatalf("auto %+v %v", got, err)
	}
	if keeps := albumKeeps(t, r.st, *got.Ask.TitleID); !keeps["First Light"] || keeps["Second Wind"] {
		t.Fatalf("keeps %v", keeps)
	}
}

// TestReapproveArtistAskIgnoresOtherArtists: a turned-down artist ask can
// be approved again while an ask for another artist is open.
func TestReapproveArtistAskIgnoresOtherArtists(t *testing.T) {
	r := musicRig(t)
	ctx := context.Background()
	const other = "a0000000-0000-0000-0000-0000000000bb"
	fa := r.svc.Library.Artists.(*fakeArtists)
	fa.artists[other] = metadata.Artist{MBID: other, Name: "Other Band", Status: "running", Albums: []metadata.Album{
		{MBID: "10000000-0000-0000-0000-0000000000bb", Name: "Elsewhere", PrimaryType: "album", ReleaseDate: "2020"}}}
	res, err := r.svc.AskArtist(ctx, r.ana, "mbid:"+mArtist, []string{mSecond})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.TurnDown(ctx, res.Ask.ID, r.luis, "not now"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.AskArtist(ctx, r.bo, "mbid:"+other, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{}); err != nil || got.State != domain.AskApproved {
		t.Fatalf("re-approve: %+v %v", got, err)
	}
}

// TestArtistAskRefusesAlbumNotInLibraryYet: for an artist in the Library,
// an album MusicBrainz lists but the Library does not know yet is refused
// (asking again after the next refresh works), so no approved ask waits
// for an album that never gets kept up to date.
func TestArtistAskRefusesAlbumNotInLibraryYet(t *testing.T) {
	r := musicRig(t)
	ctx := context.Background()
	res, _ := r.svc.AskArtist(ctx, r.ana, "mbid:"+mArtist, []string{mFirst})
	if _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	const fresh = "10000000-0000-0000-0000-0000000000ae"
	fa := r.svc.Library.Artists.(*fakeArtists)
	a := fa.artists[mArtist]
	a.Albums = append(a.Albums, metadata.Album{MBID: fresh, Name: "Fresh", PrimaryType: "album", ReleaseDate: "2026-10-01"})
	fa.artists[mArtist] = a
	if _, err := r.svc.AskArtist(ctx, r.bo, "mbid:"+mArtist, []string{fresh}); !errors.Is(err, ErrAlbumNotListedYet) {
		t.Fatalf("album the Library does not know yet: %v", err)
	}
}

func TestArtistAskViewHasArtistIDAndPicture(t *testing.T) {
	r := musicRig(t)
	ctx := context.Background()
	if _, err := r.svc.AskArtist(ctx, r.ana, "mbid:"+mArtist, []string{mLive}); err != nil {
		t.Fatal(err)
	}
	vs, err := r.svc.Views(ctx, "a.kind = 'artist'")
	if err != nil || len(vs) != 1 {
		t.Fatalf("views %+v %v", vs, err)
	}
	if v := vs[0]; v.ArtistID != mArtist || v.Poster != "/api/v1/music-trending/art/artist/"+mArtist {
		t.Fatalf("artist ask view %+v", v)
	}
}

// TestArtistApproveWidensKindsForAskedAlbum: the artist joins the Library
// (added by hand, default kinds) while an ask for a live album waits;
// approving it includes just that album and leaves the artist's kinds alone.
func TestArtistApproveIncludesOnlyAskedAlbum(t *testing.T) {
	r := musicRig(t)
	ctx := context.Background()
	live, err := r.svc.AskArtist(ctx, r.bo, "mbid:"+mArtist, []string{mLive})
	if err != nil {
		t.Fatal(err)
	}
	defs, err := library.ReadMusicDefaults(ctx, r.st.Read())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.svc.Library.AddArtist(ctx, "mbid:"+mArtist, library.ArtistAddOptions{Albums: library.AlbumsAll, WhyAdded: "by_hand",
		NewAlbums: defs.NewAlbums, AlbumTypes: defs.AlbumTypes, SecondaryTypes: defs.SecondaryTypes, ReleaseStatuses: defs.ReleaseStatuses}, domain.System); err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.Approve(ctx, live.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatalf("approve left-out album: %v", err)
	}
	if keeps := albumKeeps(t, r.st, *got.TitleID); !keeps["Live Night"] {
		t.Fatalf("album keeps %v", keeps)
	}
	var tt mediafiles.Title
	if err := database.Get(ctx, r.st.Read(), "titles", *got.TitleID, &tt); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tt.MusicSecondary, defs.SecondaryTypes) || !reflect.DeepEqual(tt.MusicTypes, defs.AlbumTypes) {
		t.Fatalf("kinds widened: %v %v", tt.MusicTypes, tt.MusicSecondary)
	}
	// A refresh keeps the asked album and does not take in other live ones.
	fa := r.svc.Library.Artists.(*fakeArtists)
	a := fa.artists[mArtist]
	a.Albums = append(a.Albums, metadata.Album{MBID: "10000000-0000-0000-0000-0000000000ae", Name: "Live Again", PrimaryType: "album", SecondaryTypes: []string{"live"}, ReleaseDate: "2023"})
	fa.artists[mArtist] = a
	if _, err := r.svc.Library.RefreshArtist(ctx, *got.TitleID, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	ps, err := database.Select[mediafiles.Part](ctx, r.st.Read(), "parts", "title_id = ? AND kind = 'album'", *got.TitleID)
	if err != nil {
		t.Fatal(err)
	}
	inc := map[string]bool{}
	for _, p := range ps {
		inc[p.Name] = p.Included
	}
	if !inc["Live Night"] || inc["Live Again"] {
		t.Fatalf("after refresh included %v", inc)
	}
}

// A refresh that runs right after the asked album is taken in (before the
// approval finishes) keeps it: it is kept up to date in the same write.
func TestIncludedAlbumSurvivesRefreshBeforeKeep(t *testing.T) {
	r := musicRig(t)
	ctx := context.Background()
	defs, err := library.ReadMusicDefaults(ctx, r.st.Read())
	if err != nil {
		t.Fatal(err)
	}
	tid, _, err := r.svc.Library.AddArtist(ctx, "mbid:"+mArtist, library.ArtistAddOptions{Albums: library.AlbumsAll, WhyAdded: "by_hand",
		NewAlbums: defs.NewAlbums, AlbumTypes: defs.AlbumTypes, SecondaryTypes: defs.SecondaryTypes, ReleaseStatuses: defs.ReleaseStatuses}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := database.Select[mediafiles.Part](ctx, r.st.Read(), "parts", "title_id = ? AND kind = 'album' AND name = 'Live Night'", tid)
	if err != nil || len(ps) != 1 || ps[0].Included {
		t.Fatalf("left-out album %v %v", ps, err)
	}
	if err := r.svc.Library.IncludeAlbums(ctx, tid, []int64{ps[0].ID}, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Library.RefreshArtist(ctx, tid, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	var p mediafiles.Part
	if err := database.Get(ctx, r.st.Read(), "parts", ps[0].ID, &p); err != nil {
		t.Fatal(err)
	}
	if !p.Included || !p.KeepUpToDate {
		t.Fatalf("after refresh included=%v keep=%v", p.Included, p.KeepUpToDate)
	}
}

// an approved album ask is looked for at once too. A new artist's
// tracks come with its MusicBrainz refresh, so the search ends the add's
// own chain; an artist already in the Library is scanned and searched.
func TestApprovedArtistAskSearchesAtOnce(t *testing.T) {
	r := musicRig(t)
	ctx := context.Background()
	res, err := r.svc.AskArtist(ctx, r.ana, "mbid:"+mArtist, []string{mLive})
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
	if err != nil || got.TitleID == nil {
		t.Fatalf("approve %+v %v", got, err)
	}
	var then string
	_ = r.st.Read().QueryRow("SELECT json_extract(params, '$.then') FROM jobs WHERE dedupe_key = ?", fmt.Sprintf("refresh_title:%d", *got.TitleID)).Scan(&then)
	if then != `["scan_title","search_wanted"]` {
		t.Fatalf("new artist's add chain %q", then)
	}
	queued := func() int {
		var n int
		_ = r.st.Read().QueryRow("SELECT count(*) FROM jobs WHERE dedupe_key = ?", fmt.Sprintf("scan_then_search:%d", *got.TitleID)).Scan(&n)
		return n
	}
	if n := queued(); n != 0 {
		t.Fatalf("a new artist also queued scan then search %d", n)
	}
	// Another album of the same artist: it is in the Library now.
	res, err = r.svc.AskArtist(ctx, r.bo, "mbid:"+mArtist, []string{mFirst})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	if n := queued(); n != 1 {
		t.Fatalf("artist in the Library: scan then search queued %d, want 1", n)
	}
}

// ana asked for the live album before the artist was in the
// Library; it was added by hand meanwhile, and its kinds leave ana's album out. Approving hers takes it in, and its tracks come with the
// refresh that follows: the search waits for that refresh (and the scan).
func TestApprovedWidenedAlbumSearchesAfterItsRefresh(t *testing.T) {
	r := musicRig(t)
	ctx := context.Background()
	live, err := r.svc.AskArtist(ctx, r.ana, "mbid:"+mArtist, []string{mLive})
	if err != nil {
		t.Fatal(err)
	}
	// The artist is added by hand meanwhile, with the usual album kinds.
	defs, err := library.ReadMusicDefaults(ctx, r.st.Read())
	if err != nil {
		t.Fatal(err)
	}
	tid, _, err := r.svc.Library.AddArtist(ctx, "mbid:"+mArtist, library.ArtistAddOptions{Albums: library.AlbumsAll, NewAlbums: defs.NewAlbums,
		AlbumTypes: defs.AlbumTypes, SecondaryTypes: defs.SecondaryTypes, ReleaseStatuses: defs.ReleaseStatuses}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var inc bool
	_ = r.st.Read().QueryRow("SELECT included FROM parts WHERE title_id = ? AND json_extract(source_ids, '$.musicbrainz') = ?", tid, mLive).Scan(&inc)
	if inc {
		t.Fatal("the live album is in the usual kinds; this test needs one left out")
	}
	// The add's jobs ran.
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE jobs SET state = 'done'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(ctx, live.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	var then string
	_ = r.st.Read().QueryRow("SELECT json_extract(params, '$.then') FROM jobs WHERE kind = 'refresh_title' AND state = 'queued' ORDER BY id DESC LIMIT 1").Scan(&then)
	if then != `["scan_title","search_wanted"]` {
		t.Fatalf("widened album's refresh chain %q", then)
	}
	var early int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM jobs WHERE dedupe_key = ? AND state = 'queued'", fmt.Sprintf("scan_then_search:%d", tid)).Scan(&early)
	if early != 0 {
		t.Fatalf("searched before the refresh too (%d)", early)
	}
}
