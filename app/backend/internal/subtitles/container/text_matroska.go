package container

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
)

// Errors from reading a text track.
var (
	ErrNoTrack  = errors.New("the video has no subtitle track with that number")
	ErrNotText  = errors.New("that subtitle track holds pictures or a format HolyArrs cannot read as text")
	ErrNotMKV   = errors.New("only text tracks inside MKV files can be read")
	ErrNoCues   = errors.New("the subtitle track holds no text")
	ErrEncoding = errors.New("the subtitle track is encrypted or packed in a way HolyArrs cannot read")
)

// TextCodec reports whether a Matroska codec id is a text subtitle that
// ReadText can turn into cues.
func TextCodec(c string) bool {
	switch strings.ToUpper(c) {
	case "S_TEXT/UTF8", "S_TEXT/ASCII", "S_TEXT/ASS", "S_TEXT/SSA", "S_ASS", "S_SSA", "S_TEXT/WEBVTT":
		return true
	}
	return false
}

// Limits on reading a text track.
const (
	maxFrame      = 64 << 10   // bytes in one subtitle frame, before and after unpacking
	maxSteps      = 50_000_000 // elements read in one file
	window        = 8 << 10
	defaultScale  = 1_000_000 // nanoseconds per Matroska tick
	openEndMS     = 3000      // length of a last cue that has no duration
	maxOpenEndMS  = 5000      // longest a cue without a duration lasts
	mkEBML        = 0x1a45dfa3
	mkSegment     = 0x18538067
	mkInfo        = 0x1549a966
	mkScale       = 0x2ad7b1
	mkTracks      = 0x1654ae6b
	mkTrackEntry  = 0xae
	mkTrackNumber = 0xd7
	mkCodecID     = 0x86
	mkEncodings   = 0x6d80
	mkEncoding    = 0x6240
	mkCompression = 0x5034
	mkCompAlgo    = 0x4254
	mkCompSetting = 0x4255
	mkEncryption  = 0x5035
	mkCluster     = 0x1f43b675
	mkTimecode    = 0xe7
	mkSimpleBlock = 0xa3
	mkBlockGroup  = 0xa0
	mkBlock       = 0xa1
	mkDuration    = 0x9b
)

// topLevel are the segment children that end a cluster of unknown size.
var topLevel = map[uint64]bool{mkCluster: true, mkInfo: true, mkTracks: true, 0x114d9b74: true, 0x1c53bb6b: true,
	0x1043a770: true, 0x1254c367: true, 0x1941a469: true}

// ExtractText opens the video at path, which must be a regular file, and
// reads the cues of its text track number track.
func ExtractText(ctx context.Context, path string, track int) ([]subtext.Cue, error) {
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
	return ReadText(ctx, f, fi.Size(), track)
}

// ReadText reads the cues of text track number track from a Matroska file of
// size bytes. It walks every cluster, reading only the frames of that track,
// with caps on frame size, cue count, text size and elements read. A file cut
// off part way gives the cues read so far.
func ReadText(ctx context.Context, r io.ReaderAt, size int64, track int) ([]subtext.Cue, error) {
	m := &mkv{e: walker{r: r, size: size}, track: uint64(track), scale: defaultScale}
	id, start, end, err := m.e.element(0, size)
	if err != nil || id != mkEBML {
		return nil, ErrNotMKV
	}
	for pos := end; pos < size; {
		id, start, end, err = m.e.element(pos, size)
		if err != nil {
			return nil, ErrNotMKV
		}
		if id == mkSegment {
			break
		}
		pos = end
	}
	if id != mkSegment {
		return nil, ErrNotMKV
	}
	if err := m.segment(ctx, start, end); err != nil {
		// Damage part way keeps the cues read before it; anything else stops.
		if !damaged(err) || len(m.cues) == 0 {
			return nil, err
		}
	}
	if !m.found {
		return nil, ErrNoTrack
	}
	if len(m.cues) == 0 {
		return nil, ErrNoCues
	}
	m.closeEnds()
	return m.cues, nil
}

