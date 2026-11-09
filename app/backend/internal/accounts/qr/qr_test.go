package qr

import (
	"errors"
	"strings"
	"testing"
)

// These check the shape. Decoding was checked by hand with OpenCV's QR
// reader for 1 to 213 bytes (versions 1 to 10), and the version picked
// matched another encoder's every time.
func TestEncodeShape(t *testing.T) {
	uri := "otpauth://totp/HolyArrs:luis?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&issuer=HolyArrs&algorithm=SHA1&digits=6&period=30"
	for text, version := range map[string]int{"a": 1, strings.Repeat("x", 15): 2, uri: 7, strings.Repeat("y", 213): 10} {
		m, err := Encode(text)
		if err != nil {
			t.Fatalf("%d bytes: %v", len(text), err)
		}
		if size := 17 + 4*version; len(m) != size || len(m[0]) != size {
			t.Fatalf("%d bytes: size %d, want %d", len(text), len(m), size)
		}
		// Finder corners: dark outer ring, light ring, dark centre.
		for _, c := range [][2]int{{0, 0}, {len(m) - 7, 0}, {0, len(m) - 7}} {
			x, y := c[0], c[1]
			if !m[y][x] || !m[y+6][x+6] || m[y+1][x+1] || !m[y+3][x+3] {
				t.Fatalf("%d bytes: finder at %v", len(text), c)
			}
		}
		if !m[len(m)-8][8] {
			t.Fatalf("%d bytes: dark module missing", len(text))
		}
		again, _ := Encode(text)
		for y := range m {
			for x := range m[y] {
				if m[y][x] != again[y][x] {
					t.Fatal("not the same code twice")
				}
			}
		}
	}
	if _, err := Encode(strings.Repeat("z", 214)); !errors.Is(err, ErrTooLong) {
		t.Fatalf("214 bytes: %v", err)
	}
}
