package music

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// TestHooks reach into the music state for the tests that span music
// and other modules (search, extras, the trailer reader). The app never
// calls them.
type TestHooks struct{ h *Handlers }

// TestHooks gives tests their hooks.
func (h *Handlers) TestHooks() TestHooks { return TestHooks{h} }

// ForgetArtMisses drops every remembered picture miss, as if their wait had passed.
func (x TestHooks) ForgetArtMisses() {
	x.h.music.mu.Lock()
	x.h.music.artMiss = nil
	x.h.music.mu.Unlock()
}

// Name is the remembered name of key ("album|mbid", "artist|mbid").
func (x TestHooks) Name(key string) metadata.MusicTrendItem {
	x.h.music.mu.Lock()
	defer x.h.music.mu.Unlock()
	return x.h.music.names[key]
}

// WaitPrefetch waits for the background picture fetches.
func (x TestHooks) WaitPrefetch() { x.h.music.prefetching.Wait() }

// ResolveMusicVideo resolves one music video like the artist page does.
func (x TestHooks) ResolveMusicVideo(ctx context.Context, key string) (metadata.TrailerStream, error) {
	return x.h.resolveMusicVideo(ctx, key)
}

// VideoWontPlay says whether a music video lookup failed for the video itself.
func (TestHooks) VideoWontPlay(err error) bool { return videoWontPlay(err) }
