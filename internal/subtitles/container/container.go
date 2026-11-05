// Package container lists the subtitle tracks inside a video. It uses the
// one bounded header reader in internal/mediainfo (Matroska/WebM and
// MP4/MOV), so a hostile file can neither hang it nor make it allocate
// much. Anything it cannot read gives an error and no tracks.
package container

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/mediainfo"
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

// Errors.
var (
	ErrUnknownFormat = mediainfo.ErrUnsupported
	ErrMalformed     = mediainfo.ErrMalformed
	ErrLimit         = mediainfo.ErrLimit
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
	ra, ok := r.(io.ReaderAt)
	if !ok {
		ra = seekReaderAt{r}
	}
	head := make([]byte, 4)
	if n, _ := ra.ReadAt(head, 0); n < 4 {
		return nil, ErrUnknownFormat
	}
	matroska := string(head) == "\x1a\x45\xdf\xa3"
	tracks, err := mediainfo.ReadTracks(ra, size)
	if err != nil {
		return nil, err
	}
	var out []Track
	for _, t := range tracks {
		if t.Kind != "subtitle" {
			continue
		}
		lang := t.Language
		if lang == "" {
			lang = "und"
			if matroska {
				lang = "eng" // the Matroska default when none is written
			}
		}
		out = append(out, Track{
			Index: t.Number, Codec: t.Codec, Language: lang, Name: t.Name,
			Forced: t.Forced, HearingImpaired: t.HearingImpaired, Default: t.Default,
			Picture: pictureCodec(t.Codec),
		})
	}
	return out, nil
}

// seekReaderAt reads at an offset through Seek and Read.
type seekReaderAt struct{ r io.ReadSeeker }

func (s seekReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if _, err := s.r.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}
	n, err := io.ReadFull(s.r, p)
	if errors.Is(err, io.ErrUnexpectedEOF) {
		err = io.EOF
	}
	return n, err
}

// PictureCodec reports whether a track codec is a picture subtitle
// (PGS, VobSub, DVB, BMP), not text.
func PictureCodec(c string) bool { return pictureCodec(c) }

func pictureCodec(c string) bool {
	c = strings.ToUpper(c)
	return strings.HasPrefix(c, "S_HDMV/PGS") || c == "S_VOBSUB" || c == "S_DVBSUB" || c == "S_IMAGE/BMP"
}
