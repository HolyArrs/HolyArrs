package mediafiles

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Our own tiny AVI and transport stream files, built byte by byte.

func le16(v uint16) []byte { return binary.LittleEndian.AppendUint16(nil, v) }
func le32(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }

func riffChunk(id string, parts ...[]byte) []byte {
	body := bytes.Join(parts, nil)
	out := append([]byte(id), le32(uint32(len(body)))...)
	out = append(out, body...)
	if len(body)%2 == 1 {
		out = append(out, 0)
	}
	return out
}

func riffList(kind string, parts ...[]byte) []byte {
	return riffChunk("LIST", append([][]byte{[]byte(kind)}, parts...)...)
}

func aviMain(usPerFrame, frames uint32) []byte {
	b := make([]byte, 56)
	binary.LittleEndian.PutUint32(b[0:], usPerFrame)
	binary.LittleEndian.PutUint32(b[16:], frames)
	return riffChunk("avih", b)
}

func aviStrh(kind string, scale, rate, length uint32) []byte {
	b := make([]byte, 56)
	copy(b, kind)
	binary.LittleEndian.PutUint32(b[20:], scale)
	binary.LittleEndian.PutUint32(b[24:], rate)
	binary.LittleEndian.PutUint32(b[32:], length)
	return riffChunk("strh", b)
}

func aviVideo(fourcc string, w, h int32, scale, rate uint32) []byte {
	b := make([]byte, 40)
	binary.LittleEndian.PutUint32(b[0:], 40)
	binary.LittleEndian.PutUint32(b[4:], uint32(w))
	binary.LittleEndian.PutUint32(b[8:], uint32(h))
	copy(b[16:], fourcc)
	return riffList("strl", aviStrh("vids", scale, rate, 0), riffChunk("strf", b))
}

func aviAudio(tag, channels uint16, name string) []byte {
	b := append(le16(tag), le16(channels)...)
	b = append(b, make([]byte, 14)...)
	parts := [][]byte{aviStrh("auds", 1, 48000, 0), riffChunk("strf", b)}
	if name != "" {
		parts = append(parts, riffChunk("strn", []byte(name+"\x00")))
	}
	return riffList("strl", parts...)
}

func aviFile(parts ...[]byte) []byte {
	body := append([]byte("AVI "), bytes.Join(parts, nil)...)
	body = append(body, riffList("movi", riffChunk("00dc", make([]byte, 64)))...)
	return riffChunk("RIFF", body)
}

func TestAVIFields(t *testing.T) {
	data := aviFile(riffList("hdrl", aviMain(41708, 1000),
		aviVideo("XVID", 720, -304, 1001, 24000),
		aviAudio(0x55, 2, "English"),
		aviAudio(0x2000, 6, ""),
	))
	got := inspectBytes(t, data)
	if got.Container != "avi" || got.VideoCodec != "xvid" || got.Width != 720 || got.Height != 304 || got.FrameRate != 23.976 {
		t.Fatalf("video: %+v", got)
	}
	if got.Duration != 41708*time.Millisecond {
		t.Errorf("duration %v", got.Duration)
	}
	if got.AudioTracks != 2 || len(got.AudioCodecs) != 2 || got.AudioCodecs[0] != "mp3" || got.AudioCodecs[1] != "ac3" ||
		got.AudioChannels[1] != 6 || !got.AudioUnknown || got.Status != "partial" || got.Tracks[1].Name != "English" {
		t.Errorf("audio: %+v", got)
	}
	if got.Resolution() != 480 {
		t.Errorf("resolution %d", got.Resolution())
	}
}

func TestAVIOpenDMLAndJunk(t *testing.T) {
	// A JUNK chunk before the header, and a file past 1 GiB whose real
	// frame count is only in the odml list.
	data := aviFile(riffChunk("JUNK", make([]byte, 31)), riffList("hdrl", aviMain(40000, 10),
		aviVideo("H264", 1920, 1080, 1, 25),
		riffList("odml", riffChunk("dmlh", le32(90000), make([]byte, 244))),
	))
	got := inspectBytes(t, data)
	if got.VideoCodec != "h264" || got.Resolution() != 1080 || got.Duration != time.Hour || got.FrameRate != 25 {
		t.Fatalf("%+v", got)
	}
}

