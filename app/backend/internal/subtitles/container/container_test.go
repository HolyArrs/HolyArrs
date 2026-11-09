package container

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// The lab files come from ffmpeg test patterns and hold no real media.
// lab-built-in.mkv carries English, forced Spanish and English SDH text
// tracks. lab-built-in.mp4 carries one English mov_text track.
func TestLabFiles(t *testing.T) {
	got, err := Probe(filepath.Join("testdata", "lab-built-in.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("%+v", got)
	}
	if got[0].Language != "eng" || got[0].Codec != "S_TEXT/UTF8" || got[0].Forced || got[0].HearingImpaired || got[0].Picture {
		t.Errorf("track 1: %+v", got[0])
	}
	if got[1].Language != "spa" || !got[1].Forced {
		t.Errorf("track 2: %+v", got[1])
	}
	if got[2].Language != "eng" || !got[2].HearingImpaired || got[2].Name != "English SDH" {
		t.Errorf("track 3: %+v", got[2])
	}
	mp, err := Probe(filepath.Join("testdata", "lab-built-in.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if len(mp) != 1 || mp[0].Language != "eng" || mp[0].Codec != "tx3g" {
		t.Fatalf("mp4: %+v", mp)
	}
	none, err := Probe(filepath.Join("testdata", "lab-none.mkv"))
	if err != nil || len(none) != 0 {
		t.Fatalf("none: %+v %v", none, err)
	}
}

func TestRefusals(t *testing.T) {
	dir := t.TempDir()
	if _, err := Probe(dir); err == nil {
		t.Error("a folder was read")
	}
	if _, err := Read(bytes.NewReader([]byte("RIFF....AVI LIST")), 16); !errors.Is(err, ErrUnknownFormat) {
		t.Errorf("avi: %v", err)
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(dir, "link.mkv")
		if err := os.Symlink(filepath.Join("testdata", "lab-built-in.mkv"), link); err == nil {
			if _, err := Probe(link); err == nil {
				t.Error("a link was followed")
			}
		}
	}
	// A cut-off file must return an error and never panic.
	b, _ := os.ReadFile(filepath.Join("testdata", "lab-built-in.mkv"))
	for _, n := range []int{4, 20, 50, 100, 200} {
		if _, err := Read(bytes.NewReader(b[:n]), int64(n)); err == nil {
			t.Errorf("mkv cut at %d read", n)
		}
	}
}

// Matroska element ids the hand-built fixtures use.
const (
	idEBML         = 0x1A45DFA3
	idDocType      = 0x4282
	idSegment      = 0x18538067
	idSeekHead     = 0x114D9B74
	idSeek         = 0x4DBB
	idSeekID       = 0x53AB
	idSeekPosition = 0x53AC
	idTracks       = 0x1654AE6B
	idCluster      = 0x1F43B675
	idTrackEntry   = 0xAE
	idTrackNumber  = 0xD7
	idTrackType    = 0x83
	idCodecID      = 0x86
	idLanguage     = 0x22B59C
)

// ebml encodes one element with a 1-byte or 8-byte size.
func ebml(id uint32, body []byte) []byte {
	var out []byte
	switch {
	case id > 0xFFFFFF:
		out = append(out, byte(id>>24), byte(id>>16), byte(id>>8), byte(id))
	case id > 0xFFFF:
		out = append(out, byte(id>>16), byte(id>>8), byte(id))
	case id > 0xFF:
		out = append(out, byte(id>>8), byte(id))
	default:
		out = append(out, byte(id))
	}
	if len(body) < 127 {
		out = append(out, 0x80|byte(len(body)))
	} else {
		s := make([]byte, 8)
		binary.BigEndian.PutUint64(s, uint64(len(body)))
		s[0] = 0x01
		out = append(out, s...)
	}
	return append(out, body...)
}

func header() []byte { return ebml(idEBML, ebml(idDocType, []byte("matroska"))) }

func TestHostileMatroska(t *testing.T) {
	// A Tracks element that claims 1 TB is refused before any read.
	huge := append(header(), 0x18, 0x53, 0x80, 0x67, 0xFF) // segment with unknown size
	huge = append(huge, 0x16, 0x54, 0xAE, 0x6B, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00)
	if _, err := Read(bytes.NewReader(huge), int64(len(huge))); !errors.Is(err, ErrLimit) && !errors.Is(err, ErrMalformed) {
		t.Errorf("huge tracks: %v", err)
	}
	// A flood of tiny Void elements stops at the element limit.
	seg := []byte{}
	for range 20000 { // more than the element budget allows
		seg = append(seg, 0xEC, 0x80) // empty Void element
	}
	many := append(header(), ebml(idSegment, seg)...)
	start := time.Now()
	if _, err := Read(bytes.NewReader(many), int64(len(many))); !errors.Is(err, ErrLimit) {
		t.Errorf("many elements: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("too slow")
	}
	// A file with another doc type is not Matroska.
	odd := append(ebml(idEBML, ebml(idDocType, []byte("other"))), ebml(idSegment, nil)...)
	if _, err := Read(bytes.NewReader(odd), int64(len(odd))); !errors.Is(err, ErrUnknownFormat) {
		t.Errorf("doc type: %v", err)
	}
	// A SeekHead that points at Tracks placed after the first Cluster is followed.
	entry := ebml(idTrackEntry, append(append(ebml(idTrackNumber, []byte{3}), ebml(idTrackType, []byte{0x11})...), ebml(idLanguage, []byte("fre"))...))
	tracks := ebml(idTracks, entry)
	cluster := ebml(idCluster, []byte{0xE7, 0x81, 0x00})
	var sh []byte
	for pad := 0; ; pad++ {
		seek := ebml(idSeek, append(ebml(idSeekID, []byte{0x16, 0x54, 0xAE, 0x6B}), ebml(idSeekPosition, []byte{byte(0)})...))
		head := ebml(idSeekHead, seek)
		pos := len(head) + len(cluster)
		seek = ebml(idSeek, append(ebml(idSeekID, []byte{0x16, 0x54, 0xAE, 0x6B}), ebml(idSeekPosition, []byte{byte(pos)})...))
		sh = ebml(idSeekHead, seek)
		if len(sh) == len(head) {
			break
		}
	}
	late := append(header(), ebml(idSegment, append(append(sh, cluster...), tracks...))...)
	got, err := Read(bytes.NewReader(late), int64(len(late)))
	if err != nil || len(got) != 1 || got[0].Language != "fre" || got[0].Index != 3 {
		t.Fatalf("late tracks: %+v %v", got, err)
	}
	// A track with no language counts as English, as Matroska says, and a
	// picture codec is flagged.
	pgs := ebml(idTrackEntry, append(ebml(idTrackType, []byte{0x11}), ebml(idCodecID, []byte("S_HDMV/PGS"))...))
	f := append(header(), ebml(idSegment, ebml(idTracks, pgs))...)
	got, err = Read(bytes.NewReader(f), int64(len(f)))
	if err != nil || len(got) != 1 || got[0].Language != "eng" || !got[0].Picture {
		t.Fatalf("pgs: %+v %v", got, err)
	}
	// EBML allows empty or zero-padded strings and empty integers that mean the
	// default. The tracks still list, with the padding trimmed.
	padded := ebml(idTrackEntry, bytes.Join([][]byte{
		ebml(idTrackNumber, []byte{4}), ebml(idTrackType, []byte{0x11}),
		ebml(idCodecID, []byte("S_TEXT/UTF8\x00")), ebml(idLanguage, []byte("eng\x00\x00")), ebml(0x88, nil),
	}, nil))
	empty := ebml(idTrackEntry, bytes.Join([][]byte{
		ebml(idTrackNumber, []byte{5}), ebml(idTrackType, []byte{0x11}), ebml(idLanguage, nil),
	}, nil))
	f = append(header(), ebml(idSegment, ebml(idTracks, append(padded, empty...)))...)
	got, err = Read(bytes.NewReader(f), int64(len(f)))
	if err != nil || len(got) != 2 || got[0].Language != "eng" || got[0].Codec != "S_TEXT/UTF8" || got[0].Index != 4 || got[1].Index != 5 {
		t.Fatalf("padded: %+v %v", got, err)
	}
}

func mp4box(typ string, body []byte) []byte {
	out := make([]byte, 8, 8+len(body))
	binary.BigEndian.PutUint32(out, uint32(8+len(body)))
	copy(out[4:], typ)
	return append(out, body...)
}

func TestHostileMP4(t *testing.T) {
	// A box that claims more bytes than its parent holds is malformed.
	bad := append(mp4box("ftyp", []byte("isom")), 0xFF, 0xFF, 0xFF, 0xF0, 'm', 'o', 'o', 'v')
	if _, err := Read(bytes.NewReader(bad), int64(len(bad))); !errors.Is(err, ErrMalformed) {
		t.Errorf("oversized: %v", err)
	}
	// Nesting that goes too deep stops.
	deep := mp4box("trak", nil)
	for range 20 {
		deep = mp4box("mdia", deep)
	}
	f := append(mp4box("ftyp", []byte("isom")), mp4box("moov", mp4box("trak", mp4box("mdia", mp4box("minf", deep))))...)
	if _, err := Read(bytes.NewReader(f), int64(len(f))); err != nil && !errors.Is(err, ErrLimit) && !errors.Is(err, ErrMalformed) {
		t.Errorf("deep: %v", err)
	}
	// A file with no moov box is malformed.
	none := mp4box("ftyp", []byte("isom"))
	if _, err := Read(bytes.NewReader(none), int64(len(none))); !errors.Is(err, ErrMalformed) {
		t.Errorf("no moov: %v", err)
	}
}

func FuzzMatroska(f *testing.F) {
	b, _ := os.ReadFile(filepath.Join("testdata", "lab-built-in.mkv"))
	f.Add(b)
	f.Add(header())
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = Read(bytes.NewReader(data), int64(len(data)))
	})
}

func FuzzMP4(f *testing.F) {
	b, _ := os.ReadFile(filepath.Join("testdata", "lab-built-in.mp4"))
	f.Add(b)
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = Read(bytes.NewReader(data), int64(len(data)))
	})
}
