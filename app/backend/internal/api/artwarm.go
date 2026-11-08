package api

import (
	"context"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
)

// The warm queue lives in music (music/artwarm.go); api keeps the warm of
// one item, which still reads the api picture routes.
const artWarmItem = music.ArtWarmItem

type artWarmKey = music.ArtWarmKey

// warmArtItems queues albums and artists (kind, MusicBrainz id) for the
// background pool, after asking their picture records (a batch call per
// hundred) so the warm stores the versioned copies pages ask for.
func (a *API) warmArtItems(list []nameKey) {
	if len(list) == 0 || !a.partIsOn(parts.Music) {
		return
	}
	a.warmPictureRecords(list)
	a.wantPackages(list)
	h := a.mh()
	if !h.HasArtWarmDo() && (a.s.Library == nil || a.s.Library.Artwork == nil) {
		return
	}
	keys := make([]artWarmKey, 0, len(list))
	for _, n := range list {
		keys = append(keys, artWarmKey{Kind: n.kind, MBID: n.mbid})
	}
	h.QueueArtWarm(keys, func(ctx context.Context, k artWarmKey) {
		// The item's records may still be on their way (asked with the list).
		a.awaitPictureRecords(ctx, k.Kind, k.MBID, 2*time.Second)
	}, a.warmArtItem)
}

// warmArtPaths queues the items behind picture routes (/api/v1/music-trending/art/{kind}/{mbid}).
func (a *API) warmArtPaths(imgs ...string) {
	list := make([]nameKey, 0, len(imgs))
	for _, img := range imgs {
		if rest, ok := strings.CutPrefix(img, musicArtPrefix); ok {
			kind, mbid, _ := strings.Cut(rest, "/")
			list = append(list, nameKey{kind: kind, mbid: mbid})
		}
	}
	a.warmArtItems(list)
}

// warmArtItem stores one item's pictures: the photo or covers through the
// picture route (a stored file costs nothing), and an artist's art list.
func (a *API) warmArtItem(ctx context.Context, k artWarmKey) {
	switch k.Kind {
	case "album":
		a.musicTrendArtFile(ctx, "album", k.MBID, "500")
	case "artist":
		a.musicTrendArtFile(ctx, "artist", k.MBID, "500")
		a.warmArtistExtras(ctx, k.MBID)
	}
}

// warmArtistExtras stores the artist's wide background and logo from the
// metadata server (the artist page asks for them first); one the server
// has not stored yet is asked again on a later warm.
func (a *API) warmArtistExtras(ctx context.Context, mbid string) {
	a.musicTrendArtFile(ctx, "wide", mbid, "1920")
	a.musicTrendArtFile(ctx, "logo", mbid, "1200")
}
