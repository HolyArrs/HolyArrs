package tags

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
)

// maxSync is how far past the tag a first MP3 frame is looked for.
const maxSync = 64 << 10

// LengthFile is the play length of an MP3 or FLAC file in milliseconds, read
// from its headers only (a FLAC STREAMINFO block, an MP3 Xing or VBRI
// header, or a constant bitrate times the size); 0 when it cannot be told.
func LengthFile(path string) int {
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return 0
	}
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	return Length(f, fi.Size())
}

// Length is LengthFile for an open file of size bytes.
func Length(r io.ReadSeeker, size int64) int {
	var h [10]byte
	if _, err := io.ReadFull(r, h[:4]); err != nil {
		return 0
	}
	start := int64(0)
	if string(h[:3]) == "ID3" {
		if _, err := io.ReadFull(r, h[4:10]); err != nil {
			return 0
		}
		n, ok := syncsafe(h[6:10])
		if !ok {
			return 0
		}
		start = int64(n) + 10
		if h[5]&0x10 != 0 {
			start += 10 // a footer
		}
		if _, err := r.Seek(start, io.SeekStart); err != nil {
			return 0
		}
		if _, err := io.ReadFull(r, h[:4]); err != nil {
			return 0
		}
	}
	if string(h[:4]) == "fLaC" {
		return flacLength(r)
	}
	if _, err := r.Seek(start, io.SeekStart); err != nil {
		return 0
	}
	buf := make([]byte, maxSync)
	n, _ := io.ReadFull(r, buf)
	return mp3Length(buf[:n], size-start)
}

// flacLength reads the STREAMINFO block, which comes first: 20 bits of
// sample rate and 36 bits of total samples.
func flacLength(r io.Reader) int {
	var h [4]byte
	if _, err := io.ReadFull(r, h[:]); err != nil || h[0]&0x7F != 0 {
		return 0
	}
	var si [34]byte
	if _, err := io.ReadFull(r, si[:]); err != nil {
		return 0
	}
	rate := int64(si[10])<<12 | int64(si[11])<<4 | int64(si[12])>>4
	samples := int64(si[13]&0x0F)<<32 | int64(binary.BigEndian.Uint32(si[14:18]))
	if rate == 0 || samples == 0 {
		return 0
	}
	return int(samples * 1000 / rate)
}

var (
	mp3Rates = [3][3]int{{44100, 48000, 32000}, {22050, 24000, 16000}, {11025, 12000, 8000}} // MPEG 1, 2, 2.5
	// kbit/s by [MPEG 1 or 2][layer 1, 2, 3][index]
	mp3Bitrates = [2][3][16]int{
		{{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448},
			{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384},
			{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}},
		{{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256},
			{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},
			{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}},
	}
)

// mp3Length finds the first frame in b (the bytes after the tag) and reads
// its Xing, Info or VBRI frame count, else takes the bitrate as constant
// over audio bytes.
func mp3Length(b []byte, audio int64) int {
	for i := 0; i+4 <= len(b); i++ {
		if b[i] != 0xFF || b[i+1]&0xE0 != 0xE0 {
			continue
		}
		ver, layer := (b[i+1]>>3)&3, (b[i+1]>>1)&3
		bri, sri := b[i+2]>>4, (b[i+2]>>2)&3
		if ver == 1 || layer == 0 || bri == 0 || bri == 15 || sri == 3 {
			continue
		}
		mpeg := map[byte]int{3: 0, 2: 1, 0: 2}[ver] // MPEG 1, 2, 2.5
		l := 3 - int(layer)                         // 0 layer 1, 1 layer 2, 2 layer 3
		rate := mp3Rates[mpeg][sri]
		kbps := mp3Bitrates[min(mpeg, 1)][l][bri]
		perFrame := map[int]int{0: 384, 1: 1152, 2: 1152}[l]
		if l == 2 && mpeg > 0 {
			perFrame = 576
		}
		mono := b[i+3]>>6 == 3
		side := 32
		switch {
		case mpeg == 0 && mono:
			side = 17
		case mpeg > 0 && !mono:
			side = 17
		case mpeg > 0 && mono:
			side = 9
		}
		frame := b[i:]
		if x := 4 + side; len(frame) >= x+12 && (bytes.Equal(frame[x:x+4], []byte("Xing")) || bytes.Equal(frame[x:x+4], []byte("Info"))) {
			if binary.BigEndian.Uint32(frame[x+4:x+8])&1 != 0 {
				if frames := int64(binary.BigEndian.Uint32(frame[x+8 : x+12])); frames > 0 {
					return int(frames * int64(perFrame) * 1000 / int64(rate))
				}
			}
		}
		if len(frame) >= 36+18 && bytes.Equal(frame[36:40], []byte("VBRI")) {
			if frames := int64(binary.BigEndian.Uint32(frame[50:54])); frames > 0 {
				return int(frames * int64(perFrame) * 1000 / int64(rate))
			}
		}
		// A constant bitrate is only trusted when the next frame is where this one says.
		pad := int(b[i+2]>>1) & 1
		size := perFrame/8*kbps*1000/rate + pad
		if l == 0 {
			size = (12*kbps*1000/rate + pad) * 4
		}
		if next := i + size; next+1 >= len(b) || b[next] != 0xFF || b[next+1]&0xE0 != 0xE0 {
			continue
		}
		if audio -= int64(i); audio <= 0 {
			return 0
		}
		return int(audio * 8 / int64(kbps))
	}
	return 0
}
