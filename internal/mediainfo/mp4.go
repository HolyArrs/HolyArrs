package mediainfo

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

var errMP4HeadersComplete = errors.New("mp4 headers complete")

type mp4BoxHeader struct {
	kind         string
	payloadStart int64
	end          int64
}

type mp4Parser struct {
	r        io.ReaderAt
	size     int64
	elements int
	tracks   int
}

type mp4TrackInfo struct {
	handler  string
	language string
	codec    string
}

func looksLikeMP4(prefix []byte) bool {
	if len(prefix) < 8 {
		return false
	}
	switch string(prefix[4:8]) {
	case "ftyp", "moov", "mdat", "wide", "free":
		return true
	default:
		return false
	}
}

func inspectMP4(r io.ReaderAt, size int64) (Info, error) {
	p := mp4Parser{r: r, size: size}
	info := Info{Container: "mp4"}
	var videoCodecs []string
	moovFound := false
	err := p.walk(0, size, 0, func(box mp4BoxHeader) error {
		if box.kind != "moov" {
			return nil
		}
		moovFound = true
		tracks, err := p.parseMovie(box.payloadStart, box.end, 1)
		if err != nil {
			return err
		}
		for _, track := range tracks {
			switch track.handler {
			case "vide":
				videoCodecs = append(videoCodecs, track.codec)
			case "soun":
				info.AudioTracks++
				if track.codec != "" {
					info.AudioCodecs = append(info.AudioCodecs, track.codec)
				}
				if normalized, ok := normalizeLanguage(track.language); ok {
					info.AudioLanguages = append(info.AudioLanguages, normalized)
				} else {
					info.AudioUnknown = true
				}
			}
		}
		return errMP4HeadersComplete
	})
	if err != nil && !errors.Is(err, errMP4HeadersComplete) {
		return Info{}, err
	}
	if !moovFound {
		return info, nil
	}
	info.VideoCodec = oneCodec(videoCodecs)
	return info, nil
}

func (p *mp4Parser) parseMovie(start, end int64, depth int) ([]mp4TrackInfo, error) {
	tracks := make([]mp4TrackInfo, 0)
	err := p.walk(start, end, depth, func(box mp4BoxHeader) error {
		if box.kind != "trak" {
			return nil
		}
		p.tracks++
		if p.tracks > maxTracks {
			return fmt.Errorf("inspect mp4: track budget exceeded")
		}
		track, err := p.parseTrack(box.payloadStart, box.end, depth+1)
		if err != nil {
			return err
		}
		tracks = append(tracks, track)
		return nil
	})
	return tracks, err
}

func (p *mp4Parser) parseTrack(start, end int64, depth int) (mp4TrackInfo, error) {
	var track mp4TrackInfo
	err := p.walk(start, end, depth, func(box mp4BoxHeader) error {
		if box.kind != "mdia" {
			return nil
		}
		parsed, err := p.parseMedia(box.payloadStart, box.end, depth+1)
		if err != nil {
			return err
		}
		track = parsed
		return nil
	})
	return track, err
}

func (p *mp4Parser) parseMedia(start, end int64, depth int) (mp4TrackInfo, error) {
	var track mp4TrackInfo
	var packedLanguage string
	var extendedLanguage string
	err := p.walk(start, end, depth, func(box mp4BoxHeader) error {
		switch box.kind {
		case "mdhd":
			value, err := p.readMDHDLanguage(box)
			if err != nil {
				return err
			}
			packedLanguage = value
		case "elng":
			value, err := p.readELNG(box)
			if err != nil {
				return err
			}
			extendedLanguage = value
		case "hdlr":
			value, err := p.readHandler(box)
			if err != nil {
				return err
			}
			track.handler = value
		case "minf":
			codec, err := p.readMediaCodec(box.payloadStart, box.end, depth+1)
			if err != nil {
				return err
			}
			track.codec = codec
		}
		return nil
	})
	if extendedLanguage != "" {
		track.language = extendedLanguage
	} else {
		track.language = packedLanguage
	}
	return track, err
}

