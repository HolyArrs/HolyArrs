package mediafiles

import "encoding/binary"

// Colour transfer characteristics (ITU-T H.273) that mark HDR video.
const (
	transferPQ  = 16
	transferHLG = 18
)

// colour collects what a track's headers say about its picture.
type colour struct {
	transfer  int  // H.273 transfer characteristics; 0 when not written
	mastering bool // mastering display or content light levels are written
	dv        bool // a Dolby Vision setup is written
	dvCompat  int  // Dolby Vision base layer compatibility id; -1 when not read
}

// hdrName names the HDR kind: "", "HDR10", "HLG", "DV" or "DV+HDR10".
func (c colour) hdrName() string {
	pq := c.transfer == transferPQ
	unknown := c.transfer == 0 || c.transfer == 2 // 2 is "unspecified"
	if c.dv {
		// Profile 8.1 (id 1) and profile 7 (id 6) carry an HDR10 base layer.
		if pq || c.dvCompat == 1 || c.dvCompat == 6 {
			return "DV+HDR10"
		}
		return "DV"
	}
	switch {
	case pq, c.mastering && unknown:
		return "HDR10"
	case c.transfer == transferHLG:
		return "HLG"
	}
	return ""
}

// readDolbyVision reads a Dolby Vision configuration record (dvcC, dvvC,
// dvwC) and marks c.
func (c *colour) readDolbyVision(b []byte) {
	c.dv = true
	if len(b) >= 5 {
		c.dvCompat = int(b[4] >> 4)
	}
}

// readColr reads an MP4 colr box payload (nclx or QuickTime nclc).
func (c *colour) readColr(b []byte) {
	if len(b) >= 8 && (string(b[:4]) == "nclx" || string(b[:4]) == "nclc") {
		if t := int(binary.BigEndian.Uint16(b[6:8])); t != 0 && t != 2 {
			c.transfer = t
		}
	}
}

// configBitDepth reads the luma bit depth from an hvcC, av1C or avcC
// record; 0 when the record does not say.
func configBitDepth(format string, b []byte) int {
	switch format {
	case "hevc":
		if len(b) >= 23 && b[0] == 1 {
			return int(b[17]&7) + 8
		}
	case "av1":
		if len(b) >= 4 && b[0]&0x7f == 1 {
			switch {
			case b[2]&0x40 == 0:
				return 8
			case b[2]&0x20 != 0:
				return 12
			default:
				return 10
			}
		}
	case "h264":
		return avcBitDepth(b)
	}
	return 0
}

// avcBitDepth reads the bit depth an avcC record adds for the High profiles
// after its parameter sets; plain profiles are 8 bit.
func avcBitDepth(b []byte) int {
	if len(b) < 7 || b[0] != 1 {
		return 0
	}
	switch b[1] {
	case 100, 110, 122, 144:
	default:
		return 8
	}
	pos := 6
	for range int(b[5] & 0x1f) {
		if pos+2 > len(b) {
			return 0
		}
		pos += 2 + int(binary.BigEndian.Uint16(b[pos:]))
	}
	if pos >= len(b) {
		return 0
	}
	count := int(b[pos])
	pos++
	for range count {
		if pos+2 > len(b) {
			return 0
		}
		pos += 2 + int(binary.BigEndian.Uint16(b[pos:]))
	}
	if pos+2 > len(b) {
		return 0
	}
	return int(b[pos+1]&7) + 8
}

