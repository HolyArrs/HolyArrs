package imgstore

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"sort"

	"github.com/gen2brain/webp"
)

// Limits on a source picture, checked before it is decoded.
const (
	MaxSourceBytes  = 20 << 20
	MaxSourcePixels = 25_000_000
)

// ErrTooBig: the source picture is over MaxSourceBytes or MaxSourcePixels.
var ErrTooBig = errors.New("imgstore: picture too big")

// Decode reads a source picture and answers it upright, in sRGB, with
// nothing of the original file kept: the EXIF orientation is applied, an
// embedded RGB colour profile is converted to sRGB (CMYK and grey are
// converted too), and every piece of metadata (EXIF, XMP, IPTC, comments,
// thumbnails, the profile itself) is left behind, since only pixels are
// returned. Anything over the limits is refused before decoding.
func Decode(raw []byte) (*image.NRGBA, error) {
	if len(raw) > MaxSourceBytes {
		return nil, ErrTooBig
	}
	cfg, format, err := decodeConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("imgstore: decode: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > MaxSourcePixels {
		return nil, ErrTooBig
	}
	var img image.Image
	if format == "webp" {
		img, err = webp.Decode(bytes.NewReader(raw))
	} else {
		img, _, err = image.Decode(bytes.NewReader(raw))
	}
	if err != nil {
		return nil, fmt.Errorf("imgstore: decode: %w", err)
	}
	var profile []byte
	orient := 1
	switch format {
	case "jpeg":
		profile, orient = jpegMeta(raw)
	case "png":
		profile = pngProfile(raw)
	}
	out := toNRGBA(img)
	if _, isCMYK := img.(*image.CMYK); !isCMYK && len(profile) > 0 {
		if p, err := parseICC(profile); err == nil && !p.isSRGB() {
			p.toSRGB(out)
		}
	}
	return orientate(out, orient), nil
}

func decodeConfig(raw []byte) (image.Config, string, error) {
	if len(raw) >= 12 && string(raw[:4]) == "RIFF" && string(raw[8:12]) == "WEBP" {
		c, err := webp.DecodeConfig(bytes.NewReader(raw))
		return c, "webp", err
	}
	return image.DecodeConfig(bytes.NewReader(raw))
}

// toNRGBA copies any decoded picture into 8-bit non-premultiplied RGBA
// (CMYK through the standard conversion, grey and paletted as they are).
func toNRGBA(img image.Image) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	switch src := img.(type) {
	case *image.CMYK:
		for y := range b.Dy() {
			for x := range b.Dx() {
				c := src.CMYKAt(b.Min.X+x, b.Min.Y+y)
				r, g, bl := color.CMYKToRGB(c.C, c.M, c.Y, c.K)
				i := out.PixOffset(x, y)
				out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = r, g, bl, 255
			}
		}
	default:
		draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	}
	return out
}

