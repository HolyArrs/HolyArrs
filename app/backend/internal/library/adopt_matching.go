package library

import (
	"regexp"
	"strings"
)

var folderBrackets = regexp.MustCompile(`\[[^\[\]]*\]`)
var folderBracketYear = regexp.MustCompile(`^\[\s*(?:18|19|20)\d{2}\s*\]$`)
var technicalParentheses = regexp.MustCompile(`\([^()]*\)`)
var releaseDelimiters = strings.NewReplacer("[", " ", "]", " ", "(", " ", ")", " ")
var trailingReleaseToken = regexp.MustCompile(`(?i)\s+(?:\d{3,4}[pi]|[48]k|blu-?ray|web-?dl|webrip|b[dr]rip|dvdrip|x26[45]|h26[45]|hevc|av1)$`)
var folderReleaseWords = regexp.MustCompile(`(?i)(?:^|[\s._-])(?:\d{3,4}[pi]|[48]k|blu-?ray|web-?dl|webrip|b[dr]rip|dvdrip|x26[45]|h26[45]|hevc|av1|hdr10\+?|hdr|sdr|dv|aac|dts|atmos)(?:$|[\s._-])`)

// matchingFolderName removes release annotations from lookup text, leaving
// disk paths untouched. A bracketed year remains available to the parser.
func matchingFolderName(s string) string {
	s = strings.Join(strings.Fields(strings.ReplaceAll(s, ".", " ")), " ")
	s = technicalParentheses.ReplaceAllStringFunc(s, func(part string) string {
		if isTechnicalAnnotation(part) {
			return " "
		}
		return part
	})
	s = strings.Join(strings.Fields(s), " ")
	if !strings.Contains(s, "[") {
		return s
	}
	clean := folderBrackets.ReplaceAllStringFunc(s, func(part string) string {
		if folderBracketYear.MatchString(part) {
			return "[" + strings.TrimSpace(strings.Trim(part, "[]")) + "]"
		}
		return " "
	})
	clean = strings.Join(strings.Fields(clean), " ")
	title := clean
	if m := ReFolderYear.FindStringSubmatch(clean); m != nil {
		title = m[1]
	}
	// In names such as [REC] (2007), the bracketed text is the title itself.
	if strings.Trim(title, " -_()[]0123456789") == "" {
		for _, part := range folderBrackets.FindAllString(s, -1) {
			if !folderBracketYear.MatchString(part) && !folderReleaseWords.MatchString(strings.Trim(part, "[]")) {
				return strings.TrimSpace(part + " " + clean)
			}
		}
		return s
	}
	return clean
}

func hasTechnicalParentheses(s string) bool {
	for _, part := range technicalParentheses.FindAllString(s, -1) {
		if isTechnicalAnnotation(part) {
			return true
		}
	}
	return false
}

func isTechnicalAnnotation(part string) bool {
	// Parenthetical subtitles may mention formats too: "Life in 4K"
	// is not a release block. Require a technical marker at its start.
	m := folderReleaseWords.FindStringIndex(strings.TrimSpace(strings.Trim(part, "()")))
	return m != nil && m[0] == 0
}

// Only trim terminal release markers, preserving words inside a title.
func cleanReleaseTitle(s string, editions bool) string {
	for i := 0; i < 8; i++ {
		clean := trailingReleaseToken.ReplaceAllString(s, "")
		if editions {
			clean = releaseEdition.ReplaceAllString(clean, "")
		}
		if clean == s || strings.TrimSpace(clean) == "" {
			break
		}
		s = clean
	}
	return strings.TrimSpace(s)
}
