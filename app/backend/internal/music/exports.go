package music

import (
	"context"
	"net/http"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music/rowtpl"
)

// What the program and other modules (discover, search, titles) call on the
// Music module.

// ArtPrefix starts every music picture route address.
const ArtPrefix = musicArtPrefix

// NameKey is an album or artist the picture route may serve.
type NameKey = nameKey

// NewNameKey makes a NameKey (kind "album" or "artist").
func NewNameKey(kind, mbid, name, artist string) NameKey {
	return nameKey{kind: kind, mbid: mbid, name: name, artist: artist}
}

// RouteMux registers every Music route on m.
func (h *Handlers) RouteMux(m *http.ServeMux) {
	h.musicRoutes(m)
	h.libraryHandlers().Routes(m)
	h.musicPageRoutes(m)
	m.HandleFunc("GET /api/v1/explore/new/music", h.exploreNewMusic)
	m.HandleFunc("GET /api/v1/metadata/album/{mbid}/follow", h.getAlbumFollow)
	m.HandleFunc("PUT /api/v1/metadata/album/{mbid}/follow", h.putAlbumFollow)
	m.HandleFunc("DELETE /api/v1/metadata/album/{mbid}/follow", h.deleteAlbumFollow)
	m.HandleFunc("GET /api/v1/music-trending", h.musicTrending)
	m.HandleFunc("GET /api/v1/explore/music", h.exploreMusic)
	m.HandleFunc("GET /api/v1/explore/music/rising", h.risingToday)
	m.HandleFunc("GET /api/v1/music-trending/art/{kind}/{mbid}", h.musicTrendArt)
	m.HandleFunc("GET /api/v1/music/genre/{slug}", h.musicGenrePage)
	m.HandleFunc("GET /api/v1/music/browse/{kind}/{slug}", h.musicBrowsePage)
	m.HandleFunc("GET /api/v1/music/browse-tiles", h.musicBrowseTiles)
	h.genreArtHandlers().Routes(m)
}

// FollowsChanged drops a person's music rows built from their follows.
func (h *Handlers) FollowsChanged(userID int64) { h.followsChanged(userID) }

// PreloadStatus is the cover preload's progress for the health page.
func (h *Handlers) PreloadStatus() any { return h.tplPreloadStatus() }

// OnEvent forgets kept answers when an ask or a title changes what they show.
func (h *Handlers) OnEvent(ev domain.Event) { h.musicOnEvent(ev) }

// StartSync starts the artist package copy loop.
func (h *Handlers) StartSync(ctx context.Context) { h.startMusicSync(ctx) }

// TplOn says whether the row template engine serves the music pages.
func (h *Handlers) TplOn() bool { return h.musicTplOn() }

// Bundle is the catalog bundle the row templates read now.
func (h *Handlers) Bundle(ctx context.Context, now time.Time) *rowtpl.Bundle {
	return h.musicBundle(ctx, now)
}

// TplPreload asks for the covers of a region's pages for bundle b.
func (h *Handlers) TplPreload(b *rowtpl.Bundle, region string) { h.tplPreload(b, region) }

// RememberNames lets the picture route serve these albums and artists.
func (h *Handlers) RememberNames(list []NameKey) { h.rememberMusicNames(list) }

// WarmNames stores these albums' and artists' pictures ahead.
func (h *Handlers) WarmNames(list []NameKey) { h.warmMusicNames(list) }

// TrendArtFile stores (or finds) a music picture; ok is false when there is none.
func (h *Handlers) TrendArtFile(ctx context.Context, kind, mbid, size string) (name, path string, ok bool) {
	return h.musicTrendArtFile(ctx, kind, mbid, size)
}

// LibArtist is a Library artist as the search dropdown reads it.
type LibArtist struct {
	ID         int64
	Name, MBID string
	Image      string
	HadAny     bool
}

// LibAlbum is a Library album as the search dropdown reads it.
type LibAlbum struct {
	PartID, TitleID int64
	Name, MBID      string
}

// LibrarySnapshot is the Library's artists and albums; HadParts are the
// albums with a file.
type LibrarySnapshot struct {
	Artists  []LibArtist
	Albums   []LibAlbum
	HadParts map[int64]bool
}

// Library reads the kept Library snapshot the music rows are built from.
func (h *Handlers) LibrarySnapshot(ctx context.Context, now time.Time) (*LibrarySnapshot, error) {
	lib, err := h.musicLibrary(ctx, now)
	if err != nil || lib == nil {
		return nil, err
	}
	out := &LibrarySnapshot{HadParts: lib.hadParts}
	for _, a := range lib.artists {
		out.Artists = append(out.Artists, LibArtist{ID: a.id, Name: a.name, MBID: a.mbid, Image: a.image, HadAny: a.hadAny})
	}
	for _, a := range lib.albums {
		out.Albums = append(out.Albums, LibAlbum{PartID: a.partID, TitleID: a.titleID, Name: a.name, MBID: a.mbid})
	}
	return out, nil
}

// AskStates are the household's open asks: artist and album mbid → asked
// or approved.
type AskStates struct{ Artists, Albums map[string]string }

// AskStatesOf reads the household's music ask states for member.
func (h *Handlers) AskStatesOf(ctx context.Context, member int64) (*AskStates, error) {
	acts, err := h.musicActsOf(ctx, member)
	if err != nil || acts == nil {
		return nil, err
	}
	return &AskStates{Artists: acts.artistState, Albums: acts.albumState}, nil
}

// VideoHooks gives the trailers the artists' music videos.
func (h *Handlers) VideoHooks() VideoHooks { return VideoHooks{h} }

// VideoHooks is metadata.Trailers' Videos hook over the Music module.
type VideoHooks struct{ h *Handlers }

func (v VideoHooks) On() bool { return v.h.ArtistVideosOn() }

func (v VideoHooks) Asks(ctx context.Context, now time.Time) []string {
	return v.h.artistAsks(ctx, now)
}

func (v VideoHooks) Due(ctx context.Context, mbid string, now time.Time) (bool, bool, bool) {
	return v.h.artistVideoDue(ctx, mbid, now)
}

func (v VideoHooks) Pass(ctx context.Context, mbid string, renew bool) error {
	return v.h.passArtistVideo(ctx, mbid, renew)
}

func (v VideoHooks) SaveAsks(ctx context.Context) { v.h.saveArtistAsks(ctx) }

func (v VideoHooks) Ready(r *http.Request, mbid string) (any, bool) {
	e, kept := v.h.keptVideos(r.Context(), mbid)
	if !kept {
		return nil, false
	}
	vid, s, ready := v.h.readyVideo(r.Context(), e.list)
	if !ready {
		return nil, false
	}
	view, ok := v.h.videoView(r, vid, s)
	return view, ok
}

func (v VideoHooks) Note(ctx context.Context, mbids []string) { v.h.noteArtistVideos(ctx, mbids) }

// KnownGenres is an artist's genres from the music genre cache, without
// queueing a lookup (nil when not known).
func (h *Handlers) KnownGenres(mbid string) []string {
	c := &h.musicx
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.genres[mbid].genres
}

// SetPreloadOn turns the music cover preload on or off (tests keep it off
// unless they ask for it).
func SetPreloadOn(on bool) { musicPreloadOn = on }
