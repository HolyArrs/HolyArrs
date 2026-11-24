package releasename

import (
	"fmt"
	"regexp"
	"strings"
)

// The quality words of a release name, in one ordered table per field. Each
// table is read once per name; a word earlier in a table wins over a later
// one, as the old apps rank them (a remux word beats a Blu-ray word, a real
// source beats a pre-release copy).

// sourceRule is one source and the words that name it.
type sourceRule struct {
	source string
	conf   Conf
	words  []string
}

// sourceRules, best first. Short words that are also ordinary words (TC, WP,
// SCR, PAL, TBS) are read only after the title, where readTags looks.
var sourceRules = []sourceRule{
	{"disc_image", Sure, []string{"iso", "bdmv", "video_ts", "brdisk", "bd25", "bd50", "bd66", "bd100", "bdiso"}},
	{"remux", Sure, []string{"remux", "bdremux", "uhdremux"}},
	{"screener", Sure, []string{"dvdscr", "dvdscreener", "bdscr", "screener", "scr"}},
	{"bluray", Sure, []string{"bluray", "blu-ray", "bdrip", "brrip", "bd", "uhdbd", "uhd-bd", "uhdbdrip", "bdlight",
		"bdmux", "hddvd", "hd-dvd", "hddvdrip"}},
	{"web_dl", Sure, []string{"web-dl", "webdl", "webhd", "web-hd", "ituneshd", "amazonhd", "amazonsd", "amazonuhd",
		"netflixhd", "netflixuhd", "disneyhd", "hbomaxhd", "maxdomehd"}},
	{"web_rip", Sure, []string{"webrip", "web-rip", "webmux", "hdrip"}},
	{"web_dl", Likely, []string{"web"}},
	{"tv", Sure, []string{"hdtv", "pdtv", "sdtv", "ahdtv", "uhdtv", "hdtvrip", "tvrip", "dsr", "dsrip", "dthrip", "dvbrip",
		"satrip", "dvb", "dvb-s", "dvb-s2", "dvb-t", "dvb-t2", "dvb-c", "rawhd", "ppv", "bs11", "bs12", "tbs", "at-x"}},
	{"dvd", Sure, []string{"dvdrip", "dvd", "dvdr", "dvd-r", "dvd5", "dvd9", "ntsc", "pal", "vhsrip", "vhs", "ldrip", "laserdisc"}},
	{"workprint", Sure, []string{"workprint", "wp"}},
	{"cam", Sure, []string{"hdcam", "cam", "camrip"}},
	{"telesync", Sure, []string{"hdts", "ts", "telesync", "pdvd"}},
	{"telecine", Sure, []string{"hdtc", "tc", "telecine"}},
	{"regional", Sure, []string{"r5", "r6"}},
}

// sourceRank maps a source word to its place in sourceRules.
var sourceRank = func() map[string]int {
	m := map[string]int{}
	for i, r := range sourceRules {
		for _, w := range r.words {
			if _, ok := m[w]; !ok {
				m[w] = i
			}
		}
	}
	return m
}()

// resWords are resolution words with no number; resRank orders them under
// a written number (2160p, 1920x1080), which always wins.
var resWords = map[string]int{"8k": 4320, "4k": 2160, "uhd": 2160, "uhdbd": 2160, "uhd-bd": 2160, "uhdbdrip": 2160,
	"uhdremux": 2160, "uhdtv": 2160, "amazonuhd": 2160, "netflixuhd": 2160, "fhd": 1080, "hd": 720}

var (
	reResAny   = regexp.MustCompile(`^(360|480|540|576|720|1080|1440|2160|4320)[pi]$`)
	reResPixel = regexp.MustCompile(`^(\d{3,4})x(\d{3,4})$`)
	reResBare  = regexp.MustCompile(`^(720|1080|2160|4320)$`)
	// reGlued is a source word written onto its resolution: BD1080p, WEB720p.
	reGlued = regexp.MustCompile(`^(bd|bluray|web|webrip|webdl|hdtv)(\d{3,4}[pi])$`)
)

