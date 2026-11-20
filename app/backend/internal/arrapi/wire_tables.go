package arrapi

import (
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

// wireQuality is one quality as each side numbers it on the wire.
type wireQuality struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Source     string `json:"source,omitempty"`
	Resolution int    `json:"resolution"`
	Modifier   string `json:"modifier,omitempty"` // the Radarr side only
}

// sideQualities are the qualities each side lists in a profile, by the id
// the old app gives each, with the source, resolution and modifier the
// old app sends beside them. Ids 20 to 22 mean different things per side.
var sideQualities = map[string][]wireQuality{
	"radarr": {
		{ID: 0, Name: "Unknown", Source: "unknown", Resolution: 0, Modifier: "none"},
		{ID: 1, Name: "SDTV", Source: "tv", Resolution: 480, Modifier: "none"},
		{ID: 2, Name: "DVD", Source: "dvd", Resolution: 0, Modifier: "none"},
		{ID: 3, Name: "WEBDL-1080p", Source: "webdl", Resolution: 1080, Modifier: "none"},
		{ID: 4, Name: "HDTV-720p", Source: "tv", Resolution: 720, Modifier: "none"},
		{ID: 5, Name: "WEBDL-720p", Source: "webdl", Resolution: 720, Modifier: "none"},
		{ID: 6, Name: "Bluray-720p", Source: "bluray", Resolution: 720, Modifier: "none"},
		{ID: 7, Name: "Bluray-1080p", Source: "bluray", Resolution: 1080, Modifier: "none"},
		{ID: 8, Name: "WEBDL-480p", Source: "webdl", Resolution: 480, Modifier: "none"},
		{ID: 9, Name: "HDTV-1080p", Source: "tv", Resolution: 1080, Modifier: "none"},
		{ID: 10, Name: "Raw-HD", Source: "tv", Resolution: 1080, Modifier: "rawhd"},
		{ID: 12, Name: "WEBRip-480p", Source: "webrip", Resolution: 480, Modifier: "none"},
		{ID: 14, Name: "WEBRip-720p", Source: "webrip", Resolution: 720, Modifier: "none"},
		{ID: 15, Name: "WEBRip-1080p", Source: "webrip", Resolution: 1080, Modifier: "none"},
		{ID: 16, Name: "HDTV-2160p", Source: "tv", Resolution: 2160, Modifier: "none"},
		{ID: 17, Name: "WEBRip-2160p", Source: "webrip", Resolution: 2160, Modifier: "none"},
		{ID: 18, Name: "WEBDL-2160p", Source: "webdl", Resolution: 2160, Modifier: "none"},
		{ID: 19, Name: "Bluray-2160p", Source: "bluray", Resolution: 2160, Modifier: "none"},
		{ID: 20, Name: "Bluray-480p", Source: "bluray", Resolution: 480, Modifier: "none"},
		{ID: 21, Name: "Bluray-576p", Source: "bluray", Resolution: 576, Modifier: "none"},
		{ID: 22, Name: "BR-DISK", Source: "bluray", Resolution: 1080, Modifier: "brdisk"},
		{ID: 23, Name: "DVD-R", Source: "dvd", Resolution: 480, Modifier: "remux"},
		{ID: 24, Name: "WORKPRINT", Source: "workprint", Resolution: 0, Modifier: "none"},
		{ID: 25, Name: "CAM", Source: "cam", Resolution: 0, Modifier: "none"},
		{ID: 26, Name: "TELESYNC", Source: "telesync", Resolution: 0, Modifier: "none"},
		{ID: 27, Name: "TELECINE", Source: "telecine", Resolution: 0, Modifier: "none"},
		{ID: 28, Name: "DVDSCR", Source: "dvd", Resolution: 480, Modifier: "screener"},
		{ID: 29, Name: "REGIONAL", Source: "dvd", Resolution: 480, Modifier: "regional"},
		{ID: 30, Name: "Remux-1080p", Source: "bluray", Resolution: 1080, Modifier: "remux"},
		{ID: 31, Name: "Remux-2160p", Source: "bluray", Resolution: 2160, Modifier: "remux"},
	},
	"sonarr": {
		{ID: 0, Name: "Unknown", Source: "unknown", Resolution: 0}, {ID: 1, Name: "SDTV", Source: "television", Resolution: 480},
		{ID: 2, Name: "DVD", Source: "dvd", Resolution: 480}, {ID: 3, Name: "WEBDL-1080p", Source: "web", Resolution: 1080},
		{ID: 4, Name: "HDTV-720p", Source: "television", Resolution: 720}, {ID: 5, Name: "WEBDL-720p", Source: "web", Resolution: 720},
		{ID: 6, Name: "Bluray-720p", Source: "bluray", Resolution: 720},
		{ID: 7, Name: "Bluray-1080p", Source: "bluray", Resolution: 1080}, {ID: 8, Name: "WEBDL-480p", Source: "web", Resolution: 480},
		{ID: 9, Name: "HDTV-1080p", Source: "television", Resolution: 1080},
		{ID: 10, Name: "Raw-HD", Source: "televisionRaw", Resolution: 1080},
		{ID: 12, Name: "WEBRip-480p", Source: "webRip", Resolution: 480},
		{ID: 13, Name: "Bluray-480p", Source: "bluray", Resolution: 480},
		{ID: 14, Name: "WEBRip-720p", Source: "webRip", Resolution: 720},
		{ID: 15, Name: "WEBRip-1080p", Source: "webRip", Resolution: 1080},
		{ID: 16, Name: "HDTV-2160p", Source: "television", Resolution: 2160},
		{ID: 17, Name: "WEBRip-2160p", Source: "webRip", Resolution: 2160},
		{ID: 18, Name: "WEBDL-2160p", Source: "web", Resolution: 2160},
		{ID: 19, Name: "Bluray-2160p", Source: "bluray", Resolution: 2160},
		{ID: 20, Name: "Bluray-1080p Remux", Source: "blurayRaw", Resolution: 1080},
		{ID: 21, Name: "Bluray-2160p Remux", Source: "blurayRaw", Resolution: 2160},
		{ID: 22, Name: "Bluray-576p", Source: "bluray", Resolution: 576},
	},
}

