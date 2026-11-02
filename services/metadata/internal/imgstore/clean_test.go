package imgstore

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"math/rand"
	"os"
	"strings"
	"testing"
)

func encJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func encPNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// exifBlock is a little-endian TIFF block with the orientation, a camera
// make and a GPS IFD holding a latitude.
func exifBlock(orientation uint16) []byte {
	var b bytes.Buffer
	le := binary.LittleEndian
	w16 := func(v uint16) { _ = binary.Write(&b, le, v) }
	w32 := func(v uint32) { _ = binary.Write(&b, le, v) }
	b.WriteString("II")
	w16(42)
	w32(8)
	// IFD0 at 8: three entries.
	w16(3)
	w16(0x0112) // Orientation
	w16(3)
	w32(1)
	w16(orientation)
	w16(0)
	w16(0x010F) // Make
	w16(2)
	w32(8)
	w32(8 + 2 + 3*12 + 4) // value after the IFD
	w16(0x8825)           // GPS IFD pointer
	w16(4)
	w32(1)
	w32(8 + 2 + 3*12 + 4 + 8)
	w32(0)
	b.WriteString("PhoneCo\x00")
	// GPS IFD: one entry (GPSLatitudeRef "N").
	w16(1)
	w16(0x0001)
	w16(2)
	w32(2)
	b.WriteString("N\x00\x00\x00")
	w32(0)
	b.WriteString("GPSSECRET")
	return b.Bytes()
}

func segment(marker byte, payload []byte) []byte {
	s := []byte{0xFF, marker, 0, 0}
	binary.BigEndian.PutUint16(s[2:], uint16(len(payload)+2))
	return append(s, payload...)
}

// withSegments puts segments right after a JPEG's start marker.
func withSegments(jpg []byte, segs ...[]byte) []byte {
	out := append([]byte{}, jpg[:2]...)
	for _, s := range segs {
		out = append(out, s...)
	}
	return append(out, jpg[2:]...)
}

func TestDecodeAppliesOrientationAndKeepsNoMetadata(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	for y := range 20 {
		for x := range 40 {
			c := color.NRGBA{20, 20, 20, 255}
			if x < 10 {
				c = color.NRGBA{250, 30, 30, 255} // a red band on the left
			}
			src.Set(x, y, c)
		}
	}
	raw := withSegments(encJPEG(t, src),
		segment(0xE1, append([]byte("Exif\x00\x00"), exifBlock(6)...)),
		segment(0xE1, []byte("http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta>XMPSECRET</x:xmpmeta>")),
		segment(0xED, []byte("Photoshop 3.0\x008BIMIPTCSECRET")),
		segment(0xFE, []byte("COMMENTSECRET")))
	img, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 20 || b.Dy() != 40 {
		t.Fatalf("turned 90 right should be 20x40, got %v", b)
	}
	// The red band was on the left; turned right it is on the top.
	if r, _, _, _ := img.At(10, 2).RGBA(); r>>8 < 200 {
		t.Fatalf("red band not on top after turning: %v", img.At(10, 2))
	}
	out, err := EncodeWebP(img)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"Exif", "EXIF", "GPS", "PhoneCo", "XMP", "IPTC", "COMMENT", "ICCP"} {
		if bytes.Contains(out, []byte(s)) {
			t.Fatalf("WebP output still holds %q", s)
		}
	}
}

func TestDecodeCMYKToRGB(t *testing.T) {
	raw, err := os.ReadFile("testdata/cmyk.jpg")
	if err != nil {
		t.Fatal(err)
	}
	img, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(16, 16).RGBA()
	if r>>8 < 220 || g>>8 > 60 || b>>8 > 60 {
		t.Fatalf("CMYK red should come out red, got %d %d %d", r>>8, g>>8, b>>8)
	}
}