// pixelRes reads a frame size (1920x1080, 1920x800 for a wide crop) by its width.
func pixelRes(w, h int) int {
	switch {
	case w >= 7000:
		return 4320
	case w >= 3500:
		return 2160
	case w >= 2400:
		return 1440
	case w >= 1800:
		return 1080
	case w >= 1200:
		return 720
	case w >= 900:
		return 540
	case h > 480:
		return 576
	}
	return 480
}

// codecWords map a codec word to its codec. x264/h.264 forms are read by reH26x.
var codecWords = map[string]string{"hevc": "h265", "avc": "h264", "av1": "av1", "vp9": "vp9", "xvid": "xvid", "divx": "divx",
	"vc1": "vc1", "vc-1": "vc1", "mpeg2": "mpeg2", "mpeg-2": "mpeg2", "ma10p": "h265", "hi10": "h264", "hi10p": "h264"}

// audioRank orders audio formats; a name that lists two keeps the better.
var audioRank = map[string]int{"mp3": 1, "aac": 2, "opus": 3, "ac3": 3, "dts": 4, "eac3": 4, "dts_es": 5,
	"dts_hd_hra": 6, "flac": 7, "lpcm": 8, "dts_hd_ma": 8, "truehd": 9, "dts_x": 9}

// audioWords map audio words with no channel count glued on.
var audioWords = map[string]string{"dts-x": "dts_x", "dtsx": "dts_x", "dts-es": "dts_es", "dts-hd": "dts_hd_ma",
	"dtshd": "dts_hd_ma", "dts-hdma": "dts_hd_ma", "dts-ma": "dts_hd_ma", "he-aac": "aac", "e-ac-3": "eac3", "pcm": "lpcm"}

var audioGlued = map[string]string{"ddpa": "eac3", "ddp": "eac3", "dd+": "eac3", "eac3": "eac3", "ac3": "ac3", "dd": "ac3",
	"dts": "dts", "aac": "aac", "truehd": "truehd", "flac": "flac", "opus": "opus", "mp3": "mp3", "lpcm": "lpcm"}

