package mediafiles

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectReaderAtMatroskaTracks(t *testing.T) {
	data := matroskaFixture("matroska",
		mkvTrack(1, "V_MPEGH/ISO/HEVC", "", "", nil),
		mkvTrack(2, "A_AAC", "eng", "", nil),
		mkvTrack(2, "A_AC3", "fre", "fr-CA", nil),
		mkvTrack(2, "A_OPUS", "und", "", nil),
	)

	got, err := InspectReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	want := Info{
		Container:      "matroska",
		VideoCodec:     "hevc",
		Status:         "partial",
		AudioLanguages: []string{"en", "fr"},
		AudioTracks:    3,
		AudioUnknown:   true,
	}
	assertInfo(t, got, want)
}

func TestInspectReaderAtWebMUsesBCP47AndVP9(t *testing.T) {
	data := matroskaFixture("webm",
		mkvTrack(1, "V_VP9", "", "", nil),
		mkvTrack(2, "A_OPUS", "ger", "de-DE", nil),
		mkvTrack(2, "A_OPUS", "deu", "", nil),
	)

	got, err := InspectReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	assertInfo(t, got, Info{
		Container:      "webm",
		VideoCodec:     "vp9",
		Status:         "complete",
		AudioLanguages: []string{"de"},
		AudioTracks:    2,
	})
}

func TestInspectReaderAtMP4MoovAtEnd(t *testing.T) {
	mdat := make([]byte, 1<<20)
	data := append(mp4Box("ftyp", []byte("isom\x00\x00\x00\x00isom")), mp4Box("mdat", mdat)...)
	data = append(data, mp4Movie(
		mp4Track("vide", "av01", "", "und"),
		mp4Track("soun", "mp4a", "en-US", "eng"),
		mp4Track("soun", "ac-3", "", "spa"),
	)...)

	got, err := InspectReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	assertInfo(t, got, Info{
		Container:      "mp4",
		VideoCodec:     "av1",
		Status:         "complete",
		AudioLanguages: []string{"en", "es"},
		AudioTracks:    2,
	})
}

