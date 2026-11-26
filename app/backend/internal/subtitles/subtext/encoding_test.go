package subtext

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

func encode(t *testing.T, e encoding.Encoding, s string) []byte {
	t.Helper()
	b, err := e.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatalf("encode %q: %v", s, err)
	}
	return b
}

func srtOf(lines ...string) string {
	var sb strings.Builder
	for i, l := range lines {
		sb.WriteString(strings.Join([]string{itoa(i + 1), "00:00:0" + itoa(i) + ",000 --> 00:00:0" + itoa(i+1) + ",000", l, "", ""}, "\n"))
	}
	return sb.String()
}

func itoa(i int) string { return string(rune('0' + i)) }

func TestDecodeOldEncodingsByLanguage(t *testing.T) {
	polish := srtOf("Zażółć gęślą jaźń.", "Śmiało, Łukasz! Źle się czuję.", "Ćwiczę na łące, żółw śpi.")
	simplified := srtOf("我们现在就走吧。", "你说的是真的吗？", "这个问题很难回答。")
	traditional := srtOf("我們現在就走吧。", "你說的是真的嗎？", "這個問題很難回答。")
	russian := srtOf("Привет, как дела?", "Мы уходим прямо сейчас.", "Это очень хорошо.")
	jp := srtOf("これはテストです。", "わたしはここにいます。", "ありがとうございました。")
	for _, tc := range []struct {
		name, lang, want, enc string
		e                     encoding.Encoding
	}{
		{"polish windows-1250", "pl", polish, "windows-1250", charmap.Windows1250},
		{"polish iso-8859-2", "pl", polish, "iso-8859-2", charmap.ISO8859_2},
		{"chinese gbk", "zh", simplified, "gb18030", simplifiedchinese.GBK},
		{"chinese gb18030", "zh", simplified, "gb18030", simplifiedchinese.GB18030},
		{"chinese big5", "zh", traditional, "big5", traditionalchinese.Big5},
		{"russian windows-1251", "ru", russian, "windows-1251", charmap.Windows1251},
		{"russian koi8-r", "ru", russian, "koi8-r", charmap.KOI8R},
		{"japanese shift_jis", "ja", jp, "shift_jis", japanese.ShiftJIS},
		{"japanese euc-jp", "ja", jp, "euc-jp", japanese.EUCJP},
		{"regional code", "zh-tw", traditional, "big5", traditionalchinese.Big5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, enc, err := Decode(encode(t, tc.e, tc.want), tc.lang)
			if err != nil || got != tc.want || enc != tc.enc {
				t.Fatalf("got %q in %s, %v", got, enc, err)
			}
			clean, err := CleanFor(encode(t, tc.e, tc.want), SRT, tc.lang)
			if err != nil || clean != tc.want {
				t.Fatalf("clean %q %v", clean, err)
			}
		})
	}
}

func TestDecodeKeepsUnicodeAndFallsBackToWestern(t *testing.T) {
	// Valid UTF-8 is kept whatever the language says.
	if got, enc, _ := Decode([]byte("Zażółć 我们"), "pl"); got != "Zażółć 我们" || enc != "utf-8" {
		t.Fatalf("utf-8: %q %s", got, enc)
	}
	// With no language, old bytes are read as Windows-1252, as before.
	if got, enc, _ := Decode([]byte("Caf\xe9"), ""); got != "Café" || enc != "windows-1252" {
		t.Fatalf("no language: %q %s", got, enc)
	}
	// A language without its own encodings falls back to Windows-1252 too.
	if got, enc, _ := Decode([]byte("Caf\xe9"), "fr"); got != "Café" || enc != "windows-1252" {
		t.Fatalf("french: %q %s", got, enc)
	}
}