// ac3Channels counts the channels a dac3 (AC-3) or dec3 (E-AC-3) box
// describes; 0 when it cannot tell. The sample entry of these codecs always
// says 2.
func ac3Channels(kind string, b []byte) int {
	modes := [8]int{2, 1, 2, 3, 3, 4, 4, 5}
	switch kind {
	case "dac3":
		if len(b) < 3 {
			return 0
		}
		return modes[(b[1]>>3)&7] + int((b[1]>>2)&1)
	case "dec3":
		// The first independent substream; a dependent one adds the
		// speakers named in chan_loc (7.1 is 5.1 plus a back pair).
		if len(b) < 5 {
			return 0
		}
		n := modes[(b[3]>>1)&7] + int(b[3]&1)
		if (b[4]>>1)&0xf > 0 && len(b) >= 6 {
			loc := int(b[4]&1)<<8 | int(b[5])
			pairs := [9]int{2, 2, 1, 1, 2, 2, 2, 1, 1} // from the top bit down
			for i, add := range pairs {
				if loc&(1<<(8-i)) != 0 {
					n += add
				}
			}
		}
		return n
	}
	return 0
}

// aacChannels reads the channel count from an esds box (MPEG-4 Systems
// descriptors) holding an AAC AudioSpecificConfig; 0 when it cannot tell.
// The mp4a sample entry often just says 2 whatever the real count is.
func aacChannels(b []byte) int {
	if len(b) < 4 {
		return 0
	}
	b = b[4:] // version and flags
	// descriptor reads one tag and its 1-4 byte size (7 bits per byte).
	descriptor := func() (byte, []byte) {
		if len(b) < 2 {
			return 0, nil
		}
		tag, size, i := b[0], 0, 1
		for more := true; more; i++ {
			if i > 4 || i >= len(b) {
				return 0, nil
			}
			size = size<<7 | int(b[i]&0x7f)
			more = b[i]&0x80 != 0
		}
		if size > len(b)-i {
			return 0, nil
		}
		body := b[i : i+size]
		b = b[i+size:]
		return tag, body
	}
	tag, body := descriptor()
	if tag != 0x03 || len(body) < 3 {
		return 0
	}
	flags := body[2]
	skip := 3
	if flags&0x80 != 0 {
		skip += 2
	}
	if flags&0x40 != 0 && len(body) > skip {
		skip += 1 + int(body[skip])
	}
	if flags&0x20 != 0 {
		skip += 2
	}
	if skip > len(body) {
		return 0
	}
	b = body[skip:]
	if tag, body = descriptor(); tag != 0x04 || len(body) < 13 {
		return 0
	}
	b = body[13:]
	if tag, body = descriptor(); tag != 0x05 {
		return 0
	}
	r := &bitReader{b: body}
	if r.u(5) == 31 {
		r.skip(6)
	}
	if r.u(4) == 15 {
		r.skip(24)
	}
	config := r.u(4)
	if r.bad {
		return 0
	}
	switch {
	case config >= 1 && config <= 6:
		return config
	case config == 7, config == 12:
		return 8
	case config == 11:
		return 7
	}
	return 0
}

// configTransfer reads the colour transfer from the codec setup (an hvcC
// SPS or an av1C sequence header), for files whose container does not
// repeat it; 0 when it cannot tell.
func configTransfer(format string, b []byte) int {
	switch format {
	case "hevc":
		return hevcTransfer(b)
	case "av1":
		return av1Transfer(b)
	}
	return 0
}

// av1Transfer reads the colour transfer from the sequence header OBU that
// follows the 4-byte av1C record (AV1 spec 5.5).
func av1Transfer(b []byte) int {
	if len(b) < 4 || b[0]&0x7f != 1 {
		return 0
	}
	b = b[4:]
	for len(b) > 0 {
		head := b[0]
		kind, pos := head>>3&0xf, 1
		if head&4 != 0 { // extension byte
			pos++
		}
		size := len(b) - pos
		if head&2 != 0 { // leb128 size
			size = 0
			for i := 0; ; i++ {
				if i == 8 || pos >= len(b) {
					return 0
				}
				size |= int(b[pos]&0x7f) << (7 * i)
				pos++
				if b[pos-1]&0x80 == 0 {
					break
				}
			}
		}
		if pos > len(b) || size < 0 || size > len(b)-pos {
			return 0
		}
		if kind == 1 {
			return av1SequenceTransfer(b[pos : pos+size])
		}
		b = b[pos+size:]
	}
	return 0
}

