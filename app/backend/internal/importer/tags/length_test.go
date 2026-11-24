package tags

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// flacOf is a FLAC header of ms milliseconds at 44.1 kHz, for tests.
func flacOf(ms int) []byte {
	out := []byte("fLaC")
	out = append(out, 0x80, 0, 0, 34)
	si := make([]byte, 34)
	samples := int64(ms) * 44100 / 1000
	rate := 44100
	si[10], si[11], si[12] = byte(rate>>12), byte(rate>>4), byte(rate<<4)
	si[13] = byte(samples>>32) & 0x0F
	binary.BigEndian.PutUint32(si[14:18], uint32(samples))
	return append(out, si...)
}

// cbrMP3 is frames of a 128 kbit/s 44.1 kHz MPEG 1 layer 3 stream.
func cbrMP3(frames int) []byte {
	size := 144 * 128000 / 44100 // 417
	var b bytes.Buffer
	for i := 0; i < frames; i++ {
		f := make([]byte, size)
		f[0], f[1], f[2], f[3] = 0xFF, 0xFB, 0x90, 0x00
		b.Write(f)
	}
	return b.Bytes()
}

func xingMP3(frames uint32) []byte {
	f := make([]byte, 417)
	f[0], f[1], f[2], f[3] = 0xFF, 0xFB, 0x90, 0x00
	copy(f[36:], "Xing")
	binary.BigEndian.PutUint32(f[40:], 1)
	binary.BigEndian.PutUint32(f[44:], frames)
	return f
}

func TestLength(t *testing.T) {
	cbr := cbrMP3(200)
	withTag := append(id3(4, [2]string{"TIT2", "Song"}), cbr...)
	cases := []struct {
		name string
		b    []byte
		want int // ms, within 50
	}{
		{"flac", flacOf(215000), 215000},
		{"flac after id3", append(id3(3, [2]string{"TIT2", "x"}), flacOf(1000)...), 1000},
		{"mp3 xing", xingMP3(9000), 9000 * 1152 * 1000 / 44100},
		{"mp3 cbr", cbr, len(cbr) * 8 / 128},
		{"mp3 cbr after tag", withTag, len(cbr) * 8 / 128},
		{"garbage", bytes.Repeat([]byte{0xFF, 0xFB, 0x00, 0x01}, 100), 0},
		{"empty", nil, 0},
		{"text", []byte("hello world, not audio"), 0},
	}
	for _, c := range cases {
		got := Length(bytes.NewReader(c.b), int64(len(c.b)))
		if d := got - c.want; d > 50 || d < -50 {
			t.Errorf("%s: %d ms, want %d", c.name, got, c.want)
		}
	}
}