func TestAVIOddInputs(t *testing.T) {
	// An unknown FourCC keeps the track with its own code; a header list
	// cut off by the end of the file still gives what is there.
	odd := aviFile(riffList("hdrl", aviMain(40000, 25), aviVideo("ABCD", 640, 480, 1, 25)))
	got := inspectBytes(t, odd)
	if got.VideoCodec != "" || got.Tracks[0].Codec != "ABCD" || got.Width != 640 || got.Status != "partial" {
		t.Errorf("odd codec: %+v", got)
	}
	whole := aviFile(riffList("hdrl", aviMain(40000, 25), aviVideo("DIVX", 640, 480, 1, 25), aviAudio(0x55, 2, "")))
	cut := whole[:len(whole)-120]
	if got, err := InspectReaderAt(bytes.NewReader(cut), int64(len(cut))); err != nil || got.VideoCodec != "xvid" {
		t.Errorf("cut: %+v %v", got, err)
	}
	// A huge size claim on every chunk.
	huge := []byte("RIFF\xff\xff\xff\xffAVI LIST\xff\xff\xff\x7fhdrlavih\xff\xff\xff\x7f")
	if _, err := InspectReaderAt(bytes.NewReader(huge), int64(len(huge))); err != nil {
		t.Errorf("huge sizes: %v", err)
	}
	// A frame count and frame time whose product wraps an int64 must not
	// come out as a believable length (it once gave 35m47s).
	wrap := aviFile(riffList("hdrl", aviMain(0x80000000, 0x80000001), aviVideo("DIVX", 640, 480, 0, 0)))
	if got := inspectBytes(t, wrap); got.Duration != 0 {
		t.Errorf("wrapped length: %v", got.Duration)
	}
	for _, n := range []int{0, 3, 11} {
		if got, err := InspectReaderAt(bytes.NewReader(whole[:n]), int64(n)); err != nil || got.Status != "unsupported" {
			t.Errorf("%d bytes: %+v %v", n, got, err)
		}
	}
}

// avcSPSBytes is an H.264 High profile SPS for 1920x1080 at 23.976.
func avcSPSBytes() []byte {
	w := &bitWriter{}
	w.put(8, 0x67)
	w.put(8, 100)
	w.put(16, 40)
	w.ue(0)
	w.ue(1) // 4:2:0
	w.ue(0) // 8 bit
	w.ue(0)
	w.put(1, 0)
	w.put(1, 0) // no scaling matrices
	w.ue(0)
	w.ue(0)
	w.ue(0)
	w.ue(4)
	w.put(1, 0)
	w.ue(119)
	w.ue(67)
	w.put(1, 1) // frames only
	w.put(1, 1)
	w.put(1, 1) // cropping
	w.ue(0)
	w.ue(0)
	w.ue(0)
	w.ue(4)
	w.put(1, 1) // VUI
	w.put(4, 0)
	w.put(1, 1) // timing
	w.put(32, 1001)
	w.put(32, 48000)
	w.put(1, 1)
	w.put(1, 1)
	return w.b
}

func tsPacket(pid int, start bool, payload []byte) []byte {
	if len(payload) > 184 {
		panic("payload too long")
	}
	p := []byte{0x47, byte(pid >> 8 & 0x1f), byte(pid), 0x10}
	if start {
		p[1] |= 0x40
	}
	if pad := 184 - len(payload); pad > 0 {
		p[3] = 0x30
		p = append(p, byte(pad-1))
		if pad > 1 {
			p = append(p, 0)
			p = append(p, bytes.Repeat([]byte{0xff}, pad-2)...)
		}
	}
	return append(p, payload...)
}

// tsPES splits one PES packet over as many TS packets as it needs.
func tsPES(pid, streamID int, pts int64, data []byte) []byte {
	head := []byte{0, 0, 1, byte(streamID), 0, 0, 0x80, 0x80, 5,
		byte(0x21 | pts>>29&0x0e), byte(pts >> 22), byte(pts>>14&0xfe | 1), byte(pts >> 7), byte(pts<<1&0xfe | 1)}
	all := append(head, data...)
	var out []byte
	for first := true; len(all) > 0; first = false {
		n := min(184, len(all))
		out = append(out, tsPacket(pid, first, all[:n])...)
		all = all[n:]
	}
	return out
}

func tsSection(pid int, table byte, body []byte) []byte {
	n := len(body) + 4
	sec := append([]byte{0, table, 0xb0 | byte(n>>8), byte(n)}, body...)
	return tsPacket(pid, true, append(sec, 0, 0, 0, 0))
}

type tsES struct {
	kind byte
	pid  int
	desc []byte
}

