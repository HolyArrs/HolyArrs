// Package container lists the subtitle tracks built into a video file.
// It reads headers through the bounded reader in internal/mediainfo, which
// covers Matroska, WebM, MP4 and MOV. A hostile file cannot hang it or make
// it allocate much. A file it cannot read gives an error and no tracks.
package container

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Track describes one subtitle track found inside a video.
type Track struct {
	Index           int    `json:"index"`    // track number as the container stores it
	Codec           string `json:"codec"`    // codec id, such as S_TEXT/UTF8, S_HDMV/PGS or tx3g
	Language        string `json:"language"` // language exactly as written in the file (eng, en-US, und)
	Name            string `json:"name,omitempty"`
	Forced          bool   `json:"forced,omitempty"`
	HearingImpaired bool   `json:"hearing_impaired,omitempty"`
	Default         bool   `json:"default,omitempty"`
	Picture         bool   `json:"picture,omitempty"` // true for picture formats such as PGS, VobSub and DVB
}

// Errors returned when a file cannot be read.
var (
	ErrUnknownFormat = mediafiles.ErrUnsupported
	ErrMalformed     = mediafiles.ErrMalformed
	ErrLimit         = mediafiles.ErrLimit
)

// Probe lists the subtitle tracks of the file at path.
// It opens regular files only, so links, folders and devices are refused.
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

// Read lists the subtitle tracks of a video that is size bytes long.
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
	tracks, err := mediafiles.ReadTracks(ra, size)
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
				lang = "eng" // Matroska treats a missing language as English
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

// seekReaderAt gives ReadAt on top of Seek and Read.
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

// PictureCodec reports whether a codec stores subtitles as pictures
// (PGS, VobSub, DVB, BMP) rather than text.
func PictureCodec(c string) bool { return pictureCodec(c) }

func pictureCodec(c string) bool {
	c = strings.ToUpper(c)
	return strings.HasPrefix(c, "S_HDMV/PGS") || c == "S_VOBSUB" || c == "S_DVBSUB" || c == "S_IMAGE/BMP"
}
