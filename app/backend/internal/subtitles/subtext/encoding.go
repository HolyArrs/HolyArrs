package subtext

import (
	"strings"
	"unicode"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

// legacy is one old encoding a subtitle in some language may come in.
type legacy struct {
	name string
	enc  encoding.Encoding
}

var (
	cp1250 = legacy{"windows-1250", charmap.Windows1250}
	iso2   = legacy{"iso-8859-2", charmap.ISO8859_2}
	cp1251 = legacy{"windows-1251", charmap.Windows1251}
	koi8r  = legacy{"koi8-r", charmap.KOI8R}
	koi8u  = legacy{"koi8-u", charmap.KOI8U}
	cp1252 = legacy{"windows-1252", charmap.Windows1252}
	cp1253 = legacy{"windows-1253", charmap.Windows1253}
	iso7   = legacy{"iso-8859-7", charmap.ISO8859_7}
	cp1254 = legacy{"windows-1254", charmap.Windows1254}
	cp1255 = legacy{"windows-1255", charmap.Windows1255}
	iso8   = legacy{"iso-8859-8", charmap.ISO8859_8}
	cp1256 = legacy{"windows-1256", charmap.Windows1256}
	iso6   = legacy{"iso-8859-6", charmap.ISO8859_6}
	cp1257 = legacy{"windows-1257", charmap.Windows1257}
	iso13  = legacy{"iso-8859-13", charmap.ISO8859_13}
	cp1258 = legacy{"windows-1258", charmap.Windows1258}
	cp874  = legacy{"windows-874", charmap.Windows874}
	gb     = legacy{"gb18030", simplifiedchinese.GB18030}
	big5   = legacy{"big5", traditionalchinese.Big5}
	sjis   = legacy{"shift_jis", japanese.ShiftJIS}
	eucjp  = legacy{"euc-jp", japanese.EUCJP}
	euckr  = legacy{"euc-kr", korean.EUCKR}
)

// Letters that show a decoding went right, by script. A wrong old encoding
// turns them into symbols, control codes or rare characters.
const (
	latinMarks = "ąćęłńóśźżčďěňřšťůžáéíúýőűöüôĺľŕăâîșțşţđ"
	cyrillic   = "оеаинтсрвлкмдпуяыьгзбчйхжшюцщэфъёіїєґўј"
	greek      = "αεοιτνσςηρπκμλυδγωθχφβξζψάέήίόύώ"
	// The most used Chinese characters, simplified and traditional. They make
	// up a large share of any real text and almost none of a misread one.
	hanzi = "的一是不了人我在有他这中大来上国个到说们为子和你地出道也时年得就那要下以生会自着去之过家学对可她里后小么心多天而能好都然没日于起" +
		"还发成事只作当想看文无开手十用主行方又如前所本见经头面公同三已老从动两长知民样现分将外但身些与高意进把法此实回二理美点月明其种声全工" +
		"己话儿者向情部正名定女问力机给等几很业最间新什打便位因重被走电四第门相次东政海口使教西再平真听世气信北少关并内加化由却代军产入先山" +
		"五太水万市眼体别处总才场师书比住员九笑性通目华报立马命张活难神数件安表原车白应路期叫死常提感金何更反合放做系计或司利受光王果亲界及" +
		"今京务制解各任至清物台象记边共风战干接它许八特觉望直服毛林题建南度统色字请交爱让认算论百吃义科怎元社术结六功指思非流每青管夫连远" +
		"吗呢吧啊谢妈爸哥姐们這個來說們為時國會過對還發後裡麼當開問從見現點東動長頭與無樣將實話氣進處邊讓電總師難歡聽寫錢錯買賣車謝媽嗎"
	kana = "のはにをがでとたしてもかなるいうれまっすあこおよりらんだそ"
)

var (
	latinSet    = runeSet(latinMarks + strings.ToUpper(latinMarks))
	cyrillicSet = runeSet(cyrillic)
	greekSet    = runeSet(greek)
	hanziSet    = runeSet(hanzi)
	kanaSet     = runeSet(kana)
)

func runeSet(s string) map[rune]bool {
	m := map[rune]bool{}
	for _, r := range s {
		m[r] = true
	}
	return m
}

// guess is how to read a language's old files: the encodings to try, best
// first, and the letters a right reading is full of.
type guess struct {
	encs   []legacy
	common map[rune]bool
}

func guessFor(lang string) guess {
	base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(lang)), "-")
	switch base {
	case "pl", "cs", "sk", "hu", "sl", "hr", "bs", "ro", "sq":
		return guess{[]legacy{cp1250, iso2}, latinSet}
	case "ru", "be", "bg", "mk", "sr":
		return guess{[]legacy{cp1251, koi8r}, cyrillicSet}
	case "uk":
		return guess{[]legacy{cp1251, koi8u}, cyrillicSet}
	case "el":
		return guess{[]legacy{cp1253, iso7}, greekSet}
	case "tr", "az":
		return guess{[]legacy{cp1254}, nil}
	case "he":
		return guess{[]legacy{cp1255, iso8}, nil}
	case "ar", "fa", "ur":
		return guess{[]legacy{cp1256, iso6}, nil}
	case "lt", "lv", "et":
		return guess{[]legacy{cp1257, iso13}, latinSet}
	case "vi":
		return guess{[]legacy{cp1258}, nil}
	case "th":
		return guess{[]legacy{cp874}, nil}
	case "zh":
		return guess{[]legacy{gb, big5}, hanziSet}
	case "ja":
		return guess{[]legacy{sjis, eucjp}, kanaSet}
	case "ko":
		return guess{[]legacy{euckr}, nil}
	}
	return guess{}
}

