package imgstore

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// FoldName is the key names and titles are matched on: Unicode NFKC, case
// folded, punctuation removed and runs of white space made one space. Every
// script is kept (stripping to ASCII broke Japanese and Korean names).
func FoldName(s string) string {
	s = cases.Fold().String(norm.NFKC.String(s))
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsPunct(r):
			continue
		case unicode.IsSpace(r):
			space = b.Len() > 0
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// SameName reports two names that fold to the same non-empty key.
func SameName(a, b string) bool {
	x := FoldName(a)
	return x != "" && x == FoldName(b)
}

// SameTitle matches album titles with or without an edition in brackets or
// after " - " ("Album (Deluxe)", "Album - Single").
func SameTitle(found, want string) bool {
	cut := func(s string) string {
		if i := strings.IndexAny(s, "(["); i > 0 {
			s = s[:i]
		}
		if i := strings.Index(s, " - "); i > 0 {
			s = s[:i]
		}
		return FoldName(s)
	}
	f, w := cut(found), cut(want)
	return f != "" && f == w
}

// editionWords marks a bracketed or " - " tail that names an edition of the
// same album, not another album: remaster(ed) [YYYY], deluxe, expanded,
// anniversary, edition, version, mono/stereo, bonus, a dated mix.
var editionWords = regexp.MustCompile(`(?i)\b(remaster(ed)?|deluxe|expanded|anniversary|edition|version|mono|stereo|bonus|(19|20)[0-9]{2} mix)\b`)

var editionTail = regexp.MustCompile(`\s*(\([^()]*\)|\[[^\[\]]*\]|\s-\s[^()\[\]]*)\s*$`)

// EditionBase folds a title with its edition tails removed ("Abbey Road
// (Remastered 2009)", "OK Computer [Deluxe Edition]", "Help! - Remastered
// 2009" all fold to the bare title). Other tails ("(Live)", "(Taylor's
// Version)") are kept.
func EditionBase(s string) string {
	for {
		m := editionTail.FindStringIndex(s)
		// "(Taylor's Version)" is a re-recording, another album.
		if m == nil || m[0] == 0 || !editionWords.MatchString(s[m[0]:]) || strings.Contains(strings.ToLower(s[m[0]:]), "taylor") {
			break
		}
		s = s[:m[0]]
	}
	return FoldName(s)
}

// SameBase reports two titles with the same non-empty edition base.
func SameBase(a, b string) bool {
	x := EditionBase(a)
	return x != "" && x == EditionBase(b)
}

// amongArtists reports the artist's whole name within an album's artist
// line ("Frank Ocean" in "Frank Ocean & JAY-Z"), word for word.
func amongArtists(line, artist string) bool {
	a := FoldName(artist)
	return a != "" && strings.Contains(" "+FoldName(line)+" ", " "+a+" ")
}
