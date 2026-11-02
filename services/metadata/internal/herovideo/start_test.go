package herovideo

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"
)

// Synthetic storyboards: one tile every 2 s, each kind drawn at 32x18.
type tileKind int

const (
	moving tileKind = iota // a new picture every tile
	still                  // the same picture
	card                   // black with white text that changes
)

func drawTile(k tileKind, i int) grey {
	g := grey{w: frameW, h: frameH, px: make([]float64, frameW*frameH)}
	for y := 0; y < frameH; y++ {
		for x := 0; x < frameW; x++ {
			var v float64
			switch k {
			case moving:
				v = float64((x*7+y*13+i*61)%256) * 0.9
			case still:
				v = float64(40 + x*5 + y*2)
			case card:
				// A title on black: a few white letters, placed anew each tile.
				if y >= 7 && y <= 10 && (x+i*3)%6 < 2 && x > 4 && x < 28 {
					v = 255
				}
			}
			g.px[y*frameW+x] = v
		}
	}
	return g
}

// board of dur seconds: kind(t) gives each tile's kind.
func synthBoard(dur float64, kind func(t float64) tileKind) []Tile {
	var out []Tile
	for i := 0; float64(i)*2 < dur; i++ {
		t := float64(i) * 2
		out = append(out, Tile{At: t, g: drawTile(kind(t), i)})
	}
	return out
}

func TestPickStartAllMovingKeepsPeak(t *testing.T) {
	tiles := synthBoard(200, func(float64) tileKind { return moving })
	p, ok := PickStart(tiles, 200, 95.5, true)
	if !ok || p.Start != 95.5 || p.Why != WhyPeak || p.Motion < clipMove {
		t.Fatalf("pick %+v ok %v", p, ok)
	}
}

func TestPickStartStillPeakPicksNearby(t *testing.T) {
	// Still from 80 s to 120 s; the peak in its middle.
	tiles := synthBoard(200, func(t float64) tileKind {
		if t >= 80 && t < 120 {
			return still
		}
		return moving
	})
	p, ok := PickStart(tiles, 200, 100, true)
	if !ok || p.Why != WhyNearPeak || math.Abs(p.Start-100) > nearReach || p.Motion < clipMove {
		t.Fatalf("pick %+v ok %v", p, ok)
	}
	// The window from the start shows little of the still part.
	if m, c, _ := judgeWindow(window(tiles, p.Start)); !moves(m, c) {
		t.Fatalf("window from %v: motion %.1f card %.2f", p.Start, m, c)
	}
	// It is the nearest such start: the next one in toward the peak does not move.
	for _, next := range []float64{p.Start - 2, p.Start + 2} {
		if math.Abs(next-100) < math.Abs(p.Start-100) {
			if m, c, _ := judgeWindow(window(tiles, next)); moves(m, c) {
				t.Fatalf("nearer start %v moves too (%.1f)", next, m)
			}
		}
	}
}

func TestPickStartAvoidsTitleCard(t *testing.T) {
	// A title card from 60 s to 76 s: its text changes, so the tiles differ,
	// but each is mostly black.
	tiles := synthBoard(200, func(t float64) tileKind {
		if t >= 60 && t < 76 {
			return card
		}
		return moving
	})
	if m, c, _ := judgeWindow(window(tiles, 62)); m < clipMove || c <= cardMax {
		t.Fatalf("card window: motion %.1f card %.2f (want motion that would pass alone)", m, c)
	}
	p, ok := PickStart(tiles, 200, 62, true)
	if !ok || p.Why != WhyNearPeak || p.Start >= 60 && p.Start < 76 {
		t.Fatalf("pick %+v ok %v", p, ok)
	}
}

func TestPickStartNothingNearMoves(t *testing.T) {
	// Still everywhere but a late stretch (and the intro, which is skipped).
	tiles := synthBoard(300, func(t float64) tileKind {
		if t < 20 || t >= 220 && t < 250 {
			return moving
		}
		return still
	})
	p, ok := PickStart(tiles, 300, 100, true)
	if !ok || p.Why != WhyMostMoving || p.Start < 300*introSkip || p.Start < 200 || p.Start > 250 {
		t.Fatalf("pick %+v ok %v", p, ok)
	}
}

