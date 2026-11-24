package releasename

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// SameName says whether a library name cleans to want (a CleanName of a
// release name). An apostrophe in the library name is tried both as a
// separator ("Schitt s Creek", "d Amelie") and as nothing ("Schitts Creek"),
// since release names show either. An all-capitals library name ("ER",
// "CSI") also matches its letters written apart ("E.R.").
func SameName(libName, want string) bool {
	if slices.Contains(NameForms(libName), want) {
		return true
	}
	return want != "" && strings.ToUpper(libName) == libName && strings.ToLower(libName) != libName && CleanName(libName) == JoinLetters(want)
}

// NameForms are the clean forms a library name can show in release names:
// as written, with apostrophes dropped, and with letters written apart by
// dots or stars joined ("S.W.A.T." as "swat", "M*A*S*H" as "mash"). A
// joined name of only one or two letters ("I.T.", "E.R.") is too short to
// tell apart from a word and is left out.
//
// Forms are kept per name (matching a release walks every Title's names, so
// a 10k-Title library would clean the same names again for each release);
// callers must not change the slice they get.
func NameForms(libName string) []string {
	formsMu.RLock()
	f, ok := formsKept[libName]
	formsMu.RUnlock()
	if ok {
		return f
	}
	f = nameForms(libName)
	if len(libName) > 300 {
		return f // no real name is this long; not kept
	}
	formsMu.Lock()
	if len(formsKept) >= 50_000 { // about 5 MB; names of removed Titles go too, refilled on use
		clear(formsKept)
	}
	formsKept[libName] = f
	formsMu.Unlock()
	return f
}

var (
	formsMu   sync.RWMutex
	formsKept = map[string][]string{}
)

func nameForms(libName string) []string {
	forms := []string{CleanName(libName)}
	if strings.ContainsAny(libName, "'’ʼ`‘´＇") {
		forms = append(forms, CleanName(reApostrophe.Replace(libName)))
	}
	// ʼ counts as a letter, so cleaning keeps it; releases write it as a gap too.
	if strings.Contains(libName, "ʼ") {
		forms = append(forms, CleanName(strings.ReplaceAll(libName, "ʼ", " ")))
	}
	if j := JoinLettersApart(libName, 2); j != libName {
		if c := CleanName(reApostrophe.Replace(j)); strings.Contains(c, " ") || len([]rune(c)) > 2 {
			forms = append(forms, CleanName(j), c)
		}
	}
	// German releases write ä, ö, ü as ae, oe, ue ("Die Mumie kehrt zurueck").
	if strings.ContainsAny(libName, "äöüÄÖÜ") {
		forms = append(forms, CleanName(reUmlaut.Replace(libName)))
	}
	// A last number written the other way: "Rambo III" as "Rambo 3", "Rocky 2" as "Rocky II".
	if c := forms[0]; strings.Contains(c, " ") {
		i := strings.LastIndexByte(c, ' ')
		if n, ok := swapNumeral[c[i+1:]]; ok {
			forms = append(forms, c[:i+1]+n)
		}
	}
	return forms
}

var reUmlaut = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "Ä", "Ae", "Ö", "Oe", "Ü", "Ue")

// swapNumeral maps a last word II to X to its digits and back. "I" and "1"
// are left out: "I" is a word as often as a number.
var swapNumeral = func() map[string]string {
	m := map[string]string{}
	for i, r := range []string{"ii", "iii", "iv", "v", "vi", "vii", "viii", "ix", "x"} {
		d := strconv.Itoa(i + 2)
		m[r], m[d] = d, r
	}
	return m
}()

// JoinLettersApart joins letters written apart by dots or stars ("S.W.A.T."
// to "SWAT", "M*A*S*H" to "MASH") when the run has at least least letters;
// shorter runs stay as they are. The matcher (NameForms) and the search
// text both use it, so they read a name the same way.
func JoinLettersApart(s string, least int) string { return joinRuns(reDotted, s, least) }

// JoinDottedLetters joins only letters written apart by dots ("S.W.A.T." to
// "SWAT", "E.T." to "ET"); a star, or a dot after a word or a number
// ("D.Gray-man", "2.0"), stays as it is. Search text uses it, as the older
// apps send names.
func JoinDottedLetters(s string) string { return joinRuns(reDots, s, 2) }

func joinRuns(re *regexp.Regexp, s string, least int) string {
	return re.ReplaceAllStringFunc(s, func(m string) string {
		j := dottedJoin(m)
		n := 0
		for _, c := range j {
			if unicode.IsLetter(c) {
				n++
			}
		}
		if n < least {
			return m
		}
		return j
	})
}

var (
	reApostrophe = strings.NewReplacer("'", "", "’", "", "ʼ", "", "`", "", "‘", "", "´", "", "＇", "")
	// reDotted is two or more single letters, each followed by a dot or star.
	reDotted = regexp.MustCompile(`(?:^|[^\pL\pN])(?:\pL[.*]){1,}\pL(?:[.*]|$|[^\pL\pN])`)
	reDots   = regexp.MustCompile(`(?:^|[^\pL\pN])(?:\pL\.){1,}\pL(?:\.|$|[^\pL\pN])`)
)

// dottedJoin joins the letters of one reDotted match, keeping the edges; a
// dot or star after the last letter becomes a space ("S.W.A.T.2017").
func dottedJoin(m string) string {
	r := []rune(m)
	var b strings.Builder
	for i, c := range r {
		if c == '.' || c == '*' {
			if i == len(r)-1 && unicode.IsLetter(r[i-1]) {
				b.WriteByte(' ')
				continue
			}
			if i > 0 && i < len(r)-1 {
				continue
			}
		}
		b.WriteRune(c)
	}
	return b.String()
}

// JoinLetters joins runs of two or more one-letter words ("e r" to "er").
func JoinLetters(clean string) string {
	words := strings.Fields(clean)
	letter := func(w string) bool { r := []rune(w); return len(r) == 1 && unicode.IsLetter(r[0]) }
	var out []string
	for i, w := range words {
		if letter(w) && i > 0 && letter(words[i-1]) {
			out[len(out)-1] += w
			continue
		}
		out = append(out, w)
	}
	return strings.Join(out, " ")
}
