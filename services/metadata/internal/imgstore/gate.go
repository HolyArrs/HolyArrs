package imgstore

import (
	"image"
	"image/color"
	"math"
	"sort"
)

// Quality floor (image bench 2026-10-24; picture plan 2026-10-26).
const (
	// MinSide is the least shorter side of a cover or photo: a smaller one
	// goes to the next source.
	MinSide       = 300
	MinWideWidth  = 1280
	MinLogoWidth  = 400
	minWideAspect = 1.3
	textRowMin    = 39   // characters in a row
	flatShareMin  = 0.90 // share of flat pixels of a promo card
	// BlurryBelow is the Laplacian variance (grey, at most 256 px) under
	// which a wide background is clearly blurry (fan uploads at 1280 px and
	// more: an out-of-focus one measured 21, good ones 190 to 1500). Photos
	// are not checked: their sources' official portraits fetched at 500
	// measure 60 and up and are sharp.
	BlurryBelow = 100.0
)

// Gate is a candidate's quality check.
type Gate struct {
	OK        bool    `json:"ok"`
	Reason    string  `json:"reason,omitempty"` // size, solid, alpha, blur, text
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	Sharpness float64 `json:"sharpness,omitempty"`
	TextRow   int     `json:"text_row,omitempty"`
	Flat      float64 `json:"flat,omitempty"`
}

// CheckGate checks a decoded candidate for its kind: the size floor, never
// a near-solid stand-in, alpha for a logo, and for an artist's wide
// background sharpness and the text/promo detector.
func CheckGate(kind string, img image.Image) Gate {
	b := img.Bounds()
	g := Gate{Width: b.Dx(), Height: b.Dy()}
	if g.Width == 0 || g.Height == 0 {
		g.Reason = "size"
		return g
	}
	switch kind {
	case KindWide:
		if g.Width < MinWideWidth || float64(g.Width)/float64(g.Height) < minWideAspect {
			g.Reason = "size"
			return g
		}
	case KindAlbum, KindArtist:
		if minInt(g.Width, g.Height) < MinSide {
			g.Reason = "size"
			return g
		}
	case KindPerson:
		// Commons photos are often small; TMDB's own are 185 wide.
		if minInt(g.Width, g.Height) < 200 {
			g.Reason = "size"
			return g
		}
	case KindTitleWide:
		if g.Width < MinWideWidth || float64(g.Width)/float64(g.Height) < minWideAspect {
			g.Reason = "size"
			return g
		}
	case KindTitleBanner:
		if g.Width < 600 {
			g.Reason = "size"
			return g
		}
	case KindLogo, KindTitleLogo:
		if g.Width < MinLogoWidth {
			g.Reason = "size"
			return g
		}
		if !hasAlpha(img) {
			g.Reason = "alpha"
			return g
		}
		g.OK = true
		return g
	}
	if NearSolid(img) {
		g.Reason = "solid"
		return g
	}
	if kind != KindWide {
		// Covers, photos, banners and title backgrounds are official or
		// curated pictures (some soft or lettered on purpose).
		g.OK = true
		return g
	}
	g.Sharpness = math.Round(laplacianVariance(Resize(img, 256))*10) / 10
	if g.Sharpness < BlurryBelow {
		g.Reason = "blur"
		return g
	}
	g.TextRow, g.Flat = textRow(img), flatShare(img)
	if g.TextRow >= textRowMin || g.Flat >= flatShareMin {
		g.Reason = "text"
		return g
	}
	g.OK = true
	return g
}

func hasAlpha(img image.Image) bool {
	b := img.Bounds()
	step := maxInt(1, minInt(b.Dx(), b.Dy())/200)
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			if _, _, _, a := img.At(x, y).RGBA(); a < 0xffff {
				return true
			}
		}
	}
	return false
}

func grey(img image.Image) ([]float64, int, int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	g := make([]float64, w*h)
	for y := range h {
		for x := range w {
			g[y*w+x] = float64(color.GrayModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.Gray).Y)
		}
	}
	return g, w, h
}

// boxBlur is a (2r+1)² mean with edges clamped.
func boxBlur(a []float64, w, h, r int) []float64 {
	tmp := make([]float64, len(a))
	out := make([]float64, len(a))
	n := float64(2*r + 1)
	cl := func(v, hi int) int { return max(0, min(v, hi-1)) }
	for y := range h {
		s := 0.0
		for k := -r; k <= r; k++ {
			s += a[y*w+cl(k, w)]
		}
		for x := range w {
			tmp[y*w+x] = s / n
			s += a[y*w+cl(x+r+1, w)] - a[y*w+cl(x-r, w)]
		}
	}
	for x := range w {
		s := 0.0
		for k := -r; k <= r; k++ {
			s += tmp[cl(k, h)*w+x]
		}
		for y := range h {
			out[y*w+x] = s / n
			s += tmp[cl(y+r+1, h)*w+x] - tmp[cl(y-r, h)*w+x]
		}
	}
	return out
}

