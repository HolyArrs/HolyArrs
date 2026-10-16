package container

import (
	"encoding/binary"
	"io"
	"strings"
)

// box is one MP4 box header: its type, where its data starts and ends.
type box struct {
	typ       string
	data, end int64
}

// readBox reads the box header at pos; end is the end of its parent.
func readBox(r io.ReadSeeker, pos, end int64) (box, error) {
	if end-pos < 8 {
		return box{}, ErrMalformed
	}
	if _, err := r.Seek(pos, io.SeekStart); err != nil {
		return box{}, err
	}
	var h [16]byte
	if _, err := io.ReadFull(r, h[:8]); err != nil {
		return box{}, ErrMalformed
	}
	size := int64(binary.BigEndian.Uint32(h[:4]))
	b := box{typ: string(h[4:8]), data: pos + 8}
	switch size {
	case 0: // to the end of the parent
		b.end = end
	case 1: // a 64-bit size follows
		if _, err := io.ReadFull(r, h[8:16]); err != nil {
			return box{}, ErrMalformed
		}
		big := binary.BigEndian.Uint64(h[8:16])
		if big > uint64(end-pos) {
			return box{}, ErrMalformed
		}
		b.data, b.end = pos+16, pos+int64(big)
	default:
		b.end = pos + size
	}
	if b.end < b.data || b.end > end {
		return box{}, ErrMalformed
	}
	return b, nil
}

// mp4 walks the boxes of one file, counting them.
type mp4 struct {
	r     io.ReadSeeker
	count int
}

// children calls fn for every box inside [from, to).
func (m *mp4) children(from, to int64, depth int, fn func(b box) error) error {
	if depth > maxDepth {
		return ErrLimit
	}
	for pos := from; pos < to; {
		if m.count++; m.count > maxElements {
			return ErrLimit
		}
		b, err := readBox(m.r, pos, to)
		if err != nil {
			return err
		}
		if err := fn(b); err != nil {
			return err
		}
		if b.end <= pos {
			return ErrMalformed
		}
		pos = b.end
	}
	return nil
}

func (m *mp4) small(b box) ([]byte, error) {
	n := b.end - b.data
	if n > maxSmallBox {
		n = maxSmallBox
	}
	if _, err := m.r.Seek(b.data, io.SeekStart); err != nil {
		return nil, err
	}
	return readBody(m.r, n, maxSmallBox)
}

func readMP4(r io.ReadSeeker, size int64) ([]Track, error) {
	m := &mp4{r: r}
	var out []Track
	found := false
	err := m.children(0, size, 0, func(b box) error {
		if b.typ != "moov" || found {
			return nil
		}
		found = true
		return m.children(b.data, b.end, 1, func(t box) error {
			if t.typ != "trak" {
				return nil
			}
			tr, sub, err := m.trak(t)
			if err != nil {
				return err
			}
			if sub && len(out) < maxTracks {
				out = append(out, tr)
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrMalformed
	}
	return out, nil
}

// trak reads one track: its id, language, handler, name and sample type.
func (m *mp4) trak(t box) (Track, bool, error) {
	tr := Track{Language: "und"}
	handler, entry := "", ""
	elng := ""
	enabled := true
	err := m.children(t.data, t.end, 2, func(b box) error {
		switch b.typ {
		case "tkhd":
			body, err := m.small(b)
			if err != nil {
				return err
			}
			if len(body) >= 24 {
				enabled = body[3]&1 == 1
				if body[0] == 1 && len(body) >= 24 {
					tr.Index = int(binary.BigEndian.Uint32(body[20:24]) & 0x7FFFFFFF)
				} else if len(body) >= 16 {
					tr.Index = int(binary.BigEndian.Uint32(body[12:16]) & 0x7FFFFFFF)
				}
			}
		case "mdia":
			return m.children(b.data, b.end, 3, func(c box) error {
				switch c.typ {
				case "mdhd":
					body, err := m.small(c)
					if err != nil {
						return err
					}
					off := 20 // version 0: four 32-bit fields after version/flags
					if len(body) > 0 && body[0] == 1 {
						off = 32
					}
					if len(body) >= off+2 {
						tr.Language = packedLanguage(binary.BigEndian.Uint16(body[off : off+2]))
					}
				case "elng":
					body, err := m.small(c)
					if err != nil {
						return err
					}
					if len(body) > 4 {
						elng = cut(body[4:])
					}
				case "hdlr":
					body, err := m.small(c)
					if err != nil {
						return err
					}
					if len(body) >= 12 {
						handler = string(body[8:12])
					}
					if len(body) > 24 {
						name := body[24:]
						if len(name) > 0 && int(name[0]) == len(name)-1 { // QuickTime: a length byte first
							name = name[1:]
						}
						tr.Name = cut(name)
					}
				case "minf":
					return m.children(c.data, c.end, 4, func(d box) error {
						if d.typ != "stbl" {
							return nil
						}
						return m.children(d.data, d.end, 5, func(e box) error {
							if e.typ != "stsd" {
								return nil
							}
							body, err := m.small(e)
							if err != nil {
								return err
							}
							if len(body) >= 16 && binary.BigEndian.Uint32(body[4:8]) > 0 {
								entry = string(body[12:16])
							}
							return nil
						})
					})
				}
				return nil
			})
		}
		return nil
	})
	if err != nil {
		return tr, false, err
	}
	if elng != "" {
		tr.Language = elng
	}
	tr.Codec = cut([]byte(entry))
	switch handler {
	case "sbtl", "subt", "text", "clcp":
	default:
		switch entry {
		case "tx3g", "wvtt", "stpp", "c608", "c708":
		default:
			return tr, false, nil
		}
	}
	tr.Default = enabled
	low := strings.ToLower(tr.Name)
	tr.Forced = strings.Contains(low, "forced")
	return tr, true, nil
}

// packedLanguage reads the ISO 639-2 code an mdhd box packs into 15 bits.
func packedLanguage(v uint16) string {
	if v == 0 || v == 0x7FFF {
		return "und"
	}
	b := []byte{byte(v>>10&0x1F) + 0x60, byte(v>>5&0x1F) + 0x60, byte(v&0x1F) + 0x60}
	for _, c := range b {
		if c < 'a' || c > 'z' {
			return "und"
		}
	}
	return string(b)
}