func tsTables(streams ...tsES) []byte {
	pat := tsSection(0, 0, []byte{0, 1, 0xc1, 0, 0, 0, 1, 0xe1, 0x00})
	body := []byte{0, 1, 0xc1, 0, 0, 0xe1, 0x01, 0xf0, 0}
	for _, s := range streams {
		body = append(body, s.kind, 0xe0|byte(s.pid>>8), byte(s.pid), 0xf0|byte(len(s.desc)>>8), byte(len(s.desc)))
		body = append(body, s.desc...)
	}
	return append(pat, tsSection(0x100, 2, body)...)
}

func langDesc(lang string, kind byte) []byte {
	return append([]byte{0x0a, 4}, append([]byte(lang), kind)...)
}

// ac3Frame is an AC-3 sync frame header for 5.1.
var ac3Frame = []byte{0x0b, 0x77, 0, 0, 0x14, 0x40, 0xe1, 0, 0, 0}

func m2ts(ts []byte) []byte {
	var out []byte
	for i := 0; i+188 <= len(ts); i += 188 {
		out = append(out, 0, 0, 0, 0)
		out = append(out, ts[i:i+188]...)
	}
	return out
}

func h264Stream(seconds int64) []byte {
	sps := append([]byte{0, 0, 0, 1}, avcSPSBytes()...)
	sps = append(sps, 0, 0, 0, 1, 0x68, 0xce, 0x3c, 0x80)
	data := tsTables(
		tsES{0x1b, 0x101, nil},
		tsES{0x81, 0x102, langDesc("eng", 0)},
		tsES{0x06, 0x103, append(langDesc("fre", 0), 0x6a, 1, 0)},
		tsES{0x06, 0x104, []byte{0x59, 8, 'g', 'e', 'r', 0x20, 0, 1, 0, 1}},
	)
	data = append(data, tsPES(0x101, 0xe0, 90000, sps)...)
	data = append(data, tsPES(0x102, 0xbd, 90000, ac3Frame)...)
	data = append(data, tsPacket(0x1fff, false, nil)...)
	return append(data, tsPES(0x101, 0xe0, 90000+seconds*90000, []byte{0, 0, 1, 9, 0xf0})...)
}

func TestTransportStreamFields(t *testing.T) {
	got := inspectBytes(t, h264Stream(60))
	if got.Container != "mpegts" || got.VideoCodec != "h264" || got.Width != 1920 || got.Height != 1080 ||
		got.FrameRate != 23.976 || got.BitDepth != 8 || got.Duration != time.Minute {
		t.Fatalf("video: %+v", got)
	}
	if got.AudioTracks != 2 || got.AudioChannels[0] != 6 || got.AudioCodecs[1] != "ac3" ||
		len(got.AudioLanguages) != 2 || got.AudioLanguages[0] != "en" || got.AudioLanguages[1] != "fr" || got.Status != "complete" {
		t.Errorf("audio: %+v", got)
	}
	if len(got.SubtitleTracks) != 1 || got.SubtitleTracks[0].Codec != "S_DVBSUB" || got.SubtitleTracks[0].Language != "de" ||
		!got.SubtitleTracks[0].HearingImpaired {
		t.Errorf("subtitles: %+v", got.SubtitleTracks)
	}
}

func TestM2TSFields(t *testing.T) {
	sps := append([]byte{0, 0, 0, 1}, mustHex(spsPQ)...)
	sps = append(sps, 0, 0, 0, 1, 0x44, 0x01)
	ts := tsTables(
		tsES{0x24, 0x1011, nil},
		tsES{0x83, 0x1100, langDesc("eng", 0)},
		tsES{0x90, 0x1200, langDesc("spa", 0)},
	)
	ts = append(ts, tsPES(0x1011, 0xe0, 1<<33-90000, sps)...) // the clock wraps past 2^33
	ts = append(ts, tsPES(0x1011, 0xe0, 90000*29, []byte{0, 0, 1, 0x46})...)
	file := m2ts(ts)
	got := inspectBytes(t, file)
	if got.Container != "m2ts" || got.VideoCodec != "hevc" || got.HDR != "HDR10" || got.Width == 0 || got.Duration != 30*time.Second {
		t.Fatalf("%+v", got)
	}
	if got.AudioCodecs[0] != "truehd" || got.AudioLanguages[0] != "en" || got.SubtitleTracks[0].Codec != "S_HDMV/PGS" || got.SubtitleTracks[0].Language != "es" {
		t.Errorf("%+v", got)
	}
	tracks, err := ReadTracks(bytes.NewReader(file), int64(len(file)))
	if err != nil || len(tracks) != 3 {
		t.Errorf("tracks %v %v", tracks, err)
	}
}

