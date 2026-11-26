package mediafiles

import (
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// DevicePenalty is what each format costs that a ticked device can't play
// directly, so the media server would have to convert it. It only lowers
// points: a Release is never refused for it.
const DevicePenalty = 10

// Devices are the device families Pick rules can say they play on.
var Devices = []string{"apple_tv", "fire_tv", "roku", "chromecast", "web_browser"}

// deviceFeatures are the formats read from a release name, in report order.
var deviceFeatures = []string{"hevc", "av1", "hdr", "dv_only", "dts", "truehd", "eac3", "ac3"}

// cantPlay lists, per device family, what its own player app commonly
// can't show directly (the common current model; Plex and Jellyfin apps).
var cantPlay = map[string][]string{
	"apple_tv":    {"av1", "dts", "truehd"},
	"fire_tv":     {"dts", "truehd"},
	"roku":        {"dts", "truehd"},
	"chromecast":  {"av1", "dts", "truehd"},
	"web_browser": {"hevc", "hdr", "dv_only", "dts", "truehd", "eac3", "ac3"},
}

// DeviceMiss is one format of a Release that some ticked devices can't play.
type DeviceMiss struct {
	Feature string
	Devices []string
}

func deviceFeatureOf(f string, p releasename.Result) bool {
	hdr := p.HDR.Value
	switch f {
	case "hevc":
		return p.Codec.Value == "h265"
	case "av1":
		return p.Codec.Value == "av1"
	case "hdr":
		return len(hdr) > 0
	case "dv_only":
		return slices.Contains(hdr, "dv") && !slices.Contains(hdr, "hdr10") && !slices.Contains(hdr, "hdr10plus")
	case "dts":
		return strings.HasPrefix(p.Audio.Format, "dts")
	case "truehd":
		return p.Audio.Format == "truehd"
	case "eac3":
		return p.Audio.Format == "eac3"
	case "ac3":
		return p.Audio.Format == "ac3"
	}
	return false
}

// deviceMisses lists the formats of p that need converting on devices.
// Dolby Vision without HDR10 only counts on devices that show HDR at all.
func deviceMisses(p releasename.Result, devices []string) []DeviceMiss {
	if len(devices) == 0 || p.Music != nil {
		return nil
	}
	var out []DeviceMiss
	for _, f := range deviceFeatures {
		if !deviceFeatureOf(f, p) {
			continue
		}
		var who []string
		for _, d := range devices {
			no := cantPlay[d]
			if slices.Contains(no, f) && (f != "dv_only" || !slices.Contains(no, "hdr")) {
				who = append(who, d)
			}
		}
		if len(who) > 0 {
			out = append(out, DeviceMiss{Feature: f, Devices: who})
		}
	}
	return out
}

var deviceWords = map[string]string{"hevc": "video is HEVC", "av1": "video is AV1", "hdr": "has HDR", "dv_only": "has Dolby Vision without HDR10",
	"dts": "audio is DTS", "truehd": "audio is TrueHD", "eac3": "audio is DDP (E-AC-3)", "ac3": "audio is AC-3"}

var deviceNames = map[string]string{"apple_tv": "Apple TV", "fire_tv": "Fire TV", "roku": "Roku", "chromecast": "Chromecast", "web_browser": "a web browser"}

func (m DeviceMiss) why() string {
	names := make([]string, len(m.Devices))
	for i, d := range m.Devices {
		names[i] = deviceNames[d]
	}
	return deviceWords[m.Feature] + ", which " + strings.Join(names, ", ") + " can't play directly"
}
