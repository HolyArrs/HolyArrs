package releasename

import (
	"regexp"
	"strconv"
	"strings"
)

// countryTags are the country marks show names carry to tell remakes apart,
// as in "The Office (US)".
var countryTags = setOf("us", "uk", "gb", "au", "nz", "ca", "ie")

var reNameTag = regexp.MustCompile(`^(.*\S)\s*\(([A-Za-z]{2}|\d{4})\)\s*$`)

// SplitNameTag splits a trailing country or year mark off a name:
// "The Office (US)" gives "The Office", "us", 0 and "Doctor Who (2005)" gives
// "Doctor Who", "", 2005. ok is false when the name has no such mark.
func SplitNameTag(name string) (base, country string, year int, ok bool) {
	if !strings.HasSuffix(strings.TrimSpace(name), ")") {
		return "", "", 0, false // no mark; skips the slow pattern for nearly every name
	}
	m := reNameTag.FindStringSubmatch(name)
	if m == nil {
		return "", "", 0, false
	}
	tag := strings.ToLower(m[2])
	if countryTags[tag] {
		return m[1], tag, 0, true
	}
	if y, err := strconv.Atoi(tag); err == nil && isYearNum(tag) {
		return m[1], "", y, true
	}
	return "", "", 0, false
}