// adobeRGBProfile is a minimal Adobe RGB (1998) matrix/TRC profile.
func adobeRGBProfile() []byte {
	be := binary.BigEndian
	xyz := func(x, y, z float64) []byte {
		b := make([]byte, 20)
		copy(b, "XYZ ")
		for i, v := range []float64{x, y, z} {
			be.PutUint32(b[8+4*i:], uint32(int32(math.Round(v*65536))))
		}
		return b
	}
	trc := make([]byte, 14)
	copy(trc, "curv")
	be.PutUint32(trc[8:], 1)
	be.PutUint16(trc[12:], 563) // 2.19921875
	tags := []struct {
		sig  string
		data []byte
	}{
		{"rXYZ", xyz(0.6097559, 0.3111145, 0.0194702)},
		{"gXYZ", xyz(0.2052401, 0.6256714, 0.0608902)},
		{"bXYZ", xyz(0.1492240, 0.0632141, 0.7445396)},
		{"rTRC", trc}, {"gTRC", trc}, {"bTRC", trc},
	}
	head := make([]byte, 128)
	copy(head[12:], "mntr")
	copy(head[16:], "RGB ")
	copy(head[20:], "XYZ ")
	copy(head[36:], "acsp")
	table := make([]byte, 4+12*len(tags))
	be.PutUint32(table, uint32(len(tags)))
	var data []byte
	off := 128 + len(table)
	for i, tg := range tags {
		e := table[4+12*i:]
		copy(e, tg.sig)
		be.PutUint32(e[4:], uint32(off+len(data)))
		be.PutUint32(e[8:], uint32(len(tg.data)))
		data = append(data, tg.data...)
		for len(data)%4 != 0 {
			data = append(data, 0)
		}
	}
	p := append(append(head, table...), data...)
	be.PutUint32(p, uint32(len(p)))
	return p
}

// withICCP puts an iCCP chunk after a PNG's IHDR.
func withICCP(t *testing.T, pngRaw, profile []byte) []byte {
	t.Helper()
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	_, _ = zw.Write(profile)
	_ = zw.Close()
	data := append([]byte("Adobe RGB\x00\x00"), z.Bytes()...)
	chunk := make([]byte, 8)
	binary.BigEndian.PutUint32(chunk, uint32(len(data)))
	copy(chunk[4:], "iCCP")
	chunk = append(chunk, data...)
	crc := crc32.ChecksumIEEE(chunk[4:])
	chunk = binary.BigEndian.AppendUint32(chunk, crc)
	ihdrEnd := 8 + 12 + 13
	return append(append(append([]byte{}, pngRaw[:ihdrEnd]...), chunk...), pngRaw[ihdrEnd:]...)
}

func TestDecodeConvertsAdobeRGBToSRGB(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3] = 100, 150, 100, 255
	}
	img, err := Decode(withICCP(t, encPNG(t, src), adobeRGBProfile()))
	if err != nil {
		t.Fatal(err)
	}
	// Expected by the published D65 matrices (Adobe RGB to XYZ to sRGB).
	g := 563.0 / 256
	lin := [3]float64{math.Pow(100.0/255, g), math.Pow(150.0/255, g), math.Pow(100.0/255, g)}
	toXYZ := [3][3]float64{{0.5767309, 0.1855540, 0.1881852}, {0.2973769, 0.6273491, 0.0752741}, {0.0270343, 0.0706872, 0.9911085}}
	toRGB := [3][3]float64{{3.2404542, -1.5371385, -0.4985314}, {-0.9692660, 1.8760108, 0.0415560}, {0.0556434, -0.2040259, 1.0572252}}
	var xyz, want [3]float64
	for r := range 3 {
		for c := range 3 {
			xyz[r] += toXYZ[r][c] * lin[c]
		}
	}
	for r := range 3 {
		v := 0.0
		for c := range 3 {
			v += toRGB[r][c] * xyz[c]
		}
		want[r] = math.Round(srgbEncode(math.Min(math.Max(v, 0), 1)) * 255)
	}
	got := img.NRGBAAt(4, 4)
	for i, v := range []uint8{got.R, got.G, got.B} {
		if math.Abs(float64(v)-want[i]) > 2 {
			t.Fatalf("channel %d: got %d, want about %.0f (all %v vs %v)", i, v, want[i], got, want)
		}
	}
	if got.G <= 150 {
		t.Fatalf("Adobe RGB green should come out more saturated in sRGB, got %v", got)
	}
}

