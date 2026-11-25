package library

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

const bigArtistID = "b1000000-0000-4000-8000-000000000001"

// bigMusic is an artist with n albums of one track each. fail answers
// those album ids with failErr; artistErr fails the artist read itself.
type bigMusic struct {
	mu        sync.Mutex
	n         int
	fail      map[string]bool
	failErr   error
	artistErr error
	busy      int
	press     int
}

func bigAlbum(i int) string { return fmt.Sprintf("20000000-0000-4000-8000-%012d", i) }

func (b *bigMusic) SearchArtists(context.Context, string) ([]metadata.Artist, error) {
	return []metadata.Artist{{MBID: bigArtistID, Name: "Big Artist"}}, nil
}

func (b *bigMusic) Artist(context.Context, string) (metadata.Artist, error) {
	if b.artistErr != nil {
		return metadata.Artist{}, b.artistErr
	}
	a := metadata.Artist{MBID: bigArtistID, Name: "Big Artist"}
	for i := 1; i <= b.n; i++ {
		a.Albums = append(a.Albums, metadata.Album{MBID: bigAlbum(i), Name: fmt.Sprintf("Album %04d", i), PrimaryType: "album", ReleaseDate: "2010-01-01"})
	}
	return a, nil
}

func (b *bigMusic) Pressings(_ context.Context, rg string) ([]metadata.Pressing, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.press++
	if b.busy > 0 {
		b.busy--
		return nil, metadata.ErrServiceBusy
	}
	if b.fail[rg] {
		return nil, b.failErr
	}
	return []metadata.Pressing{{MBID: "30000000-0000-4000-8000-" + rg[24:], Name: "Album", Status: "official", Date: "2010",
		Tracks: []metadata.Track{{ID: "40000000-0000-4000-8000-" + rg[24:], RecordingID: "50000000-0000-4000-8000-" + rg[24:], Disc: 1, Number: 1, Position: 1, Name: "Song", LengthMS: 1000}}}}, nil
}

func bigService(t *testing.T, b *bigMusic) (*Service, int64, string) {
	t.Helper()
	svc, _, _, lf := musicService(t)
	svc.Artists = b
	for i := 1; i <= b.n; i++ {
		writeFile(t, filepath.Join(lf.Path, "Big Artist", fmt.Sprintf("Album %04d (2010)", i), "01 - Song.mp3"), "a")
	}
	return svc, lf.ID, "Big Artist"
}

// no artist size fails the adopt; every album folder links.
func TestAdoptBigArtistSizes(t *testing.T) {
	old := busyWait
	busyWait = 0
	defer func() { busyWait = old }()
	for _, n := range []int{1, 100, 500, 2000} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			if testing.Short() && n > 100 {
				t.Skip("big artist")
			}
			b := &bigMusic{n: n, busy: 2}
			svc, lf, folder := bigService(t, b)
			res, err := svc.Adopt(context.Background(), lf, []AdoptChoice{{Folder: folder, ID: "mbid:" + bigArtistID}}, domain.System)
			if err != nil || len(res) != 1 || res[0].Error != "" || res[0].TitleID == 0 {
				t.Fatalf("%+v %v", res, err)
			}
			if res[0].Episodes != n || len(res[0].Unknown) != 0 {
				t.Fatalf("adopted %d of %d, left %d", res[0].Episodes, n, len(res[0].Unknown))
			}
			if a, f, _ := adoptTally(res); a != 1 || f != 0 {
				t.Fatalf("tally %d %d", a, f)
			}
		})
	}
}

// one album that cannot be read is skipped; the rest adopt.
func TestAdoptBigArtistOneAlbumRefused(t *testing.T) {
	b := &bigMusic{n: 120, fail: map[string]bool{bigAlbum(7): true}, failErr: metadata.ErrSourceOff}
	svc, lf, folder := bigService(t, b)
	res, _ := svc.Adopt(context.Background(), lf, []AdoptChoice{{Folder: folder, ID: "mbid:" + bigArtistID}}, domain.System)
	if len(res) != 1 || res[0].Error != "" || res[0].Episodes != 119 {
		t.Fatalf("%+v", res)
	}
}

