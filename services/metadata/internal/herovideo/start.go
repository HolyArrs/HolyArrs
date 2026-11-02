package herovideo

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"math"
	"sort"
)

// The start point. The hero plays from the stored start, so the first
// clipLen seconds from there are what people see. YouTube's "most
// replayed" peak is often a famous slow or title moment, so the start is
// judged on the storyboard tiles (frames at fixed steps): a window moves
// when its neighbouring tiles differ by clipMove on average and at most
// cardMax of its tiles look like a title card (one grey level covering
// cardFlat of the frame: a flat or black backdrop, alone or with text on
// it). The start is the peak when its window moves; else the nearest
// window that moves within nearReach seconds of it; else the most moving
// window after the first introSkip of the video. Without a peak, 30% of the
// duration (the app's own default) stands in for it.
//
// Measured on real storyboards (tiles about 2 s apart, grey at 32x18): cut
// and dance scenes score 30 to 75, a slow single shot 7 to 10, a held
// title or end card under 2.
const (
	clipLen   = 12.0
	clipMove  = 12.0
	cardFlat  = 0.8
	cardMax   = 0.5
	nearReach = 45.0
	introSkip = 0.10
	// defaultAt is the app's start without a peak (a share of the length).
	defaultAt = 0.30
)

// Why a start was chosen (kept with it, and logged).
const (
	WhyPeak       = "peak"         // the most replayed peak moves
	WhyNearPeak   = "near_peak"    // the nearest moving window to the peak
	WhyDefault    = "default"      // no peak: 30% in moves
	WhyNearDef    = "near_default" // no peak: the nearest moving window to 30% in
	WhyMostMoving = "most_moving"  // nothing near moved: the most moving window
	WhyUnjudged   = "unjudged"     // the storyboard could not be read now (tried again later)
	WhyNoBoard    = "no_board"     // no storyboard to judge by: the peak as it is
)

// Tile is one storyboard frame: its time in seconds and its grey picture
// at 32x18.
type Tile struct {
	At float64
	g  grey
}

// flatShare is the share of a tile its most common grey level (of eight)
// covers.
func flatShare(g grey) float64 {
	if len(g.px) == 0 {
		return 0
	}
	var bins [8]int
	for _, v := range g.px {
		bins[min(7, int(v)/32)]++
	}
	top := 0
	for _, n := range bins {
		top = max(top, n)
	}
	return float64(top) / float64(len(g.px))
}

// cardTile reports a tile that looks like a title card: one grey level
// covers cardFlat of it (a flat backdrop, alone or with text on it).
func cardTile(g grey) bool { return flatShare(g) >= cardFlat }

// window is the tiles shown from a start: from <= At <= from+clipLen.
func window(tiles []Tile, from float64) []Tile {
	i := sort.Search(len(tiles), func(i int) bool { return tiles[i].At >= from-1e-6 })
	j := i
	for j < len(tiles) && tiles[j].At <= from+clipLen+1e-6 {
		j++
	}
	return tiles[i:j]
}

// judgeWindow answers a window's local motion (the mean difference between
// neighbouring tiles) and its share of title-card tiles (all of it when it
// opens on one: that is the first thing seen); ok is false with fewer than
// two tiles.
func judgeWindow(w []Tile) (motion, card float64, ok bool) {
	if len(w) < 2 {
		return 0, 0, false
	}
	cards := 0
	for i, t := range w {
		if i > 0 {
			motion += meanDiff(w[i-1].g, t.g)
		}
		if cardTile(t.g) {
			cards++
		}
	}
	card = float64(cards) / float64(len(w))
	if cardTile(w[0].g) {
		card = 1
	}
	return motion / float64(len(w)-1), card, true
}

func moves(motion, card float64) bool { return motion >= clipMove && card <= cardMax }

// rank orders windows when none moves enough: motion, less the title cards.
func rank(motion, card float64) float64 { return motion * (1 - card) }

