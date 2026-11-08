package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// TestMusicSourceKeepsToPublicAddresses: a MusicBrainz base URL on this
// machine or the local network is refused unless the music settings allow
// the local network.
func TestMusicSourceKeepsToPublicAddresses(t *testing.T) {
	t.Setenv("HOLYARRS_MUSICBRAINZ_URL", "")
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"artists":[]}`)
	}))
	defer srv.Close()
	d := library.DefaultMusic()
	d.MusicBrainzURL = srv.URL
	if err := library.SetMusicDefaults(context.Background(), s, d, domain.System); err != nil {
		t.Fatal(err)
	}
	mb := musicSource(s, "test")
	mb.Limiter = &metadata.Limiter{SleepUntil: func(context.Context, time.Time) error { return nil }}
	if _, err := mb.SearchArtists(metadata.NoCache(context.Background()), "lab"); !errors.Is(err, metadata.ErrMusicUnavailable) {
		t.Fatalf("local address without the setting: %v", err)
	}
	d.AllowLocal = true
	if err := library.SetMusicDefaults(context.Background(), s, d, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := mb.SearchArtists(metadata.NoCache(context.Background()), "lab"); err != nil {
		t.Fatalf("local address with the setting: %v", err)
	}
}