// av1SequenceTransfer walks a sequence header to its colour description.
func av1SequenceTransfer(b []byte) int {
	r := &bitReader{b: b}
	profile := r.u(3)
	r.skip(1) // still picture
	reduced := r.flag()
	orderHint := false
	if reduced {
		r.skip(5)
	} else {
		delayBits := 0
		decoderModel := false
		if r.flag() { // timing info
			r.skip(64)
			if r.flag() { // equal picture interval: uvlc
				zeros := 0
				for !r.bad && r.u(1) == 0 {
					zeros++
				}
				if zeros < 32 {
					r.skip(zeros)
				}
			}
			if decoderModel = r.flag(); decoderModel {
				delayBits = r.u(5) + 1
				r.skip(32 + 5 + 5)
			}
		}
		displayDelay := r.flag()
		for range r.u(5) + 1 {
			if r.bad {
				return 0
			}
			r.skip(12)
			if r.u(5) > 7 {
				r.skip(1)
			}
			if decoderModel && r.flag() {
				r.skip(2*delayBits + 1)
			}
			if displayDelay && r.flag() {
				r.skip(4)
			}
		}
	}
	widthBits, heightBits := r.u(4)+1, r.u(4)+1
	r.skip(widthBits + heightBits)
	if !reduced && r.flag() { // frame id numbers
		r.skip(7)
	}
	r.skip(3) // 128x128, filter intra, intra edge
	if !reduced {
		r.skip(4) // interintra, masked, warped, dual filter
		if orderHint = r.flag(); orderHint {
			r.skip(2)
		}
		screen := 2
		if !r.flag() {
			screen = r.u(1)
		}
		if screen > 0 && !r.flag() {
			r.skip(1)
		}
		if orderHint {
			r.skip(3)
		}
	}
	r.skip(3)                     // superres, cdef, restoration
	if r.flag() && profile == 2 { // high bit depth, then twelve bit
		r.skip(1)
	}
	if profile != 1 {
		r.skip(1) // mono chrome
	}
	if !r.flag() { // no colour description
		return 0
	}
	r.skip(8)
	t := r.u(8)
	if r.bad || t == 2 {
		return 0
	}
	return t
}

// hevcTransfer reads the colour transfer from the first sequence parameter
// set in an hvcC record, for files whose container does not repeat it. It
// walks the SPS with a small own bit reader up to the VUI colour fields and
// gives 0 when anything is missing or odd.
func hevcTransfer(b []byte) int {
	if len(b) < 23 || b[0] != 1 {
		return 0
	}
	pos := 23
	for range int(b[22]) {
		if pos+3 > len(b) {
			return 0
		}
		nalType := b[pos] & 0x3f
		count := int(binary.BigEndian.Uint16(b[pos+1:]))
		pos += 3
		for range count {
			if pos+2 > len(b) {
				return 0
			}
			n := int(binary.BigEndian.Uint16(b[pos:]))
			pos += 2
			if pos+n > len(b) {
				return 0
			}
			if nalType == 33 && n > 2 {
				return spsTransfer(unescape(b[pos+2 : pos+n]))
			}
			pos += n
		}
	}
	return 0
}

// unescape drops the emulation prevention bytes (00 00 03 -> 00 00).
func unescape(b []byte) []byte {
	out := make([]byte, 0, len(b))
	zeros := 0
	for _, x := range b {
		if zeros >= 2 && x == 3 {
			zeros = 0
			continue
		}
		if x == 0 {
			zeros++
		} else {
			zeros = 0
		}
		out = append(out, x)
	}
	return out
}

type bitReader struct {
	b   []byte
	pos int // in bits
	bad bool
}

func (r *bitReader) u(n int) int {
	v := 0
	for range n {
		if r.pos < 0 || r.pos >= len(r.b)*8 {
			r.bad = true
			return 0
		}
		v = v<<1 | int(r.b[r.pos/8]>>(7-r.pos%8)&1)
		r.pos++
	}
	return v
}

