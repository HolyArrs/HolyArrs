package library

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	_ "image/png" // stored covers can be PNG
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // the metadata server stores WebP
)

// smallerMax is the largest picture Smaller decodes (25 megapixels).
const smallerMax = 25_000_000

// smallerSlots lets two pictures be made smaller at once, so a row of
// small thumbnails never takes every core.
var smallerSlots = make(chan struct{}, 2)

// ErrArtworkNotStored means the picture to make smaller is not in the cache.
var ErrArtworkNotStored = errors.New("picture: not stored")

// SmallerName is the cache name of the copy of name at most px pixels a
// side. It carries the stored file's size, so a picture replaced in place
// (a re-checked cover) gets a new copy, never the old one.
func SmallerName(name string, size int64, px int) string {
	return strings.TrimSuffix(name, filepath.Ext(name)) + "-" + strconv.Itoa(px) + "-" + strconv.FormatInt(size, 36) + ".jpg"
}

// Smaller answers a copy of the stored picture name at most px pixels a
// side, made here once (a JPEG) and kept in the cache under the same cap,
// so the metadata server is never asked for small sizes. A picture already
// that small is answered as it is.
func (c *ArtworkCache) Smaller(name string, px int) (string, string, error) {
	src, ok := c.Path(name)
	if !ok {
		return "", "", ErrArtworkNotStored
	}
	st, err := os.Stat(src)
	if err != nil {
		return "", "", err
	}
	small := SmallerName(name, st.Size(), px)
	if p, ok := c.Path(small); ok {
		return small, p, nil
	}
	f, err := os.Open(src)
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return "", "", err
	}
	if cfg.Width <= px && cfg.Height <= px {
		return name, src, nil
	}
	if int64(cfg.Width)*int64(cfg.Height) > smallerMax {
		return "", "", errors.New("picture: too large to make smaller")
	}
	if _, err := f.Seek(0, 0); err != nil {
		return "", "", err
	}
	smallerSlots <- struct{}{}
	defer func() { <-smallerSlots }()
	img, _, err := image.Decode(f)
	if err != nil {
		return "", "", err
	}
	b := img.Bounds()
	w, h := px, px
	if b.Dx() >= b.Dy() {
		h = max(1, b.Dy()*px/b.Dx())
	} else {
		w = max(1, b.Dx()*px/b.Dy())
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return "", "", err
	}
	tmp, err := os.CreateTemp(c.Dir, ".part-*")
	if err != nil {
		return "", "", err
	}
	_, err = tmp.Write(buf.Bytes())
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	out := filepath.Join(c.Dir, small)
	if err == nil {
		err = os.Rename(tmp.Name(), out)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return "", "", err
	}
	c.added(int64(buf.Len()))
	return small, out, nil
}
