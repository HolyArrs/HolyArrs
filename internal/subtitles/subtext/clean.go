// Package subtext reads subtitle text that came from outside: it unpacks
// zip answers safely, finds the encoding, cleans the text, and moves cue
// timing (offset and frame-rate fixes) for SRT, ASS and SSA. Everything is
// done in memory with size caps and no regular expressions over whole
// files.
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

// Caps on what is read.
const (
	MaxText       = 5 << 20  // bytes of one subtitle
	MaxArchive    = 8 << 20  // bytes of one download
	MaxEntries    = 50       // entries in a zip
	MaxEntryText  = 5 << 20  // bytes of one entry once unpacked
	MaxUnpacked   = 20 << 20 // bytes of all entries once unpacked
	MaxEntryRatio = 100      // unpacked to packed, per entry
)

// Errors.
var (
	ErrTooBig      = errors.New("subtitle is too big")
	ErrArchive     = errors.New("archive refused")
	ErrNotSubtitle = errors.New("not a subtitle")
)

// Formats a subtitle can be saved as.
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
	Name   string // the archive entry's base name (only for choosing; never a path)
	Format string
	Data   []byte
}

// Unpack returns the subtitles in a download: the download itself when it
// is a subtitle, or the subtitle entries of a zip. RAR and 7z are refused.
// format is the site's word for the file type, used when the data does not
// say.
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

// TextSub says whether the head of a .sub file is clearly text (MicroDVD
// "{1}{25}..." or SubViewer "[INFORMATION]"): no NUL or other control
// bytes besides tab, CR and LF, and a first non-blank line that starts with
// "{" or "[". A picture (VobSub) .sub, or anything unclear, is not text.
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

// unzip reads the subtitle entries of a zip in memory. Entries that are
// links, hold "..", are absolute or name a drive are refused outright (the
// whole archive), as are zip bombs: too many entries, an entry too big or
// unpacking past MaxEntryRatio, or too much in all.
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
		// The header sizes may lie: read at most one byte past the cap.
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

// Clean turns subtitle bytes into UTF-8 text without a byte-order mark,
// NUL bytes or other control characters (tab, CR and LF stay). A UTF-8,
// UTF-16 or UTF-32 mark is honoured; valid UTF-8 stays as it is; anything
// else is read as Windows-1252. It refuses text with no timed cue.
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
		// MicroDVD: {start}{end}text
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
