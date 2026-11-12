package container

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Element ids the text fixtures add to the ones above.
const (
	tInfo        = 0x1549A966
	tScale       = 0x2AD7B1
	tTimecode    = 0xE7
	tSimpleBlock = 0xA3
	tBlockGroup  = 0xA0
	tBlock       = 0xA1
	tDuration    = 0x9B
	tEncodings   = 0x6D80
	tEncoding    = 0x6240
	tCompression = 0x5034
	tCompAlgo    = 0x4254
	tCompSetting = 0x4255
	tEncryption  = 0x5035
)

func textTrack(num byte, codec string, extra ...[]byte) []byte {
	body := append(ebml(idTrackNumber, []byte{num}), ebml(idTrackType, []byte{0x11})...)
	body = append(body, ebml(idCodecID, []byte(codec))...)
	for _, x := range extra {
		body = append(body, x...)
	}
	return ebml(idTrackEntry, body)
}

// frame is a block body: track number, time from the cluster, flags, data.
func frame(track byte, rel int16, data []byte) []byte {
	b := []byte{0x80 | track, 0, 0, 0}
	binary.BigEndian.PutUint16(b[1:], uint16(rel))
	return append(b, data...)
}

func group(track byte, rel int16, data []byte, dur byte) []byte {
	return ebml(tBlockGroup, append(ebml(tBlock, frame(track, rel, data)), ebml(tDuration, []byte{dur})...))
}

func cluster(at uint16, children ...[]byte) []byte {
	tc := make([]byte, 2)
	binary.BigEndian.PutUint16(tc, at)
	body := ebml(tTimecode, tc)
	for _, c := range children {
		body = append(body, c...)
	}
	return ebml(idCluster, body)
}

func zipped(s string) []byte {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return buf.Bytes()
}

// labMKV builds a small file: a video track 1, SRT text on 2, zlib-packed ASS
// on 3, a picture track 4 and a header-stripped SRT track 5. The scale is 1 ms.
func labMKV() []byte {
	scale := []byte{0x0F, 0x42, 0x40} // 1,000,000 ns
	info := ebml(tInfo, ebml(tScale, scale))
	zlibEnc := ebml(tEncodings, ebml(tEncoding, ebml(tCompression, ebml(tCompAlgo, []byte{0}))))
	stripEnc := ebml(tEncodings, ebml(tEncoding, ebml(tCompression, append(ebml(tCompAlgo, []byte{3}), ebml(tCompSetting, []byte("Hel"))...))))
	tracks := ebml(idTracks, bytes.Join([][]byte{
		ebml(idTrackEntry, append(ebml(idTrackNumber, []byte{1}), ebml(idCodecID, []byte("V_MPEG4/ISO/AVC"))...)),
		textTrack(2, "S_TEXT/UTF8"), textTrack(3, "S_TEXT/ASS", zlibEnc), textTrack(4, "S_HDMV/PGS"), textTrack(5, "S_TEXT/UTF8", stripEnc),
	}, nil))
	video := ebml(tSimpleBlock, frame(1, 0, bytes.Repeat([]byte{0x55}, 300)))
	c1 := cluster(1000, video,
		group(2, 0, []byte("<i>Hello</i>\r\nthere"), 200), // 1.0 s to 1.2 s
		group(3, 500, zipped("1,0,Default,,0,0,0,,{\\i1}Hi{\\i0}\\Nyou"), 250),
		ebml(tSimpleBlock, frame(5, 100, []byte("lo"))),
	)
	c2 := cluster(4000, video, group(2, 0, []byte("Bye"), 100), ebml(tSimpleBlock, frame(5, 0, []byte("p"))))
	seg := bytes.Join([][]byte{info, tracks, c1, c2}, nil)
	return append(header(), ebml(idSegment, seg)...)
}

func read(t *testing.T, b []byte, track int) ([]string, []int64, error) {
	t.Helper()
	cues, err := ReadText(context.Background(), bytes.NewReader(b), int64(len(b)), track)
	var texts []string
	var times []int64
	for _, c := range cues {
		texts = append(texts, c.Text)
		times = append(times, c.StartMS, c.EndMS)
	}
	return texts, times, err
}

