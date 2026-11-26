// AVI (RIFF) header reading: only the hdrl list at the start of the file,
// capped at maxAVIHeader bytes, gives the streams, size, rate and length.

package mediafiles

import (
	"encoding/binary"
	"io"
	"strings"
	"time"
)

// maxAVIHeader is the most bytes of the hdrl list read into memory.
const maxAVIHeader = 256 << 10

func looksLikeAVI(prefix []byte) bool {
	return len(prefix) >= 12 && string(prefix[:4]) == "RIFF" && (string(prefix[8:12]) == "AVI " || string(prefix[8:12]) == "AVIX")
}

func inspectAVI(r io.ReaderAt, size int64) (headers, error) {
	h := headers{container: "avi"}
	// The first chunk after "RIFF size AVI " is LIST hdrl in every writer;
	// a few put a JUNK chunk first, so walk a handful of chunks.
	pos := int64(12)
	for range 8 {
		if pos+12 > size {
			return h, malformed("inspect avi: header list missing")
		}
		head, err := readAt(r, pos, 12)
		if err != nil {
			return h, err
		}
		n := int64(binary.LittleEndian.Uint32(head[4:8]))
		if string(head[:4]) == "LIST" && string(head[8:12]) == "hdrl" {
			if n < 4 {
				return h, malformed("inspect avi: header list too small")
			}
			n = min64(n-4, min64(size-pos-12, maxAVIHeader))
			body, err := readAt(r, pos+12, n)
			if err != nil {
				return h, err
			}
			readAVIHeader(body, &h)
			h.found = true
			return h, nil
		}
		pos += 8 + n + n&1
	}
	return h, malformed("inspect avi: header list missing")
}

// aviChunks calls fn for each chunk (or LIST, with its list type as id) in b.
func aviChunks(b []byte, fn func(id string, body []byte)) {
	for count := 0; len(b) >= 8 && count < maxElements; count++ {
		id := string(b[:4])
		n := int(binary.LittleEndian.Uint32(b[4:8]))
		b = b[8:]
		if n < 0 || n > len(b) {
			n = len(b) // a cut-off chunk: read what is there
		}
		body := b[:n]
		if id == "LIST" && len(body) >= 4 {
			id, body = "LIST:"+string(body[:4]), body[4:]
		}
		fn(id, body)
		if n+n&1 >= len(b) {
			return
		}
		b = b[n+n&1:]
	}
}

func readAVIHeader(b []byte, h *headers) {
	var usPerFrame, frames uint32
	var videoFrames, videoRate, videoScale uint64
	aviChunks(b, func(id string, body []byte) {
		switch id {
		case "avih":
			if len(body) >= 40 {
				usPerFrame = binary.LittleEndian.Uint32(body[0:])
				frames = binary.LittleEndian.Uint32(body[16:])
			}
		case "LIST:odml":
			aviChunks(body, func(id string, body []byte) {
				if id == "dmlh" && len(body) >= 4 {
					if n := binary.LittleEndian.Uint32(body); n > frames {
						frames = n // files past 1 GiB keep the real count here
					}
				}
			})
		case "LIST:strl":
			if len(h.tracks) >= maxTracks {
				return
			}
			t, rate, scale, length := readAVIStream(body)
			t.Number = len(h.tracks) + 1
			if t.Kind == "video" && videoScale == 0 && rate > 0 && scale > 0 {
				videoRate, videoScale, videoFrames = rate, scale, length
			}
			h.tracks = append(h.tracks, t)
		}
	})
	switch {
	case usPerFrame > 0 && frames > 0:
		h.duration = seconds(float64(frames)*float64(usPerFrame), 1e6) // in floats: a damaged header must not wrap into a believable length
	case videoScale > 0 && videoFrames > 0:
		h.duration = seconds(float64(videoFrames)*float64(videoScale), float64(videoRate))
	}
	if h.duration > 365*24*time.Hour || h.duration < 0 {
		h.duration = 0
	}
}

