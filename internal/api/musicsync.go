package api

import (
	"context"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/parts"
	"github.com/CoderLuii/holyarrs/internal/supervisor"
)

// startMusicSync starts the artist package copy (internal/musicsync): its
// pictures go to the local picture store through the picture route's own
// fetch, and after each copy that changed packages the Explore music
// pictures are warmed again.
func (a *API) startMusicSync(ctx context.Context) {
	p := a.s.Packages
	if p == nil || !a.partIsOn(parts.Music) {
		return
	}
	if p.Pictures == nil {
		p.Pictures = func(ctx context.Context, kind, mbid, size, v string) bool {
			c, cancel := context.WithTimeout(metadata.Ahead(ctx), artWarmItem)
			defer cancel()
			metadata.WaitAhead(c)
			return a.musicArt(c, kind, mbid, size, v).ok
		}
	}
	if p.OnSynced == nil {
		p.OnSynced = func(ctx context.Context) {
			if a.musicTplOn() {
				a.tplPreload(a.musicBundle(ctx, time.Now().UTC()), "us")
			}
		}
	}
	go supervisor.Guard("music-package-sync", func() { p.Run(metadata.Ahead(ctx)) })
}

// wantPackages names to the server (package-want) the artists a music
// answer lists that no kept package covers, so they get covered; an album
// counts by its artist when Explore kept it.
func (a *API) wantPackages(list []nameKey) {
	p := a.s.Packages
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
			c := &a.music
			c.mu.Lock()
			id := strings.ToLower(c.names["album|"+strings.ToLower(n.mbid)].ArtistMBID)
			c.mu.Unlock()
			if metadata.ValidMBID(id) {
				p.Uncovered(id)
			}
		}
	}
}
