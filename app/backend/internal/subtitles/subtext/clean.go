// Package subtext handles subtitle text that came from outside. It unpacks
// zip answers safely, detects the encoding, cleans the text and shifts cue
// timing with offset and frame-rate fixes for SRT, ASS and SSA. All work
// happens in memory with size caps and without regular expressions over whole
// files, so hostile input stays cheap.
package subtext

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/encoding/unicode/utf32"
	"golang.org/x/text/transform"
)

// Caps on how much is read.
const (
	MaxText       = 5 << 20  // bytes in one subtitle
	MaxArchive    = 8 << 20  // bytes in one download
	MaxEntries    = 50       // entries in one zip
	MaxEntryText  = 5 << 20  // bytes in one unpacked entry
	MaxUnpacked   = 20 << 20 // bytes in all unpacked entries together
	MaxEntryRatio = 100      // unpacked to packed size, per entry
)

// Errors.
var (
	ErrTooBig      = errors.New("subtitle is too big")
	ErrArchive     = errors.New("archive refused")
	ErrNotSubtitle = errors.New("not a subtitle")
)

// Formats a subtitle can be saved in.
const (
	SRT = "srt"
	ASS = "ass"
	SSA = "ssa"
	VTT = "vtt"
	SUB = "sub"
)

// Formats lists the subtitle extensions read from archives and beside files.
var Formats = []string{SRT, ASS, SSA, VTT, SUB}

// Entry is one subtitle found in a download.
type Entry struct {
	Name   string // base name of the archive entry, used only for choosing and never as a path
	Format string
	Data   []byte
}

