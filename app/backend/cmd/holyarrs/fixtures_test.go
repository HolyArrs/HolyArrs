package main

// Fixtures for the cross-module tests
// that run here on the program's real routes.

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

type addedShow struct {
	Title struct {
		ID    int64  `json:"id"`
		Kind  string `json:"kind"`
		Slots []any  `json:"slots"`
		Show  struct {
			DetailsSource string                `json:"details_source"`
			EpisodeOrder  string                `json:"episode_order"`
			SeasonChoice  string                `json:"season_choice"`
			NewSeasons    string                `json:"new_seasons"`
			Seasons       []shows.SeasonSummary `json:"seasons"`
			Episodes      int                   `json:"episodes"`
			Missing       int                   `json:"missing"`
			NotOutYet     int                   `json:"not_out_yet"`
		} `json:"show"`
	} `json:"title"`
	JobID int64 `json:"job_id"`
}

// The music fixtures the tests that span music and other modules
// share (the music tests keep their own copy in internal/music).

// fakeArtists is MusicBrainz over fixed answers.
type fakeArtists struct {
	mu        sync.Mutex
	artists   map[string]metadata.Artist
	pressings map[string][]metadata.Pressing
	err       error
	// block holds SearchArtists until closed; entered says one came in;
	// members records whether each search was a member's.
	block   chan struct{}
	entered chan struct{}
	members []bool
}

func (f *fakeArtists) SearchArtists(ctx context.Context, q string) ([]metadata.Artist, error) {
	f.mu.Lock()
	f.members = append(f.members, metadata.IsMemberLookup(ctx))
	block, entered := f.block, f.entered
	f.mu.Unlock()
	if block != nil {
		entered <- struct{}{}
		<-block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	out := []metadata.Artist{}
	for _, a := range f.artists {
		if strings.Contains(strings.ToLower(a.Name), strings.ToLower(q)) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeArtists) Artist(_ context.Context, mbid string) (metadata.Artist, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return metadata.Artist{}, f.err
	}
	if a, ok := f.artists[mbid]; ok {
		return a, nil
	}
	return metadata.Artist{}, metadata.ErrArtistNotFound
}

func (f *fakeArtists) Pressings(_ context.Context, mbid string) ([]metadata.Pressing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pressings[mbid], f.err
}

// AlbumArtist finds the album among the fixed artists' albums.
func (f *fakeArtists) AlbumArtist(_ context.Context, album string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, a := range f.artists {
		for _, al := range a.Albums {
			if al.MBID == album {
				return id, nil
			}
		}
	}
	return "", metadata.ErrArtistNotFound
}

const (
	apiArtist = "a0000000-0000-0000-0000-0000000000aa"
	apiAlbum  = "10000000-0000-0000-0000-0000000000aa"
	apiLive   = "10000000-0000-0000-0000-0000000000ab"
	apiSoon   = "10000000-0000-0000-0000-0000000000ac"
	apiRelA   = "20000000-0000-0000-0000-0000000000aa"
	apiRelB   = "20000000-0000-0000-0000-0000000000ab"
)

func apiMusic() *fakeArtists {
	tr := func(p string, n int) []metadata.Track {
		out := []metadata.Track{}
		for i := 1; i <= n; i++ {
			out = append(out, metadata.Track{ID: fmt.Sprintf("%s-%d", p, i), RecordingID: fmt.Sprintf("rec-%d", i), Disc: 1, Number: i, Position: i,
				Name: fmt.Sprintf("Song %d", i), LengthMS: 180000})
		}
		return out
	}
	return &fakeArtists{
		artists: map[string]metadata.Artist{apiArtist: {MBID: apiArtist, Name: "Lab Band", Type: "group", Status: "running", Albums: []metadata.Album{
			{MBID: apiAlbum, Name: "First Light", PrimaryType: "album", ReleaseDate: "2020-03-06"},
			{MBID: apiLive, Name: "Live Night", PrimaryType: "album", SecondaryTypes: []string{"live"}, ReleaseDate: "2021"},
			{MBID: apiSoon, Name: "Soon", PrimaryType: "ep", ReleaseDate: "2099-01-01"},
		}}},
		pressings: map[string][]metadata.Pressing{apiAlbum: {
			{MBID: apiRelA, Name: "First Light", Status: "official", Country: "XW", Date: "2020-03-06", Formats: []string{"Digital Media"}, Tracks: tr("a", 2)},
			{MBID: apiRelB, Name: "First Light", Status: "official", Country: "US", Date: "2020", Disambiguation: "deluxe", Formats: []string{"CD"}, Tracks: tr("b", 3)},
		}},
	}
}