type mkv struct {
	e      walker
	track  uint64
	scale  uint64
	codec  string
	found  bool
	zlib   bool
	strip  []byte
	cues   []subtext.Cue
	open   []bool // cue had no duration
	text   int
	clTime uint64
}

// damaged reports whether err comes from a damaged or cut-off file rather
// than from a refusal, a cap or a stop.
func damaged(err error) bool {
	return errors.Is(err, errMalformed) || errors.Is(err, io.ErrUnexpectedEOF)
}

func (m *mkv) segment(ctx context.Context, start, end int64) error {
	for pos := start; pos < end; {
		if err := ctx.Err(); err != nil {
			return err
		}
		id, cs, ce, err := m.e.element(pos, end)
		if err != nil {
			return err
		}
		switch id {
		case mkInfo:
			if err := m.info(cs, ce); err != nil {
				return err
			}
		case mkTracks:
			if err := m.tracks(cs, ce); err != nil {
				return err
			}
		case mkCluster:
			if !m.found {
				return ErrNoTrack // the tracks come before the media in every file HolyArrs reads
			}
			next, err := m.cluster(ctx, cs, ce)
			if err != nil {
				return err
			}
			ce = next
		}
		pos = ce
	}
	return nil
}

func (m *mkv) info(start, end int64) error {
	return m.e.each(start, end, func(id uint64, s, e int64) error {
		if id == mkScale {
			v, err := m.e.uint(s, e)
			if err == nil && v > 0 && v <= 1_000_000_000 {
				m.scale = v
			}
			return err
		}
		return nil
	})
}

func (m *mkv) tracks(start, end int64) error {
	return m.e.each(start, end, func(id uint64, s, e int64) error {
		if id != mkTrackEntry {
			return nil
		}
		var num uint64
		var codec string
		var zl, enc bool
		var strip []byte
		err := m.e.each(s, e, func(id uint64, s, e int64) error {
			var err error
			switch id {
			case mkTrackNumber:
				num, err = m.e.uint(s, e)
			case mkCodecID:
				var b []byte
				b, err = m.e.bytes(s, e, 64)
				codec = strings.TrimRight(string(b), "\x00")
			case mkEncodings:
				zl, strip, enc, err = m.encodings(s, e)
			}
			return err
		})
		if err != nil || num != m.track {
			return err
		}
		m.found, m.codec = true, codec
		if !TextCodec(codec) {
			return ErrNotText
		}
		if enc {
			return ErrEncoding
		}
		m.zlib, m.strip = zl, strip
		return nil
	})
}

// encodings reads how a track's frames are packed: zlib, a stripped header
// put back in front, or encryption, which is refused.
func (m *mkv) encodings(start, end int64) (zl bool, strip []byte, enc bool, err error) {
	err = m.e.each(start, end, func(id uint64, s, e int64) error {
		if id != mkEncoding {
			return nil
		}
		return m.e.each(s, e, func(id uint64, s, e int64) error {
			switch id {
			case mkEncryption:
				enc = true
			case mkCompression:
				algo := uint64(0)
				var set []byte
				err := m.e.each(s, e, func(id uint64, s, e int64) error {
					var err error
					switch id {
					case mkCompAlgo:
						algo, err = m.e.uint(s, e)
					case mkCompSetting:
						set, err = m.e.bytes(s, e, 256)
					}
					return err
				})
				switch {
				case err != nil:
					return err
				case algo == 0:
					zl = true
				case algo == 3:
					strip = set
				default:
					enc = true // bzlib and lzo are not read
				}
			}
			return nil
		})
	})
	return zl, strip, enc, err
}

