package mediainfo

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

const (
	ebmlHeaderID      = 0x1a45dfa3
	ebmlDocTypeID     = 0x4282
	ebmlSegmentID     = 0x18538067
	ebmlTracksID      = 0x1654ae6b
	ebmlTrackEntryID  = 0xae
	ebmlTrackTypeID   = 0x83
	ebmlCodecID       = 0x86
	ebmlCodecPrivate  = 0x63a2
	ebmlLanguageID    = 0x22b59c
	ebmlLanguageBCP47 = 0x22b59d
)

type ebmlElement struct {
	id      uint64
	start   int64
	end     int64
	unknown bool
}

type matroskaParser struct {
	r        io.ReaderAt
	size     int64
	elements int
	tracks   int
}

type matroskaTrack struct {
	kind         uint64
	codec        string
	language     string
	languageBCP  string
	codecPrivate []byte
}

func inspectMatroska(r io.ReaderAt, size int64) (Info, error) {
	p := matroskaParser{r: r, size: size}
	header, next, err := p.readElement(0, size, false)
	if err != nil {
		return Info{}, err
	}
	if header.id != ebmlHeaderID || header.unknown {
		return Info{}, fmt.Errorf("inspect matroska: invalid EBML header")
	}
	docType, err := p.readDocType(header.start, header.end, 1)
	if err != nil {
		return Info{}, err
	}
	if docType != "matroska" && docType != "webm" {
		return Info{Status: "unsupported"}, nil
	}

	info := Info{Container: docType}
	var codecs []string
	segmentFound := false
	for position := next; position < size; {
		element, following, err := p.readElement(position, size, true)
		if err != nil {
			return Info{}, err
		}
		if element.id == ebmlSegmentID {
			segmentFound = true
			tracks, err := p.readSegment(element.start, element.end, 1)
			if err != nil {
				return Info{}, err
			}
			for _, track := range tracks {
				switch track.kind {
				case 1:
					codecs = append(codecs, matroskaCodec(track.codec, track.codecPrivate))
				case 2:
					info.AudioTracks++
					if c := matroskaAudioCodec(track.codec); c != "" {
						info.AudioCodecs = append(info.AudioCodecs, c)
					}
					value := track.languageBCP
					if value == "" {
						value = track.language
					}
					if normalized, ok := normalizeLanguage(value); ok {
						info.AudioLanguages = append(info.AudioLanguages, normalized)
					} else {
						info.AudioUnknown = true
					}
				}
			}
			break
		}
		position = following
	}
	if !segmentFound {
		return Info{}, fmt.Errorf("inspect matroska: segment missing")
	}
	info.VideoCodec = oneCodec(codecs)
	return info, nil
}

func (p *matroskaParser) readDocType(start, end int64, depth int) (string, error) {
	if depth > maxDepth {
		return "", fmt.Errorf("inspect matroska: nesting depth exceeded")
	}
	docType := ""
	for position := start; position < end; {
		element, next, err := p.readElement(position, end, false)
		if err != nil {
			return "", err
		}
		if element.id == ebmlDocTypeID {
			value, err := p.readString(element)
			if err != nil {
				return "", err
			}
			docType = value
		}
		position = next
	}
	if docType == "" {
		return "", fmt.Errorf("inspect matroska: document type missing")
	}
	return docType, nil
}

func (p *matroskaParser) readSegment(start, end int64, depth int) ([]matroskaTrack, error) {
	if depth > maxDepth {
		return nil, fmt.Errorf("inspect matroska: nesting depth exceeded")
	}
	tracks := make([]matroskaTrack, 0)
	for position := start; position < end; {
		element, next, err := p.readElement(position, end, false)
		if err != nil {
			return nil, err
		}
		if element.id == ebmlTracksID {
			values, err := p.readTracks(element.start, element.end, depth+1)
			if err != nil {
				return nil, err
			}
			return append(tracks, values...), nil
		}
		position = next
	}
	return tracks, nil
}

func (p *matroskaParser) readTracks(start, end int64, depth int) ([]matroskaTrack, error) {
	if depth > maxDepth {
		return nil, fmt.Errorf("inspect matroska: nesting depth exceeded")
	}
	tracks := make([]matroskaTrack, 0)
	for position := start; position < end; {
		element, next, err := p.readElement(position, end, false)
		if err != nil {
			return nil, err
		}
		if element.id == ebmlTrackEntryID {
			p.tracks++
			if p.tracks > maxTracks {
				return nil, fmt.Errorf("inspect matroska: track budget exceeded")
			}
			track, err := p.readTrack(element.start, element.end, depth+1)
			if err != nil {
				return nil, err
			}
			tracks = append(tracks, track)
		}
		position = next
	}
	return tracks, nil
}

func (p *matroskaParser) readTrack(start, end int64, depth int) (matroskaTrack, error) {
	if depth > maxDepth {
		return matroskaTrack{}, fmt.Errorf("inspect matroska: nesting depth exceeded")
	}
	var track matroskaTrack
	for position := start; position < end; {
		element, next, err := p.readElement(position, end, false)
		if err != nil {
			return matroskaTrack{}, err
		}
		switch element.id {
		case ebmlTrackTypeID:
			track.kind, err = p.readUint(element)
		case ebmlCodecID:
			track.codec, err = p.readString(element)
		case ebmlLanguageID:
			track.language, err = p.readString(element)
		case ebmlLanguageBCP47:
			track.languageBCP, err = p.readString(element)
		case ebmlCodecPrivate:
			if element.end-element.start >= 20 {
				track.codecPrivate, err = readAt(p.r, element.start, 20)
			}
		}
		if err != nil {
			return matroskaTrack{}, err
		}
		position = next
	}
	return track, nil
}