func TestDecodeRefusesTooBigBeforeDecoding(t *testing.T) {
	// A PNG header that claims 6000x5000 (30 MP) and no pixel data.
	raw := encPNG(t, image.NewGray(image.Rect(0, 0, 1, 1)))
	binary.BigEndian.PutUint32(raw[16:], 6000)
	binary.BigEndian.PutUint32(raw[20:], 5000)
	binary.BigEndian.PutUint32(raw[29:], crc32.ChecksumIEEE(raw[12:29]))
	if _, err := Decode(raw); err != ErrTooBig {
		t.Fatalf("30 MP should be refused, got %v", err)
	}
	if _, err := Decode(make([]byte, MaxSourceBytes+1)); err != ErrTooBig {
		t.Fatalf("over 20 MB should be refused, got %v", err)
	}
	if _, err := Decode([]byte("<html>not a picture</html>")); err == nil || err == ErrTooBig {
		t.Fatalf("garbage should fail to decode, got %v", err)
	}
}

func TestResizeNeverUpscales(t *testing.T) {
	small := image.NewNRGBA(image.Rect(0, 0, 300, 200))
	if got := Resize(small, 500).Bounds(); got.Dx() != 300 || got.Dy() != 200 {
		t.Fatalf("300x200 asked at 500 must stay 300x200, got %v", got)
	}
	big := image.NewNRGBA(image.Rect(0, 0, 1000, 600))
	if got := Resize(big, 500).Bounds(); got.Dx() != 500 || got.Dy() != 300 {
		t.Fatalf("1000x600 at 500 should be 500x300, got %v", got)
	}
}

func TestResizeKeepsTransparentEdgeColour(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	img.SetNRGBA(0, 0, color.NRGBA{255, 255, 255, 255})
	img.SetNRGBA(1, 0, color.NRGBA{0, 0, 0, 0}) // transparent black
	out := Resize(img, 2).(*image.NRGBA).NRGBAAt(0, 0)
	if out.R != 255 || out.A == 0 {
		t.Fatalf("a white pixel next to a transparent one should stay white, got %v", out)
	}
}

func TestNearSolidAndMainColor(t *testing.T) {
	flat := image.NewNRGBA(image.Rect(0, 0, 500, 500))
	for i := 0; i < len(flat.Pix); i += 4 {
		flat.Pix[i], flat.Pix[i+1], flat.Pix[i+2], flat.Pix[i+3] = 0xd8, 0xd8, 0xd8, 255
	}
	if !NearSolid(flat) {
		t.Fatal("a flat grey picture is near-solid")
	}
	rnd := rand.New(rand.NewSource(1))
	busy := image.NewNRGBA(image.Rect(0, 0, 500, 500))
	for i := 0; i < len(busy.Pix); i += 4 {
		busy.Pix[i], busy.Pix[i+1], busy.Pix[i+2], busy.Pix[i+3] = uint8(rnd.Intn(256)), uint8(rnd.Intn(256)), uint8(rnd.Intn(256)), 255
	}
	if NearSolid(busy) {
		t.Fatal("a busy picture is not near-solid")
	}
	blue := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := range 100 {
		for x := range 100 {
			c := color.NRGBA{20, 40, 200, 255}
			if x > 80 {
				c = color.NRGBA{250, 250, 250, 255}
			}
			blue.SetNRGBA(x, y, c)
		}
	}
	if got := MainColor(blue); !strings.HasPrefix(got, "#") || got[5:7] < "b0" {
		t.Fatalf("main colour of a mostly blue picture should be blue, got %s", got)
	}
}