func TestTransportStreamDamage(t *testing.T) {
	// Packets with no program table: damaged, not a format we skip.
	nulls := bytes.Repeat(tsPacket(0x1fff, false, nil), 4)
	if _, err := InspectReaderAt(bytes.NewReader(nulls), int64(len(nulls))); !errors.Is(err, ErrMalformed) {
		t.Errorf("no tables: %v", err)
	}
	// Cut anywhere, the reader never fails on a stream that has its tables.
	whole := h264Stream(60)
	for n := 3 * 188; n <= len(whole); n += 97 {
		got, err := InspectReaderAt(bytes.NewReader(whole[:n]), int64(n))
		if err != nil && !errors.Is(err, ErrMalformed) {
			t.Fatalf("%d bytes: %v", n, err)
		}
		if err == nil && got.Container != "mpegts" {
			t.Fatalf("%d bytes: %+v", n, got)
		}
	}
	// A stray byte shifts every packet after it: the reader finds its step again.
	shifted := append(append(append([]byte{}, whole[:188*4]...), 0x00), whole[188*4:]...)
	if got := inspectBytes(t, shifted); got.VideoCodec != "h264" {
		t.Errorf("shifted: %+v", got)
	}
}

// bigTS is a 40 GiB stream: real packets at the head and the tail and
// empty ones between, made on the fly. It counts what is read.
type bigTS struct {
	head, tail []byte
	size, read int64
}

var nullPacket = tsPacket(0x1fff, false, nil)

func (b *bigTS) ReadAt(p []byte, off int64) (int, error) {
	b.read += int64(len(p))
	for i := range p {
		at := off + int64(i)
		switch {
		case at < int64(len(b.head)):
			p[i] = b.head[at]
		case at >= b.size-int64(len(b.tail)):
			p[i] = b.tail[at-(b.size-int64(len(b.tail)))]
		default:
			p[i] = nullPacket[at%188]
		}
	}
	return len(p), nil
}

func TestTransportStreamReadsBoundedBytes(t *testing.T) {
	whole := h264Stream(7200)
	last := len(whole) - 2*188
	big := &bigTS{head: whole[:last], tail: whole[last:], size: 188 * (40 << 30 / 188)}
	got, err := InspectReaderAt(big, big.size)
	if err != nil || got.Duration != 2*time.Hour || got.Width != 1920 {
		t.Fatalf("%+v %v", got, err)
	}
	if big.read > maxTSHead+maxTSTail+tsBlock { // blocks overlap by under a packet
		t.Errorf("read %d bytes", big.read)
	}
	// No video frame in the head at all: still stops at the head cap.
	big = &bigTS{head: tsTables(tsES{0x1b, 0x101, nil}), size: 188 * (40 << 30 / 188)}
	if _, err := InspectReaderAt(big, big.size); err != nil || big.read > maxTSHead+maxTSTail+tsBlock {
		t.Errorf("read %d bytes: %v", big.read, err)
	}
}

func FuzzInspectAVI(f *testing.F) {
	fuzzInspect(f,
		aviFile(riffList("hdrl", aviMain(41708, 1000), aviVideo("XVID", 720, 304, 1001, 24000), aviAudio(0x55, 2, "x"))),
		aviFile(riffList("hdrl", aviMain(40000, 10), riffList("odml", riffChunk("dmlh", le32(9))))),
	)
}

func FuzzInspectTS(f *testing.F) {
	fuzzInspect(f, h264Stream(5), m2ts(h264Stream(5)))
}

// TestInspectNewContainersOnDisk reads our own small files through the
// path the importer uses.
func TestInspectNewContainersOnDisk(t *testing.T) {
	dir := t.TempDir()
	files := map[string][]byte{
		"movie.avi":  aviFile(riffList("hdrl", aviMain(40000, 25), aviVideo("XVID", 720, 400, 1, 25), aviAudio(0x55, 2, ""))),
		"movie.ts":   h264Stream(60),
		"movie.m2ts": m2ts(h264Stream(60)),
		"movie.mts":  m2ts(h264Stream(60)),
		"empty.avi":  nil,
	}
	want := map[string]string{"movie.avi": "avi", "movie.ts": "mpegts", "movie.m2ts": "m2ts", "movie.mts": "m2ts", "empty.avi": ""}
	for name, data := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := Inspect(path)
		if err != nil || got.Container != want[name] {
			t.Errorf("%s: %+v %v", name, got, err)
		}
	}
}
