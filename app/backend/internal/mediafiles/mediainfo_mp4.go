package mediafiles

import (
	"encoding/binary"
	"errors"
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

// mp4TrackState is one trak while it is read.
type mp4TrackState struct {
	Track
	handler   string
	enabled   bool
	timescale uint32
	duration  uint64
	entry     []byte // the first sample entry, at most maxPrivate bytes
	entryKind string
	// frames and ticks are the samples counted at the start of the
	// time-to-sample table (stts) and how long they play.
	frames, ticks uint64
	tkWidth       int
	tkHeight      int
}

func looksLikeMP4(prefix []byte) bool {
	if len(prefix) < 8 {
		return false
	}
	switch string(prefix[4:8]) {
	case "ftyp", "moov", "mdat", "wide", "free", "skip", "pnot":
		return true
	default:
		return false
	}
}

func inspectMP4(r io.ReaderAt, size int64) (headers, error) {
	p := mp4Parser{r: r, size: size}
	h := headers{container: "mp4"}
	err := p.walk(0, size, 0, func(box mp4BoxHeader) error {
		if box.kind != "moov" {
			return nil
		}
		h.found = true
		if err := p.parseMovie(box.payloadStart, box.end, 1, &h); err != nil {
			return err
		}
		return errMP4HeadersComplete
	})
	if err != nil && !errors.Is(err, errMP4HeadersComplete) {
		return headers{}, err
	}
	return h, nil
}

func (p *mp4Parser) parseMovie(start, end int64, depth int, h *headers) error {
	h.tracks = make([]Track, 0)
	longest := h.duration
	err := p.walk(start, end, depth, func(box mp4BoxHeader) error {
		switch box.kind {
		case "mvhd":
			// version 0: 32-bit times; version 1: 64-bit times.
			b, err := p.readSmall(box, 32)
			if err != nil {
				return err
			}
			if len(b) >= 20 && b[0] == 0 {
				if d := binary.BigEndian.Uint32(b[16:20]); d != 0xffffffff {
					h.duration = seconds(float64(d), float64(binary.BigEndian.Uint32(b[12:16])))
				}
			} else if len(b) >= 32 && b[0] == 1 {
				if d := binary.BigEndian.Uint64(b[24:32]); d != ^uint64(0) {
					h.duration = seconds(float64(d), float64(binary.BigEndian.Uint32(b[20:24])))
				}
			}
		case "trak":
			p.tracks++
			if p.tracks > maxTracks {
				return overLimit("inspect mp4: track budget exceeded")
			}
			track, err := p.parseTrack(box.payloadStart, box.end, depth+1)
			if err != nil {
				return err
			}
			if d := seconds(float64(track.duration), float64(track.timescale)); d > longest {
				longest = d
			}
			h.tracks = append(h.tracks, track.finish())
		}
		return nil
	})
	if h.duration == 0 {
		h.duration = longest
	}
	return err
}

func (p *mp4Parser) parseTrack(start, end int64, depth int) (mp4TrackState, error) {
	track := mp4TrackState{enabled: true}
	err := p.walk(start, end, depth, func(box mp4BoxHeader) error {
		switch box.kind {
		case "tkhd":
			b, err := p.readSmall(box, 96)
			if err != nil {
				return err
			}
			if len(b) < 4 {
				return nil
			}
			track.enabled = b[3]&1 == 1
			id, size := 12, 76 // version 0
			if b[0] == 1 {
				id, size = 20, 88
			}
			if len(b) >= id+4 {
				track.Number = int(binary.BigEndian.Uint32(b[id:]) & 0x7fffffff)
			}
			if len(b) >= size+8 { // 16.16 fixed point
				track.tkWidth = int(binary.BigEndian.Uint32(b[size:]) >> 16)
				track.tkHeight = int(binary.BigEndian.Uint32(b[size+4:]) >> 16)
			}
		case "mdia":
			return p.parseMedia(box.payloadStart, box.end, depth+1, &track)
		}
		return nil
	})
	return track, err
}

func (p *mp4Parser) parseMedia(start, end int64, depth int, track *mp4TrackState) error {
	var packedLanguage string
	var extendedLanguage string
	err := p.walk(start, end, depth, func(box mp4BoxHeader) error {
		switch box.kind {
		case "mdhd":
			return p.readMDHD(box, &packedLanguage, track)
		case "elng":
			value, err := p.readELNG(box)
			if err != nil {
				return err
			}
			extendedLanguage = value
		case "hdlr":
			b, err := p.readSmall(box, 24+maxString+1)
			if err != nil {
				return err
			}
			if len(b) < 12 {
				return malformed("inspect mp4 hdlr: truncated header")
			}
			track.handler = string(b[8:12])
			if len(b) > 24 {
				name := b[24:]
				if int(name[0]) == len(name)-1 { // QuickTime: a length byte first
					name = name[1:]
				}
				track.Name = cut(name)
			}
		case "minf":
			return p.walk(box.payloadStart, box.end, depth+1, func(child mp4BoxHeader) error {
				if child.kind != "stbl" {
					return nil
				}
				return p.walk(child.payloadStart, child.end, depth+2, func(table mp4BoxHeader) error {
					switch table.kind {
					case "stsd":
						return p.readSTSD(table, track)
					case "stts":
						return p.readSTTS(table, track)
					}
					return nil
				})
			})
		}
		return nil
	})
	if extendedLanguage != "" {
		track.Language = extendedLanguage
	} else {
		track.Language = packedLanguage
	}
	return err
}

func (p *mp4Parser) readMDHD(box mp4BoxHeader, language *string, track *mp4TrackState) error {
	prefix, err := p.readSmall(box, 36)
	if err != nil {
		return err
	}
	if len(prefix) < 4 {
		return malformed("inspect mp4 mdhd: truncated header")
	}
	offset := 20
	if prefix[0] == 1 {
		offset = 32
		if len(prefix) >= 32 {
			track.timescale = binary.BigEndian.Uint32(prefix[20:24])
			track.duration = binary.BigEndian.Uint64(prefix[24:32])
		}
	} else if prefix[0] != 0 {
		return malformed("inspect mp4 mdhd: unsupported version")
	} else if len(prefix) >= 20 {
		track.timescale = binary.BigEndian.Uint32(prefix[12:16])
		track.duration = uint64(binary.BigEndian.Uint32(prefix[16:20]))
	}
	if len(prefix) < offset+2 {
		return malformed("inspect mp4 mdhd: truncated language")
	}
	*language = unpackLanguage(binary.BigEndian.Uint16(prefix[offset : offset+2]))
	return nil
}

func (p *mp4Parser) readELNG(box mp4BoxHeader) (string, error) {
	length := box.end - box.payloadStart
	if length < 5 || length > maxString+5 {
		return "", malformed("inspect mp4 elng: invalid string length")
	}
	payload, err := readAt(p.r, box.payloadStart, length)
	if err != nil {
		return "", err
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
		return "", malformed("inspect mp4 elng: missing terminator")
	}
	return string(value[:nul]), nil
}

// readSTSD names the codec of every sample entry (the track's format is
// known only when they agree) and keeps the first entry for its details.
func (p *mp4Parser) readSTSD(box mp4BoxHeader, track *mp4TrackState) error {
	if box.end-box.payloadStart < 8 {
		return malformed("inspect mp4 stsd: truncated header")
	}
	header, err := readAt(p.r, box.payloadStart, 8)
	if err != nil {
		return err
	}
	count := binary.BigEndian.Uint32(header[4:8])
	if count > maxTracks {
		return overLimit("inspect mp4: sample description budget exceeded")
	}
	position := box.payloadStart + 8
	codecs := make([]string, 0, count)
	for i := range count {
		if position > box.end-8 {
			return malformed("inspect mp4 stsd: truncated entry")
		}
		entry, err := readAt(p.r, position, 8)
		if err != nil {
			return err
		}
		size := int64(binary.BigEndian.Uint32(entry[:4]))
		if size < 8 || size > box.end-position {
			return malformed("inspect mp4 stsd: invalid entry size")
		}
		kind := string(entry[4:8])
		if i == 0 {
			track.entryKind = kind
			if track.entry, err = readAt(p.r, position, min64(size, maxPrivate)); err != nil {
				return err
			}
		}
		if codec := normalizeCodec(kind); codec != "" {
			codecs = append(codecs, codec)
		} else if codec := normalizeAudioCodec(kind); codec != "" {
			codecs = append(codecs, codec)
		}
		position += size
	}
	track.Format = oneCodec(codecs)
	return nil
}

// readSTTS adds up the first entries of the time-to-sample table: a steady
// frame rate has one entry, and a file remuxed from Matroska alternates
// durations rounded to the millisecond (672 and 656 ticks at 16000 a
// second for 23.976). The last sample's duration is often a guess, so it
// is left out when the whole table was read.
func (p *mp4Parser) readSTTS(box mp4BoxHeader, track *mp4TrackState) error {
	b, err := p.readSmall(box, 8+256*8)
	if err != nil || len(b) < 8 {
		return err
	}
	all := int(binary.BigEndian.Uint32(b[4:8]))
	n := min(all, (len(b)-8)/8)
	for i := range n {
		count := uint64(binary.BigEndian.Uint32(b[8+i*8:]))
		delta := uint64(binary.BigEndian.Uint32(b[12+i*8:]))
		if i == all-1 && count > 0 && (track.frames > 0 || count > 1) {
			count--
		}
		track.frames += count
		track.ticks += count * delta
	}
	return nil
}

// commonRates are the frame rates video is made at; a rate read from
// rounded timestamps that lands within 0.2% of one is that one.
var commonRates = []float64{24000.0 / 1001, 24, 25, 30000.0 / 1001, 30, 48000.0 / 1001, 48, 50, 60000.0 / 1001, 60, 100, 120000.0 / 1001, 120}

// averageRate gives frames a second from frames playing ticks at
// timescale ticks a second, snapped to a common rate when close.
func averageRate(frames, ticks uint64, timescale uint32) float64 {
	if frames == 0 || ticks == 0 || timescale == 0 {
		return 0
	}
	fps := float64(frames) * float64(timescale) / float64(ticks)
	for _, r := range commonRates {
		if fps > r*0.998 && fps < r*1.002 {
			return roundRate(r)
		}
	}
	return roundRate(fps)
}

// finish turns what was read into the track's facts.
func (t *mp4TrackState) finish() Track {
	out := t.Track
	out.Codec = cut([]byte(t.entryKind))
	out.Default = t.enabled
	switch {
	case t.handler == "vide":
		out.Kind = "video"
		c := colour{dvCompat: -1}
		t.readVisualEntry(&out, &c)
		if out.Width == 0 {
			out.Width, out.Height = t.tkWidth, t.tkHeight
		}
		if out.DisplayWidth == 0 && t.tkHeight == out.Height && t.tkWidth > out.Width {
			// The track header holds the presentation size, aspect applied.
			out.DisplayWidth = t.tkWidth
		}
		out.HDR = c.hdrName()
		out.FrameRate = averageRate(t.frames, t.ticks, t.timescale)
	case t.handler == "soun":
		out.Kind = "audio"
		t.readAudioEntry(&out)
	case isMP4Subtitle(t.handler, t.entryKind):
		out.Kind = "subtitle"
		out.Forced = strings.Contains(strings.ToLower(out.Name), "forced")
		// tx3g display flags: the top bit says every sample is forced.
		if t.entryKind == "tx3g" && len(t.entry) >= 20 && t.entry[16]&0x80 != 0 {
			out.Forced = true
		}
	}
	return out
}

func isMP4Subtitle(handler, entry string) bool {
	switch handler {
	case "sbtl", "subt", "text", "clcp":
		return true
	}
	switch entry {
	case "tx3g", "wvtt", "stpp", "c608", "c708":
		return true
	}
	return false
}

// readVisualEntry reads a visual sample entry: the coded size, then its
// child boxes (codec setup, colour, HDR and Dolby Vision records).
func (t *mp4TrackState) readVisualEntry(out *Track, c *colour) {
	e := t.entry
	if len(e) < 86 {
		return
	}
	out.Width = int(binary.BigEndian.Uint16(e[32:34]))
	out.Height = int(binary.BigEndian.Uint16(e[34:36]))
	switch t.entryKind {
	case "dvh1", "dvhe", "dva1", "dvav", "dav1":
		c.dv = true
	}
	var config []byte
	childBoxes(e[86:], func(kind string, b []byte) {
		switch kind {
		case "hvcC", "avcC", "av1C":
			config = b
		case "colr":
			c.readColr(b)
		case "mdcv", "clli", "SmDm", "CoLL":
			c.mastering = true
		case "dvcC", "dvvC", "dvwC":
			c.readDolbyVision(b)
		case "pasp": // pixel aspect: horizontal and vertical spacing
			if len(b) >= 8 {
				h, v := binary.BigEndian.Uint32(b[:4]), binary.BigEndian.Uint32(b[4:8])
				out.DisplayWidth = displayWidth(out.Width, out.Height, uint64(out.Width)*uint64(h), uint64(out.Height)*uint64(v))
			}
		}
	})
	out.BitDepth = configBitDepth(out.Format, config)
	if c.transfer == 0 {
		c.transfer = configTransfer(out.Format, config)
	}
}

// readAudioEntry reads the channel count of an audio sample entry
// (QuickTime versions 0 to 2), and the real count from a dac3, dec3 or
// esds (AAC) box.
func (t *mp4TrackState) readAudioEntry(out *Track) {
	e := t.entry
	if len(e) < 36 {
		return
	}
	children := 36
	out.Channels = int(binary.BigEndian.Uint16(e[24:26]))
	switch binary.BigEndian.Uint16(e[16:18]) {
	case 1:
		children = 52
	case 2:
		children = 72
		if len(e) < 52 {
			return
		}
		out.Channels = int(min(binary.BigEndian.Uint32(e[48:52]), 255))
	}
	if len(e) < children {
		return
	}
	childBoxes(e[children:], func(kind string, b []byte) {
		n := ac3Channels(kind, b)
		if kind == "esds" {
			n = aacChannels(b)
		}
		if n > 0 {
			out.Channels = n
		}
	})
}

// childBoxes calls fn for each whole box in b; it stops at the first one
// that does not fit.
func childBoxes(b []byte, fn func(kind string, payload []byte)) {
	for len(b) >= 8 {
		size := binary.BigEndian.Uint32(b[:4])
		if size < 8 || uint64(size) > uint64(len(b)) {
			return
		}
		fn(string(b[4:8]), b[8:size])
		b = b[size:]
	}
}

// readSmall reads at most limit bytes of a box's payload.
func (p *mp4Parser) readSmall(box mp4BoxHeader, limit int64) ([]byte, error) {
	return readAt(p.r, box.payloadStart, min64(box.end-box.payloadStart, limit))
}

func (p *mp4Parser) walk(start, end int64, depth int, visit func(mp4BoxHeader) error) error {
	if depth > maxDepth {
		return overLimit("inspect mp4: nesting depth exceeded")
	}
	if start < 0 || end < start || end > p.size {
		return malformed("inspect mp4: invalid container range")
	}
	for position := start; position < end; {
		p.elements++
		if p.elements > maxElements {
			return overLimit("inspect mp4: element budget exceeded")
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
		return mp4BoxHeader{}, malformed("inspect mp4: truncated box header")
	}
	header, err := readAt(p.r, position, 8)
	if err != nil {
		return mp4BoxHeader{}, err
	}
	size := int64(binary.BigEndian.Uint32(header[:4]))
	headerSize := int64(8)
	if size == 1 {
		if position > parentEnd-16 {
			return mp4BoxHeader{}, malformed("inspect mp4: truncated extended box")
		}
		extended, err := readAt(p.r, position+8, 8)
		if err != nil {
			return mp4BoxHeader{}, err
		}
		value := binary.BigEndian.Uint64(extended)
		if value > uint64(^uint64(0)>>1) {
			return mp4BoxHeader{}, malformed("inspect mp4: box size overflow")
		}
		size = int64(value)
		headerSize = 16
	} else if size == 0 {
		size = parentEnd - position
	}
	if size < headerSize || size > parentEnd-position {
		return mp4BoxHeader{}, malformed("inspect mp4: invalid box size")
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
	case "avc1", "avc3", "dva1", "dvav":
		return "h264"
	case "hvc1", "hev1", "dvh1", "dvhe":
		return "hevc"
	case "av01", "dav1":
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

// normalizeAudioCodec names an MP4 audio sample entry (an .m4a holds
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
