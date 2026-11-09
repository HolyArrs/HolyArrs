package music

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// An artist the metadata server does not know is kept as an empty list
// for a few minutes, so a repeated trailer batch costs no new lookups.
func TestVideosNotFoundKeptMinutes(t *testing.T) {
	var calls atomic.Int32
	a := New(Server{Store: storeAPI(t).s.Store, ArtistVideos: func(context.Context, string) ([]metadata.ArtistVideo, error) {
		calls.Add(1)
		return nil, metadata.ErrNotFound
	}})
	for i := 0; i < 3; i++ {
		if l := a.videosOf(context.Background(), apiArtist); len(l) != 0 {
			t.Fatalf("videos %v", l)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("server asked %d times", calls.Load())
	}
}

type newMusicSpy struct {
	metadata.ArtistProvider
	mu   sync.Mutex
	sent []string
}

func (s *newMusicSpy) ReleasesBetween(_ context.Context, _, _ string, _, artists []string, _ int) ([]metadata.NewRelease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, artists...)
	return nil, nil
}

// Building New Music never sends the Library's artists to the server; the
// charts' artists still are.
func TestMusicNewSendsNoLibraryArtists(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	lib := tmbid(4242)
	if err := st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "titles", &mediafiles.Title{Kind: "artist", GateDriver: "none", Name: "Mine", SortName: "mine", Aliases: []string{},
			SourceIDs: map[string]string{"musicbrainz": lib}, MetadataSource: "musicbrainz", NewPartsPolicy: "all",
			ReleaseGate: "released", MusicTypes: []string{"album"}, WhyAdded: "by_hand", Artwork: []string{}, Status: "running", State: "active"}, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	spy := &newMusicSpy{}
	chart := tmbid(9001)
	a := New(Server{Store: st, Library: &library.Service{Artists: spy},
		Deps: Deps{MusicTrending: func(context.Context, string, string) (metadata.MusicTrend, error) {
			return metadata.MusicTrend{Kind: "artist", Items: []metadata.MusicTrendItem{{Rank: 1, MBID: chart}}}, nil
		}}})
	a.buildMusicNew(context.Background(), time.Now())
	got := strings.Join(spy.sent, ",")
	if strings.Contains(got, lib) {
		t.Fatalf("Library artist sent: %s", got)
	}
	if !strings.Contains(got, chart) {
		t.Fatalf("chart artist not sent: %q", got)
	}
}
