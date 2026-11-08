package library

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// countMusic is a MusicBrainz stand-in for n artists ("Artist 0001" ...),
// each with one album ("Album 0001", 2010) of tracks Pressing tracks. It
// counts every lookup and whether it came as a background lookup.
type countMusic struct {
	mu                         sync.Mutex
	n, tracks                  int
	search, artist, press, bad int
	busy                       int // answer this many lookups with ErrMusicBusy first
}

func (c *countMusic) note(ctx context.Context, n *int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !metadata.IsMemberLookup(ctx) {
		c.bad++
	}
	if c.busy > 0 {
		c.busy--
		return metadata.ErrMusicBusy
	}
	*n++
	return nil
}

func mbidOf(i int) string { return fmt.Sprintf("a0000000-0000-0000-0000-%012d", i) }
func rgOf(i int) string   { return fmt.Sprintf("10000000-0000-0000-0000-%012d", i) }

func (c *countMusic) SearchArtists(ctx context.Context, q string) ([]metadata.Artist, error) {
	if err := c.note(ctx, &c.search); err != nil {
		return nil, err
	}
	var i int
	if _, err := fmt.Sscanf(q, "Artist %d", &i); err != nil || i < 1 || i > c.n {
		return nil, nil
	}
	return []metadata.Artist{{MBID: mbidOf(i), Name: q}}, nil
}

func (c *countMusic) Artist(ctx context.Context, mbid string) (metadata.Artist, error) {
	if err := c.note(ctx, &c.artist); err != nil {
		return metadata.Artist{}, err
	}
	var i int
	_, _ = fmt.Sscanf(mbid[len(mbid)-12:], "%d", &i)
	return metadata.Artist{MBID: mbid, Name: fmt.Sprintf("Artist %04d", i), Albums: []metadata.Album{
		{MBID: rgOf(i), Name: fmt.Sprintf("Album %04d", i), PrimaryType: "album", ReleaseDate: "2010-01-01"}}}, nil
}

func (c *countMusic) Pressings(ctx context.Context, rg string) ([]metadata.Pressing, error) {
	if err := c.note(ctx, &c.press); err != nil {
		return nil, err
	}
	var trs []metadata.Track
	for k := 1; k <= c.tracks; k++ {
		trs = append(trs, metadata.Track{ID: fmt.Sprintf("t%d", k), Disc: 1, Number: k, Position: k, Name: fmt.Sprintf("Song %d", k)})
	}
	return []metadata.Pressing{{MBID: "r-" + rg, Name: "Album", Status: "official", Date: "2010", Tracks: trs}}, nil
}

// thousandArtists lays out 1000 artist folders, each with one album folder
// of tracks files.
func thousandArtists(t *testing.T, root string, tracks int) {
	t.Helper()
	for i := 1; i <= 1000; i++ {
		for k := 1; k <= tracks; k++ {
			writeFile(t, filepath.Join(root, fmt.Sprintf("Artist %04d", i), fmt.Sprintf("Album %04d (2010)", i), fmt.Sprintf("%02d - Song %d.mp3", k, k)), "a")
		}
	}
}

