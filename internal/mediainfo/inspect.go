// Package mediainfo reads a bounded subset of media container headers.
package mediainfo

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/text/language"
)

const (
	maxElements = 4096
	maxDepth    = 12
	maxTracks   = 128
	maxString   = 256
)

// Info contains facts read from media container headers.
type Info struct {
	Container      string
	VideoCodec     string
	Status         string
	AudioLanguages []string
	AudioTracks    int
	AudioUnknown   bool
}

// Inspect reads media headers from a regular file. It does not decode samples.
func Inspect(path string) (Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return Info{}, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return Info{}, err
	}
	if !stat.Mode().IsRegular() {
		return Info{}, fmt.Errorf("inspect media: input is not a regular file")
	}
	return InspectReaderAt(f, stat.Size())
}

// InspectReaderAt reads media headers from r within the declared size.
func InspectReaderAt(r io.ReaderAt, size int64) (Info, error) {
	if r == nil || size < 0 {
		return Info{}, fmt.Errorf("inspect media: invalid input size")
	}
	if size < 4 {
		return Info{Status: "unsupported"}, nil
	}

	prefix, err := readAt(r, 0, min64(size, 8))
	if err != nil {
		return Info{}, fmt.Errorf("inspect media header: %w", err)
	}
	var info Info
	switch {
	case len(prefix) >= 4 && string(prefix[:4]) == "\x1a\x45\xdf\xa3":
		info, err = inspectMatroska(r, size)
	case looksLikeMP4(prefix):
		info, err = inspectMP4(r, size)
	default:
		return Info{Status: "unsupported"}, nil
	}
	if err != nil {
		return Info{}, err
	}
	finishInfo(&info)
	return info, nil
}

func finishInfo(info *Info) {
	sort.Strings(info.AudioLanguages)
	info.AudioLanguages = compactStrings(info.AudioLanguages)
	if info.Status == "unsupported" {
		return
	}
	if info.VideoCodec == "" || info.AudioUnknown {
		info.Status = "partial"
	} else {
		info.Status = "complete"
	}
}

func normalizeLanguage(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "und") || strings.HasPrefix(strings.ToLower(value), "und-") {
		return "", false
	}
	tag, err := language.Parse(value)
	if err != nil {
		return "", false
	}
	base, confidence := tag.Base()
	if confidence == language.No || base.String() == "und" {
		return "", false
	}
	return base.String(), true
}

func compactStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

func readAt(r io.ReaderAt, offset, length int64) ([]byte, error) {
	if offset < 0 || length < 0 || length > int64(^uint(0)>>1) {
		return nil, errors.New("invalid read range")
	}
	b := make([]byte, int(length))
	if length == 0 {
		return b, nil
	}
	n, err := r.ReadAt(b, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if n != len(b) {
		return nil, io.ErrUnexpectedEOF
	}
	return b, nil
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
