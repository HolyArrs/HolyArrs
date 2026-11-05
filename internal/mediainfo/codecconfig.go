package mediainfo

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
		if r.pos >= len(r.b)*8 {
			r.bad = true
			return 0
		}
		v = v<<1 | int(r.b[r.pos/8]>>(7-r.pos%8)&1)
		r.pos++
	}
	return v
}

func (r *bitReader) flag() bool { return r.u(1) == 1 }

// ue reads an Exp-Golomb number.
func (r *bitReader) ue() int {
	zeros := 0
	for !r.bad && r.u(1) == 0 {
		zeros++
		if zeros > 31 {
			r.bad = true
			return 0
		}
	}
	return (1 << zeros) - 1 + r.u(zeros)
}

func (r *bitReader) skip(n int) { r.pos += n }

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
	if sets > 64 || r.bad {
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
			if neg > 16 || pos > 16 {
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
		if n > 32 {
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