func (p *matroskaParser) readElement(position, parentEnd int64, allowUnknown bool) (ebmlElement, int64, error) {
	p.elements++
	if p.elements > maxElements {
		return ebmlElement{}, 0, fmt.Errorf("inspect matroska: element budget exceeded")
	}
	id, idLength, _, err := p.readVINT(position, 4, true)
	if err != nil {
		return ebmlElement{}, 0, err
	}
	size, sizeLength, unknown, err := p.readVINT(position+int64(idLength), 8, false)
	if err != nil {
		return ebmlElement{}, 0, err
	}
	payloadStart := position + int64(idLength+sizeLength)
	if payloadStart > parentEnd {
		return ebmlElement{}, 0, fmt.Errorf("inspect matroska: truncated element header")
	}
	if unknown {
		if !allowUnknown || id != ebmlSegmentID {
			return ebmlElement{}, 0, fmt.Errorf("inspect matroska: unsupported unknown element size")
		}
		return ebmlElement{id: id, start: payloadStart, end: parentEnd, unknown: true}, parentEnd, nil
	}
	if size > uint64(parentEnd-payloadStart) {
		return ebmlElement{}, 0, fmt.Errorf("inspect matroska: invalid element size")
	}
	end := payloadStart + int64(size)
	return ebmlElement{id: id, start: payloadStart, end: end}, end, nil
}

func (p *matroskaParser) readVINT(position int64, maxLength int, keepMarker bool) (uint64, int, bool, error) {
	first, err := readAt(p.r, position, 1)
	if err != nil {
		return 0, 0, false, fmt.Errorf("inspect matroska vint: %w", err)
	}
	marker := byte(0x80)
	length := 1
	for length <= 8 && first[0]&marker == 0 {
		marker >>= 1
		length++
	}
	if marker == 0 || length > maxLength {
		return 0, 0, false, fmt.Errorf("inspect matroska: invalid vint")
	}
	raw, err := readAt(p.r, position, int64(length))
	if err != nil {
		return 0, 0, false, fmt.Errorf("inspect matroska vint: %w", err)
	}
	value := uint64(raw[0])
	if !keepMarker {
		value &= uint64(marker - 1)
	}
	for _, b := range raw[1:] {
		value = value<<8 | uint64(b)
	}
	unknown := false
	if !keepMarker {
		maxValue := uint64(1)<<(7*length) - 1
		unknown = value == maxValue
	}
	return value, length, unknown, nil
}

func (p *matroskaParser) readString(element ebmlElement) (string, error) {
	length := element.end - element.start
	if length <= 0 || length > maxString {
		return "", fmt.Errorf("inspect matroska: invalid string length")
	}
	value, err := readAt(p.r, element.start, length)
	if err != nil {
		return "", err
	}
	return string(value), nil
}

func (p *matroskaParser) readUint(element ebmlElement) (uint64, error) {
	length := element.end - element.start
	if length <= 0 || length > 8 {
		return 0, fmt.Errorf("inspect matroska: invalid integer length")
	}
	value, err := readAt(p.r, element.start, length)
	if err != nil {
		return 0, err
	}
	var padded [8]byte
	copy(padded[8-len(value):], value)
	return binary.BigEndian.Uint64(padded[:]), nil
}

func matroskaCodec(codec string, private []byte) string {
	switch codec {
	case "V_MPEG4/ISO/AVC":
		return "h264"
	case "V_MPEGH/ISO/HEVC":
		return "hevc"
	case "V_AV1":
		return "av1"
	case "V_VP9":
		return "vp9"
	case "V_MPEG2":
		return "mpeg2"
	case "V_MS/VFW/FOURCC":
		if len(private) >= 20 {
			return normalizeCodec(string(private[16:20]))
		}
	}
	return ""
}

// matroskaAudioCodec names a Matroska audio codec id.
func matroskaAudioCodec(id string) string {
	switch {
	case id == "A_FLAC":
		return "flac"
	case id == "A_ALAC":
		return "alac"
	case id == "A_WAVPACK4":
		return "wavpack"
	case id == "A_TTA1":
		return "tta"
	case id == "A_TRUEHD":
		return "truehd"
	case strings.HasPrefix(id, "A_PCM"):
		return "pcm"
	case strings.HasPrefix(id, "A_AAC"):
		return "aac"
	case id == "A_MPEG/L3":
		return "mp3"
	case id == "A_MPEG/L2":
		return "mp2"
	case id == "A_VORBIS":
		return "vorbis"
	case id == "A_OPUS":
		return "opus"
	case id == "A_AC3":
		return "ac3"
	case id == "A_EAC3":
		return "eac3"
	case strings.HasPrefix(id, "A_DTS"):
		return "dts"
	}
	return ""
}
