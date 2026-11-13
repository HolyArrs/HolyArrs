package arrapi

import (
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

// wireQuality is one quality as each side numbers it on the wire.
type wireQuality struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// sideQualities are the qualities each side lists in a profile, by the id
// the old app gives each. Ids 20 to 22 mean different things per side.
var sideQualities = map[string][]wireQuality{
	"radarr": {{0, "Unknown"}, {1, "SDTV"}, {2, "DVD"}, {3, "WEBDL-1080p"}, {4, "HDTV-720p"}, {5, "WEBDL-720p"},
		{6, "Bluray-720p"}, {7, "Bluray-1080p"}, {8, "WEBDL-480p"}, {9, "HDTV-1080p"}, {10, "Raw-HD"}, {12, "WEBRip-480p"},
		{14, "WEBRip-720p"}, {15, "WEBRip-1080p"}, {16, "HDTV-2160p"}, {17, "WEBRip-2160p"}, {18, "WEBDL-2160p"},
		{19, "Bluray-2160p"}, {20, "Bluray-480p"}, {21, "Bluray-576p"}, {22, "BR-DISK"}, {23, "DVD-R"}, {24, "WORKPRINT"},
		{25, "CAM"}, {26, "TELESYNC"}, {27, "TELECINE"}, {28, "DVDSCR"}, {29, "REGIONAL"}, {30, "Remux-1080p"}, {31, "Remux-2160p"}},
	"sonarr": {{0, "Unknown"}, {1, "SDTV"}, {2, "DVD"}, {3, "WEBDL-1080p"}, {4, "HDTV-720p"}, {5, "WEBDL-720p"},
		{6, "Bluray-720p"}, {7, "Bluray-1080p"}, {8, "WEBDL-480p"}, {9, "HDTV-1080p"}, {10, "Raw-HD"}, {12, "WEBRip-480p"},
		{13, "Bluray-480p"}, {14, "WEBRip-720p"}, {15, "WEBRip-1080p"}, {16, "HDTV-2160p"}, {17, "WEBRip-2160p"},
		{18, "WEBDL-2160p"}, {19, "Bluray-2160p"}, {20, "Bluray-1080p Remux"}, {21, "Bluray-2160p Remux"}, {22, "Bluray-576p"}},
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
