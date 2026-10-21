package api

import (
	"context"
	"image"
	_ "image/jpeg"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/explore"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// backdropDarkLuma: a backdrop whose average brightness (0–255) is under
// this reads as an empty box behind a tile's name.
const backdropDarkLuma = 40

// backdropLuma caches each sampled backdrop's brightness (-1: sample failed).
var backdropLuma sync.Map

// backdropSampling is the one sample worker's queue.
var backdropSampling = make(chan string, 512)

var backdropOnce sync.Once

// backdropSample is the small w300 copy of a TMDB backdrop to sample.
func backdropSample(b string) (string, bool) {
	prefix := metadata.ImageBase + "/"
	rest, ok := strings.CutPrefix(b, prefix)
	if !ok {
		return "", false
	}
	_, file, ok := strings.Cut(rest, "/")
	if !ok || file == "" || strings.Contains(file, "/") {
		return "", false
	}
	return prefix + "w300/" + file, true
}

// averageLuma is an image's mean brightness over a coarse grid.
func averageLuma(img image.Image) float64 {
	r := img.Bounds()
	var sum, n float64
	for y := r.Min.Y; y < r.Max.Y; y += 4 {
		for x := r.Min.X; x < r.Max.X; x += 4 {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			sum += (0.2126*float64(cr) + 0.7152*float64(cg) + 0.0722*float64(cb)) / 257
			n++
		}
	}
	if n == 0 {
		return -1
	}
	return sum / n
}

func sampleBackdrop(ctx context.Context, cl *http.Client, b string) float64 {
	u, ok := backdropSample(b)
	if !ok {
		return -1
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return -1
	}
	resp, err := cl.Do(req)
	if err != nil {
		return -1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return -1
	}
	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return -1
	}
	return averageLuma(img)
}

// startBackdropDark makes the Browse pickers skip too-dark backdrops. The
// pickers only read the cache; unknown backdrops are queued and sampled
// one at a time in the background (cheap, low priority).
func startBackdropDark(ctx context.Context) {
	backdropOnce.Do(func() {
		cl := library.ArtworkClient()
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case b := <-backdropSampling:
					if _, done := backdropLuma.Load(b); !done {
						backdropLuma.Store(b, sampleBackdrop(ctx, cl, b))
					}
				}
			}
		}()
		explore.DarkBackdrop = func(b string) bool {
			v, ok := backdropLuma.Load(b)
			if !ok {
				queueBackdrop(b)
				return false
			}
			l := v.(float64)
			return l >= 0 && l < backdropDarkLuma
		}
	})
}

func queueBackdrop(b string) {
	if b == "" {
		return
	}
	select {
	case backdropSampling <- b:
	default:
	}
}

// sampleBackdrops samples the backdrops not yet known, waiting for each
// (the warm-up uses it so built sections pick with brightness known).
func sampleBackdrops(ctx context.Context, items []explore.Item, n int) {
	cl := library.ArtworkClient()
	for _, it := range items {
		if n == 0 || ctx.Err() != nil {
			return
		}
		if it.Backdrop == "" {
			continue
		}
		n--
		if _, done := backdropLuma.Load(it.Backdrop); !done {
			backdropLuma.Store(it.Backdrop, sampleBackdrop(ctx, cl, it.Backdrop))
		}
	}
}
