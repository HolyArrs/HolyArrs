package migrate

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

// ImportSkipped is a format left out of imported Pick rules, and why.
type ImportSkipped struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// maxImportTraits matches the Pick rules limit on listed traits.
const maxImportTraits = 64

func clampPoints(n int) int { return library.ClampPoints(n) }

// scoresFormats says a profile gives format scores worth bringing.
func scoresFormats(p Profile) bool {
	for _, v := range p.Scores {
		if v != 0 {
			return true
		}
	}
	return p.MinScore != 0 || p.CutoffScore != 0
}

// importedRules plans a Sonarr or Radarr profile as its own Pick rules: its
// allowed qualities as steps, its cutoff as Good enough, each format it
// scores as a trait with those points, and its score settings as the
// points thresholds. ok is false when the profile stays a ready-made match.
func (b *builder) importedRules(s *Snapshot, p Profile, preset string) (*RulesPlan, bool) {
	if b.noFormats || s.Kind != Sonarr && s.Kind != Radarr || !scoresFormats(p) {
		return nil, false
	}
	app := string(s.Kind)
	body, notes := library.OldProfileRules(library.OldProfile{Qualities: p.Qualities, CutoffQualities: p.CutoffQualities,
		UpgradeAllowed: p.UpgradeAllowed, MinScore: p.MinScore, CutoffScore: p.CutoffScore, MinUpgradeScore: p.MinUpgradeScore,
		Language: p.Language, LanguageCode: subtitles.NormLanguage(strings.ToLower(strings.TrimSpace(p.Language))), FourK: preset == "4K"})
	name := clip(b.named(p.Name), maxName)
	rp := &RulesPlan{App: app, Profile: name, Preset: name, Name: name,
		ProfileKey: fmt.Sprintf("%s:%s:%d", app, instanceLabel(s.Address), p.ID),
		Kinds:      []string{map[Kind]string{Sonarr: "show", Radarr: "movie"}[s.Kind]}, Body: &body}
	byName := map[string]Named{}
	for _, f := range s.CustomFormats {
		byName[f.Name] = f
	}
	names := make([]string, 0, len(p.Scores))
	for n, v := range p.Scores {
		if v != 0 {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		f, ok := byName[n]
		if !ok {
			rp.Skipped = append(rp.Skipped, ImportSkipped{Name: n, Reason: "its format was not read from the old app"})
			continue
		}
		specs, kinds := library.ConvertFormat(app, formatSpecs(f), library.ConvertOptions{MoreKinds: true})
		if len(kinds) > 0 {
			rp.Skipped = append(rp.Skipped, ImportSkipped{Name: n, Reason: "it tests " + strings.Join(kinds, ", ") + ", which HolyArrs cannot test"})
			continue
		}
		rp.Traits = append(rp.Traits, library.ImportedTrait{SourceID: fmt.Sprintf("%s:%s:%d", app, instanceLabel(s.Address), f.ID),
			Name: clip(f.Name, 60), Specs: specs, Points: clampPoints(p.Scores[n])})
		if clampPoints(p.Scores[n]) != p.Scores[n] {
			notes = append(notes, fmt.Sprintf("%s scored %d there; points here stop at %d", clip(f.Name, 60), p.Scores[n], clampPoints(p.Scores[n])))
		}
	}
	if len(rp.Traits) > maxImportTraits {
		sort.SliceStable(rp.Traits, func(i, j int) bool { return abs(rp.Traits[i].Points) > abs(rp.Traits[j].Points) })
		for _, t := range rp.Traits[maxImportTraits:] {
			rp.Skipped = append(rp.Skipped, ImportSkipped{Name: t.Name, Reason: "Pick rules hold at most 64 traits; the strongest scores were kept"})
		}
		rp.Traits = rp.Traits[:maxImportTraits]
	}
	rp.Notes = notes
	return rp, true
}

// ownRulesName gives imported Pick rules a name no other imported ones
// have: two apps' profiles of one name stay two Pick rules, so each app's
// titles name their own. leftOut takes each format it left out, and why.
func (b *builder) ownRulesName(rp *RulesPlan, leftOut map[string]string) {
	for _, sk := range rp.Skipped {
		leftOut[sk.Name] = sk.Reason
	}
	for n, base := 2, rp.Name; b.imported[rp.Name]; n++ {
		rp.Name = clip(base, maxName-20) + " (" + rp.App + ")"
		if n > 2 {
			rp.Name = fmt.Sprintf("%s (%s %d)", clip(base, maxName-20), rp.App, n-1)
		}
		rp.Preset = rp.Name
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// formatSpecs reads a format's conditions as the converter takes them.
func formatSpecs(f Named) []library.FormatSpec {
	out := make([]library.FormatSpec, 0, len(f.Items))
	for _, it := range f.Items {
		s := library.FormatSpec{Name: str(it["name"]), Implementation: str(it["kind"]), Negate: boolean(it["negate"]), Required: boolean(it["required"]),
			Value: specText(itemValue(it, "value")), Except: boolean(itemValue(it, "exceptlanguage"))}
		s.Min, _ = strconv.ParseFloat(specText(itemValue(it, "min")), 64)
		s.Max, _ = strconv.ParseFloat(specText(itemValue(it, "max")), 64)
		if s.Implementation == "LanguageSpecification" {
			s.Language = formatLanguage(str(it["language"]))
		}
		out = append(out, s)
	}
	return out
}

// specText writes a condition value as text.
func specText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	case bool:
		return strconv.FormatBool(x)
	}
	return fmt.Sprint(v)
}

// formatLanguage maps an old app's language name to a language test's
// value: a language code, "original" or "any"; "" when there is none.
func formatLanguage(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "original", "any":
		return n
	case "", "unknown":
		return ""
	case "flemish":
		return "nl"
	case "spanish (latino)":
		return "es"
	}
	return subtitles.NormLanguage(n)
}
