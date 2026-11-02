package imgstore

import (
	"encoding/binary"
	"errors"
	"image"
	"math"
)

// iccRGB is an RGB matrix/TRC colour profile (Adobe RGB, Display P3,
// ProPhoto and most camera profiles): three tone curves to linear light,
// then the primaries' matrix to the D50 connection space.
type iccRGB struct {
	m   [3][3]float64 // columns: red, green and blue XYZ (D50)
	trc [3][256]float64
}

var errICC = errors.New("imgstore: unsupported colour profile")

// parseICC reads an RGB matrix/TRC profile; other kinds (CMYK, grey, table
// based) answer errICC and the pixels are kept as decoded.
func parseICC(p []byte) (*iccRGB, error) {
	if len(p) < 132 || string(p[16:20]) != "RGB " || string(p[20:24]) != "XYZ " {
		return nil, errICC
	}
	tags := map[string][]byte{}
	n := int(binary.BigEndian.Uint32(p[128:]))
	for i := range n {
		e := 132 + i*12
		if e+12 > len(p) {
			return nil, errICC
		}
		off, size := int(binary.BigEndian.Uint32(p[e+4:])), int(binary.BigEndian.Uint32(p[e+8:]))
		if off < 0 || size < 0 || off+size > len(p) {
			return nil, errICC
		}
		tags[string(p[e:e+4])] = p[off : off+size]
	}
	out := &iccRGB{}
	for c, name := range []string{"rXYZ", "gXYZ", "bXYZ"} {
		t := tags[name]
		if len(t) < 20 || string(t[:4]) != "XYZ " {
			return nil, errICC
		}
		for k := range 3 {
			out.m[k][c] = s15(t[8+4*k:])
		}
	}
	for c, name := range []string{"rTRC", "gTRC", "bTRC"} {
		f, err := curve(tags[name])
		if err != nil {
			return nil, err
		}
		for v := range 256 {
			out.trc[c][v] = f(float64(v) / 255)
		}
	}
	return out, nil
}

func s15(b []byte) float64 { return float64(int32(binary.BigEndian.Uint32(b))) / 65536 }

// curve reads a 'curv' or 'para' tone curve.
func curve(t []byte) (func(float64) float64, error) {
	if len(t) < 12 {
		return nil, errICC
	}
	switch string(t[:4]) {
	case "curv":
		n := int(binary.BigEndian.Uint32(t[8:]))
		switch {
		case n == 0:
			return func(x float64) float64 { return x }, nil
		case n == 1 && len(t) >= 14:
			g := float64(binary.BigEndian.Uint16(t[12:])) / 256
			return func(x float64) float64 { return math.Pow(x, g) }, nil
		case len(t) >= 12+2*n:
			tab := make([]float64, n)
			for i := range n {
				tab[i] = float64(binary.BigEndian.Uint16(t[12+2*i:])) / 65535
			}
			return func(x float64) float64 {
				pos := x * float64(n-1)
				i := int(pos)
				if i >= n-1 {
					return tab[n-1]
				}
				return tab[i] + (tab[i+1]-tab[i])*(pos-float64(i))
			}, nil
		}
	case "para":
		typ := int(binary.BigEndian.Uint16(t[8:]))
		need := []int{1, 3, 4, 5, 7}
		if typ < 0 || typ > 4 || len(t) < 12+4*need[typ] {
			return nil, errICC
		}
		var v [7]float64
		for i := range need[typ] {
			v[i] = s15(t[12+4*i:])
		}
		g, a, b, c, d, e, f := v[0], v[1], v[2], v[3], v[4], v[5], v[6]
		pw := func(x float64) float64 { return math.Pow(math.Max(x, 0), g) }
		switch typ {
		case 0:
			return pw, nil
		case 1:
			return func(x float64) float64 {
				if x >= -b/a {
					return pw(a*x + b)
				}
				return 0
			}, nil
		case 2:
			return func(x float64) float64 {
				if x >= -b/a {
					return pw(a*x+b) + c
				}
				return c
			}, nil
		case 3:
			return func(x float64) float64 {
				if x >= d {
					return pw(a*x + b)
				}
				return c * x
			}, nil
		case 4:
			return func(x float64) float64 {
				if x >= d {
					return pw(a*x+b) + e
				}
				return c*x + f
			}, nil
		}
	}
	return nil, errICC
}

// srgbD50 is sRGB's primaries in the D50 connection space (what an sRGB
// profile's rXYZ, gXYZ and bXYZ hold).
var srgbD50 = [3][3]float64{
	{0.4360747, 0.3850649, 0.1430804},
	{0.2225045, 0.7168786, 0.0606169},
	{0.0139322, 0.0971045, 0.7141733},
}

// d50ToSRGB takes connection-space XYZ (D50) to linear sRGB, with the
// Bradford adaptation to D65 folded in.
var d50ToSRGB = [3][3]float64{
	{3.1338561, -1.6168667, -0.4906146},
	{-0.9787684, 1.9161415, 0.0334540},
	{0.0719453, -0.2289914, 1.4052427},
}

func srgbDecode(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func srgbEncode(v float64) float64 {
	if v <= 0.0031308 {
		return 12.92 * v
	}
	return 1.055*math.Pow(v, 1/2.4) - 0.055
}

// isSRGB reports a profile that is sRGB already (same primaries, a tone
// curve close to sRGB's): converting it would change nothing visible.
func (p *iccRGB) isSRGB() bool {
	for r := range 3 {
		for c := range 3 {
			if math.Abs(p.m[r][c]-srgbD50[r][c]) > 0.003 {
				return false
			}
		}
	}
	for c := range 3 {
		for _, v := range []int{32, 128, 224} {
			if math.Abs(p.trc[c][v]-srgbDecode(float64(v)/255)) > 0.01 {
				return false
			}
		}
	}
	return true
}

// toSRGB converts the picture's pixels from the profile to sRGB in place
// (colours outside sRGB are clipped).
func (p *iccRGB) toSRGB(img *image.NRGBA) {
	var m [3][3]float64
	for r := range 3 {
		for c := range 3 {
			for k := range 3 {
				m[r][c] += d50ToSRGB[r][k] * p.m[k][c]
			}
		}
	}
	const steps = 4096
	var enc [steps + 1]uint8
	for i := range enc {
		enc[i] = uint8(math.Round(srgbEncode(float64(i)/steps) * 255))
	}
	q := func(v float64) uint8 {
		switch {
		case v <= 0:
			return enc[0]
		case v >= 1:
			return enc[steps]
		}
		return enc[int(v*steps+0.5)]
	}
	b := img.Rect
	for y := b.Min.Y; y < b.Max.Y; y++ {
		i := img.PixOffset(b.Min.X, y)
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl := p.trc[0][img.Pix[i]], p.trc[1][img.Pix[i+1]], p.trc[2][img.Pix[i+2]]
			img.Pix[i] = q(m[0][0]*r + m[0][1]*g + m[0][2]*bl)
			img.Pix[i+1] = q(m[1][0]*r + m[1][1]*g + m[1][2]*bl)
			img.Pix[i+2] = q(m[2][0]*r + m[2][1]*g + m[2][2]*bl)
			i += 4
		}
	}
}
