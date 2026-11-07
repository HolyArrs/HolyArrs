// Package qr draws QR codes, so two-factor setup can show one to scan.
// It covers what HolyArrs needs: byte mode, medium error correction, and
// versions 1 to 10 (up to 213 bytes, plenty for an otpauth:// link).
package qr

import "errors"

// ErrTooLong is returned for text longer than a version 10 code holds.
var ErrTooLong = errors.New("qr: text too long")

// Per version 1..10 at medium error correction: all codewords, blocks, and
// error-correction codewords per block.
var (
	totalCodewords = [11]int{0, 26, 44, 70, 100, 134, 172, 196, 242, 292, 346}
	blocks         = [11]int{0, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5}
	eccPerBlock    = [11]int{0, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26}
	alignAt        = [11][]int{nil, nil, {6, 18}, {6, 22}, {6, 26}, {6, 30}, {6, 34}, {6, 22, 38}, {6, 24, 42}, {6, 26, 46}, {6, 28, 50}}
)

func dataCodewords(v int) int { return totalCodewords[v] - blocks[v]*eccPerBlock[v] }

// Encode returns the code's modules, row by row; true is dark. The caller
// adds the quiet zone (4 modules) around it.
func Encode(text string) ([][]bool, error) {
	data := []byte(text)
	v := 1
	for ; v <= 10; v++ {
		count := 8
		if v >= 10 {
			count = 16
		}
		if 4+count+8*len(data) <= dataCodewords(v)*8 {
			break
		}
	}
	if v > 10 {
		return nil, ErrTooLong
	}
	c := &code{v: v, size: 17 + 4*v}
	c.dark = grid(c.size)
	c.fixed = grid(c.size)
	c.drawFunctions()
	c.drawData(c.withECC(c.bits(data)))
	best, bestScore := 0, -1
	for m := 0; m < 8; m++ {
		c.mask(m)
		c.drawFormat(m)
		if s := c.penalty(); bestScore < 0 || s < bestScore {
			best, bestScore = m, s
		}
		c.mask(m) // undo: a mask is its own inverse
	}
	c.mask(best)
	c.drawFormat(best)
	return c.dark, nil
}

type code struct {
	v, size int
	dark    [][]bool // dark[y][x]
	fixed   [][]bool // finder, timing, alignment, format and version modules
}

func grid(n int) [][]bool {
	g := make([][]bool, n)
	for i := range g {
		g[i] = make([]bool, n)
	}
	return g
}

func (c *code) set(x, y int, dark bool) {
	c.dark[y][x] = dark
	c.fixed[y][x] = true
}

func (c *code) drawFunctions() {
	for i := 0; i < c.size; i++ {
		c.set(6, i, i%2 == 0)
		c.set(i, 6, i%2 == 0)
	}
	c.finder(3, 3)
	c.finder(c.size-4, 3)
	c.finder(3, c.size-4)
	pos := alignAt[c.v]
	last := len(pos) - 1
	for i := range pos {
		for j := range pos {
			if (i == 0 && j == 0) || (i == 0 && j == last) || (i == last && j == 0) {
				continue // under a finder
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					c.set(pos[i]+dx, pos[j]+dy, max(abs(dx), abs(dy)) != 1)
				}
			}
		}
	}
	c.drawFormat(0) // reserves the format modules; redrawn per mask
	if c.v >= 7 {
		rem := c.v
		for i := 0; i < 12; i++ {
			rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
		}
		bits := c.v<<12 | rem
		for i := 0; i < 18; i++ {
			on := (bits>>i)&1 == 1
			a, b := c.size-11+i%3, i/3
			c.set(a, b, on)
			c.set(b, a, on)
		}
	}
}

func (c *code) finder(cx, cy int) {
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			x, y := cx+dx, cy+dy
			if x < 0 || y < 0 || x >= c.size || y >= c.size {
				continue
			}
			d := max(abs(dx), abs(dy))
			c.set(x, y, d != 2 && d != 4)
		}
	}
}