func (r *bitReader) flag() bool { return r.u(1) == 1 }

// ue reads an Exp-Golomb number. It stops at 30 leading zeros so the result
// always fits a 32-bit int and is never negative.
func (r *bitReader) ue() int {
	zeros := 0
	for !r.bad && r.u(1) == 0 {
		zeros++
		if zeros > 30 {
			r.bad = true
			return 0
		}
	}
	return (1 << zeros) - 1 + r.u(zeros)
}

func (r *bitReader) skip(n int) {
	if n < 0 || r.pos > len(r.b)*8-n {
		r.bad = true
		return
	}
	r.pos += n
}

// spsTransfer walks an HEVC SPS (ITU-T H.265 7.3.2.2) to the VUI colour
// description.
func spsTransfer(b []byte) int {
	r := &bitReader{b: b}
	r.skip(4)
	subLayers := r.u(3)
	r.skip(1)
	// profile_tier_level
	r.skip(96)
	profile, level := make([]bool, subLayers), make([]bool, subLayers)
	for i := range subLayers {
		profile[i], level[i] = r.flag(), r.flag()
	}
	if subLayers > 0 {
		r.skip(2 * (8 - subLayers))
	}
	for i := range subLayers {
		if profile[i] {
			r.skip(88)
		}
		if level[i] {
			r.skip(8)
		}
	}
	r.ue() // sps id
	if r.ue() == 3 {
		r.skip(1)
	}
	r.ue() // width
	r.ue() // height
	if r.flag() {
		r.ue()
		r.ue()
		r.ue()
		r.ue()
	}
	r.ue() // bit depth luma
	r.ue() // bit depth chroma
	pocBits := r.ue() + 4
	first := subLayers
	if r.flag() {
		first = 0
	}
	for i := first; i <= subLayers && !r.bad; i++ {
		r.ue()
		r.ue()
		r.ue()
	}
	for range 6 {
		r.ue()
	}
	if r.flag() && r.flag() { // scaling lists present
		for size := 0; size < 4 && !r.bad; size++ {
			step := 1
			if size == 3 {
				step = 3
			}
			for m := 0; m < 6; m += step {
				if !r.flag() {
					r.ue()
					continue
				}
				coefs := min(64, 1<<(4+size<<1))
				if size > 1 {
					r.ue()
				}
				for range coefs {
					r.ue()
				}
			}
		}
	}
	r.skip(2) // amp, sao
	if r.flag() {
		r.skip(8)
		r.ue()
		r.ue()
		r.skip(1)
	}
	sets := r.ue()
	if sets < 0 || sets > 64 || r.bad {
		return 0
	}
	deltas := make([]int, sets)
	for i := range sets {
		if i > 0 && r.flag() { // predicted from the set before
			r.skip(1)
			r.ue()
			n := 0
			for range deltas[i-1] + 1 {
				used := r.flag()
				if used || r.flag() {
					n++
				}
			}
			deltas[i] = n
		} else {
			neg, pos := r.ue(), r.ue()
			if neg < 0 || pos < 0 || neg > 16 || pos > 16 {
				return 0
			}
			for range neg + pos {
				r.ue()
				r.skip(1)
			}
			deltas[i] = neg + pos
		}
		if r.bad {
			return 0
		}
	}
	if r.flag() {
		n := r.ue()
		if n < 0 || n > 32 || pocBits < 4 || pocBits > 36 {
			return 0
		}
		r.skip(n * (pocBits + 1))
	}
	r.skip(2)      // temporal mvp, strong intra smoothing
	if !r.flag() { // no VUI
		return 0
	}
	if r.flag() && r.u(8) == 255 {
		r.skip(32)
	}
	if r.flag() {
		r.skip(1)
	}
	if !r.flag() { // no video signal type
		return 0
	}
	r.skip(4)
	if !r.flag() { // no colour description
		return 0
	}
	r.skip(8)
	t := r.u(8)
	if r.bad || t == 2 {
		return 0
	}
	return t
}
