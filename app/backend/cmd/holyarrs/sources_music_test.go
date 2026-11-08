package main

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// The MusicBrainz client switches to the local one only when the music
// settings allow the local network, or a mirror is set by env.
func TestMusicSourceClientFollowsAllowLocal(t *testing.T) {
	t.Setenv("HOLYARRS_MUSICBRAINZ_URL", "")
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	d := library.DefaultMusic()
	d.AllowLocal = false
	if err := library.SetMusicDefaults(ctx, s, d, domain.System); err != nil {
		t.Fatal(err)
	}
	m := musicSource(s, "t")
	pub := m.Client(ctx)
	if m.Client(ctx) != pub {
		t.Fatal("client changed with no setting change")
	}
	d.AllowLocal = true
	if err := library.SetMusicDefaults(ctx, s, d, domain.System); err != nil {
		t.Fatal(err)
	}
	if m.Client(ctx) == pub {
		t.Fatal("AllowLocal did not switch the client")
	}
	if got := m.BaseURL(ctx); got != d.MusicBrainzURL {
		t.Fatalf("base url %q, want %q", got, d.MusicBrainzURL)
	}
	t.Setenv("HOLYARRS_MUSICBRAINZ_URL", "http://mirror.lan:5000")
	e := musicSource(s, "t")
	if got := e.BaseURL(ctx); got != "http://mirror.lan:5000" {
		t.Fatalf("env base url %q", got)
	}
}
