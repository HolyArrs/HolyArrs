package tags

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Tags are what a file says about itself; zero values are unknown.
type Tags struct {
	Title  string `json:"title,omitempty"`
	Artist string `json:"artist,omitempty"`
	Album  string `json:"album,omitempty"`
	Track  int    `json:"track,omitempty"`
	Disc   int    `json:"disc,omitempty"`
}

// Empty says no tag was found.
func (t Tags) Empty() bool { return t == Tags{} }

// Limits on what is read.
const (
	MaxTagBytes   = 256 << 10 // the most bytes of an ID3v2 tag or a FLAC comment block read
	maxBlocks     = 64        // FLAC metadata blocks looked at
	maxComments   = 4096      // Vorbis comments looked at
	maxValueBytes = 1024      // the longest value kept
	maxFrames     = 512       // ID3v2 frames looked at
)

// ErrNoTags says the file holds no tag this package reads.
var ErrNoTags = errors.New("no ID3v2 or FLAC tags")

// ErrNotRegular says the path is not a regular file.
var ErrNotRegular = errors.New("not a regular file")

// ReadFile reads the tags of an MP3 or FLAC file.
// Anything but a regular file (a folder, a link, a device or pipe) is
// refused before it is opened.
func ReadFile(path string) (Tags, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return Tags{}, err
	}
	if !fi.Mode().IsRegular() {
		return Tags{}, ErrNotRegular
	}
	f, err := os.Open(path)
	if err != nil {
		return Tags{}, err
	}
	defer f.Close()
	return Read(f)
}

// Read reads the tags at the start of r.
func Read(r io.ReadSeeker) (Tags, error) {
	var magic [4]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return Tags{}, ErrNoTags
	}
	switch {
	case string(magic[:3]) == "ID3":
		return readID3(r, magic[3])
	case string(magic[:]) == "fLaC":
		return readFLAC(r)
	}
	return Tags{}, ErrNoTags
}

// clean keeps a value readable: valid UTF-8, no control characters, at
// most maxValueBytes.
func clean(s string) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || r == 0xFEFF {
			return -1
		}
		return r
	}, s)
	if len(s) > maxValueBytes {
		n := maxValueBytes
		for n > 0 && !utf8.RuneStart(s[n]) {
			n--
		}
		s = s[:n]
	}
	return strings.TrimSpace(s)
}

// number reads "3", "03" or "3/12"; 0 when it is not a number from 1 to 999.
func number(s string) int {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 1 || n > 999 {
		return 0
	}
	return n
}

func (t *Tags) set(key, v string) {
	switch strings.ToUpper(key) {
	case "TITLE", "TIT2", "TT2":
		if t.Title == "" {
			t.Title = clean(v)
		}
	case "ARTIST", "TPE1", "TP1":
		if t.Artist == "" {
			t.Artist = clean(v)
		}
	case "ALBUM", "TALB", "TAL":
		if t.Album == "" {
			t.Album = clean(v)
		}
	case "TRACKNUMBER", "TRCK", "TRK":
		if t.Track == 0 {
			t.Track = number(v)
		}
	case "DISCNUMBER", "TPOS", "TPA":
		if t.Disc == 0 {
			t.Disc = number(v)
		}
	}
}

func syncsafe(b []byte) (int, bool) {
	n := 0
	for _, c := range b {
		if c&0x80 != 0 {
			return 0, false
		}
		n = n<<7 | int(c)
	}
	return n, true
}

// readID3 reads an ID3v2.2, 2.3 or 2.4 tag; the "ID3" and the version byte
// are read.
func readID3(r io.Reader, ver byte) (Tags, error) {
	var h [6]byte // revision, flags, size
	if _, err := io.ReadFull(r, h[:]); err != nil || ver < 2 || ver > 4 {
		return Tags{}, ErrNoTags
	}
	flags := h[1]
	if ver == 2 && flags&0x40 != 0 {
		return Tags{}, ErrNoTags // ID3v2.2 compression: no scheme was ever defined
	}
	size, ok := syncsafe(h[2:6])
	if !ok || size == 0 {
		return Tags{}, ErrNoTags
	}
	buf := make([]byte, min(size, MaxTagBytes))
	n, _ := io.ReadFull(r, buf)
	buf = buf[:n]
	if flags&0x80 != 0 && ver < 4 {
		buf = bytes.ReplaceAll(buf, []byte{0xFF, 0x00}, []byte{0xFF}) // unsynchronised tag
	}
	if flags&0x40 != 0 && ver >= 3 { // extended header
		if len(buf) < 4 {
			return Tags{}, ErrNoTags
		}
		var ext int
		if ver == 3 {
			ext = int(binary.BigEndian.Uint32(buf[:4])) + 4
		} else if ext, ok = syncsafe(buf[:4]); !ok {
			return Tags{}, ErrNoTags
		}
		if ext < 0 || ext > len(buf) {
			return Tags{}, ErrNoTags
		}
		buf = buf[ext:]
	}
	idLen, hdrLen := 4, 10
	if ver == 2 {
		idLen, hdrLen = 3, 6
	}
	var t Tags
	for i := 0; i < maxFrames && len(buf) >= hdrLen; i++ {
		id := string(buf[:idLen])
		if buf[0] == 0 {
			break // padding
		}
		var fsize int
		var fflags uint16
		switch ver {
		case 2:
			fsize = int(buf[3])<<16 | int(buf[4])<<8 | int(buf[5])
		case 3:
			fsize = int(binary.BigEndian.Uint32(buf[4:8]))
			fflags = binary.BigEndian.Uint16(buf[8:10])
		default:
			if fsize, ok = syncsafe(buf[4:8]); !ok {
				return t, nil
			}
			fflags = binary.BigEndian.Uint16(buf[8:10])
		}
		if fsize < 0 || fsize > len(buf)-hdrLen {
			break // a frame running past the tag: stop with what was read
		}
		body := buf[hdrLen : hdrLen+fsize]
		buf = buf[hdrLen+fsize:]
		// Compressed or encrypted frames (and v2.4 frames with a data
		// length or their own unsynchronisation) are skipped.
		if ver == 3 && fflags&0x00C0 != 0 || ver == 4 && fflags&0x000F != 0 {
			continue
		}
		if len(id) > 0 && id[0] == 'T' && len(body) > 0 {
			t.set(id, id3Text(body))
		}
	}
	if t.Empty() {
		return t, ErrNoTags
	}
	return t, nil
}

