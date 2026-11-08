package mediafiles

import (
	"encoding/binary"
	"io"
	"math"
	"strings"
)

const (
	ebmlHeaderID      = 0x1a45dfa3
	ebmlDocTypeID     = 0x4282
	ebmlSegmentID     = 0x18538067
	ebmlSeekHeadID    = 0x114d9b74
	ebmlSeekID        = 0x4dbb
	ebmlSeekIDID      = 0x53ab
	ebmlSeekPosition  = 0x53ac
	ebmlInfoID        = 0x1549a966
	ebmlTimecodeScale = 0x2ad7b1
	ebmlDurationID    = 0x4489
	ebmlClusterID     = 0x1f43b675
	ebmlTracksID      = 0x1654ae6b
	ebmlTrackEntryID  = 0xae
	ebmlTrackNumberID = 0xd7
	ebmlTrackTypeID   = 0x83
	ebmlCodecID       = 0x86
	ebmlCodecPrivate  = 0x63a2
	ebmlLanguageID    = 0x22b59c
	ebmlLanguageBCP47 = 0x22b59d
	ebmlNameID        = 0x536e
	ebmlFlagDefault   = 0x88
	ebmlFlagForced    = 0x55aa
	ebmlFlagHI        = 0x55ab
	ebmlDefaultDur    = 0x23e383
	ebmlVideoID       = 0xe0
	ebmlPixelWidth    = 0xb0
	ebmlPixelHeight   = 0xba
	ebmlDisplayWidth  = 0x54b0
	ebmlDisplayHeight = 0x54ba
	ebmlDisplayUnit   = 0x54b2
	ebmlColourID      = 0x55b0
	ebmlBitsPerChan   = 0x55b2
	ebmlMaxCLL        = 0x55bc
	ebmlTransferID    = 0x55ba
	ebmlMasteringID   = 0x55d0
	ebmlAudioID       = 0xe1
	ebmlChannelsID    = 0x9f
	ebmlBlockAddMap   = 0x41e4
	ebmlBlockAddType  = 0x41e7
	ebmlBlockAddExtra = 0x41ed

	// Block addition types that carry a Dolby Vision setup.
	blockAddDVCC = 0x64766343 // dvcC
	blockAddDVVC = 0x64767643 // dvvC
	blockAddDVWC = 0x64767743 // dvwC
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

func inspectMatroska(r io.ReaderAt, size int64) (headers, error) {
	p := matroskaParser{r: r, size: size}
	header, next, err := p.readElement(0, size, false)
	if err != nil {
		return headers{}, err
	}
	if header.id != ebmlHeaderID || header.unknown {
		return headers{}, malformed("inspect matroska: invalid EBML header")
	}
	docType, err := p.readDocType(header.start, header.end, 1)
	if err != nil {
		return headers{}, err
	}
	if docType != "matroska" && docType != "webm" {
		return headers{}, ErrUnsupported
	}

	h := headers{container: docType}
	for position := next; position < size; {
		element, following, err := p.readElement(position, size, true)
		if err != nil {
			return headers{}, err
		}
		if element.id == ebmlSegmentID {
			h.found = true
			if err := p.readSegment(element.start, element.end, &h); err != nil {
				return headers{}, err
			}
			return h, nil
		}
		position = following
	}
	return headers{}, malformed("inspect matroska: segment missing")
}

func (p *matroskaParser) readDocType(start, end int64, depth int) (string, error) {
	docType := ""
	err := p.each(start, end, depth, func(element ebmlElement) error {
		if element.id != ebmlDocTypeID {
			return nil
		}
		value, err := p.readString(element)
		docType = value
		return err
	})
	if err != nil {
		return "", err
	}
	if docType == "" {
		return "", malformed("inspect matroska: document type missing")
	}
	return docType, nil
}

// readSegment reads the segment's Info and Tracks. It stops at the first
// Cluster (media payload) unless a SeekHead says a header it still needs
// comes later. Once the tracks are read, anything odd further on only ends
// the reading.
func (p *matroskaParser) readSegment(start, end int64, h *headers) error {
	scale, duration := 1e6, 0.0
	seekTracks, seekInfo := int64(-1), int64(-1)
	tracksDone, infoDone := false, false
	defer func() { h.duration = seconds(duration*scale, 1e9) }()
	for position := start; position < end && !(tracksDone && infoDone); {
		element, next, err := p.readElement(position, end, true)
		if err != nil {
			if tracksDone {
				return nil
			}
			return err
		}
		switch element.id {
		case ebmlTracksID:
			if tracksDone {
				break
			}
			tracks, err := p.readTracks(element.start, element.end, 2)
			if err != nil {
				return err
			}
			h.tracks, tracksDone = tracks, true
		case ebmlInfoID:
			if infoDone {
				break
			}
			infoDone = true
			err = p.each(element.start, element.end, 2, func(child ebmlElement) error {
				var err error
				switch child.id {
				case ebmlTimecodeScale:
					var v uint64
					if v, err = p.readUint(child); err == nil && v > 0 {
						scale = float64(v)
					}
				case ebmlDurationID:
					duration, err = p.readFloat(child)
				}
				return err
			})
		case ebmlSeekHeadID:
			err = p.each(element.start, element.end, 2, func(seek ebmlElement) error {
				if seek.id != ebmlSeekID {
					return nil
				}
				var id uint64
				at := int64(-1)
				err := p.each(seek.start, seek.end, 3, func(child ebmlElement) error {
					var err error
					var v uint64
					switch child.id {
					case ebmlSeekIDID:
						id, err = p.readUint(child)
					case ebmlSeekPosition:
						if v, err = p.readUint(child); err == nil && v < uint64(end) {
							at = start + int64(v)
						}
					}
					return err
				})
				switch {
				case id == ebmlTracksID && seekTracks < 0:
					seekTracks = at
				case id == ebmlInfoID && seekInfo < 0:
					seekInfo = at
				}
				return err
			})
		case ebmlClusterID:
			// Headers come before the clusters; jump forward to one a
			// SeekHead places later, else there is nothing more to read.
			switch {
			case !tracksDone && seekTracks > position:
				next, seekTracks = seekTracks, -1
			case !infoDone && seekInfo > position:
				next, seekInfo = seekInfo, -1
			default:
				return nil
			}
		}
		if err != nil {
			if tracksDone {
				return nil
			}
			return err
		}
		if element.unknown && element.id != ebmlClusterID {
			return nil
		}
		position = next
	}
	return nil
}

func (p *matroskaParser) readTracks(start, end int64, depth int) ([]Track, error) {
	tracks := make([]Track, 0)
	err := p.each(start, end, depth, func(element ebmlElement) error {
		if element.id != ebmlTrackEntryID {
			return nil
		}
		p.tracks++
		if p.tracks > maxTracks {
			return overLimit("inspect matroska: track budget exceeded")
		}
		track, err := p.readTrack(element.start, element.end, depth+1)
		if err != nil {
			return err
		}
		tracks = append(tracks, track)
		return nil
	})
	return tracks, err
}

func (p *matroskaParser) readTrack(start, end int64, depth int) (Track, error) {
	track := Track{Default: true} // FlagDefault is 1 when not written
	var kind uint64
	var language, languageBCP string
	var private []byte
	var frameNanos uint64
	c := colour{dvCompat: -1}
	err := p.each(start, end, depth, func(element ebmlElement) error {
		var err error
		var v uint64
		switch element.id {
		case ebmlTrackNumberID:
			v, err = p.readUint(element)
			track.Number = int(v & 0x7fffffff)
		case ebmlTrackTypeID:
			kind, err = p.readUint(element)
		case ebmlCodecID:
			track.Codec, err = p.readString(element)
		case ebmlLanguageID:
			language, err = p.readString(element)
		case ebmlLanguageBCP47:
			languageBCP, err = p.readString(element)
		case ebmlNameID:
			track.Name, err = p.readString(element)
		case ebmlFlagDefault:
			v, err = p.readUint(element)
			track.Default = v == 1
		case ebmlFlagForced:
			v, err = p.readUint(element)
			track.Forced = v == 1
		case ebmlFlagHI:
			v, err = p.readUint(element)
			track.HearingImpaired = v == 1
		case ebmlDefaultDur:
			frameNanos, err = p.readUint(element)
		case ebmlCodecPrivate:
			private, err = readAt(p.r, element.start, min64(element.end-element.start, maxPrivate))
		case ebmlVideoID:
			err = p.readVideo(element, depth+1, &track, &c)
		case ebmlAudioID:
			err = p.each(element.start, element.end, depth+1, func(child ebmlElement) error {
				if child.id != ebmlChannelsID {
					return nil
				}
				v, err := p.readUint(child)
				track.Channels = int(min(v, 255))
				return err
			})
		case ebmlBlockAddMap:
			err = p.readBlockAddition(element, depth+1, &c)
		}
		return err
	})
	if err != nil {
		return Track{}, err
	}
	track.Language = language
	if languageBCP != "" {
		track.Language = languageBCP // BCP 47 wins over the old field
	}
	switch kind {
	case 1:
		track.Kind = "video"
		track.Format = matroskaCodec(track.Codec, private)
		if track.BitDepth == 0 {
			track.BitDepth = configBitDepth(track.Format, private)
		}
		if c.transfer == 0 {
			c.transfer = configTransfer(track.Format, private)
		}
		track.HDR = c.hdrName()
		if frameNanos > 0 {
			track.FrameRate = roundRate(1e9 / float64(frameNanos))
		}
	case 2:
		track.Kind = "audio"
		track.Format = matroskaAudioCodec(track.Codec)
		if track.Channels == 0 {
			track.Channels = 1 // the Matroska default
		}
	case 0x11:
		track.Kind = "subtitle"
	}
	return track, nil
}

func (p *matroskaParser) readVideo(video ebmlElement, depth int, track *Track, c *colour) error {
	var dw, dh, unit uint64
	err := p.each(video.start, video.end, depth, func(element ebmlElement) error {
		var err error
		var v uint64
		switch element.id {
		case ebmlDisplayWidth:
			dw, err = p.readUint(element)
		case ebmlDisplayHeight:
			dh, err = p.readUint(element)
		case ebmlDisplayUnit:
			unit, err = p.readUint(element)
		case ebmlPixelWidth:
			v, err = p.readUint(element)
			track.Width = int(min(v, 1<<20))
		case ebmlPixelHeight:
			v, err = p.readUint(element)
			track.Height = int(min(v, 1<<20))
		case ebmlColourID:
			err = p.each(element.start, element.end, depth+1, func(child ebmlElement) error {
				var err error
				var v uint64
				switch child.id {
				case ebmlBitsPerChan:
					v, err = p.readUint(child)
					track.BitDepth = int(min(v, 32))
				case ebmlTransferID:
					v, err = p.readUint(child)
					if v != 2 {
						c.transfer = int(min(v, 255))
					}
				case ebmlMasteringID:
					c.mastering = true
				case ebmlMaxCLL:
					v, err = p.readUint(child)
					c.mastering = c.mastering || v > 0
				}
				return err
			})
		}
		return err
	})
	// The display size defaults to the pixel size; it is in pixels (unit
	// 0) or only an aspect ratio (unit 3), which reads the same way here.
	if dh == 0 {
		dh = uint64(max(track.Height, 0))
	}
	if unit == 0 || unit == 3 {
		track.DisplayWidth = displayWidth(track.Width, track.Height, dw, dh)
	}
	return err
}

func (p *matroskaParser) readBlockAddition(mapping ebmlElement, depth int, c *colour) error {
	var kind uint64
	var extra []byte
	err := p.each(mapping.start, mapping.end, depth, func(element ebmlElement) error {
		var err error
		switch element.id {
		case ebmlBlockAddType:
			kind, err = p.readUint(element)
		case ebmlBlockAddExtra:
			extra, err = readAt(p.r, element.start, min64(element.end-element.start, 64))
		}
		return err
	})
	if kind == blockAddDVCC || kind == blockAddDVVC || kind == blockAddDVWC {
		c.readDolbyVision(extra)
	}
	return err
}

// each calls visit for every element directly inside [start, end).
func (p *matroskaParser) each(start, end int64, depth int, visit func(ebmlElement) error) error {
	if depth > maxDepth {
		return overLimit("inspect matroska: nesting depth exceeded")
	}
	for position := start; position < end; {
		element, next, err := p.readElement(position, end, false)
		if err != nil {
			return err
		}
		if err := visit(element); err != nil {
			return err
		}
		position = next
	}
	return nil
}

func (p *matroskaParser) readElement(position, parentEnd int64, allowUnknown bool) (ebmlElement, int64, error) {
	p.elements++
	if p.elements > maxElements {
		return ebmlElement{}, 0, overLimit("inspect matroska: element budget exceeded")
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
		return ebmlElement{}, 0, malformed("inspect matroska: truncated element header")
	}
	// A Segment may run past a cut-off file, and a Cluster is never read:
	// both end where their parent ends when their size is unknown or too big.
	if allowUnknown && (id == ebmlSegmentID || id == ebmlClusterID) && (unknown || size > uint64(parentEnd-payloadStart)) {
		return ebmlElement{id: id, start: payloadStart, end: parentEnd, unknown: true}, parentEnd, nil
	}
	if unknown {
		return ebmlElement{}, 0, malformed("inspect matroska: unsupported unknown element size")
	}
	if size > uint64(parentEnd-payloadStart) {
		return ebmlElement{}, 0, malformed("inspect matroska: invalid element size")
	}
	end := payloadStart + int64(size)
	return ebmlElement{id: id, start: payloadStart, end: end}, end, nil
}

func (p *matroskaParser) readVINT(position int64, maxLength int, keepMarker bool) (uint64, int, bool, error) {
	first, err := readAt(p.r, position, 1)
	if err != nil {
		return 0, 0, false, err
	}
	marker := byte(0x80)
	length := 1
	for length <= 8 && first[0]&marker == 0 {
		marker >>= 1
		length++
	}
	if marker == 0 || length > maxLength {
		return 0, 0, false, malformed("inspect matroska: invalid vint")
	}
	raw, err := readAt(p.r, position, int64(length))
	if err != nil {
		return 0, 0, false, err
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

// readString reads a string element, keeping at most maxString bytes and
// stopping at the first NUL: EBML lets strings be padded with zero bytes,
// and an empty one is just "".
func (p *matroskaParser) readString(element ebmlElement) (string, error) {
	length := min64(element.end-element.start, maxString)
	if length <= 0 {
		return "", nil
	}
	value, err := readAt(p.r, element.start, length)
	return cut(value), err
}

func (p *matroskaParser) readUint(element ebmlElement) (uint64, error) {
	length := element.end - element.start
	if length == 0 {
		return 0, nil // EBML: an empty integer is its default, 0
	}
	if length < 0 || length > 8 {
		return 0, malformed("inspect matroska: invalid integer length")
	}
	value, err := readAt(p.r, element.start, length)
	if err != nil {
		return 0, err
	}
	var padded [8]byte
	copy(padded[8-len(value):], value)
	return binary.BigEndian.Uint64(padded[:]), nil
}

func (p *matroskaParser) readFloat(element ebmlElement) (float64, error) {
	switch element.end - element.start {
	case 0:
		return 0, nil
	case 4:
		b, err := readAt(p.r, element.start, 4)
		if err != nil {
			return 0, err
		}
		return float64(math.Float32frombits(binary.BigEndian.Uint32(b))), nil
	case 8:
		b, err := readAt(p.r, element.start, 8)
		if err != nil {
			return 0, err
		}
		return math.Float64frombits(binary.BigEndian.Uint64(b)), nil
	}
	return 0, malformed("inspect matroska: invalid float length")
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