// drawFormat writes the 15 format bits (medium = 00, then the mask) twice.
func (c *code) drawFormat(mask int) {
	data := mask // medium error correction is 00
	rem := data
	for i := 0; i < 10; i++ {
		rem = (rem << 1) ^ ((rem >> 9) * 0x537)
	}
	bits := (data<<10 | rem) ^ 0x5412
	bit := func(i int) bool { return (bits>>i)&1 == 1 }
	for i := 0; i <= 5; i++ {
		c.set(8, i, bit(i))
	}
	c.set(8, 7, bit(6))
	c.set(8, 8, bit(7))
	c.set(7, 8, bit(8))
	for i := 9; i < 15; i++ {
		c.set(14-i, 8, bit(i))
	}
	for i := 0; i < 8; i++ {
		c.set(c.size-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		c.set(8, c.size-15+i, bit(i))
	}
	c.set(8, c.size-8, true) // the dark module
}

// bits makes the data codewords: mode, length, bytes, terminator, padding.
func (c *code) bits(data []byte) []byte {
	var out []byte
	var acc, n int
	put := func(val, width int) {
		for i := width - 1; i >= 0; i-- {
			acc = acc<<1 | (val>>i)&1
			if n++; n == 8 {
				out = append(out, byte(acc))
				acc, n = 0, 0
			}
		}
	}
	count := 8
	if c.v >= 10 {
		count = 16
	}
	put(0b0100, 4)
	put(len(data), count)
	for _, b := range data {
		put(int(b), 8)
	}
	room := dataCodewords(c.v)
	put(0, min(4, room*8-len(out)*8-n))
	if n > 0 {
		put(0, 8-n)
	}
	for pad := 0; len(out) < room; pad++ {
		out = append(out, [2]byte{0xEC, 0x11}[pad%2])
	}
	return out
}

// withECC splits the data into blocks, adds Reed-Solomon codewords to each,
// and interleaves them.
func (c *code) withECC(data []byte) []byte {
	nb, ecc := blocks[c.v], eccPerBlock[c.v]
	short := totalCodewords[c.v] / nb
	numShort := nb - totalCodewords[c.v]%nb
	div := rsDivisor(ecc)
	var parts, eccs [][]byte
	for i, at := 0, 0; i < nb; i++ {
		n := short - ecc
		if i >= numShort {
			n++
		}
		parts = append(parts, data[at:at+n])
		eccs = append(eccs, rsRemainder(data[at:at+n], div))
		at += n
	}
	var out []byte
	for i := 0; i <= short-ecc; i++ {
		for _, p := range parts {
			if i < len(p) {
				out = append(out, p[i])
			}
		}
	}
	for i := 0; i < ecc; i++ {
		for _, e := range eccs {
			out = append(out, e[i])
		}
	}
	return out
}

// drawData places the codewords in the zigzag order, two columns at a time.
func (c *code) drawData(words []byte) {
	i := 0
	for right := c.size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5 // skip the vertical timing column
		}
		for vert := 0; vert < c.size; vert++ {
			for j := 0; j < 2; j++ {
				x := right - j
				y := vert
				if (right+1)&2 == 0 {
					y = c.size - 1 - vert // upward
				}
				if !c.fixed[y][x] && i < len(words)*8 {
					c.dark[y][x] = (words[i>>3]>>(7-i&7))&1 == 1
					i++
				}
			}
		}
	}
}

func (c *code) mask(m int) {
	for y := 0; y < c.size; y++ {
		for x := 0; x < c.size; x++ {
			if c.fixed[y][x] {
				continue
			}
			var flip bool
			switch m {
			case 0:
				flip = (x+y)%2 == 0
			case 1:
				flip = y%2 == 0
			case 2:
				flip = x%3 == 0
			case 3:
				flip = (x+y)%3 == 0
			case 4:
				flip = (x/3+y/2)%2 == 0
			case 5:
				flip = x*y%2+x*y%3 == 0
			case 6:
				flip = (x*y%2+x*y%3)%2 == 0
			case 7:
				flip = ((x+y)%2+x*y%3)%2 == 0
			}
			if flip {
				c.dark[y][x] = !c.dark[y][x]
			}
		}
	}
}

// penalty scores a masked code by the standard's four rules; lower reads better.
func (c *code) penalty() int {
	n := c.size
	at := func(x, y int, rows bool) bool {
		if rows {
			return c.dark[y][x]
		}
		return c.dark[x][y]
	}
	score := 0
	for _, rows := range []bool{true, false} {
		for y := 0; y < n; y++ {
			run := 1
			for x := 1; x <= n; x++ {
				if x < n && at(x, y, rows) == at(x-1, y, rows) {
					run++
					continue
				}
				if run >= 5 {
					score += 3 + run - 5
				}
				run = 1
			}
			// 1:1:3:1:1 with four light modules on one side.
			for x := 0; x+11 <= n; x++ {
				a := [11]bool{}
				for k := range a {
					a[k] = at(x+k, y, rows)
				}
				p1 := a[0] && !a[1] && a[2] && a[3] && a[4] && !a[5] && a[6] && !a[7] && !a[8] && !a[9] && !a[10]
				p2 := !a[0] && !a[1] && !a[2] && !a[3] && a[4] && !a[5] && a[6] && a[7] && a[8] && !a[9] && a[10]
				if p1 || p2 {
					score += 40
				}
			}
		}
	}
	dark := 0
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if c.dark[y][x] {
				dark++
			}
			if x+1 < n && y+1 < n {
				d := c.dark[y][x]
				if c.dark[y][x+1] == d && c.dark[y+1][x] == d && c.dark[y+1][x+1] == d {
					score += 3
				}
			}
		}
	}
	total := n * n
	k := (abs(dark*20-total*10)+total-1)/total - 1
	return score + k*10
}

func rsDivisor(degree int) []byte {
	out := make([]byte, degree)
	out[degree-1] = 1
	root := byte(1)
	for i := 0; i < degree; i++ {
		for j := range out {
			out[j] = gfMul(out[j], root)
			if j+1 < len(out) {
				out[j] ^= out[j+1]
			}
		}
		root = gfMul(root, 2)
	}
	return out
}

func rsRemainder(data, div []byte) []byte {
	out := make([]byte, len(div))
	for _, b := range data {
		f := b ^ out[0]
		copy(out, out[1:])
		out[len(out)-1] = 0
		for i := range out {
			out[i] ^= gfMul(div[i], f)
		}
	}
	return out
}

func gfMul(x, y byte) byte {
	z := 0
	for i := 7; i >= 0; i-- {
		z = (z << 1) ^ ((z >> 7) * 0x11D)
		z ^= int(y>>i&1) * int(x)
	}
	return byte(z)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
