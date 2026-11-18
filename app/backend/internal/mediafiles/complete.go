package mediafiles

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
)

// What a cut-off video is called. The wording is shown to people as the
// reason an import was refused.
const (
	CutOff  = "the video is cut off; the download stopped early"
	NoIndex = "the video has no index; the download stopped early"
)

const ebmlCuesID = 0x1c53bb6b

// Truncated says whether a Matroska/WebM or MP4/MOV file stops before its
// own structure does (a download that ended early): "" when it looks whole,
// when its kind is not one of these, or when it cannot be told; otherwise
// CutOff or NoIndex. It reads box and element headers only, never samples,
// so damage in the middle of a file is not seen.
func Truncated(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", nil
	}
	return TruncatedReaderAt(f, st.Size()), nil
}

// TruncatedReaderAt is Truncated for a file already open.
func TruncatedReaderAt(r io.ReaderAt, size int64) string {
	prefix, err := readAt(r, 0, min64(size, 8))
	if err != nil || len(prefix) < 4 {
		return ""
	}
	switch {
	case string(prefix[:4]) == "\x1a\x45\xdf\xa3":
		return truncatedMatroska(r, size)
	case looksLikeMP4(prefix):
		return truncatedMP4(r, size)
	}
	return ""
}

// The top-level box types a cut-off download can stop inside. Anything else
// that claims to run past the end is stray data after the last box.
var mp4BoxTypes = map[string]bool{
	"ftyp": true, "styp": true, "moov": true, "moof": true, "mdat": true, "free": true, "skip": true,
	"wide": true, "uuid": true, "meta": true, "sidx": true, "ssix": true, "mfra": true, "pdin": true,
	"udta": true, "prft": true, "emsg": true, "pnot": true, "junk": true,
}

// truncatedMP4 walks the top-level boxes. A box that runs past the end of
// the file is a cut-off; a file that ends cleanly without a movie box has
// lost its index.
func truncatedMP4(r io.ReaderAt, size int64) string {
	haveMovie := false
	position := int64(0)
	for i := 0; i < maxElements && position+8 <= size; i++ {
		head, err := readAt(r, position, 8)
		if err != nil {
			return ""
		}
		length, headLen := int64(binary.BigEndian.Uint32(head[:4])), int64(8)
		switch length {
		case 0: // the box runs to the end of the file
			length = size - position
		case 1:
			ext, err := readAt(r, position+8, 8)
			if err != nil {
				return CutOff
			}
			v := binary.BigEndian.Uint64(ext)
			if v > uint64(1)<<62 {
				return ""
			}
			length, headLen = int64(v), 16
		}
		if length < headLen {
			return "" // not a box we understand: leave it to the reader
		}
		if position+length > size {
			if !mp4BoxTypes[string(head[4:8])] {
				return "" // bytes after the last box, not a cut box
			}
			return CutOff
		}
		if string(head[4:8]) == "moov" {
			haveMovie = true
		}
		position += length
	}
	if !haveMovie && position+8 > size {
		return NoIndex
	}
	return ""
}

// truncatedMatroska checks that the Segment fits in the file and that every
// place its SeekHead points at (the Cues index above all) is inside it.
func truncatedMatroska(r io.ReaderAt, size int64) string {
	p := &matroskaParser{r: r, size: size}
	header, next, err := p.readElement(0, size, false)
	if err != nil || header.id != ebmlHeaderID {
		return ""
	}
	id, idLen, _, err := p.readVINT(next, 4, true)
	if err != nil || id != ebmlSegmentID {
		return ""
	}
	n, sizeLen, unknown, err := p.readVINT(next+int64(idLen), 8, false)
	if err != nil {
		return ""
	}
	start := next + int64(idLen+sizeLen)
	if !unknown && (n > uint64(size) || start+int64(n) > size) {
		return CutOff
	}
	for position := start; position < size; {
		el, following, err := p.readElement(position, size, true)
		if err != nil {
			if errors.Is(err, ErrMalformed) {
				return CutOff
			}
			return ""
		}
		switch el.id {
		case ebmlClusterID:
			return ""
		case ebmlSeekHeadID:
			if seekPastEnd(p, el, start, size) {
				return NoIndex
			}
		}
		if el.unknown {
			return ""
		}
		position = following
	}
	return ""
}

func seekPastEnd(p *matroskaParser, head ebmlElement, segment, size int64) bool {
	past := false
	_ = p.each(head.start, head.end, 2, func(seek ebmlElement) error {
		if seek.id != ebmlSeekID {
			return nil
		}
		var id uint64
		var at uint64
		_ = p.each(seek.start, seek.end, 3, func(c ebmlElement) error {
			switch c.id {
			case ebmlSeekIDID:
				id, _ = p.readUint(c)
			case ebmlSeekPosition:
				at, _ = p.readUint(c)
			}
			return nil
		})
		if id == ebmlCuesID && (at >= uint64(size) || segment+int64(at) >= size) {
			past = true
		}
		return nil
	})
	return past
}