func TestReadText_TracksAndPacking(t *testing.T) {
	b := labMKV()
	texts, times, err := read(t, b, 2)
	if err != nil || strings.Join(texts, "|") != "<i>Hello</i>\nthere|Bye" || times[0] != 1000 || times[1] != 1200 || times[2] != 4000 || times[3] != 4100 {
		t.Fatalf("srt track: %q %v %v", texts, times, err)
	}
	texts, times, err = read(t, b, 3)
	if err != nil || len(texts) != 1 || texts[0] != "<i>Hi</i>\nyou" || times[0] != 1500 || times[1] != 1750 {
		t.Fatalf("ass track: %q %v %v", texts, times, err)
	}
	// Blocks with no duration end at the next cue, or a few seconds on.
	texts, times, err = read(t, b, 5)
	if err != nil || strings.Join(texts, "|") != "Hello|Help" || times[1] != 4000 || times[3] != 4000+openEndMS {
		t.Fatalf("stripped track: %q %v %v", texts, times, err)
	}
	if _, _, err := read(t, b, 4); !errors.Is(err, ErrNotText) {
		t.Fatalf("picture track: %v", err)
	}
	if _, _, err := read(t, b, 1); !errors.Is(err, ErrNotText) {
		t.Fatalf("video track: %v", err)
	}
	if _, _, err := read(t, b, 9); !errors.Is(err, ErrNoTrack) {
		t.Fatalf("missing track: %v", err)
	}
}

func TestReadText_RefusesEncryptedAndNonMKV(t *testing.T) {
	enc := ebml(tEncodings, ebml(tEncoding, ebml(tEncryption, nil)))
	f := append(header(), ebml(idSegment, ebml(idTracks, textTrack(2, "S_TEXT/UTF8", enc)))...)
	if _, _, err := read(t, f, 2); !errors.Is(err, ErrEncoding) {
		t.Fatalf("encrypted: %v", err)
	}
	if _, _, err := read(t, []byte("RIFF....AVI LIST"), 2); !errors.Is(err, ErrNotMKV) {
		t.Fatalf("avi: %v", err)
	}
	mp4, _ := os.ReadFile(filepath.Join("testdata", "lab-built-in.mp4"))
	if _, _, err := read(t, mp4, 1); !errors.Is(err, ErrNotMKV) {
		t.Fatalf("mp4: %v", err)
	}
}

// A file cut off in its second cluster keeps the cues before the cut.
func TestReadText_CutOffKeepsEarlierCues(t *testing.T) {
	b := labMKV()
	texts, _, err := read(t, b[:len(b)-20], 2)
	if err != nil || len(texts) != 1 || texts[0] != "<i>Hello</i>\nthere" {
		t.Fatalf("%q %v", texts, err)
	}
}

// A cluster of unknown size ends where the next cluster starts.
func TestReadText_UnknownSizeCluster(t *testing.T) {
	b := labMKV()
	i := bytes.Index(b, []byte{0x1F, 0x43, 0xB6, 0x75})
	if i < 0 || b[i+4] != 0x01 {
		t.Fatalf("fixture layout changed")
	}
	copy(b[i+4:i+12], []byte{0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})
	texts, _, err := read(t, b, 2)
	if err != nil || len(texts) != 2 {
		t.Fatalf("%q %v", texts, err)
	}
}

// The lab file's English text track reads as cues.
func TestReadText_LabFile(t *testing.T) {
	p := filepath.Join("testdata", "lab-built-in.mkv")
	tracks, err := Probe(p)
	if err != nil || len(tracks) == 0 {
		t.Fatal(err)
	}
	cues, err := ExtractText(context.Background(), p, tracks[0].Index)
	if err != nil || len(cues) == 0 || cues[0].Text == "" || cues[0].EndMS <= cues[0].StartMS {
		t.Fatalf("%+v %v", cues, err)
	}
}

// A stopped job stops the read.
func TestReadText_StopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := labMKV()
	if _, err := ReadText(ctx, bytes.NewReader(b), int64(len(b)), 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
}

func FuzzReadText(f *testing.F) {
	lab, _ := os.ReadFile(filepath.Join("testdata", "lab-built-in.mkv"))
	f.Add(lab, 1)
	f.Add(labMKV(), 2)
	f.Add(labMKV(), 3)
	f.Fuzz(func(t *testing.T, data []byte, track int) {
		start := time.Now()
		cues, err := ReadText(context.Background(), bytes.NewReader(data), int64(len(data)), track)
		if err == nil && len(cues) == 0 {
			t.Fatal("no error and no cues")
		}
		if time.Since(start) > 2*time.Second {
			t.Fatalf("slow read: %v", time.Since(start))
		}
	})
}
