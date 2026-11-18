package mediafiles

import "strings"

// audioLabels are the display names of the audio codecs the release parser
// and the media inspection read.
var audioLabels = map[string]string{
	"aac": "AAC", "ac3": "AC3", "eac3": "EAC3", "dts": "DTS", "dts_hd_ma": "DTS-HD", "truehd": "TrueHD",
	"flac": "FLAC", "alac": "ALAC", "mp3": "MP3", "opus": "Opus", "vorbis": "Vorbis", "pcm": "PCM",
}

// AudioLabel names one audio codec for a file name, such as EAC3; "" when
// the codec is not known. It reads the release parser's format ("eac3") and
// the media inspection's ("ac3").
func AudioLabel(codec string) string {
	c := strings.ToLower(strings.TrimSpace(codec))
	if c == "" {
		return ""
	}
	if l, ok := audioLabels[c]; ok {
		return l
	}
	return strings.ToUpper(c)
}

// hdrLabels are the display names of the HDR kinds the release parser and the
// media inspection read.
var hdrLabels = map[string]string{"dv": "DolbyVision", "hdr": "HDR10", "hdr10": "HDR10", "hdr10plus": "HDR10+", "hlg": "HLG"}

// HDRLabel names a file's HDR kinds for a file name, such as DolbyVision or
// DolbyVisionHDR10; "" for SDR. It reads the release parser's kinds ("dv",
// "hdr10", "hdr10plus") and the media inspection's name ("DV+HDR10"), so a
// file name says the same thing whichever named it.
func HDRLabel(kinds string) string {
	// "HDR10+" is one word with a plus in it, not two.
	in := strings.ReplaceAll(strings.ToLower(kinds), "hdr10+", "hdr10plus ")
	var keys []string
	seen := map[string]bool{}
	for _, k := range strings.FieldsFunc(in, func(r rune) bool {
		return r == '+' || r == ' ' || r == ',' || r == '/'
	}) {
		if _, ok := hdrLabels[k]; !ok || seen[k] {
			continue
		}
		seen[k] = true
		keys = append(keys, k)
	}
	var out strings.Builder
	for _, k := range keys {
		// HDR10+ already says HDR10, so the base layer is not named again.
		if seen["hdr10plus"] && (k == "hdr10" || k == "hdr") || seen["hdr10"] && k == "hdr" {
			continue
		}
		out.WriteString(hdrLabels[k])
	}
	return out.String()
}
