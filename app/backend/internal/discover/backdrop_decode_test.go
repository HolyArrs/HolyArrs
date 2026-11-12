package discover

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/jpeg"
	"testing"
)

// pngHeader builds a PNG whose header claims w x h but holds no pixels.
func pngHeader(w, h uint32) []byte {
	var b bytes.Buffer
	b.Write([]byte("\x89PNG\r\n\x1a\n"))
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 2 // 8-bit RGB
	binary.Write(&b, binary.BigEndian, uint32(len(ihdr)))
	chunk := append([]byte("IHDR"), ihdr...)
	b.Write(chunk)
	binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return b.Bytes()
}

func TestDecodeBackdropRefusesHugeClaimedSize(t *testing.T) {
	t.Parallel()
	// 10000 x 10000: far past the cap, yet small enough for the PNG reader
	// of a 32-bit build, which refuses 60000 x 60000 on its own.
	_, err := decodeBackdrop(bytes.NewReader(pngHeader(10000, 10000)))
	if !errors.Is(err, errBackdropTooLarge) {
		t.Fatalf("want errBackdropTooLarge, got %v", err)
	}
}

func TestDecodeBackdropReadsNormalPicture(t *testing.T) {
	t.Parallel()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 64, 36)), nil); err != nil {
		t.Fatal(err)
	}
	img, err := decodeBackdrop(&b)
	if err != nil || img.Bounds().Dx() != 64 {
		t.Fatalf("decode: %v %v", img, err)
	}
}