// sideLanguages name each side's language ids. They match up to 25 and
// differ from 26 on; only Radarr has "Any".
var sideLanguages = map[string]map[int64]string{
	"radarr": {-1: "Any", -2: "Original", 0: "Unknown", 1: "English", 2: "French", 3: "Spanish", 4: "German", 5: "Italian",
		6: "Danish", 7: "Dutch", 8: "Japanese", 9: "Icelandic", 10: "Chinese", 11: "Russian", 12: "Polish", 13: "Vietnamese",
		14: "Swedish", 15: "Norwegian", 16: "Finnish", 17: "Turkish", 18: "Portuguese", 19: "Flemish", 20: "Greek", 21: "Korean",
		22: "Hungarian", 23: "Hebrew", 24: "Lithuanian", 25: "Czech", 26: "Hindi", 27: "Romanian", 28: "Thai", 29: "Bulgarian",
		30: "Portuguese (Brazil)", 31: "Arabic", 32: "Ukrainian", 33: "Persian", 34: "Bengali", 35: "Slovak", 36: "Latvian",
		37: "Spanish (Latino)", 38: "Catalan", 39: "Croatian", 40: "Serbian", 41: "Bosnian", 42: "Estonian", 43: "Tamil",
		44: "Indonesian", 45: "Telugu", 46: "Macedonian", 47: "Slovenian", 48: "Malayalam", 49: "Kannada", 50: "Albanian",
		51: "Afrikaans", 52: "Marathi", 53: "Tagalog", 54: "Urdu", 55: "Romansh", 56: "Mongolian", 57: "Georgian"},
	"sonarr": {-2: "Original", 0: "Unknown", 1: "English", 2: "French", 3: "Spanish", 4: "German", 5: "Italian", 6: "Danish",
		7: "Dutch", 8: "Japanese", 9: "Icelandic", 10: "Chinese", 11: "Russian", 12: "Polish", 13: "Vietnamese", 14: "Swedish",
		15: "Norwegian", 16: "Finnish", 17: "Turkish", 18: "Portuguese", 19: "Flemish", 20: "Greek", 21: "Korean", 22: "Hungarian",
		23: "Hebrew", 24: "Lithuanian", 25: "Czech", 26: "Arabic", 27: "Hindi", 28: "Bulgarian", 29: "Malayalam", 30: "Ukrainian",
		31: "Slovak", 32: "Thai", 33: "Portuguese (Brazil)", 34: "Spanish (Latino)", 35: "Romanian", 36: "Latvian", 37: "Persian",
		38: "Catalan", 39: "Croatian", 40: "Serbian", 41: "Bosnian", 42: "Estonian", 43: "Tamil", 44: "Indonesian",
		45: "Macedonian", 46: "Slovenian"},
}

// languageCode maps an old app's language name to a HolyArrs language
// code, "original" or "any"; "" when there is none.
func languageCode(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "original", "any":
		return n
	case "", "unknown":
		return ""
	case "flemish":
		return "nl"
	case "spanish (latino)":
		return "es"
	}
	return subtitles.NormLanguage(n)
}

// languageByCode finds a side's language for a HolyArrs code: its id and
// name, ok false when the side has none.
func languageByCode(side, code string) (int64, string, bool) {
	best := int64(-99)
	for id, n := range sideLanguages[side] {
		if id > 0 && languageCode(n) == code && (best == -99 || id < best) {
			best = id
		}
	}
	if best == -99 {
		return 0, "", false
	}
	return best, sideLanguages[side][best], true
}

// fullQuality is a file's or release's quality as the side sends it: the
// table entry for its name, plus a first revision. A name the side has no
// entry for keeps its name and resolution, with id 0 and source unknown,
// because Bazarr reads the resolution from the name.
func fullQuality(side, name string, resolution int) map[string]any {
	look := name
	if side == "radarr" && strings.HasPrefix(look, "Bluray-") && strings.HasSuffix(look, " Remux") {
		look = "Remux-" + strings.TrimSuffix(strings.TrimPrefix(look, "Bluray-"), " Remux") // Radarr's own name
	}
	if side == "radarr" && strings.HasPrefix(look, "BR-DISK-") {
		look = "BR-DISK"
	}
	q := wireQuality{Name: name, Source: "unknown", Resolution: resolution}
	if side == "radarr" {
		q.Modifier = "none"
	}
	if name == "" {
		q = sideQualities[side][0]
	}
	for _, s := range sideQualities[side] {
		if s.Name == look {
			q = s
			break
		}
	}
	return map[string]any{"quality": q, "revision": map[string]any{"version": 1, "real": 0, "isRepack": false}}
}
