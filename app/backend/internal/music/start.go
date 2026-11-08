package music

import (
	"context"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

// Start starts Music's start-up work: the package sync and, with the row
// templates on, one read of the catalog bundle so the first music page does
// not wait for its download; then every engine page's covers are stored
// ahead (preload.go). The household's own region joins when its pages are
// first asked.
func (h *Handlers) Start(ctx context.Context) {
	h.startMusicSync(ctx)
	if h.musicTplOn() {
		go jobs.Guard("music-bundle-warm", func() {
			h.tplPreload(h.musicBundle(ctx, time.Now().UTC()), "us")
		})
	}
}