func readAVIStream(b []byte) (t Track, rate, scale, length uint64) {
	var kind string
	aviChunks(b, func(id string, body []byte) {
		switch id {
		case "strh":
			if len(body) >= 36 {
				kind = string(body[:4])
				scale = uint64(binary.LittleEndian.Uint32(body[20:]))
				rate = uint64(binary.LittleEndian.Uint32(body[24:]))
				length = uint64(binary.LittleEndian.Uint32(body[32:]))
				if kind == "vids" && scale > 0 {
					t.FrameRate = roundRate(float64(rate) / float64(scale))
				}
			}
		case "strf":
			switch kind {
			case "vids":
				t.Kind = "video"
				if len(body) >= 20 {
					t.Width = int(int32(binary.LittleEndian.Uint32(body[4:])))
					t.Height = int(int32(binary.LittleEndian.Uint32(body[8:])))
					t.Width, t.Height = max(t.Width, -t.Width), max(t.Height, -t.Height) // top-down pictures write a negative height
					if t.Width > 1<<16 || t.Height > 1<<16 {
						t.Width, t.Height = 0, 0
					}
					t.Codec = strings.TrimRight(cut(body[16:20]), " ")
					t.Format = aviVideoCodec(t.Codec)
				}
			case "auds":
				t.Kind = "audio"
				if len(body) >= 4 {
					tag := binary.LittleEndian.Uint16(body)
					if tag == 0xfffe && len(body) >= 26 { // extensible: the real tag starts the sub-format
						tag = binary.LittleEndian.Uint16(body[24:])
					}
					t.Codec = "0x" + strings.ToUpper(hex16(tag))
					t.Format = aviAudioCodec(tag)
					if ch := int(binary.LittleEndian.Uint16(body[2:])); ch > 0 && ch <= 32 {
						t.Channels = ch
					}
				}
			case "txts":
				t.Kind = "subtitle"
			}
		case "strn":
			t.Name = cut(body)
		}
	})
	if t.Kind == "" {
		switch kind {
		case "vids":
			t.Kind = "video"
		case "auds":
			t.Kind = "audio"
		}
	}
	return t, rate, scale, length
}

func hex16(v uint16) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[v>>12], digits[v>>8&15], digits[v>>4&15], digits[v&15]})
}

// aviVideoCodec names an AVI video FourCC.
func aviVideoCodec(fourcc string) string {
	switch strings.ToUpper(fourcc) {
	case "XVID", "DIVX", "DX50", "DIV3", "DIV4", "DIV5", "FMP4", "MP4V", "M4S2", "3IV2":
		return "xvid"
	case "H264", "X264", "AVC1", "DAVC", "VSSH":
		return "h264"
	case "HEVC", "H265", "X265", "HVC1", "HEV1":
		return "hevc"
	case "MPG2", "MPEG", "MPG1", "MPG2V":
		return "mpeg2"
	case "WVC1", "WMV3", "WMVA":
		return "vc1"
	case "VP80", "VP8 ":
		return "vp8"
	case "VP90":
		return "vp9"
	case "AV01":
		return "av1"
	case "MJPG", "AVRN", "LJPG":
		return "mjpeg"
	}
	return ""
}

// aviAudioCodec names a WAVEFORMATEX format tag.
func aviAudioCodec(tag uint16) string {
	switch tag {
	case 0x0001, 0x0003:
		return "pcm"
	case 0x0050:
		return "mp2"
	case 0x0055:
		return "mp3"
	case 0x2000:
		return "ac3"
	case 0x2001:
		return "dts"
	case 0x00ff, 0x1600, 0x1601, 0x1602, 0x1610, 0x706d, 0x4143:
		return "aac"
	case 0x0161, 0x0162, 0x0163:
		return "wma"
	case 0xf1ac:
		return "flac"
	case 0x566f, 0x674f, 0x6750, 0x6751:
		return "vorbis"
	}
	return ""
}