// orientate applies an EXIF orientation (1 to 8) so the picture is upright.
func orientate(src *image.NRGBA, o int) *image.NRGBA {
	if o < 2 || o > 8 {
		return src
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for y := range h {
		for x := range w {
			var dx, dy int
			switch o {
			case 2: // mirrored
				dx, dy = w-1-x, y
			case 3: // turned 180
				dx, dy = w-1-x, h-1-y
			case 4: // mirrored upside down
				dx, dy = x, h-1-y
			case 5: // mirrored, turned 90 left
				dx, dy = y, x
			case 6: // turned 90 right
				dx, dy = h-1-y, x
			case 7: // mirrored, turned 90 right
				dx, dy = h-1-y, w-1-x
			case 8: // turned 90 left
				dx, dy = y, w-1-x
			}
			copy(dst.Pix[dst.PixOffset(dx, dy):dst.PixOffset(dx, dy)+4], src.Pix[src.PixOffset(x, y):src.PixOffset(x, y)+4])
		}
	}
	return dst
}

// EncodeWebP writes a picture as WebP (quality 80); the output holds pixels
// only.
func EncodeWebP(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := webp.Encode(&buf, img, webp.Options{Quality: 80, Method: 4}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// EncodePNG writes a picture as PNG (logos keep their transparency); Go's
// encoder writes no metadata chunks.
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// EncodeJPEG writes a picture as JPEG (quality 82), for asks that name
// .jpg; nothing is stored in this format.
func EncodeJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 82}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MainColor is a picture's most common colour (the mean of the fullest
// 4-bit bucket on a 64 px copy), as #rrggbb; installs paint it while the
// picture loads.
func MainColor(img image.Image) string {
	small := Resize(img, 64)
	b := small.Bounds()
	type acc struct{ r, g, b, n int }
	buckets := map[int]*acc{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := small.At(x, y).RGBA()
			if a < 0x8000 {
				continue
			}
			r8, g8, b8 := int(r>>8), int(g>>8), int(bl>>8)
			k := r8>>4<<8 | g8>>4<<4 | b8>>4
			c := buckets[k]
			if c == nil {
				c = &acc{}
				buckets[k] = c
			}
			c.r, c.g, c.b, c.n = c.r+r8, c.g+g8, c.b+b8, c.n+1
		}
	}
	if len(buckets) == 0 {
		return "#000000"
	}
	keys := make([]int, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	best := buckets[keys[0]]
	for _, k := range keys[1:] {
		if buckets[k].n > best.n {
			best = buckets[k]
		}
	}
	return fmt.Sprintf("#%02x%02x%02x", best.r/best.n, best.g/best.n, best.b/best.n)
}

// NearSolid reports a picture that is almost one flat colour (the spread of
// every channel on a 64 px copy is under 6 levels): a blank stand-in, never
// a real cover or photo.
func NearSolid(img image.Image) bool {
	small := Resize(img, 64)
	b := small.Bounds()
	var sum, sq [3]float64
	n := 0.0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := small.At(x, y).RGBA()
			if a < 0x8000 {
				continue
			}
			for i, v := range [3]uint32{r >> 8, g >> 8, bl >> 8} {
				f := float64(v)
				sum[i] += f
				sq[i] += f * f
			}
			n++
		}
	}
	if n == 0 {
		return true
	}
	for i := range 3 {
		m := sum[i] / n
		if math.Sqrt(math.Max(sq[i]/n-m*m, 0)) >= 6 {
			return false
		}
	}
	return true
}

// Resize scales img so its longer side is at most max (never upscales),
// averaging the source pixels under each target pixel, each colour weighted
// by its opacity (so a logo's transparent edge does not darken it).
func Resize(img image.Image, max int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= max && h <= max {
		return img
	}
	nw, nh := max, h*max/w
	if h > w {
		nw, nh = w*max/h, max
	}
	nw, nh = maxInt(nw, 1), maxInt(nh, 1)
	src, ok := img.(*image.NRGBA)
	if !ok || src.Rect.Min != (image.Point{}) {
		src = image.NewNRGBA(image.Rect(0, 0, w, h))
		draw.Draw(src, src.Bounds(), img, b.Min, draw.Src)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := range nh {
		y0, y1 := y*h/nh, maxInt((y+1)*h/nh, y*h/nh+1)
		for x := range nw {
			x0, x1 := x*w/nw, maxInt((x+1)*w/nw, x*w/nw+1)
			var r, g, bl, a, n uint64
			for sy := y0; sy < y1; sy++ {
				i := src.PixOffset(x0, sy)
				for sx := x0; sx < x1; sx++ {
					al := uint64(src.Pix[i+3])
					r += uint64(src.Pix[i]) * al
					g += uint64(src.Pix[i+1]) * al
					bl += uint64(src.Pix[i+2]) * al
					a += al
					i += 4
					n++
				}
			}
			j := dst.PixOffset(x, y)
			if a > 0 {
				dst.Pix[j], dst.Pix[j+1], dst.Pix[j+2] = uint8(r/a), uint8(g/a), uint8(bl/a)
			}
			dst.Pix[j+3] = uint8(a / n)
		}
	}
	return dst
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
