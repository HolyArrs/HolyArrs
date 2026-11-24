// MPEG transport stream header reading (.ts, .m2ts, .mts): the program
// tables give the streams and their languages, the first video frame's
// sequence parameter set gives the picture size, and the first and last
// time stamps give the length. Reading stops after maxTSHead bytes from the
// start and maxTSTail bytes from the end, read in tsBlock pieces, so a file
// of any size costs at most a few MiB of reads and about one block of memory.

package mediafiles

import (
	"encoding/binary"
	"io"
)

const (
	maxTSHead = 8 << 20
	maxTSTail = 2 << 20
	tsBlock   = 64 << 10
	// maxTSVideoPES is the most bytes of the first video frames kept while
	// looking for the sequence parameter set; maxTSAudioPES the same for the
	// first audio frame.
	maxTSVideoPES = 512 << 10
	maxTSAudioPES = 8 << 10
	maxTSSection  = 4096
	ptsWrap       = int64(1) << 33
)

// tsLayout says whether prefix starts a transport stream: 188-byte packets
// (broadcast .ts), 192-byte ones with a 4-byte time code first (Blu-ray
// .m2ts/.mts) or 204-byte ones with error check bytes after. A file cut
// mid-packet starts with part of one, so start is where whole packets begin.
func tsLayout(prefix []byte) (packet, sync int, start int64, ok bool) {
	for off := 0; off < 204; off++ {
		for _, l := range [][2]int{{188, 0}, {192, 4}, {204, 0}} {
			at := off + l[1]
			if len(prefix) > at+2*l[0] && prefix[at] == 0x47 && prefix[at+l[0]] == 0x47 && prefix[at+2*l[0]] == 0x47 {
				return l[0], l[1], int64(off), true
			}
		}
	}
	return 0, 0, 0, false
}

type tsStream struct {
	pid   int
	track Track
	pes   []byte
	from  int // where the next look for a parameter set starts in pes
	done  bool
}

type tsParser struct {
	packet, sync int
	pmtPID       int
	sections     map[int][]byte
	streams      []*tsStream
	byPID        map[int]*tsStream
	havePMT      bool
	clockPID     int // the stream whose time stamps measure the length
	firstPTS     int64
	lastPTS      int64
	packets      int
}

func inspectTS(r io.ReaderAt, size int64, packet, sync int, start int64) (headers, error) {
	h := headers{container: "mpegts"}
	if packet == 192 {
		h.container = "m2ts"
	}
	p := &tsParser{packet: packet, sync: sync, pmtPID: -1, sections: map[int][]byte{}, byPID: map[int]*tsStream{}, clockPID: -1, firstPTS: -1, lastPTS: -1}
	headEnd := min64(size, start+maxTSHead)
	for pos := start; pos < headEnd && !p.headDone(); {
		n := min64(tsBlock, headEnd-pos)
		b, err := readAt(r, pos, n)
		if err != nil {
			return h, err
		}
		used := p.scan(b, false)
		if used == 0 {
			break
		}
		pos += int64(used)
		if p.packets > maxElements*64 && !p.havePMT {
			break // a long stretch with no program table: not a stream we can name
		}
	}
	if !p.havePMT {
		return h, malformed("inspect %s: program table missing", h.container)
	}
	h.found = true
	if p.clockPID >= 0 && p.firstPTS >= 0 && size > headEnd/2 {
		start := max(0, size-maxTSTail)
		for pos := start; pos < size; {
			n := min64(tsBlock, size-pos)
			b, err := readAt(r, pos, n)
			if err != nil {
				break
			}
			used := p.scan(b, true)
			if used == 0 {
				break
			}
			pos += int64(used)
		}
		if p.lastPTS >= 0 {
			h.duration = seconds(float64((p.lastPTS-p.firstPTS+ptsWrap)%ptsWrap), 90000)
		}
	}
	for _, s := range p.streams {
		h.tracks = append(h.tracks, s.track)
	}
	return h, nil
}

// headDone says the head holds everything it can give.
func (p *tsParser) headDone() bool {
	if !p.havePMT || p.firstPTS < 0 && p.clockPID >= 0 {
		return false
	}
	for _, s := range p.streams {
		if !s.done {
			return false
		}
	}
	return true
}

