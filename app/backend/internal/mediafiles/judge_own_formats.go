package mediafiles

import (
	"slices"
	"strconv"
	"strings"
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
}

const specTimeout = 50 * time.Millisecond

// CompileSpecPattern reads a synced pattern the way the old apps do
// (ignoring case).
func CompileSpecPattern(p string) (*regexp2.Regexp, error) {
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
	out := make([]ownSpec, 0, len(specs))
	for _, s := range specs {
		o := ownSpec{OwnSpec: s}
		switch s.Kind {
		case "name", "group":
			re, err := CompileSpecPattern(s.Value)
			if err != nil {
				return nil, false
			}
			o.re = re
		case "source":
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
		default:
			return nil, false
		}
		out = append(out, o)
	}
	return out, len(out) > 0
}

func (s ownSpec) test(p releasename.Result) bool {
	var hit bool
	switch s.Kind {
	case "name":
		hit, _ = s.re.MatchString(p.Original)
	case "group":
		hit = p.Group.Value != ""
		if hit {
			hit, _ = s.re.MatchString(p.Group.Value)
		}
	case "source":
		hit = p.Source.Conf != releasename.Unknown && slices.Contains(s.sources, p.Source.Value)
	case "resolution":
		hit = p.Resolution.Conf != releasename.Unknown && p.Resolution.Value == s.res
	case "size":
		hit = s.size.fits(p.Size)
	}
	return hit != s.Negate
}

// formatMatches runs a synced trait's tests: per kind, every required
// test passes and at least one of the others (if any).
func formatMatches(name string, specs []ownSpec, p releasename.Result) (bool, string) {
	type kindState struct{ optional, optionalHit bool }
	kinds := map[string]*kindState{}
	for _, s := range specs {
		k := kinds[s.Kind]
		if k == nil {
			k = &kindState{}
			kinds[s.Kind] = k
		}
		ok := s.test(p)
		if s.Required {
			if !ok {
				return false, name + ": " + s.Name + " does not fit"
			}
			continue
		}
		k.optional = true
		k.optionalHit = k.optionalHit || ok
	}
	for kind, k := range kinds {
		if k.optional && !k.optionalHit {
			return false, name + ": no " + kind + " test fits"
		}
	}
	return true, name + ": fits"
}
