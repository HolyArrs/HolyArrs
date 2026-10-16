// Package container lists the subtitle tracks inside a video file. It reads
// only the track headers of Matroska/WebM and MP4/MOV files, in plain Go,
// with hard limits on how much it reads, how deep it goes and how many
// elements it looks at, so a hostile file can neither hang it nor make it
// allocate much. Anything it cannot read gives an error and no tracks.
package container

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Track is one subtitle track inside a video.
type Track struct {
	Index           int    `json:"index"`    // the container's own track number
	Codec           string `json:"codec"`    // S_TEXT/UTF8, S_HDMV/PGS, tx3g ...
	Language        string `json:"language"` // as the file says it (eng, en-US, und)
	Name            string `json:"name,omitempty"`
	Forced          bool   `json:"forced,omitempty"`
	HearingImpaired bool   `json:"hearing_impaired,omitempty"`
	Default         bool   `json:"default,omitempty"`
	Picture         bool   `json:"picture,omitempty"` // a picture format (PGS, VobSub, DVB)
}

// Limits on what one file may make the readers do.
const (
	maxElements    = 20000   // elements or boxes looked at per file
	maxDepth       = 8       // nesting
	maxTracksBody  = 1 << 20 // bytes of a Matroska Tracks element read into memory
	maxSmallBox    = 4096    // bytes of an MP4 hdlr, mdhd, elng or stsd read
	maxTracks      = 256     // tracks kept
	maxString      = 1024    // bytes of a name or codec kept
	maxHeaderBytes = 4096    // bytes of the EBML header
)

// Errors.
var (
	ErrUnknownFormat = errors.New("not a Matroska or MP4 file")
	ErrMalformed     = errors.New("malformed container")
	ErrLimit         = errors.New("container is past a reading limit")
)

// Probe lists the subtitle tracks of the file at path. Only regular files
// are read (links, folders and devices are refused).
func Probe(path string) ([]Track, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Read(f, fi.Size())
}

// Read lists the subtitle tracks of a video of size bytes.
func Read(r io.ReadSeeker, size int64) ([]Track, error) {
	var head [12]byte
	n, _ := io.ReadFull(r, head[:])
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	switch {
	case n >= 4 && head[0] == 0x1A && head[1] == 0x45 && head[2] == 0xDF && head[3] == 0xA3:
		return readMatroska(r, size)
	case n >= 8 && isMP4Box(string(head[4:8])):
		return readMP4(r, size)
	}
	return nil, ErrUnknownFormat
}

func isMP4Box(t string) bool {
	switch t {
	case "ftyp", "moov", "mdat", "free", "skip", "wide", "pnot":
		return true
	}
	return false
}

// cut keeps at most maxString bytes of a string read from a file, without
// NULs and on a character boundary.
func cut(b []byte) string {
	s := strings.TrimRight(string(b), "\x00")
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	if len(s) > maxString {
		s = s[:maxString]
		for len(s) > 0 && s[len(s)-1]&0xC0 == 0x80 {
			s = s[:len(s)-1]
		}
	}
	return strings.ToValidUTF8(s, "")
}

// readBody reads n bytes, refusing more than limit.
func readBody(r io.Reader, n int64, limit int64) ([]byte, error) {
	if n < 0 || n > limit {
		return nil, ErrLimit
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, ErrMalformed
	}
	return b, nil
}
