// Media header reading: a bounded subset of media container headers.
//
// One reader serves Matroska/WebM and MP4/MOV. It reads only headers (never
// samples), and hard limits on elements, nesting, tracks and bytes per read
// keep a hostile file from hanging it or making it allocate much.

package mediafiles

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/language"
)

const (
	maxElements = 4096
	maxDepth    = 12
	maxTracks   = 128
	maxString   = 256
	// maxPrivate is the most bytes of one codec setup (Matroska
	// CodecPrivate, an MP4 sample entry) read into memory.
	maxPrivate = 8192
)

// Errors. Every reading error wraps ErrMalformed or ErrLimit, except plain
// I/O errors from the reader itself.
var (
	ErrUnsupported = errors.New("not a Matroska or MP4 file")
	ErrMalformed   = errors.New("malformed container")
	ErrLimit       = errors.New("container is past a reading limit")
)

// Track is one track as the container headers describe it.
type Track struct {
	Kind     string // "video", "audio", "subtitle"; "" for any other kind
	Number   int    // the container's own track number
	Codec    string // codec id as the file writes it: V_MPEGH/ISO/HEVC, hvc1, A_AAC, S_TEXT/UTF8, tx3g
	Format   string // short codec name (hevc, h264, aac, flac ...); "" when not known
	Language string // as the file writes it (eng, en-US); "" when the file names none
	Name     string

	Default         bool
	Forced          bool
	HearingImpaired bool

	// Video.
	Width, Height int
	// DisplayWidth is the on-screen width at the coded height when the
	// pixels are not square (an anamorphic DVD); 0 when they are or the
	// headers do not say.
	DisplayWidth int
	FrameRate    float64 // frames per second, 0 when not written
	BitDepth     int
	HDR          string // "", "HDR10", "HLG", "DV" or "DV+HDR10"

	// Audio.
	Channels int
}

// SubtitleTrack is one subtitle track inside a video.
type SubtitleTrack struct {
	Codec           string // as the file writes it: S_TEXT/UTF8, S_HDMV/PGS, tx3g ...
	Language        string // ISO 639-1 base ("en"); "" when unknown
	Forced          bool
	Default         bool
	HearingImpaired bool
}

// Info contains facts read from media container headers.
type Info struct {
	Container      string
	VideoCodec     string
	Status         string
	AudioLanguages []string
	AudioTracks    int
	AudioUnknown   bool
	// AudioCodecs are the audio tracks' codecs ("alac", "flac", "aac",
	// "mp3", ...); a track whose codec is not known adds nothing.
	AudioCodecs []string
	// AudioChannels has one entry per audio track, in file order; 0 when
	// the headers do not say.
	AudioChannels []int

	// Width, Height, DisplayWidth, FrameRate, BitDepth and HDR describe
	// the first video track.
	Width, Height int
	DisplayWidth  int
	FrameRate     float64
	BitDepth      int
	HDR           string

	Duration time.Duration
	// Bitrate is the whole file's average in bits per second (file size
	// over duration); 0 when the duration is not known.
	Bitrate int64

	SubtitleTracks []SubtitleTrack
	// Tracks are every track the headers list, in file order.
	Tracks []Track
}

// headers is what one container reader found.
type headers struct {
	container string
	tracks    []Track
	duration  time.Duration
	found     bool // the movie header (Segment, moov) was there
}

// losslessCodecs are the audio codecs that keep every sample.
var losslessCodecs = map[string]bool{"alac": true, "flac": true, "wavpack": true, "pcm": true, "tta": true, "truehd": true}

// AudioWord is the one codec every audio track shares, upper case ("ALAC",
// "AAC"); "" when there is none or they differ.
func (i Info) AudioWord() string {
	if len(i.AudioCodecs) == 0 {
		return ""
	}
	for _, c := range i.AudioCodecs[1:] {
		if c != i.AudioCodecs[0] {
			return ""
		}
	}
	return strings.ToUpper(i.AudioCodecs[0])
}

// Lossless says whether every audio track read is lossless; known is false
// when no audio codec could be read (an .m4a may hold AAC or ALAC).
func (i Info) Lossless() (lossless, known bool) {
	if len(i.AudioCodecs) == 0 {
		return false, false
	}
	for _, c := range i.AudioCodecs {
		if !losslessCodecs[c] {
			return false, true
		}
	}
	return true, true
}

// Resolution names the picture height class of the first video track: 2160,
// 1080, 720, 576 or 480, or 0 when no size was read. Files are often
// cropped (a 1920x800 film is still 1080), so a track counts as a class when
// either its width or its height comes close to that class's full frame.
// The width is the on-screen one when the pixels are not square: a cropped
// anamorphic PAL DVD (720x436 shown as 1018x436) is still 576.
func (i Info) Resolution() int {
	w, h := max(i.Width, i.DisplayWidth), i.Height
	switch {
	case w <= 0 && h <= 0:
		return 0
	case w >= 3200 || h >= 2100:
		return 2160
	case w >= 1800 || h >= 1000:
		return 1080
	case w >= 1200 || h >= 700:
		return 720
	case w >= 1000 || h >= 550:
		return 576
	default:
		return 480
	}
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
	h, err := readHeaders(r, size)
	if errors.Is(err, ErrUnsupported) {
		return Info{Status: "unsupported"}, nil
	}
	if err != nil {
		return Info{}, err
	}
	return buildInfo(h, size), nil
}

