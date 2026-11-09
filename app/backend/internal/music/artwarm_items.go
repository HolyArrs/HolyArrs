package music

import (
	"context"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
)

// warmArtItems queues albums and artists (kind, MusicBrainz id) for the
// background pool, after asking their picture records (a batch call per
// hundred) so the warm stores the versioned copies pages ask for.
func (h *Handlers) warmArtItems(list []nameKey) {
	if len(list) == 0 || !h.PartIsOn(parts.Music) {
		return
	}
	h.warmPictureRecords(list)
	h.wantPackages(list)
	if !h.HasArtWarmDo() && (h.lib() == nil || h.lib().Artwork == nil) {
		return
	}
	keys := make([]ArtWarmKey, 0, len(list))
	for _, n := range list {
		keys = append(keys, ArtWarmKey{Kind: n.kind, MBID: n.mbid})
	}
	h.QueueArtWarm(keys, func(ctx context.Context, k ArtWarmKey) {
		// The item's records may still be on their way (asked with the list).
		h.AwaitPictureRecords(ctx, k.Kind, k.MBID, 2*time.Second)
	}, h.warmArtItem)
}

// warmArtPaths queues the items behind picture routes (/api/v1/music-trending/art/{kind}/{mbid}).
func (h *Handlers) warmArtPaths(imgs ...string) {
	list := make([]nameKey, 0, len(imgs))
	for _, img := range imgs {
		if rest, ok := strings.CutPrefix(img, musicArtPrefix); ok {
			kind, mbid, _ := strings.Cut(rest, "/")
			list = append(list, nameKey{kind: kind, mbid: mbid})
		}
	}
	h.warmArtItems(list)
}

// warmArtItem stores one item's pictures: the photo or covers through the
// picture route (a stored file costs nothing), and an artist's art list.
func (h *Handlers) warmArtItem(ctx context.Context, k ArtWarmKey) {
	switch k.Kind {
	case "album":
		h.musicTrendArtFile(ctx, "album", k.MBID, "500")
	case "artist":
		h.musicTrendArtFile(ctx, "artist", k.MBID, "500")
		h.warmArtistExtras(ctx, k.MBID)
	}
}

// warmArtistExtras stores the artist's wide background and logo from the
// metadata server (the artist page asks for them first); one the server
// has not stored yet is asked again on a later warm.
func (h *Handlers) warmArtistExtras(ctx context.Context, mbid string) {
	h.musicTrendArtFile(ctx, "wide", mbid, "1920")
	h.musicTrendArtFile(ctx, "logo", mbid, "1200")
}

// warmPictureRecords asks the picture records of the listed items.
func (h *Handlers) warmPictureRecords(list []nameKey) {
	kinds, mbids := make([]string, len(list)), make([]string, len(list))
	for i, n := range list {
		kinds[i], mbids[i] = n.kind, n.mbid
	}
	h.WarmPictureRecords(kinds, mbids)
}