func TestInspectReaderAtSkipsMediaPayload(t *testing.T) {
	ftyp := mp4Box("ftyp", []byte("isom\x00\x00\x00\x00isom"))
	mdat := mp4Box("mdat", make([]byte, 1<<20))
	moov := mp4Movie(mp4Track("vide", "avc1", "", "und"))
	data := append(append(ftyp, mdat...), moov...)
	reader := guardedReaderAt{
		ReaderAt: bytes.NewReader(data),
		start:    int64(len(ftyp) + 8),
		end:      int64(len(ftyp) + len(mdat)),
	}

	got, err := InspectReaderAt(reader, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if got.VideoCodec != "h264" {
		t.Fatalf("VideoCodec = %q, want h264", got.VideoCodec)
	}
}

func TestInspectReaderAtStopsAfterMP4MovieHeaders(t *testing.T) {
	data := append(mp4Box("ftyp", []byte("isom\x00\x00\x00\x00isom")), mp4Movie(mp4Track("vide", "avc1", "", "und"))...)
	for range maxElements + 1 {
		data = append(data, mp4Box("free", nil)...)
	}

	got, err := InspectReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if got.VideoCodec != "h264" {
		t.Fatalf("VideoCodec = %q, want h264", got.VideoCodec)
	}
}

func TestInspectReaderAtStopsAfterMatroskaTracks(t *testing.T) {
	data := matroskaFixture("matroska", mkvTrack(1, "V_AV1", "", "", nil))
	// An unknown-sized Cluster after Tracks is media payload, not a header we need.
	data = append(data, []byte{0x1f, 0x43, 0xb6, 0x75, 0xff}...)

	got, err := InspectReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if got.VideoCodec != "av1" {
		t.Fatalf("VideoCodec = %q, want av1", got.VideoCodec)
	}
}

func TestInspectUsesHeadersInsteadOfFilename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Definitely.English.XviD.mkv")
	data := append(mp4Box("ftyp", []byte("isom\x00\x00\x00\x00isom")), mp4Movie(
		mp4Track("vide", "hvc1", "", "und"),
		mp4Track("soun", "mp4a", "fr-FR", "und"),
	)...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Inspect(path)
	if err != nil {
		t.Fatal(err)
	}
	assertInfo(t, got, Info{
		Container:      "mp4",
		VideoCodec:     "hevc",
		Status:         "complete",
		AudioLanguages: []string{"fr"},
		AudioTracks:    1,
	})
}

func TestInspectRejectsNonRegularInput(t *testing.T) {
	_, err := Inspect(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "regular") {
		t.Fatalf("Inspect(directory) error = %v, want regular-file error", err)
	}
}

func TestInspectReaderAtCodecNormalization(t *testing.T) {
	tests := []struct {
		entry string
		want  string
	}{
		{"avc1", "h264"},
		{"avc3", "h264"},
		{"hvc1", "hevc"},
		{"hev1", "hevc"},
		{"av01", "av1"},
		{"vp09", "vp9"},
		{"mp2v", "mpeg2"},
		{"XVID", "xvid"},
		{"wvc1", "vc1"},
		{"mp4v", ""},
	}
	for _, tc := range tests {
		t.Run(tc.entry, func(t *testing.T) {
			data := append(mp4Box("ftyp", []byte("isom\x00\x00\x00\x00isom")), mp4Movie(mp4Track("vide", tc.entry, "", "und"))...)
			got, err := InspectReaderAt(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			if got.VideoCodec != tc.want {
				t.Fatalf("VideoCodec = %q, want %q", got.VideoCodec, tc.want)
			}
			wantStatus := "complete"
			if tc.want == "" {
				wantStatus = "partial"
			}
			if got.Status != wantStatus {
				t.Fatalf("Status = %q, want %q", got.Status, wantStatus)
			}
		})
	}
}

func TestInspectReaderAtDoesNotHideUnknownSecondVideoTrack(t *testing.T) {
	data := append(mp4Box("ftyp", []byte("isom\x00\x00\x00\x00isom")), mp4Movie(
		mp4Track("vide", "avc1", "", "und"),
		mp4Track("vide", "zzzz", "", "und"),
	)...)

	got, err := InspectReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if got.VideoCodec != "" || got.Status != "partial" {
		t.Fatalf("Info = %#v, want unknown codec and partial status", got)
	}
}

func TestInspectReaderAtMatroskaVFWCodecNormalization(t *testing.T) {
	private := make([]byte, 20)
	copy(private[16:], "WVC1")
	data := matroskaFixture("matroska", mkvTrack(1, "V_MS/VFW/FOURCC", "", "", private))

	got, err := InspectReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if got.VideoCodec != "vc1" {
		t.Fatalf("VideoCodec = %q, want vc1", got.VideoCodec)
	}
}

func TestInspectReaderAtUnsupportedIsExplicit(t *testing.T) {
	data := []byte("not a supported media container")
	got, err := InspectReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	assertInfo(t, got, Info{Status: "unsupported"})
}

func TestInspectReaderAtRejectsMalformedLengths(t *testing.T) {
	tests := map[string][]byte{
		"mp4_truncated_box": append([]byte{0, 0, 1, 0}, []byte("ftyp")...),
		"mp4_overflow":      append([]byte{0, 0, 0, 1}, append([]byte("moov"), bytes.Repeat([]byte{0xff}, 8)...)...),
		"matroska_vint":     {0x1a, 0x45, 0xdf, 0xa3, 0x00},
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := InspectReaderAt(bytes.NewReader(data), int64(len(data))); err == nil {
				t.Fatal("InspectReaderAt() error = nil, want malformed input error")
			}
		})
	}
}

func TestInspectReaderAtRejectsInvalidSize(t *testing.T) {
	if _, err := InspectReaderAt(bytes.NewReader(nil), -1); err == nil {
		t.Fatal("InspectReaderAt() error = nil, want invalid size error")
	}
}

func TestInspectReaderAtRejectsTrackAndStringBudgets(t *testing.T) {
	tracks := make([][]byte, maxTracks+1)
	for i := range tracks {
		tracks[i] = mkvTrack(2, "A_AAC", "eng", "", nil)
	}
	tooManyTracks := matroskaFixture("matroska", tracks...)
	if _, err := InspectReaderAt(bytes.NewReader(tooManyTracks), int64(len(tooManyTracks))); err == nil || !strings.Contains(err.Error(), "track budget") {
		t.Fatalf("track budget error = %v", err)
	}

	// A string over the cap is cut, not refused.
	longCodec := matroskaFixture("matroska", mkvTrack(1, strings.Repeat("x", maxString+1), "", "", nil))
	got, err := InspectReaderAt(bytes.NewReader(longCodec), int64(len(longCodec)))
	if err != nil || len(got.Tracks) != 1 || len(got.Tracks[0].Codec) != maxString {
		t.Fatalf("long string: %v %+v", err, got.Tracks)
	}
}

