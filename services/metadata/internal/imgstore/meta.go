package imgstore

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"
)

// jpegMeta reads what Decode needs from a JPEG's segments before they are
// dropped: the embedded colour profile (APP2 ICC_PROFILE chunks joined in
// order) and the EXIF orientation (APP1 Exif, tag 0x0112; 1 when absent).
func jpegMeta(raw []byte) (profile []byte, orientation int) {
	orientation = 1
	if len(raw) < 4 || raw[0] != 0xFF || raw[1] != 0xD8 {
		return nil, orientation
	}
	chunks := map[int][]byte{}
	count := 0
	for i := 2; i+4 <= len(raw); {
		if raw[i] != 0xFF {
			break
		}
		marker := raw[i+1]
		if marker == 0xFF {
			i++
			continue
		}
		if marker == 0xD8 || marker >= 0xD0 && marker <= 0xD7 || marker == 0x01 {
			i += 2
			continue
		}
		if marker == 0xDA || marker == 0xD9 {
			break // image data starts: no more metadata segments
		}
		n := int(binary.BigEndian.Uint16(raw[i+2:]))
		if n < 2 || i+2+n > len(raw) {
			break
		}
		seg := raw[i+4 : i+2+n]
		switch {
		case marker == 0xE1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")):
			if o := exifOrientation(seg[6:]); o != 0 {
				orientation = o
			}
		case marker == 0xE2 && bytes.HasPrefix(seg, []byte("ICC_PROFILE\x00")) && len(seg) > 14:
			chunks[int(seg[12])] = seg[14:]
			count = int(seg[13])
		}
		i += 2 + n
	}
	if count > 0 && len(chunks) == count {
		for k := 1; k <= count; k++ {
			c, ok := chunks[k]
			if !ok {
				return nil, orientation
			}
			profile = append(profile, c...)
		}
	}
	return profile, orientation
}

// exifOrientation reads tag 0x0112 from IFD0 of a TIFF block (0 when it is
// not there or the block is broken).
func exifOrientation(t []byte) int {
	if len(t) < 8 {
		return 0
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}
	off := int(bo.Uint32(t[4:]))
	if off < 8 || off+2 > len(t) {
		return 0
	}
	n := int(bo.Uint16(t[off:]))
	for e := range n {
		p := off + 2 + e*12
		if p+12 > len(t) {
			return 0
		}
		if bo.Uint16(t[p:]) == 0x0112 && bo.Uint16(t[p+2:]) == 3 {
			if v := int(bo.Uint16(t[p+8:])); v >= 1 && v <= 8 {
				return v
			}
			return 0
		}
	}
	return 0
}

// pngProfile reads a PNG's iCCP colour profile (nil when there is none).
func pngProfile(raw []byte) []byte {
	if len(raw) < 8 || string(raw[1:4]) != "PNG" {
		return nil
	}
	for i := 8; i+8 <= len(raw); {
		n := int(binary.BigEndian.Uint32(raw[i:]))
		typ := string(raw[i+4 : i+8])
		if n < 0 || i+12+n > len(raw) {
			return nil
		}
		data := raw[i+8 : i+8+n]
		switch typ {
		case "iCCP":
			z := bytes.IndexByte(data, 0)
			if z < 0 || z+2 > len(data) || data[z+1] != 0 {
				return nil
			}
			r, err := zlib.NewReader(bytes.NewReader(data[z+2:]))
			if err != nil {
				return nil
			}
			defer r.Close()
			p, err := io.ReadAll(io.LimitReader(r, 4<<20))
			if err != nil {
				return nil
			}
			return p
		case "IDAT", "IEND":
			return nil
		}
		i += 12 + n
	}
	return nil
}
