package container

import (
	"io"
	"strings"
)

// Matroska element ids (with their length marker, as written).
const (
	idEBML          = 0x1A45DFA3
	idDocType       = 0x4282
	idSegment       = 0x18538067
	idSeekHead      = 0x114D9B74
	idSeek          = 0x4DBB
	idSeekID        = 0x53AB
	idSeekPosition  = 0x53AC
	idTracks        = 0x1654AE6B
	idCluster       = 0x1F43B675
	idTrackEntry    = 0xAE
	idTrackNumber   = 0xD7
	idTrackType     = 0x83
	idCodecID       = 0x86
	idLanguage      = 0x22B59C
	idLanguageBCP47 = 0x22B59D
	idName          = 0x536E
	idFlagForced    = 0x55AA
	idFlagHI        = 0x55AB
	idFlagDefault   = 0x88
	trackSubtitle   = 0x11
)

const unknownSize = -1

// readVint reads an EBML variable-size integer from r: an element id
// (keepMarker) or a size. A size of all ones is unknownSize.
func readVint(r io.Reader, keepMarker bool, maxLen int) (int64, int, error) {
	var b [8]byte
	if _, err := io.ReadFull(r, b[:1]); err != nil {
		return 0, 0, ErrMalformed
	}
	n := 1
	for mask := byte(0x80); n <= 8 && b[0]&mask == 0; mask >>= 1 {
		n++
	}
	if n > maxLen {
		return 0, 0, ErrMalformed
	}
	if n > 1 {
		if _, err := io.ReadFull(r, b[1:n]); err != nil {
			return 0, 0, ErrMalformed
		}
	}
	v := int64(b[0])
	if !keepMarker {
		v &= int64(0xFF >> n)
	}
	allOnes := v == int64(0xFF>>n)
	for i := 1; i < n; i++ {
		v = v<<8 | int64(b[i])
		allOnes = allOnes && b[i] == 0xFF
	}
	if !keepMarker && allOnes {
		return unknownSize, n, nil
	}
	return v, n, nil
}

// element is one element header: id, data size and where the data starts.
type element struct {
	id, size int64
}

func readElement(r io.Reader) (element, int, error) {
	id, n1, err := readVint(r, true, 4)
	if err != nil {
		return element{}, 0, err
	}
	size, n2, err := readVint(r, false, 8)
	if err != nil {
		return element{}, 0, err
	}
	return element{id: id, size: size}, n1 + n2, nil
}

func readMatroska(r io.ReadSeeker, fileSize int64) ([]Track, error) {
	// EBML header: small, and it must say matroska or webm.
	h, _, err := readElement(r)
	if err != nil || h.id != idEBML || h.size == unknownSize {
		return nil, ErrMalformed
	}
	hb, err := readBody(r, h.size, maxHeaderBytes)
	if err != nil {
		return nil, err
	}
	docType := ""
	_ = walk(hb, 1, func(id int64, body []byte) {
		if id == idDocType {
			docType = cut(body)
		}
	})
	if docType != "matroska" && docType != "webm" {
		return nil, ErrUnknownFormat
	}
	seg, _, err := readElement(r)
	if err != nil || seg.id != idSegment {
		return nil, ErrMalformed
	}
	segStart, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	segEnd := fileSize
	if seg.size != unknownSize && segStart+seg.size < fileSize {
		segEnd = segStart + seg.size
	}
	tracksAt := int64(-1) // from a SeekHead
	pos := segStart
	for seen := 0; pos < segEnd; seen++ {
		if seen >= maxElements {
			return nil, ErrLimit
		}
		if _, err := r.Seek(pos, io.SeekStart); err != nil {
			return nil, err
		}
		e, hl, err := readElement(r)
		if err != nil {
			return nil, err
		}
		data := pos + int64(hl)
		switch {
		case e.id == idTracks:
			if e.size == unknownSize {
				return nil, ErrMalformed
			}
			body, err := readBody(r, e.size, maxTracksBody)
			if err != nil {
				return nil, err
			}
			return parseTracks(body)
		case e.id == idSeekHead && e.size != unknownSize && e.size <= 65536:
			body, err := readBody(r, e.size, 65536)
			if err != nil {
				return nil, err
			}
			if at := seekTracks(body); at >= 0 {
				tracksAt = segStart + at
			}
		case e.id == idCluster:
			// Track headers come before the clusters; jump to them when the
			// SeekHead says they are later, else there are none to read.
			if tracksAt > pos {
				pos = tracksAt
				tracksAt = -1
				continue
			}
			return nil, ErrMalformed
		}
		if e.size == unknownSize {
			return nil, ErrMalformed
		}
		next := data + e.size
		if next <= pos || next > segEnd {
			return nil, ErrMalformed
		}
		pos = next
	}
	return nil, ErrMalformed
}

