package library

import (
	"sort"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// FormatSpec is one test of an old app's format, as TRaSH Guides and the
// old apps' own lists give it.
type FormatSpec struct {
	Name           string
	Implementation string
	Negate         bool
	Required       bool
	// Value is the test's value: a pattern, or a number as text.
	Value string
	// Min and Max are a size test's ends in GB, or a year test's years.
	Min, Max float64
	// Language is a language test's language: a code, "original" or
	// "any" ("" when not known), and Except its except switch.
	Language string
	Except   bool
}

// ConvertOptions turn on the test kinds only an old-app import brings:
// TRaSH sync keeps its earlier kinds, so its traits never change.
type ConvertOptions struct {
	MoreKinds bool
}

// ConvertFormat turns a format's tests into trait tests; skipped lists the
// test kinds it could not take (then the whole format is skipped, since a
// half-tested format would match the wrong Releases).
func ConvertFormat(app string, in []FormatSpec, opt ConvertOptions) (specs []mediafiles.OwnSpec, skipped []string) {
	var out []mediafiles.OwnSpec
	bad := map[string]bool{}
	if len(in) > mediafiles.SpecMaxTests {
		return nil, []string{"more than 100 tests"}
	}
	for _, s := range in {
		o := mediafiles.OwnSpec{Name: s.Name, Negate: s.Negate, Required: s.Required}
		str := strings.TrimSpace(s.Value)
		num, _ := strconv.Atoi(str)
		switch s.Implementation {
		case "ReleaseTitleSpecification", "ReleaseGroupSpecification", "EditionSpecification":
			o.Kind = "name"
			switch s.Implementation {
			case "ReleaseGroupSpecification":
				o.Kind = "group"
			case "EditionSpecification":
				if !opt.MoreKinds || app != "radarr" {
					bad["edition"] = true
					continue
				}
				o.Kind = "edition"
			}
			if _, err := mediafiles.CompileSpecPattern(str); err != nil || str == "" {
				bad["unreadable pattern"] = true
				continue
			}
			o.Value = str
		case "SourceSpecification":
			m := trashSonarrSources
			if app == "radarr" {
				m = trashRadarrSources
			}
			v, ok := m[num]
			if !ok {
				bad["source"] = true
				continue
			}
			o.Kind, o.Value = "source", v
		case "ResolutionSpecification":
			if num <= 0 {
				bad["resolution"] = true
				continue
			}
			o.Kind, o.Value = "resolution", strconv.Itoa(num)
		case "SizeSpecification":
			if _, _, ok := mediafiles.ParseSizeRange(mediafiles.SizeRangeValue(s.Min, s.Max)); !ok {
				bad["size"] = true
				continue
			}
			o.Kind, o.Value = "size", mediafiles.SizeRangeValue(s.Min, s.Max)
		case "QualityModifierSpecification":
			// Movies: 4 is a full disc, 5 a remux; the rest have no match here.
			switch num {
			case 4:
				o.Kind, o.Value = "source", "disc_image"
			case 5:
				o.Kind, o.Value = "source", "remux"
			default:
				bad["quality modifier"] = true
				continue
			}
			// An import keeps it a kind of its own, so it and a source
			// test must both pass, as there.
			if opt.MoreKinds {
				o.Kind = "modifier"
			}
		case "LanguageSpecification":
			code := s.Language
			if !opt.MoreKinds || code == "" || s.Except && code == "any" {
				bad["language"] = true
				continue
			}
			if s.Except {
				code = "!" + code
			}
			o.Kind, o.Value = "language", code
		case "ReleaseTypeSpecification":
			v := map[int]string{1: "single", 2: "multi", 3: "season_pack"}[num]
			if !opt.MoreKinds || app != "sonarr" || v == "" {
				bad["release type"] = true
				continue
			}
			o.Kind, o.Value = "release_type", v
		case "YearSpecification":
			lo, hi := int(s.Min), int(s.Max)
			if !opt.MoreKinds || app != "radarr" || lo <= 0 || hi < lo {
				bad["year"] = true
				continue
			}
			o.Kind, o.Value = "year", strconv.Itoa(lo)+"-"+strconv.Itoa(hi)
		default:
			w := trashSkippedKinds[s.Implementation]
			if w == "" {
				w = strings.ToLower(strings.TrimSuffix(s.Implementation, "Specification"))
			}
			bad[w] = true
			continue
		}
		out = append(out, o)
	}
	kinds := make([]string, 0, len(bad))
	for k := range bad {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return out, kinds
}