// textRow finds the longest row of letter-sized, high-contrast blobs in a
// horizontal band (bench textdet: grey at 960 px wide, a 25 px box blur,
// pixels 45 levels above or below it, 4-connected blobs 6-48 px high and
// 2-50 px wide, at most 2.5 times as wide as high, filling 15-90% of their
// box; in 40 px bands every 10 px, blobs of similar height whose gaps are
// under 3 heights are counted).
func textRow(img image.Image) int {
	src := Resize(img, 960)
	a, w, h := grey(src)
	if w < 10 || h < 10 {
		return 0
	}
	bl := boxBlur(a, w, h, 12)
	best := 0
	for pass := range 2 {
		mask := make([]bool, len(a))
		for i := range a {
			d := a[i] - bl[i]
			if pass == 1 {
				d = -d
			}
			mask[i] = d > 45
		}
		type blob struct{ cy, hh, cx float64 }
		var blobs []blob
		seen := make([]bool, len(a))
		stack := []int{}
		for i := range mask {
			if !mask[i] || seen[i] {
				continue
			}
			seen[i] = true
			stack = append(stack[:0], i)
			x0, y0, x1, y1, n := w, h, 0, 0, 0
			for len(stack) > 0 {
				p := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				x, y := p%w, p/w
				n++
				x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
				for _, q := range [4]int{p - 1, p + 1, p - w, p + w} {
					if q < 0 || q >= len(mask) || (q == p-1 && x == 0) || (q == p+1 && x == w-1) {
						continue
					}
					if mask[q] && !seen[q] {
						seen[q] = true
						stack = append(stack, q)
					}
				}
			}
			hh, ww := y1-y0+1, x1-x0+1
			if hh >= 6 && hh <= 48 && ww >= 2 && ww <= 50 && float64(ww) <= 2.5*float64(hh) {
				// The box's fill counts every labelled pixel inside it, as
				// the bench's (lab[box] > 0).mean() does; n is close enough.
				fill := float64(n) / float64(hh*ww)
				if fill > 0.15 && fill < 0.9 {
					blobs = append(blobs, blob{float64(y0+y1+1) / 2, float64(hh), float64(x0+x1+1) / 2})
				}
			}
		}
		if len(blobs) == 0 {
			continue
		}
		for y0 := 0; y0 < h; y0 += 10 {
			var sel []blob
			for _, b := range blobs {
				if b.cy >= float64(y0) && b.cy < float64(y0+40) {
					sel = append(sel, b)
				}
			}
			if len(sel) < 4 {
				continue
			}
			hs := make([]float64, len(sel))
			for i, b := range sel {
				hs[i] = b.hh
			}
			med := median(hs)
			var xs []float64
			for _, b := range sel {
				if b.hh > 0.6*med && b.hh < 1.5*med {
					xs = append(xs, b.cx)
				}
			}
			if len(xs) < 4 {
				continue
			}
			sort.Float64s(xs)
			run := 0
			for i := 1; i < len(xs); i++ {
				if xs[i]-xs[i-1] < 3*med {
					run++
				}
			}
			best = max(best, run)
		}
	}
	return best
}

func median(v []float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// flatShare is the share of pixels with almost no gradient (a promo card
// is mostly flat colour), on a grey copy of at most 480 px.
func flatShare(img image.Image) float64 {
	a, w, h := grey(Resize(img, 480))
	if w < 2 || h < 2 {
		return 0
	}
	flat := 0
	for y := range h - 1 {
		for x := range w - 1 {
			i := y*w + x
			if math.Abs(a[i+1]-a[i])+math.Abs(a[i+w]-a[i]) < 4 {
				flat++
			}
		}
	}
	return math.Round(float64(flat)/float64((w-1)*(h-1))*1000) / 1000
}

func laplacianVariance(img image.Image) float64 {
	a, w, h := grey(img)
	if w < 3 || h < 3 {
		return 0
	}
	var sum, sumSq float64
	n := 0
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			v := a[(y-1)*w+x] + a[(y+1)*w+x] + a[y*w+x-1] + a[y*w+x+1] - 4*a[y*w+x]
			sum += v
			sumSq += v * v
			n++
		}
	}
	m := sum / float64(n)
	return sumSq/float64(n) - m*m
}
