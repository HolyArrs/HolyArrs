package subtext

// The word lists below are this project's own. Each phrase is matched in
// lower case as whole words.

// adAnyLanguage marks ads and credits in any language: subtitle site names
// and the credits that speech recognition is known to invent from silence.
var adAnyLanguage = []string{
	"opensubtitles", "subscene", "addic7ed", "podnapisi", "yify", "yts", "tvsubtitles",
	"subdl", "gestdown", "bsplayer", "subsource", "napiprojekt",
	"amara.org", "amara org",
}

// adByLanguage holds the ad and credit phrases of each language.
var adByLanguage = map[string][]string{
	"en": {
		"subtitles by", "subtitle by", "subtitled by", "subs by", "captions by", "captioning by", "captioned by",
		"synced by", "sync by", "synced and corrected by", "resynced by", "re-synced by", "corrected by",
		"ripped by", "encoded by", "transcript by", "transcribed by", "translated by", "translation by",
		"support us", "become a member", "vip member", "advertise here", "advertise your product",
		"advertise your brand", "rate this subtitle", "downloaded from", "thanks for watching",
		"thank you for watching", "please subscribe", "like and subscribe",
	},
}

// webTLDs are the top-level domains that make a word read as a web address.
var webTLDs = map[string]bool{
	"com": true, "net": true, "org": true, "tv": true, "io": true, "info": true, "me": true, "co": true,
	"to": true, "cc": true, "biz": true, "xyz": true, "app": true, "site": true, "online": true,
	"club": true, "ws": true, "ru": true, "su": true, "mx": true,
}

// enLForI are English words OCR writes with a lower-case l for the capital I.
// None of them is an English word as written.
var enLForI = map[string]bool{
	"l": true, "l'm": true, "l'll": true, "l've": true, "l'd": true,
	"lt": true, "lt's": true, "lf": true, "ln": true, "ls": true,
}

// enLowerI are the English words that start with a capital I in any
// position, for lines turned from capitals into sentence case.
var enLowerI = map[string]bool{"i": true, "i'm": true, "i'll": true, "i've": true, "i'd": true}