// Unpack returns the subtitles in a download: the download itself when it is
// a subtitle, or the subtitle entries of a zip. RAR and 7z are refused.
// nameHint is the site's file name, used when the data does not reveal the type.
func Unpack(data []byte, nameHint string) ([]Entry, error) {
	if len(data) > MaxArchive {
		return nil, ErrTooBig
	}
	switch {
	case bytes.HasPrefix(data, []byte("PK\x03\x04")), bytes.HasPrefix(data, []byte("PK\x05\x06")):
		return unzip(data)
	case bytes.HasPrefix(data, []byte("Rar!")), bytes.HasPrefix(data, []byte("7z\xBC\xAF\x27\x1C")):
		return nil, fmt.Errorf("%w: RAR and 7z are not opened", ErrArchive)
	case bytes.HasPrefix(data, []byte{0x1F, 0x8B}):
		return nil, fmt.Errorf("%w: gzip is not opened", ErrArchive)
	}
	if len(data) > MaxText {
		return nil, ErrTooBig
	}
	f := formatOf(nameHint)
	if f == "" {
		f = sniff(data)
	}
	if f == "" {
		return nil, ErrNotSubtitle
	}
	return []Entry{{Name: path.Base(strings.ReplaceAll(nameHint, `\`, "/")), Format: f, Data: data}}, nil
}

func formatOf(name string) string {
	ext := strings.ToLower(path.Ext(strings.ReplaceAll(name, `\`, "/")))
	for _, f := range Formats {
		if ext == "."+f {
			return f
		}
	}
	return ""
}

// TextSub reports whether the start of a .sub file is plainly text, such as
// MicroDVD "{1}{25}..." or SubViewer "[INFORMATION]". It must hold no NUL or
// other control bytes besides tab, CR and LF, and its first non-blank line
// must start with "{" or "[". A VobSub picture .sub, or anything unclear, is
// not text.
func TextSub(head []byte) bool {
	head = bytes.TrimPrefix(head, []byte("\xef\xbb\xbf"))
	for _, c := range head {
		if c < 0x20 && c != '\t' && c != '\r' && c != '\n' {
			return false
		}
	}
	t := bytes.TrimLeft(head, " \t\r\n")
	return len(t) > 0 && (t[0] == '{' || t[0] == '[')
}

// sniff guesses the format from the first bytes of text.
func sniff(b []byte) string {
	head := strings.ToLower(string(b[:min(len(b), 4096)]))
	switch {
	case strings.Contains(head, "[script info]") && strings.Contains(head, "v4+"):
		return ASS
	case strings.Contains(head, "[script info]"):
		return SSA
	case strings.HasPrefix(strings.TrimLeft(head, "\xef\xbb\xbf \r\n"), "webvtt"):
		return VTT
	case strings.Contains(head, "-->"):
		return SRT
	}
	return ""
}

// unzip reads the subtitle entries of a zip in memory. Entries that are links,
// contain "..", are absolute or name a drive make the whole archive refused.
// So do zip bombs: too many entries, an entry too large or past MaxEntryRatio,
// or too much in total.
func unzip(data []byte) ([]Entry, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrArchive, err)
	}
	if len(zr.File) > MaxEntries {
		return nil, fmt.Errorf("%w: more than %d entries", ErrArchive, MaxEntries)
	}
	var out []Entry
	var total int64
	for _, zf := range zr.File {
		name := strings.ReplaceAll(zf.Name, `\`, "/")
		if strings.HasPrefix(name, "/") || strings.Contains(name, ":") || zf.Mode()&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: entry %q leaves the archive", ErrArchive, zf.Name)
		}
		for _, part := range strings.Split(name, "/") {
			if part == ".." {
				return nil, fmt.Errorf("%w: entry %q leaves the archive", ErrArchive, zf.Name)
			}
		}
		if zf.FileInfo().IsDir() {
			continue
		}
		f := formatOf(name)
		if f == "" {
			continue
		}
		if zf.UncompressedSize64 > MaxEntryText || (zf.CompressedSize64 > 0 && zf.UncompressedSize64/zf.CompressedSize64 > MaxEntryRatio) {
			return nil, fmt.Errorf("%w: entry %q unpacks too big", ErrArchive, zf.Name)
		}
		rc, err := zf.Open()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrArchive, err)
		}
		// Header sizes can lie, so read at most one byte past the cap.
		b, err := io.ReadAll(io.LimitReader(rc, MaxEntryText+1))
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrArchive, err)
		}
		if len(b) > MaxEntryText || (zf.CompressedSize64 > 0 && uint64(len(b))/max(zf.CompressedSize64, 1) > MaxEntryRatio) {
			return nil, fmt.Errorf("%w: entry %q unpacks too big", ErrArchive, zf.Name)
		}
		if total += int64(len(b)); total > MaxUnpacked {
			return nil, fmt.Errorf("%w: unpacks to more than %d bytes", ErrArchive, MaxUnpacked)
		}
		out = append(out, Entry{Name: path.Base(name), Format: f, Data: b})
	}
	if len(out) == 0 {
		return nil, ErrNotSubtitle
	}
	return out, nil
}

// Clean turns subtitle bytes into UTF-8 text with no byte-order mark, NUL bytes
// or other control characters; tab, CR and LF stay. A UTF-8, UTF-16 or UTF-32
// mark is honoured, valid UTF-8 is kept as is and anything else is read as
// Windows-1252. Text without a timed cue is refused.
func Clean(b []byte, format string) (string, error) {
	if len(b) > MaxText {
		return "", ErrTooBig
	}
	text, err := decode(b)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.Grow(len(text))
	for _, r := range text {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
		case r < 0x20 || r == 0x7F || (r >= 0x80 && r < 0xA0) || r == utf8.RuneError || r == 0xFEFF:
			continue
		}
		sb.WriteRune(r)
	}
	out := sb.String()
	if !hasCue(out, format) {
		return "", ErrNotSubtitle
	}
	return out, nil
}

func decode(b []byte) (string, error) {
	var enc transform.Transformer
	switch {
	case bytes.HasPrefix(b, []byte{0x00, 0x00, 0xFE, 0xFF}):
		enc = utf32.UTF32(utf32.BigEndian, utf32.ExpectBOM).NewDecoder()
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE, 0x00, 0x00}):
		enc = utf32.UTF32(utf32.LittleEndian, utf32.ExpectBOM).NewDecoder()
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		enc = unicode.UTF16(unicode.BigEndian, unicode.ExpectBOM).NewDecoder()
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		enc = unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewDecoder()
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		return string(b[3:]), nil
	case utf8.Valid(b):
		return string(b), nil
	default:
		enc = charmap.Windows1252.NewDecoder()
	}
	out, _, err := transform.Bytes(enc, b)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotSubtitle, err)
	}
	if len(out) > 2*MaxText {
		return "", ErrTooBig
	}
	return string(out), nil
}

// hasCue reports whether text holds at least one timed cue.
func hasCue(text, format string) bool {
	switch format {
	case ASS, SSA:
		for line := range strings.Lines(text) {
			if strings.HasPrefix(strings.TrimSpace(line), "Dialogue:") {
				return true
			}
		}
		return false
	case SUB:
		// MicroDVD lines look like {start}{end}text.
		for line := range strings.Lines(text) {
			if strings.HasPrefix(strings.TrimSpace(line), "{") {
				return true
			}
		}
		return false
	}
	for line := range strings.Lines(text) {
		if _, _, ok := cueTimes(line); ok {
			return true
		}
	}
	return false
}
