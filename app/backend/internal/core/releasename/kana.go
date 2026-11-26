package releasename

import (
	"strings"
	"unicode"
)

// kanaReading is the title with the kana of mixed words spelled out, for the
// matcher to try next to the usual title. Only words that mix Latin letters
// with kana change ("Re:ゼロ" becomes "Re:zero"). A word with kanji or hangul
// in it, or one of kana alone, is handled as latinRun handles it. It returns
// "" when no word was spelled out, so the usual title is the only reading.
func kanaReading(words []string) string {
	var spelled []string
	for i, w := range words {
		if !mixedKana(w) {
			continue
		}
		if spelled == nil {
			spelled = append([]string(nil), words...)
		}
		spelled[i] = romaji(w)
	}
	if spelled == nil {
		return ""
	}
	return strings.Join(latinRun(spelled), " ")
}

// mixedKana says a word has both a Latin letter and a kana, and nothing from
// Han or Hangul. Without the Latin letter the spelling would only invent
// words, and a kanji beside kana ("ゼロから始める") has readings a table
// cannot give.
func mixedKana(w string) bool {
	latin, kana := false, false
	for _, c := range w {
		switch {
		case isLatin(c):
			latin = true
		case isKana(c):
			kana = true
		case unicode.In(c, unicode.Han, unicode.Hangul):
			return false
		}
	}
	return latin && kana
}

func isKana(c rune) bool {
	return unicode.In(c, unicode.Hiragana, unicode.Katakana) || c == 'ー' || c == '・'
}

// kanaRows holds the plain katakana syllables, five vowels to a row.
var kanaRows = []struct{ first, romaji string }{
	{"アイウエオ", "a i u e o"},
	{"カキクケコ", "ka ki ku ke ko"},
	{"ガギグゲゴ", "ga gi gu ge go"},
	{"サシスセソ", "sa shi su se so"},
	{"ザジズゼゾ", "za ji zu ze zo"},
	{"タチツテト", "ta chi tsu te to"},
	{"ダヂヅデド", "da ji zu de do"},
	{"ナニヌネノ", "na ni nu ne no"},
	{"ハヒフヘホ", "ha hi fu he ho"},
	{"バビブベボ", "ba bi bu be bo"},
	{"パピプペポ", "pa pi pu pe po"},
	{"マミムメモ", "ma mi mu me mo"},
	{"ヤ・ユ・ヨ", "ya - yu - yo"},
	{"ラリルレロ", "ra ri ru re ro"},
	{"ワヰ・ヱヲ", "wa i - e o"},
	{"ヴ", "vu"},
	{"ン", "n"},
}

var kanaTable = func() map[rune]string {
	m := map[rune]string{}
	for _, row := range kanaRows {
		syl := strings.Fields(row.romaji)
		for i, c := range []rune(row.first) {
			if syl[i] != "-" {
				m[c] = syl[i]
			}
		}
	}
	return m
}()

// smallVowel is what a small ァィゥェォ after a syllable puts in place of
// its vowel ("フ" + "ィ" is "fi", "テ" + "ィ" is "ti").
var smallVowel = map[rune]string{'ァ': "a", 'ィ': "i", 'ゥ': "u", 'ェ': "e", 'ォ': "o"}

// smallY is what a small ャュョ after an i-syllable adds ("キ" + "ャ" is "kya").
var smallY = map[rune]string{'ャ': "a", 'ュ': "u", 'ョ': "o"}

// stem is a syllable without its vowel, ready for a small vowel to follow.
func stem(syl string) string {
	if syl == "u" {
		return "w" // ウ + ィ is "wi"
	}
	return syl[:len(syl)-1]
}

// romaji spells out the hiragana and katakana of s in Latin letters, in
// plain Hepburn without macrons: a long-vowel mark adds nothing, and "ン" is
// always "n". Every other character stays as it is.
func romaji(s string) string {
	var out []string // one entry per syllable or other character, so a later mark can reach back
	var isSyl []bool // whether the entry is a syllable that a small kana may change
	add := func(x string, syl bool) {
		out = append(out, x)
		isSyl = append(isSyl, syl)
	}
	double := false
	for _, c := range s {
		if c >= 'ぁ' && c <= 'ゖ' {
			c += 'ァ' - 'ぁ' // hiragana to katakana
		}
		last := len(out) - 1
		afterSyl := last >= 0 && isSyl[last]
		_, vowel := smallVowel[c]
		y, small := smallY[c]
		switch {
		case c == 'ッ':
			double = true
			continue
		case c == 'ー':
			continue
		case c == '・':
			add(" ", false)
		case vowel && afterSyl:
			out[last] = stem(out[last]) + smallVowel[c]
			isSyl[last] = false
		case small && afterSyl && strings.HasSuffix(out[last], "i"):
			base := strings.TrimSuffix(out[last], "i")
			if !strings.HasSuffix(base, "sh") && !strings.HasSuffix(base, "ch") && base != "j" {
				base += "y"
			}
			out[last] = base + y
			isSyl[last] = false
		case small:
			add("y"+y, false)
		case kanaTable[c] != "":
			syl := kanaTable[c]
			if double {
				if strings.HasPrefix(syl, "ch") {
					syl = "t" + syl
				} else if !strings.ContainsAny(syl[:1], "aiueon") {
					syl = syl[:1] + syl
				}
			}
			add(syl, true)
		default:
			add(string(c), false)
		}
		double = false
	}
	return strings.Join(out, "")
}