// cluster reads one cluster and returns where it ends; a cluster of unknown
// size ends at the next top-level element.
func (m *mkv) cluster(ctx context.Context, start, end int64) (int64, error) {
	m.clTime = 0
	for pos := start; pos < end; {
		if err := ctx.Err(); err != nil {
			return pos, err
		}
		id, s, e, err := m.e.element(pos, end)
		if err != nil {
			return pos, err
		}
		if topLevel[id] {
			return pos, nil
		}
		switch id {
		case mkTimecode:
			if m.clTime, err = m.e.uint(s, e); err != nil {
				return pos, err
			}
		case mkSimpleBlock:
			if err := m.block(s, e, 0, false); err != nil {
				return pos, err
			}
		case mkBlockGroup:
			var bs, be int64 = -1, -1
			var dur uint64
			hasDur := false
			err := m.e.each(s, e, func(id uint64, s, e int64) error {
				switch id {
				case mkBlock:
					bs, be = s, e
				case mkDuration:
					var err error
					dur, err = m.e.uint(s, e)
					hasDur = err == nil
					return err
				}
				return nil
			})
			if err != nil {
				return pos, err
			}
			if bs >= 0 {
				if err := m.block(bs, be, dur, hasDur); err != nil {
					return pos, err
				}
			}
		}
		pos = e
	}
	return end, nil
}

// block reads one block of the wanted track into a cue.
func (m *mkv) block(start, end int64, dur uint64, hasDur bool) error {
	head, err := m.e.at(start, int(min(end-start, 12)))
	if err != nil {
		return err
	}
	num, n, ok := vint(head, false)
	if !ok || num != m.track || len(head) < n+3 {
		return nil
	}
	if head[n+2]&0x06 != 0 {
		return nil // laced frames never carry subtitles
	}
	rel := int64(int16(binary.BigEndian.Uint16(head[n:])))
	data, err := m.e.bytes(start+int64(n)+3, end, maxFrame)
	if err != nil {
		return nil // an oversized frame is skipped
	}
	if data, err = m.unpack(data); err != nil {
		return nil
	}
	text := m.frameText(data)
	if text == "" {
		return nil
	}
	if len(m.cues) >= subtext.MaxCues || m.text+len(text) > subtext.MaxText {
		return subtext.ErrTooBig
	}
	t := int64(m.clTime) + rel
	if t < 0 {
		t = 0
	}
	startMS := ms(uint64(t), m.scale)
	c := subtext.Cue{StartMS: startMS, Text: text}
	if hasDur && dur > 0 {
		c.EndMS = startMS + ms(dur, m.scale)
	}
	m.cues = append(m.cues, c)
	m.open = append(m.open, !(hasDur && dur > 0))
	m.text += len(text)
	return nil
}

func ms(ticks, scale uint64) int64 {
	v := float64(ticks) * float64(scale) / 1e6
	if v > 1e11 {
		v = 1e11
	}
	return int64(v + 0.5)
}

func (m *mkv) unpack(data []byte) ([]byte, error) {
	if len(m.strip) > 0 {
		data = append(append([]byte{}, m.strip...), data...)
	}
	if !m.zlib {
		return data, nil
	}
	zr, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	out, err := io.ReadAll(io.LimitReader(zr, maxFrame+1))
	if err != nil || len(out) > maxFrame {
		return nil, errors.New("frame too big")
	}
	return out, nil
}

func (m *mkv) frameText(data []byte) string {
	s := strings.ToValidUTF8(string(data), "")
	switch strings.ToUpper(m.codec) {
	case "S_TEXT/ASS", "S_TEXT/SSA", "S_ASS", "S_SSA":
		// ReadOrder, Layer, Style, Name, MarginL, MarginR, MarginV, Effect, Text
		parts := strings.SplitN(s, ",", 9)
		if len(parts) != 9 {
			return ""
		}
		return subtext.ASSText(parts[8])
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimSpace(strings.ReplaceAll(s, "\x00", ""))
}

// closeEnds gives cues with no duration an end: the next cue's start, at
// most a few seconds on.
func (m *mkv) closeEnds() {
	idx := make([]int, len(m.cues))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return m.cues[idx[a]].StartMS < m.cues[idx[b]].StartMS })
	cues, open := make([]subtext.Cue, len(idx)), make([]bool, len(idx))
	for k, i := range idx {
		cues[k], open[k] = m.cues[i], m.open[i]
	}
	m.cues, m.open = cues, open
	for i := range m.cues {
		if !m.open[i] {
			continue
		}
		end := m.cues[i].StartMS + openEndMS
		if i+1 < len(m.cues) && m.cues[i+1].StartMS > m.cues[i].StartMS {
			end = min(m.cues[i+1].StartMS, m.cues[i].StartMS+maxOpenEndMS)
		}
		m.cues[i].EndMS = end
	}
}