// id3Text decodes a text frame's body: an encoding byte, then the text.
// Only the first of several values is kept.
func id3Text(b []byte) string {
	enc, b := b[0], b[1:]
	switch enc {
	case 0: // ISO-8859-1
		if i := bytes.IndexByte(b, 0); i >= 0 {
			b = b[:i]
		}
		rs := make([]rune, len(b))
		for i, c := range b {
			rs[i] = rune(c)
		}
		return string(rs)
	case 1, 2: // UTF-16 with a byte order mark, UTF-16BE
		big := enc == 2
		if len(b) >= 2 && enc == 1 {
			switch {
			case b[0] == 0xFF && b[1] == 0xFE:
				b = b[2:]
			case b[0] == 0xFE && b[1] == 0xFF:
				big, b = true, b[2:]
			}
		}
		u := make([]uint16, 0, len(b)/2)
		for i := 0; i+1 < len(b); i += 2 {
			var c uint16
			if big {
				c = uint16(b[i])<<8 | uint16(b[i+1])
			} else {
				c = uint16(b[i+1])<<8 | uint16(b[i])
			}
			if c == 0 {
				break
			}
			u = append(u, c)
		}
		return string(utf16.Decode(u))
	case 3: // UTF-8
		if i := bytes.IndexByte(b, 0); i >= 0 {
			b = b[:i]
		}
		return string(b)
	}
	return ""
}

// readFLAC reads the Vorbis comment block of a FLAC file; "fLaC" is read.
// Other blocks (pictures can be large) are skipped without reading them.
func readFLAC(r io.ReadSeeker) (Tags, error) {
	for i := 0; i < maxBlocks; i++ {
		var h [4]byte
		if _, err := io.ReadFull(r, h[:]); err != nil {
			return Tags{}, ErrNoTags
		}
		last, kind := h[0]&0x80 != 0, h[0]&0x7F
		size := int(h[1])<<16 | int(h[2])<<8 | int(h[3])
		if kind == 127 {
			return Tags{}, ErrNoTags // invalid block type
		}
		if kind == 4 {
			if size > MaxTagBytes {
				return Tags{}, ErrNoTags
			}
			buf := make([]byte, size)
			if _, err := io.ReadFull(r, buf); err != nil {
				return Tags{}, ErrNoTags
			}
			return vorbis(buf)
		}
		if last {
			break
		}
		if _, err := r.Seek(int64(size), io.SeekCurrent); err != nil {
			return Tags{}, ErrNoTags
		}
	}
	return Tags{}, ErrNoTags
}

// vorbis reads a Vorbis comment block: a vendor string, then a count of
// KEY=value comments, every length a little-endian 32-bit number.
func vorbis(b []byte) (Tags, error) {
	next := func() ([]byte, bool) {
		if len(b) < 4 {
			return nil, false
		}
		n := binary.LittleEndian.Uint32(b[:4])
		b = b[4:]
		if uint64(n) > uint64(len(b)) {
			return nil, false
		}
		v := b[:n]
		b = b[n:]
		return v, true
	}
	if _, ok := next(); !ok { // vendor
		return Tags{}, ErrNoTags
	}
	if len(b) < 4 {
		return Tags{}, ErrNoTags
	}
	count := binary.LittleEndian.Uint32(b[:4])
	b = b[4:]
	var t Tags
	for i := uint32(0); i < count && i < maxComments; i++ {
		c, ok := next()
		if !ok {
			break
		}
		k, v, ok := strings.Cut(string(c), "=")
		if ok {
			t.set(k, v)
		}
	}
	if t.Empty() {
		return t, ErrNoTags
	}
	return t, nil
}