func (p *mp4Parser) readMediaCodec(start, end int64, depth int) (string, error) {
	codec := ""
	err := p.walk(start, end, depth, func(box mp4BoxHeader) error {
		if box.kind != "stbl" {
			return nil
		}
		return p.walk(box.payloadStart, box.end, depth+1, func(child mp4BoxHeader) error {
			if child.kind != "stsd" {
				return nil
			}
			value, err := p.readSTSD(child)
			if err != nil {
				return err
			}
			codec = value
			return nil
		})
	})
	return codec, err
}

func (p *mp4Parser) readMDHDLanguage(box mp4BoxHeader) (string, error) {
	prefix, err := readAt(p.r, box.payloadStart, min64(36, box.end-box.payloadStart))
	if err != nil {
		return "", fmt.Errorf("inspect mp4 mdhd: %w", err)
	}
	if len(prefix) < 4 {
		return "", fmt.Errorf("inspect mp4 mdhd: truncated header")
	}
	offset := 20
	if prefix[0] == 1 {
		offset = 32
	} else if prefix[0] != 0 {
		return "", fmt.Errorf("inspect mp4 mdhd: unsupported version")
	}
	if len(prefix) < offset+2 {
		return "", fmt.Errorf("inspect mp4 mdhd: truncated language")
	}
	return unpackLanguage(binary.BigEndian.Uint16(prefix[offset : offset+2])), nil
}

func (p *mp4Parser) readELNG(box mp4BoxHeader) (string, error) {
	length := box.end - box.payloadStart
	if length < 5 || length > maxString+5 {
		return "", fmt.Errorf("inspect mp4 elng: invalid string length")
	}
	payload, err := readAt(p.r, box.payloadStart, length)
	if err != nil {
		return "", fmt.Errorf("inspect mp4 elng: %w", err)
	}
	value := payload[4:]
	nul := -1
	for i, b := range value {
		if b == 0 {
			nul = i
			break
		}
	}
	if nul < 0 {
		return "", fmt.Errorf("inspect mp4 elng: missing terminator")
	}
	return string(value[:nul]), nil
}

func (p *mp4Parser) readHandler(box mp4BoxHeader) (string, error) {
	if box.end-box.payloadStart < 12 {
		return "", fmt.Errorf("inspect mp4 hdlr: truncated header")
	}
	prefix, err := readAt(p.r, box.payloadStart, 12)
	if err != nil {
		return "", fmt.Errorf("inspect mp4 hdlr: %w", err)
	}
	return string(prefix[8:12]), nil
}

func (p *mp4Parser) readSTSD(box mp4BoxHeader) (string, error) {
	if box.end-box.payloadStart < 8 {
		return "", fmt.Errorf("inspect mp4 stsd: truncated header")
	}
	header, err := readAt(p.r, box.payloadStart, 8)
	if err != nil {
		return "", fmt.Errorf("inspect mp4 stsd: %w", err)
	}
	count := binary.BigEndian.Uint32(header[4:8])
	if count > maxTracks {
		return "", fmt.Errorf("inspect mp4: sample description budget exceeded")
	}
	position := box.payloadStart + 8
	codecs := make([]string, 0, count)
	for range count {
		if position > box.end-8 {
			return "", fmt.Errorf("inspect mp4 stsd: truncated entry")
		}
		entry, err := readAt(p.r, position, 8)
		if err != nil {
			return "", fmt.Errorf("inspect mp4 stsd: %w", err)
		}
		size := int64(binary.BigEndian.Uint32(entry[:4]))
		if size < 8 || size > box.end-position {
			return "", fmt.Errorf("inspect mp4 stsd: invalid entry size")
		}
		if codec := normalizeCodec(string(entry[4:8])); codec != "" {
			codecs = append(codecs, codec)
		} else if codec := normalizeAudioCodec(string(entry[4:8])); codec != "" {
			codecs = append(codecs, codec)
		}
		position += size
	}
	return oneCodec(codecs), nil
}

