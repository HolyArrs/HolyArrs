package subtitles

import "strings"

// Old apps (Sonarr, Radarr) number languages their own way; these tables
// turn those ids and names into HolyArrs language codes.

// OldAppLanguages name each side's language ids. They match up to 25 and
// differ from 26 on; only Radarr has "Any".
var OldAppLanguages = map[string]map[int64]string{
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

// OldAppLanguageCode maps an old app's language name to a HolyArrs language
// code, "original" or "any"; "" when there is none.
func OldAppLanguageCode(name string) string {
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
	return NormLanguage(n)
}

// OldAppLanguageCodeByID is the HolyArrs code of a side's language id ("sonarr"
// numbering for Sonarr, Radarr's otherwise); "" for none or a non-language.
func OldAppLanguageCodeByID(side string, id int64) string {
	if side != "sonarr" {
		side = "radarr"
	}
	if id <= 0 {
		return ""
	}
	return OldAppLanguageCode(OldAppLanguages[side][id])
}
