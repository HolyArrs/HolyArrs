package api

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
)

// MusicProvider is what the shared handlers (live search, New & Popular)
// ask about music. With Music off it says so and gives nothing, so those
// handlers never reach the music code themselves.
type MusicProvider interface {
	// On reports whether music belongs in shared answers.
	On() bool
	// Suggest gives the metadata server's artist, album and song matches for q.
	Suggest(ctx context.Context, r *http.Request, q string) []SuggestRow
	// WarmNew starts New & Popular's Music lists building in the background.
	WarmNew()
}

// musicProvider gives the Music provider for the part's state now (safe
// mode keeps every part's answers, as the other shared checks do).
func (a *API) musicProvider() MusicProvider {
	if a.partIsOn(parts.Music) || a.s.Parts.SafeMode {
		return musicOn{a}
	}
	return musicOff{}
}

type musicOn struct{ a *API }

func (m musicOn) On() bool { return true }
func (m musicOn) Suggest(ctx context.Context, r *http.Request, q string) []SuggestRow {
	return m.a.suggestMusicServer(ctx, r, q)
}
func (m musicOn) WarmNew() { m.a.warmMusicNew() }

type musicOff struct{}

func (musicOff) On() bool                                                    { return false }
func (musicOff) Suggest(context.Context, *http.Request, string) []SuggestRow { return nil }
func (musicOff) WarmNew()                                                    {}
