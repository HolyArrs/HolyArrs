package mediainfo

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"testing"
	"time"
)

// The HEVC parameter sets below come from ffmpeg/x265 encoding three frames
// of the testsrc pattern (64x48, 10 bit): no real media.
const (
	// hvcC header fields (profile, level, 10-bit luma) without arrays.
	hvccHeader = "0102200000009000000000001ef000fcfdfafa00000f"
	// SPS with VUI colour description: BT.2020, PQ (16).
	spsPQ = "42010102200000030090000003000003001ea020831365959ae4caf016a1220120800001f480002ee004a2"
	// SPS with VUI colour description: BT.2020, HLG (18).
	spsHLG = "42010102200000030090000003000003001ea020831365959ae4caf016a122412080000003008000000c84"
)

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// hvcc builds an hvcC record holding one SPS.
func hvcc(sps string) []byte {
	b := mustHex(hvccHeader)
	s := mustHex(sps)
	b = append(b, 1, 0xa1, 0, 1, byte(len(s)>>8), byte(len(s)))
	return append(b, s...)
}

// dovi builds a Dolby Vision configuration record.
func dovi(profile, compat byte) []byte {
	b := make([]byte, 24)
	b[0], b[2], b[3], b[4] = 1, profile<<1, 6<<3|5, compat<<4
	return b
}

// --- Matroska builders ---

func mkvID(id uint32) []byte {
	switch {
	case id > 0xffffff:
		return []byte{byte(id >> 24), byte(id >> 16), byte(id >> 8), byte(id)}
	case id > 0xffff:
		return []byte{byte(id >> 16), byte(id >> 8), byte(id)}
	case id > 0xff:
		return []byte{byte(id >> 8), byte(id)}
	}
	return []byte{byte(id)}
}

func mkvEl(id uint32, parts ...[]byte) []byte {
	return ebmlFixtureElement(mkvID(id), bytes.Join(parts, nil))
}

func mkvUint(id uint32, v uint64) []byte {
	b := binary.BigEndian.AppendUint64(nil, v)
	for len(b) > 1 && b[0] == 0 {
		b = b[1:]
	}
	return mkvEl(id, b)
}

func mkvStr(id uint32, s string) []byte { return mkvEl(id, []byte(s)) }

func mkvFloat(id uint32, v float64) []byte {
	return mkvEl(id, binary.BigEndian.AppendUint64(nil, math.Float64bits(v)))
}

func mkvMovie(segment ...[]byte) []byte {
	header := mkvEl(ebmlHeaderID, mkvStr(ebmlDocTypeID, "matroska"))
	return append(header, mkvEl(ebmlSegmentID, segment...)...)
}

func mkvInfo(scale uint64, duration float64) []byte {
	return mkvEl(ebmlInfoID, mkvUint(ebmlTimecodeScale, scale), mkvFloat(ebmlDurationID, duration))
}

func mkvEntry(kind uint64, number uint64, codec string, parts ...[]byte) []byte {
	head := append(mkvUint(ebmlTrackNumberID, number), mkvUint(ebmlTrackTypeID, kind)...)
	head = append(head, mkvStr(ebmlCodecID, codec)...)
	return mkvEl(ebmlTrackEntryID, append([][]byte{head}, parts...)...)
}

func mkvDVMapping(record []byte) []byte {
	return mkvEl(ebmlBlockAddMap, mkvUint(ebmlBlockAddType, blockAddDVCC), mkvEl(ebmlBlockAddExtra, record))
}