func TestPickStartWithoutPeak(t *testing.T) {
	tiles := synthBoard(200, func(float64) tileKind { return moving })
	if p, ok := PickStart(tiles, 200, 0, false); !ok || p.Start != 60 || p.Why != WhyDefault {
		t.Fatalf("pick %+v ok %v", p, ok)
	}
	tiles = synthBoard(200, func(t float64) tileKind {
		if t >= 50 && t < 75 {
			return still
		}
		return moving
	})
	if p, ok := PickStart(tiles, 200, 0, false); !ok || p.Why != WhyNearDef || p.Start == 60 {
		t.Fatalf("pick %+v ok %v", p, ok)
	}
	if p, ok := PickStart(nil, 200, 42, true); ok || p.Start != 42 {
		t.Fatalf("no tiles: %+v ok %v", p, ok)
	}
}

// sheetPNG lays tiles out cols x rows at tw x th each.
func sheetPNG(tiles []grey, cols, rows, tw, th int) []byte {
	img := image.NewGray(image.Rect(0, 0, cols*tw, rows*th))
	for k, g := range tiles {
		r, c := k/cols, k%cols
		for y := 0; y < th; y++ {
			for x := 0; x < tw; x++ {
				img.SetGray(c*tw+x, r*th+y, color.Gray{Y: uint8(g.px[(y*g.h/th)*g.w+x*g.w/tw])})
			}
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func TestSheetTilesTimes(t *testing.T) {
	var gs []grey
	for i := range 6 {
		g := grey{w: frameW, h: frameH, px: make([]float64, frameW*frameH)}
		for j := range g.px {
			g.px[j] = float64(i * 40)
		}
		gs = append(gs, g)
	}
	img, _, err := image.Decode(bytes.NewReader(sheetPNG(gs, 3, 2, 48, 27)))
	if err != nil {
		t.Fatal(err)
	}
	// Two sheets of six tiles 2 s apart; the second covers 6 s; the video ends at 19 s.
	tiles := SheetTiles([]image.Image{img, img}, []float64{12, 6}, 3, 2, 48, 27, 2, 19)
	var at []float64
	for _, tl := range tiles {
		at = append(at, tl.At)
	}
	if fmt.Sprint(at) != "[0 2 4 6 8 10 12 14 16]" {
		t.Fatalf("times %v", at)
	}
	// Each tile cut from its place: the second sheet starts over at the first.
	for i, want := range []float64{0, 40, 80, 120, 160, 200, 0, 40, 80} {
		if g := tiles[i].g; math.Abs(g.px[len(g.px)/2]-want) > 1 || g.stddev() > 1 {
			t.Fatalf("tile %d: %v, want %v", i, g.px[len(g.px)/2], want)
		}
	}
}

// LALISA (awkkyBH2zEo, 206 s): YouTube's smallest storyboard, 10x10 tiles of
// 48x27 about 2.06 s apart. Its most replayed peak, 146.97 s, is the gold
// dance scene, which moves: the peak stays. Its first seconds are black and
// slow black-and-white shots, its last ones a held end card.
func TestPickStartLALISA(t *testing.T) {
	img, _, err := image.Decode(bytes.NewReader(fixture(t, "lalisa_storyboard.jpg")))
	if err != nil {
		t.Fatal(err)
	}
	tiles := SheetTiles([]image.Image{img}, []float64{206}, 10, 10, 48, 27, 2.06, 206)
	if len(tiles) != 100 {
		t.Fatalf("%d tiles", len(tiles))
	}
	p, ok := PickStart(tiles, 206, 146.97, true)
	if !ok || p.Start != 146.97 || p.Why != WhyPeak || p.Motion < 30 {
		t.Fatalf("pick %+v ok %v", p, ok)
	}
	// The end card does not pass, and a peak there moves back into the video.
	if m, c, _ := judgeWindow(window(tiles, 199.82)); moves(m, c) {
		t.Fatalf("end card moves: %.1f %.2f", m, c)
	}
	if p, _ := PickStart(tiles, 206, 199.82, true); p.Why != WhyNearPeak || p.Start+clipLen > 206 {
		t.Fatalf("end-card peak: %+v", p)
	}
}

// Through the service: the start is judged on the storyboard of the same
// yt-dlp read, stored with its reason, and a start stored before the
// judgement is judged once by the background pass.
func TestStartJudgedOnStoryboard(t *testing.T) {
	// 100 tiles 2 s apart: still from 80 s to 120 s, the peak at 100 s.
	var gs []grey
	for i := range 100 {
		k := moving
		if i >= 40 && i < 60 {
			k = still
		}
		gs = append(gs, drawTile(k, i))
	}
	var e *env
	reads := 0
	e = newEnv(t, drake, func(o *Options) {
		o.HeatmapMax = 1
		o.Heatmap = func(context.Context, string) (float64, float64, bool, error) {
			t.Error("heatmap read apart from the storyboard")
			return 0, 0, false, nil
		}
		o.VideoJSON = func(_ context.Context, id string) ([]byte, error) {
			reads++
			return []byte(`{"duration":200,"heatmap":[{"start_time":20,"value":0.4},{"start_time":100,"value":1}],
"formats":[{"format_id":"sb3","width":48,"height":27,"columns":10,"rows":10,"fps":0.5,"fragments":[{"url":"` + e.f.srv.URL + `/sheet.jpg","duration":200}]},{"format_id":"18"}]}`), nil
		}
	})
	e.f.sheets = sheetPNG(gs, 10, 10, 48, 27)
	ctx := context.Background()
	if err := e.s.Collect(ctx, artist); err != nil {
		t.Fatal(err)
	}
	a := e.answer(t)
	first := a.Videos[0]
	if first.Start == nil || *first.Start == 100 || math.Abs(*first.Start-100) > nearReach || reads != 1 || e.f.count("sheet") != 1 {
		t.Fatalf("start %v, reads %d, sheets %d", first.Start, reads, e.f.count("sheet"))
	}
	var why string
	var motion float64
	_ = e.s.db.QueryRow(`SELECT why, motion FROM verdicts WHERE video=?`, first.YouTubeID).Scan(&why, &motion)
	if why != WhyNearPeak || motion < clipMove {
		t.Fatalf("why %q motion %v", why, motion)
	}
	if _, deep := e.s.Due(ctx, 10); len(deep) != 0 {
		t.Fatalf("judged start due again: %v", deep)
	}
	// A start stored by an older build (the peak, not judged).
	if _, err := e.s.db.Exec(`UPDATE verdicts SET start=100, motion=NULL, why='' WHERE video=?`, first.YouTubeID); err != nil {
		t.Fatal(err)
	}
	if err := e.s.rebuild(ctx, artist); err != nil {
		t.Fatal(err)
	}
	if s := e.answer(t).Videos[0].Start; s == nil || *s != 100 {
		t.Fatalf("old start %v", s)
	}
	_, deep := e.s.Due(ctx, 10)
	if len(deep) != 1 || deep[0] != first.YouTubeID {
		t.Fatalf("due %v", deep)
	}
	if err := e.s.DeepCheck(ctx, first.YouTubeID); err != nil {
		t.Fatal(err)
	}
	if s := e.answer(t).Videos[0].Start; s == nil || *s != *first.Start || reads != 2 {
		t.Fatalf("rejudged start %v (want %v), reads %d", s, *first.Start, reads)
	}
	if _, deep := e.s.Due(ctx, 10); len(deep) != 0 {
		t.Fatalf("due after rejudging: %v", deep)
	}
	// A sheet that fails keeps the peak, judged again a day later.
	_, _ = e.s.db.Exec(`UPDATE verdicts SET motion=NULL, why='' WHERE video=?`, first.YouTubeID)
	e.f.sheets = nil
	e.s.o.VideoJSON = func(context.Context, string) ([]byte, error) {
		return []byte(`{"duration":200,"heatmap":[{"start_time":100,"value":1}],
"formats":[{"format_id":"sb3","width":48,"height":27,"columns":10,"rows":10,"fps":0.5,"fragments":[{"url":"` + e.f.srv.URL + `/sheet.jpg","duration":200}]}]}`), nil
	}
	if err := e.s.DeepCheck(ctx, first.YouTubeID); err != nil {
		t.Fatal(err)
	}
	_ = e.s.db.QueryRow(`SELECT why FROM verdicts WHERE video=?`, first.YouTubeID).Scan(&why)
	if _, deep := e.s.Due(ctx, 10); why != WhyUnjudged || len(deep) != 0 {
		t.Fatalf("why %q, due %v", why, deep)
	}
	e.clk.add(recheckEvery)
	if _, deep := e.s.Due(ctx, 10); len(deep) != 1 {
		t.Fatalf("due a day later %v", deep)
	}
}