func TestInspectReaderAtInvalidExtendedLanguageDoesNotFallBack(t *testing.T) {
	data := append(mp4Box("ftyp", []byte("isom\x00\x00\x00\x00isom")), mp4Movie(
		mp4Track("vide", "avc1", "", "und"),
		mp4Track("soun", "mp4a", "???", "eng"),
	)...)
	got, err := InspectReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if got.AudioTracks != 1 || !got.AudioUnknown || len(got.AudioLanguages) != 0 || got.Status != "partial" {
		t.Fatalf("Info = %#v, want one audio track with unknown language", got)
	}
}

func TestInspectReaderAtEnforcesElementBudget(t *testing.T) {
	payload := make([]byte, 0, (maxElements+1)*8)
	for range maxElements + 1 {
		payload = append(payload, mp4Box("free", nil)...)
	}
	data := append(mp4Box("ftyp", []byte("isom\x00\x00\x00\x00isom")), payload...)
	if _, err := InspectReaderAt(bytes.NewReader(data), int64(len(data))); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("InspectReaderAt() error = %v, want budget error", err)
	}
}

func assertInfo(t *testing.T, got, want Info) {
	t.Helper()
	if got.Container != want.Container || got.VideoCodec != want.VideoCodec || got.Status != want.Status ||
		got.AudioTracks != want.AudioTracks || got.AudioUnknown != want.AudioUnknown ||
		strings.Join(got.AudioLanguages, ",") != strings.Join(want.AudioLanguages, ",") {
		t.Fatalf("Info = %#v, want %#v", got, want)
	}
}

type guardedReaderAt struct {
	io.ReaderAt
	start int64
	end   int64
}

func (r guardedReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	readEnd := offset + int64(len(p))
	if offset < r.end && readEnd > r.start {
		return 0, fmt.Errorf("media payload read at %d", offset)
	}
	return r.ReaderAt.ReadAt(p, offset)
}

func mp4Movie(tracks ...[]byte) []byte {
	return mp4Box("moov", bytes.Join(tracks, nil))
}

func mp4Track(handler, codec, extendedLanguage, packedLanguage string) []byte {
	mdia := make([]byte, 0)
	mdia = append(mdia, mp4MDHD(packedLanguage)...)
	if extendedLanguage != "" {
		mdia = append(mdia, mp4Box("elng", append([]byte{0, 0, 0, 0}, append([]byte(extendedLanguage), 0)...))...)
	}
	hdlr := append([]byte{0, 0, 0, 0, 0, 0, 0, 0}, []byte(handler)...)
	hdlr = append(hdlr, make([]byte, 12)...)
	mdia = append(mdia, mp4Box("hdlr", hdlr)...)
	entry := make([]byte, 16)
	binary.BigEndian.PutUint32(entry[:4], uint32(len(entry)))
	copy(entry[4:8], codec)
	stsd := append([]byte{0, 0, 0, 0, 0, 0, 0, 1}, entry...)
	mdia = append(mdia, mp4Box("minf", mp4Box("stbl", mp4Box("stsd", stsd)))...)
	return mp4Box("trak", mp4Box("mdia", mdia))
}

func mp4MDHD(language string) []byte {
	if language == "" {
		language = "und"
	}
	payload := make([]byte, 24)
	binary.BigEndian.PutUint16(payload[20:22], packLanguage(language))
	return mp4Box("mdhd", payload)
}

func packLanguage(value string) uint16 {
	if len(value) != 3 {
		return 0x7fff
	}
	return uint16(value[0]-0x60)<<10 | uint16(value[1]-0x60)<<5 | uint16(value[2]-0x60)
}

func mp4Box(kind string, payload []byte) []byte {
	box := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(box[:4], uint32(len(box)))
	copy(box[4:8], kind)
	copy(box[8:], payload)
	return box
}