// ReadTracks lists every track of a Matroska/WebM or MP4/MOV file of size
// bytes. It returns ErrUnsupported for other files and ErrMalformed when the
// movie header is missing.
func ReadTracks(r io.ReaderAt, size int64) ([]Track, error) {
	h, err := readHeaders(r, size)
	if err != nil {
		return nil, err
	}
	if !h.found {
		return nil, malformed("inspect %s: movie header missing", h.container)
	}
	return h.tracks, nil
}

func readHeaders(r io.ReaderAt, size int64) (headers, error) {
	if r == nil || size < 0 {
		return headers{}, fmt.Errorf("inspect media: invalid input size")
	}
	if size < 4 {
		return headers{}, ErrUnsupported
	}
	prefix, err := readAt(r, 0, min64(size, 8))
	if err != nil {
		return headers{}, fmt.Errorf("inspect media header: %w", err)
	}
	switch {
	case len(prefix) >= 4 && string(prefix[:4]) == "\x1a\x45\xdf\xa3":
		return inspectMatroska(r, size)
	case looksLikeMP4(prefix):
		return inspectMP4(r, size)
	}
	return headers{}, ErrUnsupported
}

func buildInfo(h headers, size int64) Info {
	info := Info{Container: h.container, Duration: h.duration, Tracks: h.tracks}
	var videoCodecs []string
	firstVideo := true
	for _, t := range h.tracks {
		switch t.Kind {
		case "video":
			videoCodecs = append(videoCodecs, t.Format)
			if firstVideo {
				firstVideo = false
				info.Width, info.Height, info.FrameRate, info.BitDepth, info.HDR = t.Width, t.Height, t.FrameRate, t.BitDepth, t.HDR
				info.DisplayWidth = t.DisplayWidth
			}
		case "audio":
			info.AudioTracks++
			info.AudioChannels = append(info.AudioChannels, t.Channels)
			if t.Format != "" {
				info.AudioCodecs = append(info.AudioCodecs, t.Format)
			}
			if normalized, ok := normalizeLanguage(t.Language); ok {
				info.AudioLanguages = append(info.AudioLanguages, normalized)
			} else {
				info.AudioUnknown = true
			}
		case "subtitle":
			lang, _ := normalizeLanguage(t.Language)
			info.SubtitleTracks = append(info.SubtitleTracks, SubtitleTrack{
				Codec: t.Codec, Language: lang, Forced: t.Forced, Default: t.Default, HearingImpaired: t.HearingImpaired,
			})
		}
	}
	info.VideoCodec = oneCodec(videoCodecs)
	if seconds := info.Duration.Seconds(); seconds >= 1 {
		info.Bitrate = int64(float64(size) * 8 / seconds)
	}
	sort.Strings(info.AudioLanguages)
	info.AudioLanguages = compactStrings(info.AudioLanguages)
	if info.VideoCodec == "" || info.AudioUnknown {
		info.Status = "partial"
	} else {
		info.Status = "complete"
	}
	return info
}

// seconds turns a count of ticks at scale ticks per second into a duration;
// 0 when either is missing or the result is past any real file (a year).
func seconds(ticks float64, scale float64) time.Duration {
	if scale <= 0 || ticks <= 0 || math.IsNaN(ticks) || math.IsInf(ticks, 0) {
		return 0
	}
	s := ticks / scale
	if s > 365*24*3600 {
		return 0
	}
	return time.Duration(s * float64(time.Second))
}

// roundRate keeps three decimals of a frame rate (23.976); 0 for rates no
// video has.
func roundRate(fps float64) float64 {
	if fps <= 0 || fps > 1000 || math.IsNaN(fps) {
		return 0
	}
	return math.Round(fps*1000) / 1000
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

// cut keeps at most maxString bytes of a string read from a file, without
// NULs and on a character boundary.
func cut(b []byte) string {
	s := string(b)
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	if len(s) > maxString {
		s = s[:maxString]
	}
	return strings.ToValidUTF8(s, "")
}

func malformed(format string, a ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, a...), ErrMalformed)
}

func overLimit(format string, a ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, a...), ErrLimit)
}

func readAt(r io.ReaderAt, offset, length int64) ([]byte, error) {
	if offset < 0 || length < 0 || length > int64(^uint(0)>>1) {
		return nil, malformed("invalid read range")
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
		return nil, fmt.Errorf("%w: %w", ErrMalformed, io.ErrUnexpectedEOF)
	}
	return b, nil
}

// displayWidth gives the on-screen width of a w x h picture shown at the
// aspect dw:dh, or 0 when that is the same as w or not known.
func displayWidth(w, h int, dw, dh uint64) int {
	if w <= 0 || h <= 0 || h > 1<<20 || dw == 0 || dh == 0 || dw > 1<<40 || dh > 1<<40 {
		return 0
	}
	d := uint64(h) * dw / dh
	if d == uint64(w) || d > 1<<20 {
		return 0
	}
	return int(d)
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