func inspectBytes(t *testing.T, data []byte) Info {
	t.Helper()
	got, err := InspectReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestMatroskaFields(t *testing.T) {
	video := mkvEntry(1, 1, "V_MPEGH/ISO/HEVC",
		mkvUint(ebmlDefaultDur, 41708333),
		mkvEl(ebmlVideoID,
			mkvUint(ebmlPixelWidth, 3840), mkvUint(ebmlPixelHeight, 2160),
			mkvEl(ebmlColourID, mkvUint(ebmlBitsPerChan, 10), mkvUint(ebmlTransferID, 16), mkvEl(ebmlMasteringID))),
		mkvDVMapping(dovi(8, 1)))
	eac3 := mkvEntry(2, 2, "A_EAC3", mkvStr(ebmlLanguageID, "eng"), mkvEl(ebmlAudioID, mkvUint(ebmlChannelsID, 6)))
	aac := mkvEntry(2, 3, "A_AAC", mkvStr(ebmlLanguageID, "ger"))
	pgs := mkvEntry(0x11, 4, "S_HDMV/PGS", mkvStr(ebmlLanguageID, "fre"), mkvStr(ebmlNameID, "Forced"),
		mkvUint(ebmlFlagForced, 1), mkvUint(ebmlFlagDefault, 0))
	data := mkvMovie(mkvInfo(1000000, 5400000), mkvEl(ebmlTracksID, video, eac3, aac, pgs))

	got := inspectBytes(t, data)
	if got.Width != 3840 || got.Height != 2160 || got.Resolution() != 2160 {
		t.Errorf("size %dx%d res %d", got.Width, got.Height, got.Resolution())
	}
	if got.FrameRate != 23.976 || got.BitDepth != 10 || got.HDR != "DV+HDR10" {
		t.Errorf("fps %v depth %d hdr %q", got.FrameRate, got.BitDepth, got.HDR)
	}
	if got.Duration != 90*time.Minute {
		t.Errorf("duration %v", got.Duration)
	}
	if want := int64(len(data)) * 8 / 5400; got.Bitrate != want {
		t.Errorf("bitrate %d, want %d", got.Bitrate, want)
	}
	if len(got.AudioChannels) != 2 || got.AudioChannels[0] != 6 || got.AudioChannels[1] != 1 {
		t.Errorf("channels %v", got.AudioChannels)
	}
	if len(got.SubtitleTracks) != 1 {
		t.Fatalf("subtitles %+v", got.SubtitleTracks)
	}
	if s := got.SubtitleTracks[0]; s.Codec != "S_HDMV/PGS" || s.Language != "fr" || !s.Forced || s.Default {
		t.Errorf("subtitle %+v", s)
	}
	if len(got.Tracks) != 4 || got.Tracks[3].Number != 4 || got.Tracks[3].Name != "Forced" || got.Tracks[0].Format != "hevc" {
		t.Errorf("tracks %+v", got.Tracks)
	}
	if got.Status != "complete" || got.VideoCodec != "hevc" {
		t.Errorf("status %q codec %q", got.Status, got.VideoCodec)
	}
}

func TestMatroskaHDRKinds(t *testing.T) {
	colour := func(parts ...[]byte) []byte { return mkvEl(ebmlVideoID, mkvEl(ebmlColourID, parts...)) }
	for name, c := range map[string]struct {
		parts [][]byte
		codec string
		hdr   string
		depth int
	}{
		"hlg":            {[][]byte{colour(mkvUint(ebmlTransferID, 18))}, "V_MPEGH/ISO/HEVC", "HLG", 0},
		"mastering only": {[][]byte{colour(mkvEl(ebmlMasteringID), mkvUint(ebmlMaxCLL, 1000))}, "V_MPEGH/ISO/HEVC", "HDR10", 0},
		"sdr":            {[][]byte{colour(mkvUint(ebmlTransferID, 1), mkvUint(ebmlBitsPerChan, 8))}, "V_MPEGH/ISO/HEVC", "", 8},
		"dv profile 5":   {[][]byte{mkvDVMapping(dovi(5, 0))}, "V_MPEGH/ISO/HEVC", "DV", 0},
		"dv profile 7":   {[][]byte{mkvDVMapping(dovi(7, 6))}, "V_MPEGH/ISO/HEVC", "DV+HDR10", 0},
		"pq in hvcC sps": {[][]byte{mkvEl(ebmlCodecPrivate, hvcc(spsPQ))}, "V_MPEGH/ISO/HEVC", "HDR10", 10},
		"hlg in hvcC":    {[][]byte{mkvEl(ebmlCodecPrivate, hvcc(spsHLG))}, "V_MPEGH/ISO/HEVC", "HLG", 10},
		"av1C 12 bit":    {[][]byte{mkvEl(ebmlCodecPrivate, []byte{0x81, 0x08, 0x60, 0})}, "V_AV1", "", 12},
	} {
		data := mkvMovie(mkvEl(ebmlTracksID, mkvEntry(1, 1, c.codec, c.parts...)))
		got := inspectBytes(t, data)
		if got.HDR != c.hdr || got.BitDepth != c.depth {
			t.Errorf("%s: hdr %q depth %d", name, got.HDR, got.BitDepth)
		}
	}
}

// Info after Tracks is still read, and a SeekHead leads past a Cluster to
// headers placed after it.
func TestMatroskaHeadersInAnyOrder(t *testing.T) {
	tracks := mkvEl(ebmlTracksID, mkvEntry(1, 1, "V_AV1"))
	info := mkvEl(ebmlInfoID, mkvUint(ebmlTimecodeScale, 1000), mkvEl(ebmlDurationID, binary.BigEndian.AppendUint32(nil, math.Float32bits(2e6))))
	got := inspectBytes(t, mkvMovie(tracks, info))
	if got.Duration != 2*time.Second || got.VideoCodec != "av1" {
		t.Errorf("info after tracks: %v %q", got.Duration, got.VideoCodec)
	}

	cluster := mkvEl(ebmlClusterID, []byte{0xe7, 0x81, 0x00})
	seekHead := func(at int) []byte {
		return mkvEl(ebmlSeekHeadID, mkvEl(ebmlSeekID, mkvEl(ebmlSeekIDID, mkvID(ebmlInfoID)), mkvUint(ebmlSeekPosition, uint64(at))))
	}
	head := seekHead(0)
	head = seekHead(len(head) + len(tracks) + len(cluster))
	got = inspectBytes(t, mkvMovie(head, tracks, cluster, info))
	if got.Duration != 2*time.Second {
		t.Errorf("info after cluster: %v", got.Duration)
	}
}

// --- MP4 builders ---

func bx(kind string, parts ...[]byte) []byte { return mp4Box(kind, bytes.Join(parts, nil)) }

func u16(v int) []byte    { return binary.BigEndian.AppendUint16(nil, uint16(v)) }
func u32(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }

func mvhd(timescale, duration uint32) []byte {
	return bx("mvhd", make([]byte, 12), u32(timescale), u32(duration), make([]byte, 80))
}

func tkhd(id uint32, enabled bool, w, h int) []byte {
	b := make([]byte, 84)
	if enabled {
		b[3] = 1
	}
	binary.BigEndian.PutUint32(b[12:], id)
	binary.BigEndian.PutUint32(b[76:], uint32(w)<<16)
	binary.BigEndian.PutUint32(b[80:], uint32(h)<<16)
	return bx("tkhd", b)
}

func mdhd(timescale, duration uint32, lang string) []byte {
	return bx("mdhd", make([]byte, 12), u32(timescale), u32(duration), u16(int(packLanguage(lang))), make([]byte, 2))
}

func hdlr(handler, name string) []byte {
	return bx("hdlr", make([]byte, 8), []byte(handler), make([]byte, 12), []byte(name), []byte{0})
}

func visualEntry(kind string, w, h int, children ...[]byte) []byte {
	b := make([]byte, 78)
	b[7] = 1
	binary.BigEndian.PutUint16(b[24:], uint16(w))
	binary.BigEndian.PutUint16(b[26:], uint16(h))
	return bx(kind, append([][]byte{b}, children...)...)
}

func audioEntry(kind string, channels int, children ...[]byte) []byte {
	b := make([]byte, 28)
	b[7] = 1
	binary.BigEndian.PutUint16(b[16:], uint16(channels))
	return bx(kind, append([][]byte{b}, children...)...)
}

func stts(entries ...uint32) []byte {
	return bx("stts", make([]byte, 4), u32(uint32(len(entries)/2)), func() []byte {
		var b []byte
		for _, e := range entries {
			b = append(b, u32(e)...)
		}
		return b
	}())
}

func trak(head []byte, timescale uint32, handler, lang, name string, entry []byte, extra ...[]byte) []byte {
	stbl := bx("stbl", append([][]byte{bx("stsd", make([]byte, 4), u32(1), entry)}, extra...)...)
	return bx("trak", head, bx("mdia", mdhd(timescale, 0, lang), hdlr(handler, name), bx("minf", stbl)))
}

func mp4File(moov ...[]byte) []byte {
	return append(mp4Box("ftyp", []byte("isom\x00\x00\x00\x00isom")), bx("moov", moov...)...)
}

func TestMP4Fields(t *testing.T) {
	video := trak(tkhd(1, true, 3840, 1600), 24000, "vide", "und", "",
		visualEntry("hvc1", 3840, 2160,
			bx("hvcC", hvcc(spsHLG)), // the colr box wins over the SPS
			bx("colr", []byte("nclx"), u16(9), u16(16), u16(9), []byte{0}),
			bx("mdcv", make([]byte, 24)),
			bx("dvcC", dovi(8, 1))),
		stts(100, 1001))
	eac3 := trak(tkhd(2, true, 0, 0), 48000, "soun", "eng", "", audioEntry("ec-3", 2,
		bx("dec3", []byte{0x01, 0x00, 0x20, 0x0f, 0x02, 0x80}))) // 5.1 + back pair
	ac3 := trak(tkhd(3, false, 0, 0), 48000, "soun", "spa", "", audioEntry("ac-3", 2, bx("dac3", []byte{0x10, 0x3c, 0x40})))
	aac := trak(tkhd(4, false, 0, 0), 48000, "soun", "jpn", "", audioEntry("mp4a", 2))
	tx3g := make([]byte, 30)
	tx3g[7], tx3g[8] = 1, 0x80 // every sample forced
	sub := trak(tkhd(5, false, 0, 0), 1000, "sbtl", "fre", "French", bx("tx3g", tx3g))
	data := mp4File(mvhd(1000, 90*60*1000), video, eac3, ac3, aac, sub)

	got := inspectBytes(t, data)
	if got.Width != 3840 || got.Height != 2160 || got.FrameRate != 23.976 || got.BitDepth != 10 || got.HDR != "DV+HDR10" {
		t.Errorf("video %dx%d fps %v depth %d hdr %q", got.Width, got.Height, got.FrameRate, got.BitDepth, got.HDR)
	}
	if got.Duration != 90*time.Minute || got.Bitrate != int64(len(data))*8/5400 {
		t.Errorf("duration %v bitrate %d", got.Duration, got.Bitrate)
	}
	if want := []int{8, 6, 2}; len(got.AudioChannels) != 3 || got.AudioChannels[0] != want[0] || got.AudioChannels[1] != want[1] || got.AudioChannels[2] != want[2] {
		t.Errorf("channels %v", got.AudioChannels)
	}
	if len(got.SubtitleTracks) != 1 {
		t.Fatalf("subtitles %+v", got.SubtitleTracks)
	}
	if s := got.SubtitleTracks[0]; s.Codec != "tx3g" || s.Language != "fr" || !s.Forced || s.Default {
		t.Errorf("subtitle %+v", s)
	}
	if got.Tracks[4].Number != 5 || got.Tracks[4].Name != "French" || !got.Tracks[0].Default || got.Tracks[2].Default {
		t.Errorf("tracks %+v", got.Tracks)
	}
}

func TestMP4FallbacksAndOtherCodecs(t *testing.T) {
	// No mvhd duration: the longest track's mdhd counts. A short sample
	// entry leaves the tkhd size. avcC High 10 gives 10 bit.
	avcc := []byte{1, 110, 0, 40, 0xff, 0xe1, 0, 2, 0x67, 0x6e, 1, 0, 1, 0x68, 0xfd, 0xfa, 0xfa, 0}
	head := bx("trak", tkhd(1, true, 1280, 720), bx("mdia",
		bx("mdhd", make([]byte, 12), u32(25), u32(250), u16(int(packLanguage("und"))), make([]byte, 2)),
		hdlr("vide", ""), bx("minf", bx("stbl", bx("stsd", make([]byte, 4), u32(1), visualEntry("avc1", 0, 0, bx("avcC", avcc))), stts(5, 1, 245, 1)))))
	got := inspectBytes(t, mp4File(head))
	if got.Duration != 10*time.Second || got.Width != 1280 || got.Height != 720 || got.BitDepth != 10 || got.FrameRate != 25 || got.Resolution() != 720 {
		t.Errorf("fallbacks: %v %dx%d depth %d fps %v", got.Duration, got.Width, got.Height, got.BitDepth, got.FrameRate)
	}
	// HLG from colr; av1C 10 bit.
	av1 := trak(tkhd(1, true, 0, 0), 30, "vide", "und", "", visualEntry("av01", 1920, 1080,
		bx("av1C", []byte{0x81, 0x08, 0x40, 0}), bx("colr", []byte("nclx"), u16(9), u16(18), u16(9), []byte{0})))
	got = inspectBytes(t, mp4File(av1))
	if got.HDR != "HLG" || got.BitDepth != 10 || got.VideoCodec != "av1" || got.Resolution() != 1080 {
		t.Errorf("av1: %+v", got)
	}
}

func TestCodecRecords(t *testing.T) {
	if n := ac3Channels("dac3", []byte{0x10, 0x3c, 0x40}); n != 6 {
		t.Errorf("dac3 5.1 = %d", n)
	}
	if n := ac3Channels("dac3", []byte{0x10, 0x10, 0x40}); n != 2 {
		t.Errorf("dac3 stereo = %d", n)
	}
	if n := ac3Channels("dec3", []byte{0x01, 0x00, 0x20, 0x0f, 0x00}); n != 6 {
		t.Errorf("dec3 5.1 = %d", n)
	}
	if n := spsTransfer(unescape(mustHex(spsPQ)[2:])); n != 16 {
		t.Errorf("PQ sps transfer = %d", n)
	}
	if n := hevcTransfer(hvcc(spsHLG)); n != 18 {
		t.Errorf("HLG hvcC transfer = %d", n)
	}
	if got := unescape([]byte{0, 0, 3, 1, 0, 0, 3}); !bytes.Equal(got, []byte{0, 0, 1, 0, 0}) {
		t.Errorf("unescape %x", got)
	}
	for _, cut := range []int{0, 5, 10, 20, 40} { // truncated SPS: no panic, no answer
		if n := spsTransfer(mustHex(spsPQ)[2 : 2+cut]); n != 0 {
			t.Errorf("cut %d gave %d", cut, n)
		}
	}
	for c, want := range map[colour]string{
		{transfer: 16, dvCompat: -1}:           "HDR10",
		{transfer: 18, dvCompat: -1}:           "HLG",
		{transfer: 1, dvCompat: -1}:            "",
		{transfer: 1, mastering: true}:         "",
		{mastering: true, dvCompat: -1}:        "HDR10",
		{dv: true, dvCompat: 4, transfer: 18}:  "DV",
		{dv: true, dvCompat: -1, transfer: 16}: "DV+HDR10",
	} {
		if got := c.hdrName(); got != want {
			t.Errorf("%+v: %q, want %q", c, got, want)
		}
	}
}

func TestResolution(t *testing.T) {
	for _, c := range []struct{ w, h, want int }{
		{3840, 2160, 2160}, {3840, 1600, 2160}, {1920, 1080, 1080}, {1920, 800, 1080}, {1440, 1080, 1080},
		{1280, 720, 720}, {1280, 536, 720}, {720, 576, 576}, {720, 480, 480}, {640, 360, 480}, {0, 0, 0},
	} {
		if got := (Info{Width: c.w, Height: c.h}).Resolution(); got != c.want {
			t.Errorf("%dx%d = %d, want %d", c.w, c.h, got, c.want)
		}
	}
}

func TestReadTracksErrors(t *testing.T) {
	if _, err := ReadTracks(bytes.NewReader([]byte("RIFF....AVI ")), 12); !errors.Is(err, ErrUnsupported) {
		t.Errorf("avi: %v", err)
	}
	none := mp4Box("ftyp", []byte("isom"))
	if _, err := ReadTracks(bytes.NewReader(none), int64(len(none))); !errors.Is(err, ErrMalformed) {
		t.Errorf("no moov: %v", err)
	}
	if _, err := ReadTracks(bytes.NewReader(none[:10]), 20); !errors.Is(err, ErrMalformed) {
		t.Errorf("short read: %v", err)
	}
}

// limitedReaderAt fails the test when one read asks for more than the
// reader's per-read cap: every read allocates exactly what it asks for.
type limitedReaderAt struct {
	t *testing.T
	r *bytes.Reader
}

func (l limitedReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if len(p) > maxPrivate {
		l.t.Fatalf("read of %d bytes at %d", len(p), off)
	}
	return l.r.ReadAt(p, off)
}

func fuzzInspect(f *testing.F, seeds ...[]byte) {
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		info, err := InspectReaderAt(limitedReaderAt{t, bytes.NewReader(data)}, int64(len(data)))
		if err == nil && len(info.Tracks) > maxTracks {
			t.Fatalf("%d tracks", len(info.Tracks))
		}
		_, _ = ReadTracks(limitedReaderAt{t, bytes.NewReader(data)}, int64(len(data)))
	})
}

