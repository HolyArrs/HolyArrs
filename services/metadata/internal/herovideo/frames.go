package herovideo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // YouTube's frames and storyboard sheets
	_ "image/png"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// The still check. YouTube keeps three automatic frames of every video
// (1.jpg, 2.jpg, 3.jpg at a quarter, half and three quarters). Each is made
// grey and shrunk to 32x18; the largest mean absolute difference between
// any two is the score. A still picture over audio scores under StillBelow,
// a real video over VideoAbove; anything between, or frames that say
// nothing, gets the storyboard check later.
const (
	StillBelow = 3.0
	VideoAbove = 12.0
	frameW     = 32
	frameH     = 18
	// Storyboard check: tiles flatter than tileFlat (a blank frame) are
	// left out; neighbouring tiles differing by more than motionDiff count
	// as motion, and a video needs motionKeep of them. At most maxSheets
	// sheets are read.
	tileFlat   = 2.0
	motionDiff = 8.0
	motionKeep = 0.35
	maxSheets  = 6
)

// grey is a picture as grey values (ITU-R 601 luma, 0..255).
type grey struct {
	w, h int
	px   []float64
}

func toGrey(img image.Image) grey {
	b := img.Bounds()
	g := grey{w: b.Dx(), h: b.Dy(), px: make([]float64, b.Dx()*b.Dy())}
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			g.px[y*g.w+x] = float64(color.GrayModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.Gray).Y)
		}
	}
	return g
}

// shrink averages the grey values under each pixel of a w x h picture.
func (g grey) shrink(w, h int) grey {
	out := grey{w: w, h: h, px: make([]float64, w*h)}
	if g.w == 0 || g.h == 0 {
		return out
	}
	for y := 0; y < h; y++ {
		y0, y1 := y*g.h/h, max((y+1)*g.h/h, y*g.h/h+1)
		for x := 0; x < w; x++ {
			x0, x1 := x*g.w/w, max((x+1)*g.w/w, x*g.w/w+1)
			sum, n := 0.0, 0
			for sy := y0; sy < y1 && sy < g.h; sy++ {
				for sx := x0; sx < x1 && sx < g.w; sx++ {
					sum += g.px[sy*g.w+sx]
					n++
				}
			}
			if n > 0 {
				out.px[y*w+x] = sum / float64(n)
			}
		}
	}
	return out
}

// crop is the part of g at (x, y), w x h.
func (g grey) crop(x, y, w, h int) grey {
	out := grey{w: w, h: h, px: make([]float64, w*h)}
	for r := 0; r < h; r++ {
		copy(out.px[r*w:(r+1)*w], g.px[(y+r)*g.w+x:(y+r)*g.w+x+w])
	}
	return out
}

func (g grey) stddev() float64 {
	if len(g.px) == 0 {
		return 0
	}
	var sum, sq float64
	for _, v := range g.px {
		sum += v
		sq += v * v
	}
	n := float64(len(g.px))
	m := sum / n
	return math.Sqrt(math.Max(0, sq/n-m*m))
}

// meanDiff is the mean absolute difference of two same-size pictures.
func meanDiff(a, b grey) float64 {
	if len(a.px) == 0 || len(a.px) != len(b.px) {
		return 0
	}
	var d float64
	for i := range a.px {
		d += math.Abs(a.px[i] - b.px[i])
	}
	return d / float64(len(a.px))
}

// FrameScore is the still check's score: the largest mean absolute
// difference between any two of the frames, each grey at 32x18.
func FrameScore(frames []image.Image) float64 {
	small := make([]grey, len(frames))
	for i, f := range frames {
		small[i] = toGrey(f).shrink(frameW, frameH)
	}
	best := 0.0
	for i := range small {
		for j := i + 1; j < len(small); j++ {
			best = math.Max(best, meanDiff(small[i], small[j]))
		}
	}
	return best
}

// Motion is the storyboard check's share of neighbouring tiles that differ
// (blank tiles left out). Each sheet holds cols x rows tiles.
func Motion(sheets []image.Image, cols, rows int) float64 {
	cols, rows = max(cols, 1), max(rows, 1)
	var tiles []grey
	for _, s := range sheets {
		g := toGrey(s)
		w, h := g.w/cols, g.h/rows
		if w == 0 || h == 0 {
			continue
		}
		for r := 0; r < rows; r++ {
			for c := 0; c < cols; c++ {
				if t := g.crop(c*w, r*h, w, h); t.stddev() > tileFlat {
					tiles = append(tiles, t)
				}
			}
		}
	}
	moving, pairs := 0, 0
	for i := 1; i < len(tiles); i++ {
		pairs++
		if meanDiff(tiles[i-1], tiles[i]) > motionDiff {
			moving++
		}
	}
	return float64(moving) / float64(max(1, pairs))
}