func (p *mp4Parser) walk(start, end int64, depth int, visit func(mp4BoxHeader) error) error {
	if depth > maxDepth {
		return fmt.Errorf("inspect mp4: nesting depth exceeded")
	}
	if start < 0 || end < start || end > p.size {
		return fmt.Errorf("inspect mp4: invalid container range")
	}
	for position := start; position < end; {
		p.elements++
		if p.elements > maxElements {
			return fmt.Errorf("inspect mp4: element budget exceeded")
		}
		box, err := p.readBox(position, end)
		if err != nil {
			return err
		}
		if err := visit(box); err != nil {
			return err
		}
		position = box.end
	}
	return nil
}

func (p *mp4Parser) readBox(position, parentEnd int64) (mp4BoxHeader, error) {
	if position > parentEnd-8 {
		return mp4BoxHeader{}, fmt.Errorf("inspect mp4: truncated box header")
	}
	header, err := readAt(p.r, position, 8)
	if err != nil {
		return mp4BoxHeader{}, fmt.Errorf("inspect mp4 box: %w", err)
	}
	size := int64(binary.BigEndian.Uint32(header[:4]))
	headerSize := int64(8)
	if size == 1 {
		if position > parentEnd-16 {
			return mp4BoxHeader{}, fmt.Errorf("inspect mp4: truncated extended box")
		}
		extended, err := readAt(p.r, position+8, 8)
		if err != nil {
			return mp4BoxHeader{}, fmt.Errorf("inspect mp4 box: %w", err)
		}
		value := binary.BigEndian.Uint64(extended)
		if value > uint64(^uint64(0)>>1) {
			return mp4BoxHeader{}, fmt.Errorf("inspect mp4: box size overflow")
		}
		size = int64(value)
		headerSize = 16
	} else if size == 0 {
		size = parentEnd - position
	}
	if size < headerSize || size > parentEnd-position {
		return mp4BoxHeader{}, fmt.Errorf("inspect mp4: invalid box size")
	}
	return mp4BoxHeader{
		kind:         string(header[4:8]),
		payloadStart: position + headerSize,
		end:          position + size,
	}, nil
}

func unpackLanguage(value uint16) string {
	if value == 0x7fff || value < 0x400 {
		return ""
	}
	letters := []byte{
		byte((value>>10)&0x1f) + 0x60,
		byte((value>>5)&0x1f) + 0x60,
		byte(value&0x1f) + 0x60,
	}
	for _, letter := range letters {
		if letter < 'a' || letter > 'z' {
			return ""
		}
	}
	return string(letters)
}

func normalizeCodec(value string) string {
	switch value {
	case "avc1", "avc3":
		return "h264"
	case "hvc1", "hev1":
		return "hevc"
	case "av01":
		return "av1"
	case "vp09":
		return "vp9"
	case "mp2v", "m2v1":
		return "mpeg2"
	case "XVID", "xvid", "DIVX", "divx":
		return "xvid"
	case "wvc1", "WVC1", "vc-1":
		return "vc1"
	default:
		return ""
	}
}

// normalizeAudioCodec names an MP4 audio sample entry (#43: an .m4a holds
// ALAC or AAC).
func normalizeAudioCodec(value string) string {
	switch value {
	case "alac":
		return "alac"
	case "fLaC":
		return "flac"
	case "mp4a":
		return "aac"
	case "Opus":
		return "opus"
	case ".mp3":
		return "mp3"
	case "ac-3":
		return "ac3"
	case "ec-3":
		return "eac3"
	case "lpcm", "sowt", "twos", "ipcm", "fpcm":
		return "pcm"
	default:
		return ""
	}
}

func oneCodec(values []string) string {
	if len(values) == 0 {
		return ""
	}
	first := values[0]
	for _, value := range values[1:] {
		if !strings.EqualFold(value, first) {
			return ""
		}
	}
	return first
}