// walker reads EBML elements through a small window, so walking past large video
// frames costs one read per frame header.
type walker struct {
	r     io.ReaderAt
	size  int64
	buf   []byte
	off   int64
	steps int
}

var errMalformed = errors.New("the video file is damaged or not a Matroska file")

// at returns n bytes at pos. Short reads at the end of the file return what
// is there.
func (e *walker) at(pos int64, n int) ([]byte, error) {
	if pos < 0 || pos >= e.size || n <= 0 {
		return nil, errMalformed
	}
	if pos >= e.off && pos+int64(n) <= e.off+int64(len(e.buf)) {
		return e.buf[pos-e.off : pos-e.off+int64(n)], nil
	}
	want := max(n, window)
	want = int(min(int64(want), e.size-pos))
	if cap(e.buf) < want {
		e.buf = make([]byte, want)
	}
	e.buf = e.buf[:want]
	got, err := e.r.ReadAt(e.buf, pos)
	if got < want && err == nil {
		err = io.ErrUnexpectedEOF
	}
	e.buf, e.off = e.buf[:got], pos
	if got < n {
		if err == nil || errors.Is(err, io.EOF) {
			err = errMalformed
		}
		return nil, err
	}
	return e.buf[:n], nil
}

// element reads the element header at pos inside a parent ending at end. An
// unknown or oversized size runs to the parent's end, which only segments and
// clusters may do.
func (e *walker) element(pos, end int64) (id uint64, start, stop int64, err error) {
	if e.steps++; e.steps > maxSteps {
		return 0, 0, 0, errors.New("the video holds too many elements")
	}
	head, err := e.at(pos, int(min(12, e.size-pos)))
	if err != nil {
		return 0, 0, 0, err
	}
	id, n, ok := vint(head, true)
	if !ok || n > 4 {
		return 0, 0, 0, errMalformed
	}
	sz, m, ok := vint(head[n:], false)
	if !ok {
		return 0, 0, 0, errMalformed
	}
	start = pos + int64(n+m)
	unknown := sz == (uint64(1)<<(7*m))-1
	if unknown || sz > uint64(end-start) {
		if id != mkSegment && id != mkCluster {
			return 0, 0, 0, errMalformed
		}
		return id, start, end, nil
	}
	return id, start, start + int64(sz), nil
}

// each calls f for every child element between start and end.
func (e *walker) each(start, end int64, f func(id uint64, s, e int64) error) error {
	for pos := start; pos < end; {
		id, s, stop, err := e.element(pos, end)
		if err != nil {
			return err
		}
		if err := f(id, s, stop); err != nil {
			return err
		}
		pos = stop
	}
	return nil
}

func (e *walker) uint(start, end int64) (uint64, error) {
	if end-start > 8 {
		return 0, errMalformed
	}
	b, err := e.bytes(start, end, 8)
	if err != nil {
		return 0, err
	}
	var v uint64
	for _, x := range b {
		v = v<<8 | uint64(x)
	}
	return v, nil
}

func (e *walker) bytes(start, end int64, limit int) ([]byte, error) {
	n := end - start
	if n < 0 || n > int64(limit) {
		return nil, errMalformed
	}
	if n == 0 {
		return nil, nil
	}
	b, err := e.at(start, int(n))
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), b...), nil
}

// vint reads an EBML variable-length number from the start of b. keep keeps
// the length marker, as element ids do.
func vint(b []byte, keep bool) (uint64, int, bool) {
	if len(b) == 0 || b[0] == 0 {
		return 0, 0, false
	}
	n := 1
	for mask := byte(0x80); b[0]&mask == 0; mask >>= 1 {
		n++
	}
	if n > 8 || len(b) < n {
		return 0, 0, false
	}
	v := uint64(b[0])
	if !keep {
		v &= uint64(0xff >> n)
	}
	for _, x := range b[1:n] {
		v = v<<8 | uint64(x)
	}
	return v, n, true
}