// decodeLegacy reads bytes that are not Unicode. The subtitle's language
// picks the old encodings to try and the one whose text scores best wins.
// Windows-1252 is the last resort, and the only choice when the language is
// unknown. A tie goes to the encoding listed first.
func decodeLegacy(b []byte, lang string) ([]byte, string, error) {
	g := guessFor(lang)
	encs := append(g.encs, cp1252)
	var best []byte
	bestName, bestScore := "", 0
	var firstErr error
	for _, e := range encs {
		out, err := e.enc.NewDecoder().Bytes(b)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		// One choice needs no scoring.
		if len(encs) == 1 {
			return out, e.name, nil
		}
		s := textScore(out, g.common)
		if best == nil || s > bestScore {
			best, bestName, bestScore = out, e.name, s
		}
	}
	if best == nil {
		return nil, "", firstErr
	}
	return best, bestName, nil
}

// textScore rates a decoded text. Common letters of the language score high,
// other letters a little, and replacement characters, control codes and
// private-use characters count against it.
func textScore(b []byte, common map[rune]bool) int {
	score := 0
	for _, r := range string(b) {
		switch {
		case r < 0x80:
		case r == unicode.ReplacementChar || (r >= 0x80 && r < 0xA0) || unicode.Is(unicode.Co, r):
			score -= 10
		case common[r]:
			score += 3
		case unicode.IsLetter(r):
			if common != nil && !unicode.IsLower(r) && cyrillicOrGreek(r) {
				continue // a misread Cyrillic or Greek text is mostly capitals
			}
			score++
		default:
			score-- // a symbol where a letter belongs
		}
	}
	return score
}

func cyrillicOrGreek(r rune) bool {
	return unicode.Is(unicode.Cyrillic, r) || unicode.Is(unicode.Greek, r)
}

// Decode turns subtitle bytes into UTF-8. A byte-order mark is honoured and
// valid UTF-8 is kept; anything else is read in the old encoding that suits
// lang best, a two-letter language code that may be empty. It returns the
// text and the encoding it was read in.
func Decode(b []byte, lang string) (string, string, error) {
	return decode(b, lang)
}