// scan reads the whole packets in b and returns how many bytes it used;
// tail scans only look for the last time stamp.
func (p *tsParser) scan(b []byte, tail bool) int {
	pos := 0
	for pos+p.packet <= len(b) {
		if b[pos+p.sync] != 0x47 {
			pos++ // lost step: walk to the next sync byte
			continue
		}
		p.packets++
		p.packet188(b[pos+p.sync:pos+p.sync+188], tail)
		pos += p.packet
	}
	if pos == 0 && len(b) >= p.packet {
		return len(b) - p.packet + 1
	}
	return pos
}

func (p *tsParser) packet188(ts []byte, tail bool) {
	start := ts[1]&0x40 != 0
	pid := int(ts[1]&0x1f)<<8 | int(ts[2])
	control := ts[3] >> 4 & 3
	if control&1 == 0 {
		return
	}
	at := 4
	if control&2 != 0 {
		at += 1 + int(ts[4])
	}
	if at >= len(ts) {
		return
	}
	payload := ts[at:]
	if tail {
		if pid == p.clockPID && start {
			if pts, ok := pesPTS(payload); ok {
				p.lastPTS = pts
			}
		}
		return
	}
	switch {
	case pid == 0:
		if sec := p.section(pid, start, payload); sec != nil && p.pmtPID < 0 {
			p.readPAT(sec)
		}
	case pid == p.pmtPID:
		if sec := p.section(pid, start, payload); sec != nil && !p.havePMT {
			p.readPMT(sec)
		}
	default:
		s := p.byPID[pid]
		if s == nil {
			return
		}
		if start {
			pts, ok := pesPTS(payload)
			if ok && pid == p.clockPID && p.firstPTS < 0 {
				p.firstPTS = pts
			}
			if len(payload) < 9 || payload[0] != 0 || payload[1] != 0 || payload[2] != 1 {
				return
			}
			skip := 9 + int(payload[8])
			if skip > len(payload) {
				return
			}
			payload = payload[skip:]
		} else if len(s.pes) == 0 {
			return // the middle of a frame whose start came before
		}
		if !s.done {
			s.pes = append(s.pes, payload...)
			p.readFrame(s)
		}
	}
}

// section gathers one table section of pid; nil until it is whole.
func (p *tsParser) section(pid int, start bool, payload []byte) []byte {
	buf := p.sections[pid]
	if start {
		skip := 1 + int(payload[0])
		if skip > len(payload) {
			return nil
		}
		buf = append(buf[:0], payload[skip:]...)
	} else if len(buf) > 0 {
		buf = append(buf, payload...)
	}
	if len(buf) > maxTSSection {
		buf = buf[:0]
	}
	p.sections[pid] = buf
	if len(buf) < 3 {
		return nil
	}
	n := 3 + int(binary.BigEndian.Uint16(buf[1:])&0xfff)
	if len(buf) < n {
		return nil
	}
	return buf[:n]
}

func (p *tsParser) readPAT(sec []byte) {
	if sec[0] != 0 || len(sec) < 12 {
		return
	}
	for i := 8; i+4 <= len(sec)-4; i += 4 {
		program := binary.BigEndian.Uint16(sec[i:])
		if program != 0 {
			p.pmtPID = int(binary.BigEndian.Uint16(sec[i+2:]) & 0x1fff)
			return
		}
	}
}

