package mediafiles

import (
	"bytes"
	"testing"
	"time"
)

// A recording cut mid-packet starts with part of a packet before the first
// sync byte; it is still a transport stream.
func TestTransportStreamUnalignedStart(t *testing.T) {
	for _, junk := range []int{1, 100, 187, 191} {
		file := append(bytes.Repeat([]byte{0x11}, junk), h264Stream(60)...)
		got := inspectBytes(t, file)
		if got.Container != "mpegts" || got.Width != 1920 || got.Duration != time.Minute {
			t.Errorf("junk %d: %+v", junk, got)
		}
	}
	m := m2ts(h264Stream(60))
	got := inspectBytes(t, append(bytes.Repeat([]byte{0x22}, 50), m...))
	if got.Container != "m2ts" || got.Width != 1920 {
		t.Errorf("m2ts junk: %+v", got)
	}
}

// A video stream with no sequence parameter set in its first frames must
// not cost time that grows with the square of the bytes kept.
func TestTransportStreamNoSPSIsQuick(t *testing.T) {
	data := tsTables(tsES{0x1b, 0x101, nil})
	frame := bytes.Repeat([]byte{0, 0, 1, 0x41, 0x9a, 0x55, 0x55, 0x55}, 2000)
	for i := 0; len(data) < 1<<20; i++ {
		data = append(data, tsPES(0x101, 0xe0, int64(90000+i*3750), frame)...)
	}
	start := time.Now()
	got := inspectBytes(t, data)
	if d := time.Since(start); d > 300*time.Millisecond {
		t.Errorf("took %v", d)
	}
	if got.VideoCodec != "h264" {
		t.Errorf("%+v", got)
	}
}