// readQuality reads source, resolution, codec and audio from the tags.
func readQuality(r *Result, low []string, flags map[string]bool) {
	// Source: the best-ranked word wins.
	best := len(sourceRules)
	complete := false
	for i, t := range low {
		if m := reGlued.FindStringSubmatch(t); m != nil {
			t = m[1]
		}
		if t == "tv" && i > 0 && low[i-1] == "hd" { // "HD.TV"
			t = "hdtv"
		}
		if k, ok := sourceRank[t]; ok && k < best {
			best = k
		}
		complete = complete || t == "complete"
	}
	encoded := reH26x.MatchString(" "+strings.Join(low, " ")+" ") || hasAny(low, "xvid", "divx")
	// A whole Blu-ray: "COMPLETE.BLURAY" with no encode or remux word.
	if complete && best == sourceRank["bluray"] && !encoded && hasAny(low, "bluray", "blu-ray", "uhd") {
		best = sourceRank["iso"]
	}
	if best < len(sourceRules) {
		r.Source = Field[string]{sourceRules[best].source, sourceRules[best].conf}
		if r.Source.Value == "disc_image" {
			flags["disc_image"] = true
		}
	} else {
		r.Source.Conf = Unknown
	}

	// Resolution. A written number wins over a word; two different written
	// numbers make it a guess and the later one wins. UHD and 4K count only
	// when no number is written ("720p.UHD.BluRay" is a 720p cut of a UHD
	// disc, as Sonarr and Radarr read it).
	var seen []int
	word := 0
	for i, t := range low {
		if m := reGlued.FindStringSubmatch(t); m != nil {
			t = m[2]
		}
		switch m := reResAny.FindStringSubmatch(t); {
		case m != nil:
			seen = append(seen, atoi(m[1]))
		case reResPixel.MatchString(t):
			p := reResPixel.FindStringSubmatch(t)
			seen = append(seen, pixelRes(atoi(p[1]), atoi(p[2])))
		case reResBare.MatchString(t) && (i > 0 && sourceOrRes(low[i-1]) || i+1 < len(low) && sourceOrRes(low[i+1])):
			seen = append(seen, atoi(t)) // "UHDBD.2160", a number next to a source word
		default:
			if v := resWords[t]; v > word && (t != "hd" || word == 0) {
				word = v
			}
		}
	}
	switch {
	case len(seen) > 0:
		r.Resolution = sure(seen[len(seen)-1])
		for _, v := range seen {
			if v != r.Resolution.Value {
				r.Resolution.Conf = Guess
				r.Notes = append(r.Notes, fmt.Sprintf("read as %dp; the name also says %dp", r.Resolution.Value, v))
				break
			}
		}
	case word == 720: // bare "HD" says little
		r.Resolution = likely(720)
	case word > 0:
		r.Resolution = sure(word)
	default:
		r.Resolution.Conf = Unknown
		// What the source says when no size is written, as the old apps read it.
		switch r.Source.Value {
		case "remux", "disc_image":
			r.Resolution = likely(1080)
		case "bluray":
			switch {
			case hasAny(low, "xvid", "divx", "bdrip", "brrip", "bdlight", "hddvdrip"):
				// A rip with no size written is an SD rip, as Sonarr and Radarr read it.
				r.Resolution = likely(480)
			default:
				r.Resolution = likely(1080)
			}
		case "dvd":
			if hasAny(low, "pal") {
				r.Resolution = likely(576)
			} else {
				r.Resolution = likely(480)
			}
		case "tv", "screener", "regional":
			r.Resolution = likely(480)
		case "web_rip":
			if hasAny(low, "hdrip") {
				r.Resolution = likely(720)
			}
		}
	}

	// Codec.
	line := " " + strings.Join(low, " ") + " "
	r.Codec.Conf = Unknown
	if m := reH26x.FindStringSubmatch(line); m != nil {
		r.Codec = sure(map[string]string{"4": "h264", "5": "h265"}[m[1]])
	} else {
		for _, t := range low {
			if c, ok := codecWords[t]; ok && (r.Codec.Value == "" || t == "hevc" || t == "avc") {
				r.Codec = sure(c)
			}
		}
	}
	if r.Codec.Value == "" && hasAny(low, "dvdr", "dvd-r", "dvd5", "dvd9") {
		r.Codec = likely("mpeg2") // a whole DVD
	}

	// Audio: the best format named.
	for i, t := range low {
		f := audioWords[t]
		if m := reDDP.FindStringSubmatch(t); f == "" && m != nil {
			f = audioGlued[m[1]]
			if m[1] == "ddpa" {
				r.Audio.Atmos = true
			}
		}
		if f == "dts" && i+1 < len(low) && low[i+1] == "x" { // "DTS.X"
			f = "dts_x"
		}
		if f == "dts_hd_ma" && i+1 < len(low) && low[i+1] == "hra" {
			f = "dts_hd_hra"
		}
		if f != "" && audioRank[f] > audioRank[r.Audio.Format] {
			r.Audio.Format = f
		}
	}
	if m := reChannels.FindStringSubmatch(line); m != nil {
		r.Audio.Channels = m[1] + "." + m[2]
	}
	r.Audio.Atmos = r.Audio.Atmos || hasAny(low, "atmos")
}

// sourceOrRes says a word is a source or a resolution word.
func sourceOrRes(t string) bool {
	_, src := sourceRank[t]
	return src || resWords[t] > 0
}

func hasAny(low []string, words ...string) bool {
	for _, t := range low {
		for _, w := range words {
			if t == w {
				return true
			}
		}
	}
	return false
}