func matroskaFixture(docType string, tracks ...[]byte) []byte {
	header := ebmlFixtureElement([]byte{0x1a, 0x45, 0xdf, 0xa3}, ebmlFixtureElement([]byte{0x42, 0x82}, []byte(docType)))
	trackPayload := bytes.Join(tracks, nil)
	segment := ebmlFixtureElement([]byte{0x18, 0x53, 0x80, 0x67}, ebmlFixtureElement([]byte{0x16, 0x54, 0xae, 0x6b}, trackPayload))
	return append(header, segment...)
}

func mkvTrack(kind byte, codec, languageValue, bcp47 string, private []byte) []byte {
	payload := ebmlFixtureElement([]byte{0x83}, []byte{kind})
	payload = append(payload, ebmlFixtureElement([]byte{0x86}, []byte(codec))...)
	if languageValue != "" {
		payload = append(payload, ebmlFixtureElement([]byte{0x22, 0xb5, 0x9c}, []byte(languageValue))...)
	}
	if bcp47 != "" {
		payload = append(payload, ebmlFixtureElement([]byte{0x22, 0xb5, 0x9d}, []byte(bcp47))...)
	}
	if private != nil {
		payload = append(payload, ebmlFixtureElement([]byte{0x63, 0xa2}, private)...)
	}
	return ebmlFixtureElement([]byte{0xae}, payload)
}

func ebmlFixtureElement(id, payload []byte) []byte {
	size := ebmlSize(len(payload))
	result := make([]byte, 0, len(id)+len(size)+len(payload))
	result = append(result, id...)
	result = append(result, size...)
	result = append(result, payload...)
	return result
}

func ebmlSize(size int) []byte {
	switch {
	case size < 0x7f:
		return []byte{0x80 | byte(size)}
	case size < 0x3fff:
		return []byte{0x40 | byte(size>>8), byte(size)}
	default:
		panic("test EBML payload too large")
	}
}

// An .m4a or .mka holds lossless (ALAC, FLAC) or lossy (AAC) audio:
// the codec read from its headers says which.
func TestInspectAudioCodecsLossless(t *testing.T) {
	for _, c := range []struct {
		name      string
		data      []byte
		lossless  bool
		known     bool
		wantCodec string
	}{
		{"mp4 alac", append(mp4Box("ftyp", []byte("M4A \x00\x00\x00\x00M4A ")), mp4Movie(mp4Track("soun", "alac", "", "eng"))...), true, true, "alac"},
		{"mp4 aac", append(mp4Box("ftyp", []byte("M4A \x00\x00\x00\x00M4A ")), mp4Movie(mp4Track("soun", "mp4a", "", "eng"))...), false, true, "aac"},
		{"mka flac", matroskaFixture("matroska", mkvTrack(2, "A_FLAC", "eng", "", nil)), true, true, "flac"},
		{"mka aac", matroskaFixture("matroska", mkvTrack(2, "A_AAC/MPEG4/LC", "eng", "", nil)), false, true, "aac"},
		{"no audio codec", matroskaFixture("matroska", mkvTrack(2, "A_SOMETHING", "eng", "", nil)), false, false, ""},
	} {
		got, err := InspectReaderAt(bytes.NewReader(c.data), int64(len(c.data)))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		l, k := got.Lossless()
		if l != c.lossless || k != c.known || strings.Join(got.AudioCodecs, ",") != c.wantCodec {
			t.Errorf("%s: codecs %v lossless %v known %v", c.name, got.AudioCodecs, l, k)
		}
	}
}

// AudioWord names the codec only when every audio track shares it.
func TestInspectAudioWord(t *testing.T) {
	two := append(mp4Box("ftyp", []byte("M4A \x00\x00\x00\x00M4A ")), mp4Movie(mp4Track("soun", "alac", "", "eng"), mp4Track("soun", "alac", "", "eng"))...)
	mixed := append(mp4Box("ftyp", []byte("M4A \x00\x00\x00\x00M4A ")), mp4Movie(mp4Track("soun", "alac", "", "eng"), mp4Track("soun", "mp4a", "", "eng"))...)
	for name, c := range map[string]struct {
		data []byte
		want string
	}{"two alac": {two, "ALAC"}, "mixed": {mixed, ""}} {
		got, err := InspectReaderAt(bytes.NewReader(c.data), int64(len(c.data)))
		if err != nil || got.AudioWord() != c.want {
			t.Errorf("%s: %q %v", name, got.AudioWord(), err)
		}
	}
}
