package music

import (
	"context"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// startMusicSync starts the artist package copy (internal/music): its
// pictures go to the local picture store through the picture route's own
// fetch, and after each copy that changed packages the Explore music
// pictures are warmed again.
func (h *Handlers) startMusicSync(ctx context.Context) {
	p := h.Packages
	if p == nil || !h.PartIsOn(parts.Music) {
		return
	}
	if p.Pictures == nil {
		p.Pictures = func(ctx context.Context, kind, mbid, size, v string) bool {
			c, cancel := context.WithTimeout(metadata.Ahead(ctx), ArtWarmItem)
			defer cancel()
			metadata.WaitAhead(c)
			return h.musicArt(c, kind, mbid, size, v).ok
		}
	}
	if p.OnSynced == nil {
		p.OnSynced = func(ctx context.Context) {
			if h.musicTplOn() {
				h.tplPreload(h.musicBundle(ctx, time.Now().UTC()), "us")
			}
		}
	}
	go jobs.Guard("music-package-sync", func() { p.Run(metadata.Ahead(ctx)) })
}

// wantPackages names to the server (package-want) the artists a music
// answer lists that no kept package covers, so they get covered; an album
// counts by its artist when Explore kept it.
func (h *Handlers) wantPackages(list []nameKey) {
	p := h.Packages
	if p == nil {
		return
	}
	for _, n := range list {
		switch n.kind {
		case "artist":
			p.Uncovered(n.mbid)
		case "album":
			if p.HasAlbum(n.mbid) {
				continue
			}
			c := &h.music
			c.mu.Lock()
			id := strings.ToLower(c.names["album|"+strings.ToLower(n.mbid)].ArtistMBID)
			c.mu.Unlock()
			if metadata.ValidMBID(id) {
				p.Uncovered(id)
			}
		}
	}
}