// a refused lookup for every album, or a server that is offline,
// fails the folder with a reason and the job is never "done".
func TestAdoptBigArtistRefusedOrOffline(t *testing.T) {
	all := map[string]bool{}
	for i := 1; i <= 5; i++ {
		all[bigAlbum(i)] = true
	}
	for name, b := range map[string]*bigMusic{
		"refused": {n: 5, fail: all, failErr: metadata.ErrSourceOff},
		"offline": {n: 5, artistErr: metadata.ErrServiceUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			svc, lf, folder := bigService(t, b)
			res, err := svc.Adopt(context.Background(), lf, []AdoptChoice{{Folder: folder, ID: "mbid:" + bigArtistID}}, domain.System)
			if err != nil || len(res) != 1 || res[0].Error == "" {
				t.Fatalf("%+v %v", res, err)
			}
			a, f, why := adoptTally(res)
			if a != 0 || f != 1 || strings.TrimSpace(why) == "" {
				t.Fatalf("tally %d %d %q", a, f, why)
			}
		})
	}
}

// a server that turns down every album stops after a few, not after
// asking for each of a big artist's albums with waits in between.
func TestAdoptBigArtistServerBusyStopsEarly(t *testing.T) {
	old := busyWait
	busyWait = 0
	defer func() { busyWait = old }()
	b := &bigMusic{n: 200, busy: 1 << 30}
	svc, lf, folder := bigService(t, b)
	res, _ := svc.Adopt(context.Background(), lf, []AdoptChoice{{Folder: folder, ID: "mbid:" + bigArtistID}}, domain.System)
	if len(res) != 1 || res[0].Error == "" {
		t.Fatalf("%+v", res)
	}
	if b.press > 3*refusedAlbumsStop+3 {
		t.Fatalf("asked %d times", b.press)
	}
}

// albums with no editions plus refused albums: nothing was read, so it fails.
func TestAdoptBigArtistEmptyAndRefusedFails(t *testing.T) {
	b := &bigMusicEmpty{bigMusic: bigMusic{n: 4, fail: map[string]bool{bigAlbum(2): true, bigAlbum(3): true, bigAlbum(4): true}, failErr: metadata.ErrSourceOff}}
	svc, lf, folder := bigService(t, &b.bigMusic)
	svc.Artists = b
	res, _ := svc.Adopt(context.Background(), lf, []AdoptChoice{{Folder: folder, ID: "mbid:" + bigArtistID}}, domain.System)
	if len(res) != 1 || res[0].Error == "" {
		t.Fatalf("%+v", res)
	}
}

// bigMusicEmpty lists no editions for the first album.
type bigMusicEmpty struct{ bigMusic }

func (b *bigMusicEmpty) Pressings(ctx context.Context, rg string) ([]metadata.Pressing, error) {
	if rg == bigAlbum(1) {
		return nil, nil
	}
	return b.bigMusic.Pressings(ctx, rg)
}

// skipped albums are asked again on the next refresh run, and an artist
// whose albums were all refused or gone fails rather than reading as done.
func TestRefreshArtistSkippedAlbumsRetryNextRun(t *testing.T) {
	b := &bigMusic{n: 3, fail: map[string]bool{bigAlbum(2): true}, failErr: metadata.ErrSourceOff}
	svc, lf, folder := bigService(t, b)
	res, _ := svc.Adopt(context.Background(), lf, []AdoptChoice{{Folder: folder, ID: "mbid:" + bigArtistID}}, domain.System)
	if len(res) != 1 || res[0].TitleID == 0 {
		t.Fatalf("%+v", res)
	}
	var ti mediafiles.Title
	err := database.Get(context.Background(), svc.Store.Read(), "titles", res[0].TitleID, &ti)
	if err != nil || ti.RefreshedAt != nil {
		t.Fatalf("refreshed_at %v %v; want cleared so the next run asks again", ti.RefreshedAt, err)
	}
	if yes, _ := ShouldRefreshArtist(ti, nil, svc.now()); !yes {
		t.Fatal("artist with skipped albums not picked by the next refresh")
	}
	// Album 1 now gone, album 2 still refused, album 3 refused: nothing read.
	b.mu.Lock()
	b.fail = map[string]bool{bigAlbum(2): true, bigAlbum(3): true}
	b.mu.Unlock()
	gone := &goneFirst{bigMusic: b}
	svc.Artists = gone
	if _, err := svc.RefreshArtist(context.Background(), res[0].TitleID, domain.System, nil); err == nil {
		t.Fatal("refresh with no album read reported success")
	}
}

type goneFirst struct{ *bigMusic }

func (g *goneFirst) Pressings(ctx context.Context, rg string) ([]metadata.Pressing, error) {
	if rg == bigAlbum(1) {
		return nil, metadata.ErrArtistNotFound
	}
	return g.bigMusic.Pressings(ctx, rg)
}