func FuzzInspectMatroska(f *testing.F) {
	video := mkvEntry(1, 1, "V_MPEGH/ISO/HEVC", mkvUint(ebmlDefaultDur, 41708333), mkvEl(ebmlCodecPrivate, hvcc(spsPQ)),
		mkvEl(ebmlVideoID, mkvUint(ebmlPixelWidth, 1920), mkvEl(ebmlColourID, mkvUint(ebmlTransferID, 16))), mkvDVMapping(dovi(8, 1)))
	audio := mkvEntry(2, 2, "A_EAC3", mkvStr(ebmlLanguageID, "eng"), mkvEl(ebmlAudioID, mkvUint(ebmlChannelsID, 6)))
	sub := mkvEntry(0x11, 3, "S_TEXT/UTF8", mkvUint(ebmlFlagForced, 1))
	fuzzInspect(f,
		mkvMovie(mkvInfo(1000000, 5000), mkvEl(ebmlTracksID, video, audio, sub)),
		matroskaFixture("webm", mkvTrack(1, "V_VP9", "", "", nil), mkvTrack(2, "A_OPUS", "ger", "de-DE", nil)),
	)
}

func FuzzInspectMP4(f *testing.F) {
	video := trak(tkhd(1, true, 1920, 1080), 24000, "vide", "und", "", visualEntry("hvc1", 1920, 1080,
		bx("hvcC", hvcc(spsHLG)), bx("colr", []byte("nclx"), u16(9), u16(16), u16(9), []byte{0}), bx("dvcC", dovi(8, 1))), stts(10, 1001))
	audio := trak(tkhd(2, true, 0, 0), 48000, "soun", "eng", "", audioEntry("ec-3", 2, bx("dec3", []byte{1, 0, 0x20, 0x0f, 0x02, 0x80})))
	sub := trak(tkhd(3, true, 0, 0), 1000, "sbtl", "fre", "", bx("tx3g", make([]byte, 30)))
	fuzzInspect(f, mp4File(mvhd(1000, 5000), video, audio, sub))
}