func (p *tsParser) readPMT(sec []byte) {
	if sec[0] != 2 || len(sec) < 16 {
		return
	}
	p.havePMT = true
	end := len(sec) - 4 // CRC
	i := 12 + int(binary.BigEndian.Uint16(sec[10:])&0xfff)
	for i+5 <= end && len(p.streams) < maxTracks {
		kind := sec[i]
		pid := int(binary.BigEndian.Uint16(sec[i+1:]) & 0x1fff)
		n := int(binary.BigEndian.Uint16(sec[i+3:]) & 0xfff)
		i += 5
		if i+n > end {
			n = end - i
		}
		t := tsTrack(kind, sec[i:i+n])
		i += n
		if t.Kind == "" || p.byPID[pid] != nil {
			continue
		}
		t.Number = pid
		s := &tsStream{pid: pid, track: t}
		// Only the first frame of each video and of AC-3/E-AC-3/AAC audio
		// adds anything; the rest are known from the table alone.
		s.done = !(t.Kind == "video" && (t.Format == "h264" || t.Format == "hevc") ||
			t.Kind == "audio" && (t.Format == "ac3" || t.Format == "eac3" || t.Format == "aac"))
		p.streams = append(p.streams, s)
		p.byPID[pid] = s
		if t.Kind == "video" && p.clockPID < 0 {
			p.clockPID = pid
		}
	}
	if p.clockPID < 0 {
		for _, s := range p.streams {
			if s.track.Kind == "audio" {
				p.clockPID = s.pid
				break
			}
		}
	}
}

// tsTrack names a stream from its type and descriptors.
func tsTrack(kind byte, desc []byte) Track {
	var t Track
	set := func(k, format, codec string) { t.Kind, t.Format, t.Codec = k, format, codec }
	switch kind {
	case 0x01, 0x02:
		set("video", "mpeg2", "mpeg2video")
	case 0x10:
		set("video", "xvid", "mpeg4video")
	case 0x1b:
		set("video", "h264", "h264")
	case 0x24:
		set("video", "hevc", "hevc")
	case 0xea:
		set("video", "vc1", "vc1")
	case 0x03, 0x04:
		set("audio", "mp2", "mpegaudio")
	case 0x0f, 0x11:
		set("audio", "aac", "aac")
	case 0x80:
		set("audio", "pcm", "lpcm")
	case 0x81:
		set("audio", "ac3", "ac3")
	case 0x84, 0x87, 0xa1:
		set("audio", "eac3", "eac3")
	case 0x82, 0x85, 0x86, 0xa2:
		set("audio", "dts", "dts")
	case 0x83:
		set("audio", "truehd", "truehd")
	case 0x90:
		set("subtitle", "pgs", "S_HDMV/PGS")
	case 0x92:
		set("subtitle", "text", "S_HDMV/TEXTST")
	}
	for i := 0; i+2 <= len(desc); {
		tag, n := desc[i], int(desc[i+1])
		i += 2
		if i+n > len(desc) {
			break
		}
		body := desc[i : i+n]
		i += n
		switch tag {
		case 0x0a: // language
			if len(body) >= 4 {
				t.Language = cut(body[:3])
				t.HearingImpaired = body[3] == 2
			}
		case 0x59: // DVB subtitles
			if kind == 0x06 {
				set("subtitle", "dvbsub", "S_DVBSUB")
			}
			if len(body) >= 4 {
				t.Language = cut(body[:3])
				t.HearingImpaired = body[3] >= 0x20 && body[3] <= 0x24
			}
		case 0x6a:
			if kind == 0x06 {
				set("audio", "ac3", "ac3")
			}
		case 0x7a:
			if kind == 0x06 {
				set("audio", "eac3", "eac3")
			}
		case 0x7b:
			if kind == 0x06 {
				set("audio", "dts", "dts")
			}
		case 0x05: // registration
			if kind == 0x06 && len(body) >= 4 && t.Kind == "" {
				switch string(body[:4]) {
				case "AC-3":
					set("audio", "ac3", "ac3")
				case "EAC3":
					set("audio", "eac3", "eac3")
				case "DTS1", "DTS2", "DTS3":
					set("audio", "dts", "dts")
				case "Opus":
					set("audio", "opus", "opus")
				}
			}
		}
	}
	return t
}

// pesPTS reads the presentation time stamp at the start of a PES packet.
func pesPTS(b []byte) (int64, bool) {
	if len(b) < 14 || b[0] != 0 || b[1] != 0 || b[2] != 1 || b[7]&0x80 == 0 || b[6]&0xc0 != 0x80 {
		return 0, false
	}
	pts := int64(b[9]>>1&7)<<30 | int64(b[10])<<22 | int64(b[11]>>1)<<15 | int64(b[12])<<7 | int64(b[13]>>1)
	return pts, true
}

