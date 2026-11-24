package releasename

import (
	"slices"
	"strings"
	"testing"
)

// languageCorpus is what the old apps read from each name (see
// work/releases/1.2.1/language-parsing-report.md); flags lists the
// language flags the name must carry.
var languageCorpus = []struct {
	name  string
	langs string
	flags string
}{
	// Toloka names.
	{"Oppenheimer (2023) BDRip-AVC Ukr/Eng | Sub Eng", "uk en", ""},
	{"The Bear S01 (2022) WEB-DL 1080p 2xUkr/Eng | Sub Eng", "uk en", ""},
	{"Dune (1984) BDRip 1080p H.265 Ukr/Eng | Sub Eng [Theatrical Cut]", "uk en", ""},
	{"Dune: Part One (2021) BDRemux 1080p 2xUkr/Eng | Sub Ukr/Eng", "uk en", ""},
	{"Shogun S01 (2024) WEB-DL 4K 2160p H.265 HDR 3xUkr/Eng | Sub Ukr/Eng", "uk en", ""},
	{"Severance S02 (2025) WEB-DL-AVC Ukr | Sub Ukr/Eng", "uk", ""},
	{"Severance S01 (2022) WEB-DL 1080p Eng | Sub Ukr/Eng", "en", ""},
	// The report's 38 other names.
	{"Movie.Title.2019.1080p.BluRay.x264-GRP", "", ""},
	{"Movie.Title.2019.FRENCH.1080p.WEB.H264-GRP", "fr", ""},
	{"Movie.Title.2019.MULTi.1080p.BluRay.x264-GRP", "", "multi"},
	{"Movie.Title.2019.MULTi.VFF.1080p.BluRay.x264-GRP", "fr", "multi"},
	{"Movie.Title.2019.German.DL.1080p.BluRay.x264-GRP", "de", "original"},
	{"Movie.Title.2019.German.ML.1080p.BluRay.x264-GRP", "de en", "original"},
	{"Movie.Title.2019.German.1080p.WEB-DL.h264-GRP", "de", ""},
	{"Movie.Title.2019.1080p.BluRay.x264.ENG.SUBS-GRP", "", ""},
	{"Movie.Title.2019.1080p.WEBRip.x264.VOSTFR-GRP", "", ""},
	{"Movie.Title.2019.1080p.BluRay.x264.ITA.ENG-GRP", "it en", ""},
	{"Movie.Title.2019.iTALiAN.1080p.BluRay.x264-GRP", "it", ""},
	{"Movie.Title.2019.1080p.BluRay.x264.Dual.Audio.Hindi.English-GRP", "hi en", "dual"},
	{"Movie.Title.2019.SPANISH.1080p.WEB-DL-GRP", "es", ""},
	{"Movie.Title.2019.Castellano.1080p.BluRay-GRP", "es", ""},
	{"Movie.Title.2019.Latino.1080p.WEB-DL-GRP", "es", ""},
	{"Movie.Title.2019.1080p.WEB-DL.Dublado.PT-BR-GRP", "pt", ""},
	{"Movie.Title.2019.PL.DUB.1080p.WEB-DL-GRP", "pl", ""},
	{"Movie.Title.2019.PLDUB.1080p.WEB-DL-GRP", "pl", ""},
	{"Movie.Title.2019.CZ.1080p.BluRay-GRP", "cs", ""},
	{"Movie.Title.2019.HUN.1080p.BluRay-GRP", "hu", ""},
	{"Movie.Title.2019.NORDiC.1080p.BluRay-GRP", "", ""},
	{"Movie.Title.2019.SWEDiSH.1080p.WEB-DL-GRP", "sv", ""},
	{"Movie.Title.2019.1080p.BluRay.Rus.Ukr.Eng-GRP", "ru uk en", ""},
	{"Movie.Title.2019.BDRip.1080p.Rus.Eng.Sub.Rus.Eng", "ru en", ""},
	{"Show.Name.S01E01.1080p.WEB.h264-GRP", "", ""},
	{"Show.Name.S01E01.1080p.WEB.h264.Hebrew.Subs-GRP", "", ""},
	{"[SubsPlease] Show Name - 01 (1080p) [ABCDEF12]", "", ""},
	{"Show.Name.S01E01.JAPANESE.1080p.WEB.h264-GRP", "ja", ""},
	{"Show.Name.S01.1080p.NF.WEB-DL.DDP5.1.KOREAN-GRP", "ko", ""},
	{"Show.Name.S01E01.1080p.WEB.h264.KOR.ENG.SUB-GRP", "", ""},
	{"Movie Title (2020) [1080p] [WEBRip] [5.1] [YTS.MX] [Thai]", "th", ""},
	{"Movie.Title.2019.TRUEFRENCH.1080p.BluRay-GRP", "fr", ""},
	{"Movie.Title.2019.VFQ.1080p.WEB-GRP", "fr", ""},
	{"Movie.Title.2019.1080p.BluRay.Multi.Subs-GRP", "", "multi"},
	{"Movie.Title.2019.1080p.BluRay.ENG.ITA.SPA.FRE.GER-GRP", "en it es fr de", ""},
	{"Movie.Title.2019.1080p.BluRay.x264-GRP[rus]", "ru", ""},
	{"Movie.Title.2019.Ukrainian.1080p.WEB-DL-GRP", "uk", ""},
	// 60 more.
	{"Movie.Title.2019.FRENCH.ORIG.1080p.BluRay.x264-GRP", "fr", "original"},
	{"Movie.Title.2019.German.Original.1080p.WEB.h264-GRP", "de", "original"},
	{"Movie.Title.2019.German.DL.1080p.WEB.h264-GRP", "de", "original"},
	{"Movie.Title.2019.German.1080p.WEB.DL.h264-GRP", "de", ""},
	{"Movie.Title.2019.French.DL.1080p.BluRay.x264-GRP", "fr", "dual"},
	{"Movie.Title.2019.ESP.1080p.WEB-DL-GRP", "es", ""},
	{"Movie.Title.2019.Espanol.1080p.WEB-DL-GRP", "es", ""},
	{"Movie.Title.2019.1080p.BluRay.Brazilian.x264-GRP", "pt", ""},
	{"Movie.Title.2019.1080p.WEB-DL.Lektor.PL-GRP", "pl", ""},
	{"Movie.Title.2019.PL-DUB.1080p.WEB-DL-GRP", "pl", ""},
	{"Movie.Title.2019.1080p.BluRay.DE.x264-GRP", "de", ""},
	{"Movie.Title.2019.SK.1080p.WEB-DL-GRP", "sk", ""},
	{"Movie.Title.2019.LT.1080p.WEB-DL-GRP", "lt", ""},
	{"Movie.Title.2019.BG.1080p.WEB-DL-GRP", "bg", ""},
	{"Movie.Title.2019.1080p.BluRay.CZ.SUB-GRP", "", ""},
	{"Movie.Title.2019.1080p.BluRay.de.x264-GRP", "", ""},
	{"Movie.Title.2019.Bulgarian.1080p.WEB-DL-GRP", "bg", ""},
	{"Movie.Title.2019.1080p.BGAUDIO.WEB-DL-GRP", "bg", ""},
	{"Movie.Title.2019.Romanian.1080p.BluRay-GRP", "ro", ""},
	{"Movie.Title.2019.RoDubbed.1080p.WEB-DL-GRP", "ro", ""},
	{"Movie.Title.2019.Catalan.1080p.WEB-DL-GRP", "ca", ""},
	{"Movie.Title.2019.Latvian.1080p.WEB-DL-GRP", "lv", ""},
	{"Movie.Title.2019.Lithuanian.1080p.WEB-DL-GRP", "lt", ""},
	{"Movie.Title.2019.Slovak.1080p.WEB-DL-GRP", "sk", ""},
	{"Movie.Title.2019.Icelandic.1080p.WEB-DL-GRP", "is", ""},
	{"Movie.Title.2019.Persian.1080p.WEB-DL-GRP", "fa", ""},
	{"Movie.Title.2019.Bengali.1080p.WEB-DL-GRP", "bn", ""},
	{"Movie.Title.2019.1080p.WEB-DL.Tamil.Telugu.Hindi-GRP", "ta te hi", ""},
	{"Movie.Title.2019.Malayalam.1080p.WEB-DL-GRP", "ml", ""},
	{"Movie.Title.2019.Kannada.1080p.WEB-DL-GRP", "kn", ""},
	{"Movie.Title.2019.Georgian.1080p.WEB-DL-GRP", "ka", ""},
	{"Movie.Title.2019.Urdu.1080p.WEB-DL-GRP", "ur", ""},
	{"Movie.Title.2019.Estonian.1080p.WEB-DL-GRP", "et", ""},
	{"Movie.Title.2019.Serbian.1080p.WEB-DL-GRP", "sr", ""},
	{"Movie.Title.2019.Croatian.1080p.WEB-DL-GRP", "hr", ""},
	{"Movie.Title.2019.Slovenian.1080p.WEB-DL-GRP", "sl", ""},
	{"Movie.Title.2019.Indonesian.1080p.WEB-DL-GRP", "id", ""},
	{"Movie.Title.2019.Albanian.1080p.WEB-DL-GRP", "sq", ""},
	{"Movie.Title.2019.Tagalog.1080p.WEB-DL-GRP", "tl", ""},
	{"Movie.Title.2019.Macedonian.1080p.WEB-DL-GRP", "mk", ""},
	{"Movie.Title.2019.1080p.BluRay.HUNDUB.x264-GRP", "hu", ""},
	{"Movie.Title.2019.1080p.BluRay.HebDub.x264-GRP", "he", ""},
	{"Movie.Title.2019.SwissGerman.1080p.WEB-DL-GRP", "de", ""},
	{"Movie.Title.2019.1080p.BluRay.Cantonese.x264-GRP", "zh", ""},
	{"Movie.Title.2019.1080p.BluRay.x264.FRA.ENG-GRP", "fr en", ""},
	{"Movie.Title.2019.1080p.BluRay.x264.ENG.SUB.ITA-GRP", "it", ""},
	{"Movie.Title.2019.1080p.BluRay.x264.Rus.Eng.Subs-GRP", "", ""},
	{"Show.Name.S02E03.1080p.WEB.h264.GER.SUBBED-GRP", "de", ""},
	{"Show.Name.S01E01.1080p.WEB.h264.Thai.Sub-GRP", "", ""},
	{"Show.Name.S01E01.1080p.WEB.h264.English.Subs-GRP", "", ""},
	{"Movie Title (2020) [1080p] [WEBRip] [YTS.MX] [Hindi]", "hi", ""},
	{"Movie Title (2020) [1080p] [WEBRip] [YTS.MX] [Spanish]", "es", ""},
	{"Movie Title (2020) [1080p] [WEBRip] [GRP]", "", ""},
	{"The.Hun.2019.720p.WEB.h264-GRP", "", ""},
	{"The.Polish.Bride.1998.720p.WEB.H264-GRP", "", ""},
	{"Thai.Kitchen.Nights.2021.1080p.WEB.H264-GRP", "", ""},
	{"Movie.Title.2019.MULTi.TRUEFRENCH.1080p.BluRay-GRP", "fr", "multi"},
	{"Movie.Title.2019.German.ML.1080p.WEB.h264-GRP", "de en", "original"},
	{"Movie.Title.2019.German.English.DL.1080p.BluRay-GRP", "de en", "dual"},
	{"Movie.Title.2019.Spanish.Latino.1080p.WEB-DL-GRP", "es", ""},
	{"Movie.Title.2019.Farsi.1080p.WEB-DL-GRP", "fa", ""},
	{"Movie Title (2020) [1080p] [x265] [Hindi-English]", "hi en", ""},
	{"Movie Title (2020) [1080p] [x265] [Hindi-GRP]", "", ""},
	// Verifier names.
	{"Movie.Title.2019.1080p.BluRay.DUAL-AUDIO.ENG.JPN-GRP", "en ja", "dual"},
	{"Movie.Title.2019.Multi-Audio.1080p.BluRay-GRP", "", "multi"},
	{"Movie.Title.2019.1080p.WEB.English.Subtitles-GRP", "", ""},
	{"Movie.Title.2019.MULTi.VOSTFR.1080p.BluRay-GRP", "", "multi"},
	{"Movie.Title.2019.German.AC3D.DL.1080p.BluRay-GRP", "de", "dual original"},
}

func TestLanguageCorpus(t *testing.T) {
	pass := 0
	for _, c := range languageCorpus {
		r := Parse(c.name, Hints{})
		got := strings.Join(r.Languages, " ")
		ok := got == c.langs
		for f := range strings.FieldsSeq(c.flags) {
			ok = ok && slices.Contains(r.Flags, f)
		}
		if ok {
			pass++
		} else {
			t.Errorf("%q: languages %q flags %v, want %q flags %q", c.name, got, r.Flags, c.langs, c.flags)
		}
	}
	t.Logf("language corpus: %d of %d read like the old apps", pass, len(languageCorpus))
}
