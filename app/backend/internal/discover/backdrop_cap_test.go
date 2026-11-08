package discover

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"math/rand/v2"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// backdropReadCap is the most a backdrop sample may read: a w300 TMDB
// backdrop is tens of KB, so 2 MiB is far above any real one.
const backdropReadCap = 2 << 20

type countingBody struct {
	r io.Reader
	n *atomic.Int64
}

func (c countingBody) Read(p []byte) (int, error) {
	k, err := c.r.Read(p)
	c.n.Add(int64(k))
	return k, err
}

func (countingBody) Close() error { return nil }

type bodyTransport struct {
	body []byte
	read *atomic.Int64
}

func (t bodyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"image/jpeg"}}, Request: r,
		Body: countingBody{r: bytes.NewReader(t.body), n: t.read}}, nil
}

// noiseJPEG is a valid JPEG larger than the cap (noise does not compress).
func noiseJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1400, 1400))
	rng := rand.New(rand.NewPCG(1, 2))
	for y := 0; y < 1400; y++ {
		for x := 0; x < 1400; x++ {
			img.Set(x, y, color.RGBA{uint8(rng.UintN(256)), uint8(rng.UintN(256)), uint8(rng.UintN(256)), 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	if b.Len() <= backdropReadCap {
		t.Fatalf("test picture is only %d bytes", b.Len())
	}
	return b.Bytes()
}

// A backdrop body is decoded through a 2 MiB read cap, so an oversized or
// hostile picture from the artwork host cannot make the decoder read (and
// allocate for) an unbounded body. Over the cap the sample fails (-1).
func TestBackdropSampleReadCap(t *testing.T) {
	t.Parallel()
	var read atomic.Int64
	cl := &http.Client{Transport: bodyTransport{body: noiseJPEG(t), read: &read}}
	got := sampleBackdrop(context.Background(), cl, metadata.ImageBase+"/original/big.jpg")
	if n := read.Load(); n > backdropReadCap {
		t.Fatalf("read %d bytes of the backdrop body, cap is %d", n, backdropReadCap)
	}
	if got != -1 {
		t.Fatalf("an over-cap backdrop sampled as %v, want -1", got)
	}
}

// A normal small backdrop still samples.
func TestBackdropSampleSmallStillWorks(t *testing.T) {
	t.Parallel()
	img := image.NewGray(image.Rect(0, 0, 300, 169))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	var read atomic.Int64
	cl := &http.Client{Transport: bodyTransport{body: b.Bytes(), read: &read}}
	if got := sampleBackdrop(context.Background(), cl, metadata.ImageBase+"/original/small.jpg"); got < 150 {
		t.Fatalf("small backdrop luma %v", got)
	}
}