// walk calls fn for every element directly inside body.
func walk(body []byte, depth int, fn func(id int64, body []byte)) error {
	if depth > maxDepth {
		return ErrLimit
	}
	br := &sliceReader{b: body}
	for count := 0; br.off < len(body); count++ {
		if count >= maxElements {
			return ErrLimit
		}
		e, _, err := readElement(br)
		if err != nil || e.size == unknownSize || e.size > int64(len(body)-br.off) {
			return ErrMalformed
		}
		fn(e.id, body[br.off:br.off+int(e.size)])
		br.off += int(e.size)
	}
	return nil
}

type sliceReader struct {
	b   []byte
	off int
}

func (s *sliceReader) Read(p []byte) (int, error) {
	if s.off >= len(s.b) {
		return 0, io.EOF
	}
	n := copy(p, s.b[s.off:])
	s.off += n
	return n, nil
}

func uintOf(b []byte) int64 {
	if len(b) > 8 {
		return -1
	}
	var v int64
	for _, x := range b {
		v = v<<8 | int64(x)
	}
	return v
}

// seekTracks finds the Tracks position in a SeekHead (-1 when not there).
func seekTracks(body []byte) int64 {
	at := int64(-1)
	_ = walk(body, 1, func(id int64, seek []byte) {
		if id != idSeek {
			return
		}
		var sid, spos int64 = 0, -1
		_ = walk(seek, 2, func(id int64, b []byte) {
			switch id {
			case idSeekID:
				sid = uintOf(b)
			case idSeekPosition:
				spos = uintOf(b)
			}
		})
		if sid == idTracks && spos >= 0 {
			at = spos
		}
	})
	return at
}

func parseTracks(body []byte) ([]Track, error) {
	var out []Track
	var bad error
	err := walk(body, 1, func(id int64, entry []byte) {
		if id != idTrackEntry || bad != nil {
			return
		}
		t := Track{Language: "eng"} // the Matroska default when none is written
		typ := int64(-1)
		var bcp string
		forced, hi := false, false
		def := true // FlagDefault defaults to 1
		if err := walk(entry, 2, func(id int64, b []byte) {
			switch id {
			case idTrackNumber:
				t.Index = int(uintOf(b) & 0x7FFFFFFF)
			case idTrackType:
				typ = uintOf(b)
			case idCodecID:
				t.Codec = cut(b)
			case idLanguage:
				t.Language = cut(b)
			case idLanguageBCP47:
				bcp = cut(b)
			case idName:
				t.Name = cut(b)
			case idFlagForced:
				forced = uintOf(b) == 1
			case idFlagHI:
				hi = uintOf(b) == 1
			case idFlagDefault:
				def = uintOf(b) == 1
			}
		}); err != nil {
			bad = err
			return
		}
		if typ != trackSubtitle || len(out) >= maxTracks {
			return
		}
		if bcp != "" {
			t.Language = bcp // BCP 47 wins over the old field
		}
		t.Forced, t.HearingImpaired, t.Default = forced, hi, def
		t.Picture = pictureCodec(t.Codec)
		out = append(out, t)
	})
	if err == nil {
		err = bad
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func pictureCodec(c string) bool {
	c = strings.ToUpper(c)
	return strings.HasPrefix(c, "S_HDMV/PGS") || c == "S_VOBSUB" || c == "S_DVBSUB" || c == "S_IMAGE/BMP"
}
