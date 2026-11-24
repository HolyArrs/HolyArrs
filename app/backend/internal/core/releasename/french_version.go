package releasename

import (
	"regexp"
	"strings"
)

// French versions of a dub, as French release names tag them.
const (
	FrenchFrance        = "VFF" // France dub (VFF, TRUEFRENCH)
	FrenchQuebec        = "VFQ" // Québec dub (VFQ, FRENCH.CA, MULTI.CA)
	FrenchInternational = "VFI" // international dub (VFI)
	FrenchBoth          = "VF2" // both the France and the Québec dub
)

var reFrenchWords = regexp.MustCompile(`[\p{L}\p{N}]+`)

// FrenchVersion is the French version a release name tags: VFF for VFF
// and TRUEFRENCH, VFQ for VFQ and FRENCH.CA / MULTI.CA, VFI, or VF2 for
// both dubs (or a name that tags VFF and VFQ). Plain FRENCH, VF and MULTI
// say no version: "".
func FrenchVersion(name string) string {
	ff, fq, fi := false, false, false
	prev := ""
	for _, w := range reFrenchWords.FindAllString(strings.ToLower(name), -1) {
		switch w {
		case "vff", "truefrench":
			ff = true
		case "vfq":
			fq = true
		case "vfi":
			fi = true
		case "vf2":
			return FrenchBoth
		case "ca":
			// CA alone is also Catalan's code, so it counts only right
			// after FRENCH or MULTI (FRENCH.CA, MULTi-CA).
			fq = fq || prev == "french" || prev == "multi"
		}
		prev = w
	}
	return pickFrench(ff, fq, fi)
}

func pickFrench(ff, fq, fi bool) string {
	switch {
	case ff && fq:
		return FrenchBoth
	case fq:
		return FrenchQuebec
	case ff:
		return FrenchFrance
	case fi:
		return FrenchInternational
	}
	return ""
}

// FrenchVersionOfTrack reads the French version of one French audio track
// from its language tag (fr-CA, fr-FR) and its name ("French (CA)",
// "VFQ", "Canada", "France", "VFI"); "" when neither says. A track the
// tag or name does not mark as French is "".
func FrenchVersionOfTrack(lang, name string) string {
	l := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(lang), "_", "-"))
	n := strings.ToLower(name)
	base, _, _ := strings.Cut(l, "-")
	french := base == "fr" || base == "fre" || base == "fra" || strings.HasPrefix(base, "french") || strings.HasPrefix(base, "fran") ||
		strings.Contains(n, "fran") || strings.Contains(n, "french") ||
		FrenchVersion(name) != ""
	if !french {
		return ""
	}
	if v := FrenchVersion(name); v != "" {
		return v
	}
	switch {
	case strings.HasSuffix(l, "-ca"), strings.Contains(n, "(ca)"), strings.Contains(n, "canad"),
		strings.Contains(n, "québ"), strings.Contains(n, "queb"):
		return FrenchQuebec
	case strings.HasSuffix(l, "-fr"), strings.Contains(n, "(fr)"), strings.Contains(n, "france"):
		return FrenchFrance
	case strings.Contains(n, "international"):
		return FrenchInternational
	}
	return ""
}

// FrenchVersionOfTracks joins the versions of a file's French audio tracks:
// a France and a Québec track give VF2.
func FrenchVersionOfTracks(versions ...string) string {
	ff, fq, fi := false, false, false
	for _, v := range versions {
		switch v {
		case FrenchBoth:
			return FrenchBoth
		case FrenchFrance:
			ff = true
		case FrenchQuebec:
			fq = true
		case FrenchInternational:
			fi = true
		}
	}
	return pickFrench(ff, fq, fi)
}

// FrenchVersionOfNFO reads the French version from a release's .nfo: the
// MediaInfo text most French .nfo files hold. Only the audio parts count
// (their Language and Title lines), so a French subtitle never names the
// dub. An .nfo without MediaInfo audio parts gives "".
func FrenchVersionOfNFO(text string) string {
	var versions []string
	inAudio := false
	lang, title := "", ""
	flush := func() {
		if inAudio {
			versions = append(versions, FrenchVersionOfTrack(lang, title))
		}
		lang, title = "", ""
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		key, val, ok := strings.Cut(line, ":")
		key = strings.ToLower(strings.TrimRight(strings.TrimSpace(key), ". "))
		if !ok && line != "" && len(line) < 40 {
			// A section heading: "Audio", "Audio #2", "Text #1", "Video".
			flush()
			low := strings.ToLower(line)
			inAudio = strings.HasPrefix(low, "audio")
			continue
		}
		if !inAudio || !ok {
			continue
		}
		val = strings.TrimSpace(val)
		switch key {
		case "language", "langue":
			lang = val
			// MediaInfo writes "French (CA)": the region is read with the name.
			title += " " + val
		case "title", "titre":
			title += " " + val
		}
	}
	flush()
	return FrenchVersionOfTracks(versions...)
}