// verdictOf reads a frame score.
func verdictOf(score float64) string {
	switch {
	case score < StillBelow:
		return VerdictStill
	case score > VideoAbove:
		return VerdictVideo
	}
	return VerdictPending
}

// errGone: YouTube has no such video (its frames are missing).
var errGone = errors.New("herovideo: video gone")

// fetchBytes reads one picture's file.
func (s *Service) fetchBytes(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.o.UserAgent)
	resp, err := s.o.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errGone
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{resp.StatusCode}
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

func (s *Service) fetchImage(ctx context.Context, u string) (image.Image, error) {
	b, err := s.fetchBytes(ctx, u)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	return img, err
}

// frames scores a video by its three automatic frames. telling is false
// when the frames say nothing: the same file three times, or all three
// blank (YouTube sends a black stand-in when it has no frames), so the
// storyboard check decides.
func (s *Service) frames(ctx context.Context, id string) (score float64, telling bool, err error) {
	var files [3][]byte
	var errs [3]error
	var wg sync.WaitGroup
	for i := range files {
		wg.Add(1)
		go func() {
			defer wg.Done()
			files[i], errs[i] = s.fetchBytes(ctx, fmt.Sprintf("%s/vi/%s/%d.jpg", s.o.Thumbs, id, i+1))
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if errors.Is(err, errGone) {
			return 0, false, errGone
		}
	}
	for _, err := range errs {
		if err != nil {
			return 0, false, err
		}
	}
	imgs := make([]image.Image, len(files))
	blank := true
	for i, b := range files {
		if imgs[i], _, err = image.Decode(bytes.NewReader(b)); err != nil {
			return 0, false, err
		}
		blank = blank && toGrey(imgs[i]).shrink(frameW, frameH).stddev() < tileFlat
	}
	same := bytes.Equal(files[0], files[1]) && bytes.Equal(files[1], files[2])
	return FrameScore(imgs), !same && !blank, nil
}

// sheetURL reports a storyboard sheet on YouTube's picture hosts (or the
// configured picture root), the only addresses the check reads.
func (s *Service) sheetURL(raw string) bool {
	if strings.HasPrefix(raw, s.o.Thumbs+"/") {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "ytimg.com" || strings.HasSuffix(h, ".ytimg.com")
}

// storyboard runs the full check on a video: yt-dlp lists its storyboard
// (the smallest "sb" format); up to maxSheets sheets are read and the
// share of moving tiles decides.
func (s *Service) storyboard(ctx context.Context, id string) (string, error) {
	raw, err := s.o.VideoJSON(ctx, id)
	if err != nil {
		return "", err
	}
	var d struct {
		Formats []struct {
			ID        string `json:"format_id"`
			Width     int    `json:"width"`
			Columns   int    `json:"columns"`
			Rows      int    `json:"rows"`
			Fragments []struct {
				URL string `json:"url"`
			} `json:"fragments"`
		} `json:"formats"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return "", err
	}
	best, bestW := -1, 0
	for i, f := range d.Formats {
		if !strings.HasPrefix(f.ID, "sb") || len(f.Fragments) == 0 {
			continue
		}
		w := f.Width
		if w == 0 {
			w = 999
		}
		if best < 0 || w < bestW {
			best, bestW = i, w
		}
	}
	if best < 0 {
		return "", errors.New("herovideo: no storyboard")
	}
	f := d.Formats[best]
	cols, rows := f.Columns, f.Rows
	if cols <= 0 {
		cols = 5
	}
	if rows <= 0 {
		rows = 5
	}
	var sheets []image.Image
	for i, frag := range f.Fragments {
		if i >= maxSheets {
			break
		}
		if !s.sheetURL(frag.URL) {
			return "", errors.New("herovideo: storyboard sheet off YouTube")
		}
		img, err := s.fetchImage(ctx, frag.URL)
		if err != nil {
			return "", err
		}
		sheets = append(sheets, img)
	}
	if Motion(sheets, cols, rows) >= motionKeep {
		return VerdictVideo, nil
	}
	return VerdictStill, nil
}
