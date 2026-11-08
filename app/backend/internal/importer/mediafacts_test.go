package importer

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func TestFactsOfStoresHeaderFacts(t *testing.T) {
	i := mediafiles.Info{Status: "complete", Container: "matroska", VideoCodec: "hevc", AudioLanguages: []string{"en"}, AudioTracks: 1,
		AudioCodecs: []string{"eac3"}, AudioChannels: []int{6}, Width: 1920, Height: 800, FrameRate: 23.976023, BitDepth: 10, HDR: "HDR10",
		Duration: 2*time.Hour + 500*time.Millisecond, Bitrate: 8_000_000,
		SubtitleTracks: []mediafiles.SubtitleTrack{{Language: "en"}, {Language: "fr"}, {Language: "en", Forced: true}, {Language: ""}}}
	f := factsOf(i)
	want := map[string]any{"resolution": "1080p", "width": 1920, "height": 800, "runtime_seconds": 7201, "audio_channels": "5.1",
		"audio_codec": "eac3", "hdr": "HDR10", "bit_depth": 10, "frame_rate": 23.976, "bitrate": int64(8_000_000),
		"video_codec": "hevc", "container": "matroska", "facts_version": FactsVersion}
	for k, v := range want {
		if f[k] != v {
			t.Errorf("%s = %#v, want %#v", k, f[k], v)
		}
	}
	if !reflect.DeepEqual(f["subtitle_languages"], []string{"en", "fr"}) {
		t.Errorf("subtitles %#v", f["subtitle_languages"])
	}
	bare := factsOf(mediafiles.Info{Status: "unsupported"})
	for _, k := range HeaderFactKeys {
		if _, ok := bare[k]; ok {
			t.Errorf("unknown %s stored as %#v", k, bare[k])
		}
	}
	if NeedsFacts(bare) || !NeedsFacts(map[string]any{"inspection_status": "complete"}) || NeedsFacts(map[string]any{"facts_version": json.Number("2")}) {
		t.Error("facts version check")
	}
	if channelWords(2) != "2.0" || channelWords(8) != "7.1" {
		t.Error("channel words")
	}
}

func TestSampleByLength(t *testing.T) {
	for _, c := range []struct {
		secs, minutes int
		want          bool
	}{
		{0, 120, false},
		{10, 0, true},
		{60, 0, false},
		{60, 2, false},
		{60, 8, true},
		{120, 8, false},
		{240, 22, true},
		{400, 22, false},
		{300, 120, true},
		{90 * 60, 120, false},
	} {
		facts := map[string]any{}
		if c.secs > 0 {
			facts["runtime_seconds"] = float64(c.secs)
		}
		if got := sampleByLength(facts, c.minutes); got != c.want {
			t.Errorf("%ds for %d min = %v, want %v", c.secs, c.minutes, got, c.want)
		}
	}
}

func TestLengthFloorFollowsSizeFloors(t *testing.T) {
	special := map[string]any{"runtime_seconds": 240}
	if !sampleByLength(special, fakeRules{floors: true}.lengthMinutes(45)) {
		t.Error("floors on: 4 minutes for 45 should be a sample")
	}
	if sampleByLength(special, fakeRules{}.lengthMinutes(45)) {
		t.Error("floors off: refused by length")
	}
	if !sampleByLength(map[string]any{"runtime_seconds": 10}, fakeRules{}.lengthMinutes(45)) {
		t.Error("floors off: 10 seconds should still be a sample")
	}
}

func TestLieOfUsesRealPictureSize(t *testing.T) {
	rules := mediafiles.PickRulesBody{}
	name := "Harbor.of.Glass.2018.1080p.BluRay.x264-GRP"
	if why := lieOf(name, name, false, "", rules, map[string]any{"width": 1280, "height": 720}); !strings.Contains(why, "file is 720p, name said 1080p") {
		t.Fatalf("720p named 1080p: %q", why)
	}
	if why := lieOf(name, "", false, "", rules, map[string]any{"width": 1280, "height": 536}); why == "" {
		t.Fatal("720p named 1080p, no Release name")
	}
	if why := lieOf(name, name, false, "", rules, map[string]any{"width": 1920, "height": 800}); why != "" {
		t.Fatal(why)
	}
	if why := lieOf("Harbor.of.Glass.2018.720p.BluRay", name, false, "", rules, map[string]any{"width": 1920, "height": 1080}); why != "" {
		t.Fatal(why)
	}
	dvd := "Harbor.of.Glass.2018.576p.DVDRip.x264-GRP"
	if why := lieOf(dvd, dvd, false, "", rules, map[string]any{"width": 720, "height": 436, "display_width": 1018}); why != "" {
		t.Fatal(why)
	}
	if f := factsOf(mediafiles.Info{Width: 720, Height: 436, DisplayWidth: 1018}); f["display_width"] != 1018 || f["resolution"] != "576p" {
		t.Errorf("anamorphic facts %#v", f)
	}
	if why := lieOf("Harbor.of.Glass.2018.720p.BluRay", name, false, "", rules, nil); !strings.Contains(why, "file is 720p, name said 1080p") {
		t.Fatalf("name check lost: %q", why)
	}
}
