package mediafiles

// se reads a signed Exp-Golomb number.
func (r *bitReader) se() int {
	v := r.ue()
	if v&1 == 1 {
		return (v + 1) / 2
	}
	return -v / 2
}

// avcSPS reads the picture size, bit depth and frame rate from an H.264
// sequence parameter set (ITU-T H.264 7.3.2.1.1) without its NAL header;
// zeros for anything it cannot read.
func avcSPS(b []byte) (width, height, depth int, fps float64) {
	r := &bitReader{b: b}
	profile := r.u(8)
	r.skip(16) // constraints, level
	r.ue()     // sps id
	chroma, depth := 1, 8
	switch profile {
	case 100, 110, 122, 244, 44, 83, 86, 118, 128, 138, 139, 134, 135:
		chroma = r.ue()
		if chroma == 3 {
			r.skip(1)
		}
		depth = r.ue() + 8
		r.ue()
		r.skip(1)
		if r.flag() { // scaling matrices
			lists := 8
			if chroma == 3 {
				lists = 12
			}
			for i := 0; i < lists && !r.bad; i++ {
				if !r.flag() {
					continue
				}
				size, last, next := 16, 8, 8
				if i >= 6 {
					size = 64
				}
				for j := 0; j < size && !r.bad; j++ {
					if next != 0 {
						next = (last + r.se() + 256) % 256
					}
					if next != 0 {
						last = next
					}
				}
			}
		}
	}
	r.ue() // log2 max frame num
	switch r.ue() {
	case 0:
		r.ue()
	case 1:
		r.skip(1)
		r.se()
		r.se()
		n := r.ue()
		if n > 255 {
			return 0, 0, 0, 0
		}
		for range n {
			r.se()
		}
	}
	r.ue()
	r.skip(1)
	mbW, mbH := r.ue()+1, r.ue()+1
	frameOnly := r.flag()
	if !frameOnly {
		r.skip(1)
	}
	r.skip(1)
	field := 2
	if frameOnly {
		field = 1
	}
	width, height = mbW*16, mbH*16*field
	if r.flag() { // cropping
		cx, cy := 2, 2*field
		switch chroma {
		case 0, 3:
			cx, cy = 1, field
		case 2:
			cy = field
		}
		l, rt, t, bt := r.ue(), r.ue(), r.ue(), r.ue()
		width -= cx * (l + rt)
		height -= cy * (t + bt)
	}
	if r.bad || width <= 0 || height <= 0 || width > 1<<14 || height > 1<<14 {
		return 0, 0, 0, 0
	}
	if r.flag() { // VUI
		if r.flag() && r.u(8) == 255 {
			r.skip(32)
		}
		if r.flag() {
			r.skip(1)
		}
		if r.flag() {
			r.skip(4)
			if r.flag() {
				r.skip(24)
			}
		}
		if r.flag() {
			r.ue()
			r.ue()
		}
		if r.flag() {
			units, scale := r.u(32), r.u(32)
			if !r.bad && units > 0 {
				fps = roundRate(float64(scale) / float64(2*units))
			}
		}
	}
	return width, height, depth, fps
}

// hevcSPSSize reads the picture size and bit depth from an HEVC sequence
// parameter set (ITU-T H.265 7.3.2.2) without its 2-byte NAL header.
func hevcSPSSize(b []byte) (width, height, depth int) {
	r := &bitReader{b: b}
	r.skip(4)
	subLayers := r.u(3)
	r.skip(1)
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
	r.ue()
	chroma := r.ue()
	if chroma == 3 {
		r.skip(1)
	}
	width, height = r.ue(), r.ue()
	if r.flag() {
		cx, cy := 2, 2
		switch chroma {
		case 0, 3:
			cx, cy = 1, 1
		case 2:
			cy = 1
		}
		l, rt, t, bt := r.ue(), r.ue(), r.ue(), r.ue()
		width -= cx * (l + rt)
		height -= cy * (t + bt)
	}
	depth = r.ue() + 8
	if r.bad || width <= 0 || height <= 0 || width > 1<<14 || height > 1<<14 || depth > 16 {
		return 0, 0, 0
	}
	return width, height, depth
}