// 4.15 review: a music scan reads at most the music scan limit (200) of
// artist folders, one search and one album list per artist and one
// Pressings list per album, all as background lookups; the next part
// starts after Next.
func TestMusicScanPagesAtItsOwnLimit(t *testing.T) {
	_, s, _, lf := musicService(t)
	ctx := context.Background()
	thousandArtists(t, lf.Path, 1)
	cm := &countMusic{n: 1000, tracks: 1}
	p, err := PlanMusicAdoptFrom(ctx, s.Read(), cm, lf.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Limit != mediafiles.DefaultMusicAdoptScanLimit || len(p.Folders) != 200 || !p.More || p.Next != "Artist 0200" || p.Counts["matched"] != 200 {
		t.Fatalf("limit %d folders %d more %v next %q counts %v", p.Limit, len(p.Folders), p.More, p.Next, p.Counts)
	}
	if cm.search != 200 || cm.artist != 200 || cm.press != 200 || cm.bad != 0 {
		t.Fatalf("calls search %d artist %d pressings %d not background %d", cm.search, cm.artist, cm.press, cm.bad)
	}
	p2, err := PlanMusicAdoptFrom(ctx, s.Read(), cm, lf.ID, p.Next)
	if err != nil || len(p2.Folders) != 200 || p2.Folders[0].Folder != "Artist 0201" || p2.Next != "Artist 0400" {
		t.Fatalf("second part %v %d %+v", err, len(p2.Folders), p2.Next)
	}
	// The movie and show limit is not the music one.
	ns := mediafiles.NamingDefaults()
	if ns.AdoptScanLimit == ns.MusicAdoptScanLimit {
		t.Fatal("music shares the folder limit")
	}
}

// 4.15 review: an album folder of many files lists only the first ones in
// the stored plan, with cut set; the counts cover them all.
func TestMusicScanCutsFileDetail(t *testing.T) {
	_, s, _, lf := musicService(t)
	for k := 1; k <= 60; k++ {
		writeFile(t, filepath.Join(lf.Path, "Artist 0001", "Album 0001 (2010)", fmt.Sprintf("%02d - Song %d.mp3", k, k)), "a")
	}
	cm := &countMusic{n: 1, tracks: 60}
	p, err := PlanMusicAdoptFrom(context.Background(), s.Read(), cm, lf.ID, "")
	if err != nil || len(p.Folders) != 1 || len(p.Folders[0].Albums) != 1 {
		t.Fatalf("%v %+v", err, p)
	}
	al := p.Folders[0].Albums[0]
	if !al.Cut || len(al.Files) != maxAlbumFileDetail || al.Matched != 60 || p.Folders[0].Tracks != 60 {
		t.Fatalf("cut %v files %d matched %d", al.Cut, len(al.Files), al.Matched)
	}
}

// 4.15 review: each artist folder's result is saved as it is done; a scan
// stopped after three goes on from them without looking them up again.
func TestMusicScanKeepsFinishedParts(t *testing.T) {
	_, s, _, lf := musicService(t)
	thousandArtists(t, lf.Path, 1)
	cm := &countMusic{n: 1000, tracks: 1}
	ctx, cancel := context.WithCancel(context.Background())
	var last AdoptPlan
	saves := 0
	_, err := planMusicAdopt(ctx, s.Read(), cm, lf.ID, "", nil, func(p AdoptPlan) error {
		saves++
		last = p
		if saves == 3 {
			cancel()
		}
		return nil
	})
	if err == nil || len(last.Folders) != 3 || !last.Partial || !last.More || last.Next != "Artist 0003" {
		t.Fatalf("err %v saved %d partial %v next %q", err, len(last.Folders), last.Partial, last.Next)
	}
	before := cm.search
	p, err := planMusicAdopt(context.Background(), s.Read(), cm, lf.ID, "", &last, nil)
	if err != nil || len(p.Folders) != 200 || p.Partial || p.Next != "Artist 0200" || p.Counts["matched"] != 200 {
		t.Fatalf("resumed %v %d %v %q %v", err, len(p.Folders), p.Partial, p.Next, p.Counts)
	}
	if cm.search-before != 197 {
		t.Fatalf("looked up %d artists again", cm.search-before)
	}
}

// 4.15 review: when the shared MusicBrainz queue is full, the scan waits
// and asks again instead of marking the artist unmatched.
func TestMusicScanWaitsWhenBusy(t *testing.T) {
	_, s, _, lf := musicService(t)
	writeFile(t, filepath.Join(lf.Path, "Artist 0001", "Album 0001 (2010)", "01 - Song 1.mp3"), "a")
	old := busyWait
	busyWait = time.Millisecond
	defer func() { busyWait = old }()
	cm := &countMusic{n: 1, tracks: 1, busy: 3}
	p, err := PlanMusicAdoptFrom(context.Background(), s.Read(), cm, lf.ID, "")
	if err != nil || p.Counts["matched"] != 1 {
		t.Fatalf("%v %+v", err, p.Counts)
	}
}