// readFrame looks for the sequence parameter set (video) or the first
// frame header (audio) in what the stream gathered so far.
func (p *tsParser) readFrame(s *tsStream) {
	t := &s.track
	if t.Kind == "audio" {
		if ch := audioFrameChannels(t.Format, s.pes); ch > 0 {
			t.Channels = ch
			s.done = true
		}
		if len(s.pes) >= maxTSAudioPES {
			s.done = true
		}
	} else {
		nal, whole, at := findSPS(t.Format, s.pes, s.from)
		s.from = at
		if nal != nil {
			if whole || len(s.pes) >= maxTSVideoPES {
				readSPS(t, nal)
				s.done = true
			}
		}
		if len(s.pes) >= maxTSVideoPES {
			s.done = true
		}
	}
	if s.done {
		s.pes = nil
	}
}

// findSPS finds the first sequence parameter set NAL unit (without its
// start code) in an Annex B byte stream; whole says the next start code
// was seen, so the unit is complete.
// It looks from index from on and returns where the next look should start,
// so a stream gathered packet by packet is read once, not again each time.
func findSPS(format string, b []byte, from int) (nal []byte, whole bool, next int) {
	i := max(0, from)
	for ; i+3 < len(b); i++ {
		if b[i] != 0 || b[i+1] != 0 || b[i+2] != 1 {
			continue
		}
		head := b[i+3]
		sps := format == "h264" && head&0x1f == 7 || format == "hevc" && head>>1&0x3f == 33
		if !sps {
			continue
		}
		rest := b[i+3:]
		for j := 1; j+2 < len(rest); j++ {
			if rest[j] == 0 && rest[j+1] == 0 && (rest[j+2] == 1 || rest[j+2] == 0) {
				return rest[:j], true, i
			}
		}
		return rest, false, i
	}
	return nil, false, i
}

func readSPS(t *Track, nal []byte) {
	if len(nal) > maxPrivate {
		nal = nal[:maxPrivate]
	}
	switch t.Format {
	case "h264":
		t.Width, t.Height, t.BitDepth, t.FrameRate = avcSPS(unescape(nal[1:]))
	case "hevc":
		if len(nal) < 3 {
			return
		}
		body := unescape(nal[2:])
		t.Width, t.Height, t.BitDepth = hevcSPSSize(body)
		t.HDR = colour{transfer: spsTransfer(body), dvCompat: -1}.hdrName()
	}
}

// audioFrameChannels reads the channel count from the first AC-3, E-AC-3
// or ADTS AAC frame header in b; 0 until one is found.
func audioFrameChannels(format string, b []byte) int {
	for i := 0; i+8 <= len(b); i++ {
		switch {
		case (format == "ac3" || format == "eac3") && b[i] == 0x0b && b[i+1] == 0x77:
			if ch := ac3FrameChannels(b[i:]); ch > 0 {
				return ch
			}
		case format == "aac" && b[i] == 0xff && b[i+1]&0xf6 == 0xf0:
			if c := int(b[i+2]&1)<<2 | int(b[i+3]>>6); c > 0 {
				if c == 7 {
					return 8
				}
				return c
			}
		}
	}
	return 0
}

// ac3FrameChannels reads an AC-3 or E-AC-3 sync frame header.
func ac3FrameChannels(b []byte) int {
	if len(b) < 8 {
		return 0
	}
	modes := [8]int{2, 1, 2, 3, 3, 4, 4, 5}
	bsid := b[5] >> 3
	if bsid > 10 && bsid <= 16 { // E-AC-3
		return modes[b[4]>>1&7] + int(b[4]&1)
	}
	if bsid > 10 {
		return 0
	}
	r := &bitReader{b: b[6:]}
	mode := r.u(3)
	if mode&1 != 0 && mode != 1 {
		r.skip(2)
	}
	if mode&4 != 0 {
		r.skip(2)
	}
	if mode == 2 {
		r.skip(2)
	}
	lfe := r.u(1)
	if r.bad {
		return 0
	}
	return modes[mode] + lfe
}