// Pick is a chosen start: the second, its window's local motion and why.
type Pick struct {
	Start  float64
	Motion float64
	Why    string
}

// PickStart chooses the start from the storyboard tiles (sorted by time) of
// a video of dur seconds, given its most replayed peak (found false: none).
// judged is false when the tiles cannot judge any window.
func PickStart(tiles []Tile, dur, peak float64, found bool) (p Pick, judged bool) {
	anchor, here, near := peak, WhyPeak, WhyNearPeak
	if !found {
		anchor, here, near = math.Round(dur*defaultAt*100)/100, WhyDefault, WhyNearDef
	}
	if m, c, ok := judgeWindow(window(tiles, anchor)); ok && moves(m, c) {
		return Pick{Start: anchor, Motion: m, Why: here}, true
	}
	// Starts on the tiles whose whole window fits in the video.
	type cand struct {
		at, motion, card float64
	}
	var cands []cand
	for _, t := range tiles {
		if dur > 0 && t.At+clipLen > dur {
			continue
		}
		if m, c, ok := judgeWindow(window(tiles, t.At)); ok {
			cands = append(cands, cand{t.At, m, c})
		}
	}
	if len(cands) == 0 {
		return Pick{Start: anchor}, false
	}
	// Nearest to the anchor first; at the same distance the later one (the
	// replayed part goes on after its peak).
	byNear := append([]cand(nil), cands...)
	sort.SliceStable(byNear, func(i, j int) bool {
		di, dj := math.Abs(byNear[i].at-anchor), math.Abs(byNear[j].at-anchor)
		if math.Abs(di-dj) > 1e-6 {
			return di < dj
		}
		return byNear[i].at > byNear[j].at
	})
	for _, c := range byNear {
		if math.Abs(c.at-anchor) > nearReach {
			break
		}
		if moves(c.motion, c.card) {
			return Pick{Start: c.at, Motion: c.motion, Why: near}, true
		}
	}
	best := -1
	for i, c := range cands {
		if c.at < dur*introSkip {
			continue
		}
		if best < 0 || rank(c.motion, c.card) > rank(cands[best].motion, cands[best].card) {
			best = i
		}
	}
	if best < 0 {
		// A video too short to skip its intro: any window.
		for i, c := range cands {
			if best < 0 || rank(c.motion, c.card) > rank(cands[best].motion, cands[best].card) {
				best = i
			}
		}
	}
	return Pick{Start: cands[best].at, Motion: cands[best].motion, Why: WhyMostMoving}, true
}

// board is a video's storyboard as yt-dlp lists it, with its heatmap peak.
type board struct {
	Duration float64 `json:"duration"`
	Heatmap  []struct {
		Start float64 `json:"start_time"`
		Value float64 `json:"value"`
	} `json:"heatmap"`
	Formats []sbFormat `json:"formats"`
}

