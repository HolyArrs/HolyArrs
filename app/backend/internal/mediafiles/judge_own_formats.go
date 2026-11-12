package mediafiles

import (
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dlclark/regexp2"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// Synced traits (TRaSH Guides) carry patterns written for the old apps'
// .NET engine, with look-arounds Go's own engine lacks, so they run on
// regexp2 with a time limit per test.

type ownSpec struct {
	OwnSpec
	re      *regexp2.Regexp
	sources []string
	res     int
	size    sizeRange
	// lang is a language test's code ("original", "any" or a language),
	// except says it fits any other language.
	lang   string
	except bool
	years  [2]int
}

const specTimeout = 50 * time.Millisecond

// Limits on synced tests, so an imported trait cannot make judging slow:
// patterns past SpecMaxPattern characters are refused, and a trait with
// more than SpecMaxTests tests never matches.
const (
	SpecMaxPattern = 1000
	SpecMaxTests   = 100
)

// ReleaseTypes are the values a release_type test takes.
var ReleaseTypes = []string{"single", "multi", "season_pack"}

// CompileSpecPattern reads a synced pattern the way the old apps do
// (ignoring case; \d and \w take any script's digits and letters).
func CompileSpecPattern(p string) (*regexp2.Regexp, error) {
	if len(p) > SpecMaxPattern {
		return nil, errors.New("the pattern is longer than 1000 characters")
	}
	re, err := regexp2.Compile(p, regexp2.IgnoreCase)
	if err != nil {
		return nil, err
	}
	re.MatchTimeout = specTimeout
	return re, nil
}

// compileSpecs prepares a synced trait's tests; ok is false when one
// cannot be read, and the trait then never matches.
func compileSpecs(specs []OwnSpec) ([]ownSpec, bool) {
	if len(specs) > SpecMaxTests {
		return nil, false
	}
	out := make([]ownSpec, 0, len(specs))
	for _, s := range specs {
		o := ownSpec{OwnSpec: s}
		switch s.Kind {
		case "name", "group", "edition":
			re, err := CompileSpecPattern(s.Value)
			if err != nil {
				return nil, false
			}
			o.re = re
		case "source", "modifier":
			// A modifier (remux, full disc) is read from the source too, but
			// is a kind of its own, so it and a source test must both pass.
			o.sources = strings.Split(s.Value, ",")
		case "resolution":
			n, err := strconv.Atoi(s.Value)
			if err != nil {
				return nil, false
			}
			o.res = n
		case "size":
			r, ok := compileSizeRange(s.Value)
			if !ok {
				return nil, false
			}
			o.size = r
		case "language":
			v, except := strings.CutPrefix(strings.ToLower(strings.TrimSpace(s.Value)), "!")
			if v == "" || except && v == "any" {
				return nil, false
			}
			o.lang, o.except = v, except
		case "release_type":
			if !slices.Contains(ReleaseTypes, s.Value) {
				return nil, false
			}
		case "year":
			lo, hi, found := strings.Cut(s.Value, "-")
			a, err1 := strconv.Atoi(strings.TrimSpace(lo))
			b, err2 := strconv.Atoi(strings.TrimSpace(hi))
			if !found || err1 != nil || err2 != nil || a <= 0 || b < a {
				return nil, false
			}
			o.years = [2]int{a, b}
		default:
			return nil, false
		}
		out = append(out, o)
	}
	return out, len(out) > 0
}

// timedOut names the traits whose pattern ran out of time, logged once each.
var timedOut sync.Map

// match runs a pattern; a pattern out of time counts as no match.
func (s ownSpec) match(trait, text string) bool {
	// A pattern that ran out of time once is not run again, so a slow
	// imported pattern costs its timeout once, not on every Release.
	key := trait + "|" + s.Value
	if _, slow := timedOut.Load(key); slow {
		return false
	}
	hit, err := s.re.MatchString(text)
	if err != nil {
		if _, seen := timedOut.LoadOrStore(key, true); !seen {
			slog.Warn("a trait test ran out of time and counts as not fitting", "trait", trait, "test", s.Name)
		}
		return false
	}
	return hit
}

// languagesOf is the language list a language test reads: a library
// file's audio tracks, else the languages in the name, else the title's
// original language (what a Release with no language word is in).
func languagesOf(p releasename.Result) []string {
	if len(p.AudioLanguages) > 0 {
		return p.AudioLanguages
	}
	if len(p.Languages) > 0 {
		return p.Languages
	}
	if p.OriginalLanguage != "" {
		return []string{p.OriginalLanguage}
	}
	return nil
}

func (s ownSpec) language(p releasename.Result) (hit, known bool) {
	langs := languagesOf(p)
	if len(langs) == 0 {
		return false, false
	}
	want := s.lang
	switch want {
	case "any":
		return true, true
	case "original":
		if p.OriginalLanguage == "" {
			// Not known: it is in no list, so it never fits (negate turns
			// that into a fit) and every language is another one.
			return s.except, true
		}
		want = p.OriginalLanguage
	}
	for _, l := range langs {
		if s.except && !strings.EqualFold(l, want) || !s.except && strings.EqualFold(l, want) {
			return true, true
		}
	}
	return false, true
}

func releaseType(p releasename.Result) string {
	switch {
	case p.Show == nil || len(p.Show.Seasons) == 0 && len(p.Show.Episodes) == 0 && len(p.Show.Absolute) == 0 && p.Show.AirDate == "":
		return ""
	case len(p.Show.Episodes) == 0 && len(p.Show.Absolute) == 0 && p.Show.AirDate == "":
		return "season_pack"
	case len(p.Show.Episodes) > 1 || len(p.Show.Absolute) > 1:
		return "multi"
	}
	return "single"
}

func (s ownSpec) test(trait string, p releasename.Result) bool {
	var hit bool
	switch s.Kind {
	case "name":
		hit = s.match(trait, p.Original)
	case "group":
		hit = p.Group.Value != ""
		if hit {
			hit = s.match(trait, p.Group.Value)
		}
	case "edition":
		// No edition is no fit, as there (the title is not read).
		if text := strings.Join(p.Edition, " "); text != "" {
			hit = s.match(trait, text)
		}
	case "language":
		var known bool
		if hit, known = s.language(p); !known {
			return false // no language list never fits, negated or not
		}
	case "release_type":
		hit = releaseType(p) == s.Value
	case "year":
		// The name's year, else the title's.
		y := p.Year.Value
		if y <= 0 {
			y = p.TitleYear
		}
		hit = y > 0 && y >= s.years[0] && y <= s.years[1]
	case "source", "modifier":
		hit = p.Source.Conf != releasename.Unknown && slices.Contains(s.sources, p.Source.Value)
	case "resolution":
		hit = p.Resolution.Conf != releasename.Unknown && p.Resolution.Value == s.res
	case "size":
		hit = s.size.fits(p.Size)
	}
	return hit != s.Negate
}

// formatMatches runs a synced trait's tests: a kind passes when no
// required test in it failed and at least one of its tests passed (so a
// required test that passes carries its kind). A trait with no tests fits
// everything.
func formatMatches(name string, specs []ownSpec, p releasename.Result) (bool, string) {
	kinds := map[string]bool{}
	for _, s := range specs {
		ok := s.test(name, p)
		if s.Required && !ok {
			return false, name + ": " + s.Name + " does not fit"
		}
		kinds[s.Kind] = kinds[s.Kind] || ok
	}
	for kind, hit := range kinds {
		if !hit {
			return false, name + ": no " + kind + " test fits"
		}
	}
	return true, name + ": fits"
}