type sbFormat struct {
	ID        string  `json:"format_id"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	Columns   int     `json:"columns"`
	Rows      int     `json:"rows"`
	FPS       float64 `json:"fps"`
	Fragments []struct {
		URL      string  `json:"url"`
		Duration float64 `json:"duration"`
	} `json:"fragments"`
}

// peak is the heatmap's highest point.
func (b board) peak() (at float64, found bool) {
	best := -1.0
	for _, h := range b.Heatmap {
		if h.Value > best {
			best, at = h.Value, h.Start
		}
	}
	return at, best >= 0
}

// grid is each sheet's columns and rows (5x5 when not given).
func (f sbFormat) grid() (int, int) {
	c, r := f.Columns, f.Rows
	if c <= 0 {
		c = 5
	}
	if r <= 0 {
		r = 5
	}
	return c, r
}

// step is the seconds between tiles.
func (f sbFormat) step() float64 {
	if f.FPS > 0 {
		return 1 / f.FPS
	}
	c, r := f.grid()
	if len(f.Fragments) > 0 && f.Fragments[0].Duration > 0 {
		return f.Fragments[0].Duration / float64(c*r)
	}
	return 0
}

// startFormat is the storyboard for the start: the finest steps within
// maxSheets sheets, the smallest tiles among equal steps.
func (b board) startFormat() (sbFormat, bool) {
	var best sbFormat
	found := false
	for _, f := range b.Formats {
		if len(f.ID) < 2 || f.ID[:2] != "sb" || len(f.Fragments) == 0 || len(f.Fragments) > maxSheets || f.step() <= 0 {
			continue
		}
		switch {
		case !found, f.step() < best.step()*0.95:
		case f.step() <= best.step()*1.05 && f.Width < best.Width:
		default:
			continue
		}
		best, found = f, true
	}
	return best, found
}

// SheetTiles cuts sheets into timed tiles: each sheet holds cols x rows
// tiles of w x h (0: the sheet divided evenly), step seconds apart, and
// covers its given seconds (0: a full sheet); tiles past a sheet's seconds
// or the video's end (dur, 0: unknown) are left out.
func SheetTiles(sheets []image.Image, spans []float64, cols, rows, w, h int, step, dur float64) []Tile {
	var out []Tile
	at := 0.0
	for i, s := range sheets {
		g := toGrey(s)
		tw, th := w, h
		if tw <= 0 || th <= 0 || tw*cols > g.w || th*rows > g.h {
			tw, th = g.w/cols, g.h/rows
		}
		span := float64(cols*rows) * step
		if i < len(spans) && spans[i] > 0 {
			span = spans[i]
		}
		for k := 0; k < cols*rows; k++ {
			t := at + float64(k)*step
			if t >= at+span-1e-6 || dur > 0 && t >= dur {
				break
			}
			r, c := k/cols, k%cols
			if (c+1)*tw > g.w || (r+1)*th > g.h || tw == 0 || th == 0 {
				break
			}
			out = append(out, Tile{At: math.Round(t*100) / 100, g: g.crop(c*tw, r*th, tw, th).shrink(frameW, frameH)})
		}
		at += span
	}
	return out
}

// errNoBoard: the video lists no storyboard to judge a start by.
var errNoBoard = errors.New("herovideo: no storyboard")

// startOf reads a video's start: one yt-dlp read (heatmap and storyboard)
// and the sheets of the start's storyboard. err is yt-dlp's failure;
// sheetErr a sheet that could not be read (the peak is kept, unjudged).
func (s *Service) startOf(ctx context.Context, id string) (p Pick, dur float64, peak float64, found bool, sheetErr, err error) {
	raw, err := s.o.VideoJSON(ctx, id)
	if err != nil {
		return Pick{}, 0, 0, false, nil, err
	}
	var b board
	if err := json.Unmarshal(raw, &b); err != nil {
		return Pick{}, 0, 0, false, nil, err
	}
	dur = b.Duration
	peak, found = b.peak()
	keep := Pick{Start: peak, Motion: -1, Why: WhyNoBoard}
	f, ok := b.startFormat()
	if !ok {
		return keep, dur, peak, found, errNoBoard, nil
	}
	sheets := make([]image.Image, 0, len(f.Fragments))
	spans := make([]float64, 0, len(f.Fragments))
	for _, frag := range f.Fragments {
		if !s.sheetURL(frag.URL) {
			return keep, dur, peak, found, errors.New("herovideo: storyboard sheet off YouTube"), nil
		}
		img, err := s.fetchImage(ctx, frag.URL)
		if err != nil {
			keep.Why = WhyUnjudged
			return keep, dur, peak, found, err, nil
		}
		sheets = append(sheets, img)
		spans = append(spans, frag.Duration)
	}
	cols, rows := f.grid()
	tiles := SheetTiles(sheets, spans, cols, rows, f.Width, f.Height, f.step(), dur)
	got, judged := PickStart(tiles, dur, peak, found)
	if !judged {
		return keep, dur, peak, found, errNoBoard, nil
	}
	return got, dur, peak, found, nil, nil
}
