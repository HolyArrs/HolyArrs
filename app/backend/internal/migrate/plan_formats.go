package migrate

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

// ImportSkipped is a format left out of imported Pick rules, and why.
type ImportSkipped struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// maxImportTraits matches the Pick rules limit on listed traits.
const maxImportTraits = 64

func clampPoints(n int) int { return max(-library.TrashMaxPoints, min(library.TrashMaxPoints, n)) }

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
	body := mediafiles.GoodEveryday()
	if preset == "4K" {
		body = mediafiles.FourK()
	}
	body.Traits = nil
	var notes []string
	// The old apps keep size limits per quality and refuse no traits of
	// their own, so the ready-made rules' limits and refusals are left out.
	body.SizeLimits = map[string]mediafiles.SizeLimit{}
	notes = append(notes, "its size limits are not brought; set them under Pick rules")
	discs := slices.ContainsFunc(p.Qualities, func(q string) bool {
		q = strings.ToLower(q)
		return strings.Contains(q, "br-disk") || strings.Contains(q, "brdisk")
	})
	steps, good := profileSteps(p)
	body.Never = slices.DeleteFunc(slices.Clone(body.Never), func(t string) bool {
		// A step it allows is not also refused as a trait.
		return t == "hardcoded_subs" || t == "3d" || t == "disc_image" && discs || (t == "cam" || t == "telesync") && slices.Contains(steps, t)
	})
	body.MinPoints = clampPoints(p.MinScore)
	g := clampPoints(p.CutoffScore)
	if len(steps) > 0 && good != "" {
		for i := range body.Steps {
			body.Steps[i].Allowed = slices.Contains(steps, body.Steps[i].Quality)
		}
		body.GoodEnoughAt = good
		if !p.UpgradeAllowed {
			// It never replaced a file: the first copy is good enough.
			for _, s := range body.Steps {
				if s.Allowed {
					body.GoodEnoughAt = s.Quality
					break
				}
			}
			g = body.MinPoints
		}
	}
	body.GoodEnoughPoints = &g
	// Older apps have no minimum upgrade step: any gain counts, as 1 does.
	body.UpgradeMinPoints = clampPoints(max(p.MinUpgradeScore, 1))
	body.Order = "quality_then_points"
	notes = append(notes, qualityNotes(p)...)
	// Sonarr profiles and Radarr's "Any" take every language; "Original"
	// is what the ready-made rules already ask.
	every := mediafiles.LanguageRule{Wanted: mediafiles.PickLanguages(), OriginalOK: true}
	switch lang := strings.ToLower(strings.TrimSpace(p.Language)); lang {
	case "original":
		body.Language = mediafiles.LanguageRule{Wanted: []string{}, OriginalOK: true}
	case "", "any", "unknown":
		body.Language = every
	default:
		want := withLang(body, subtitles.NormLanguage(lang))
		if want.Language.Wanted[0] != "" && mediafiles.CheckPickRules("x", []string{"movie"}, want) == nil {
			body = want
		} else {
			body.Language = every
			notes = append(notes, "its language "+clip(p.Language, 40)+" has no match here, so any language is taken")
		}
	}
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

// qualityNotes says where a profile's qualities do not line up with the
// ladder here: a quality with no step, and an order of its own.
func qualityNotes(p Profile) []string {
	var notes, lost []string
	last, reordered := -1, false
	for _, q := range p.Qualities {
		i := slices.Index(mediafiles.Ladder, qualityStep(q))
		if i < 0 {
			lost = append(lost, clip(q, 40))
			continue
		}
		reordered = reordered || i < last
		last = max(last, i)
	}
	if len(lost) > 0 {
		notes = append(notes, "no step here for "+strings.Join(lost, ", "))
	}
	if reordered {
		notes = append(notes, "it ranks its qualities in an order of its own; here they keep the ladder's order")
	}
	return notes
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

func withLang(b mediafiles.PickRulesBody, code string) mediafiles.PickRulesBody {
	b.Language = mediafiles.LanguageRule{Wanted: []string{code}}
	return b
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

// profileSteps maps a profile's allowed qualities to ladder steps, and its
// cutoff to the best allowed step it names (else the best allowed step).
func profileSteps(p Profile) (steps []string, good string) {
	for _, q := range p.Qualities {
		if s := qualityStep(q); s != "" && !slices.Contains(steps, s) {
			steps = append(steps, s)
		}
	}
	best := func(names []string) string {
		top := -1
		for _, q := range names {
			if i := slices.Index(mediafiles.Ladder, qualityStep(q)); i > top && slices.Contains(steps, mediafiles.Ladder[i]) {
				top = i
			}
		}
		if top < 0 {
			return ""
		}
		return mediafiles.Ladder[top]
	}
	if good = best(p.CutoffQualities); good == "" {
		good = best(p.Qualities)
	}
	return steps, good
}

// qualityStep maps an old app's quality name ("WEBDL-1080p", "Bluray-2160p
// Remux", "HDTV-720p", "DVD") to a ladder step; "" when none fits.
func qualityStep(name string) string {
	n := strings.ToLower(name)
	has := func(w ...string) bool {
		for _, x := range w {
			if strings.Contains(n, x) {
				return true
			}
		}
		return false
	}
	band := ""
	switch {
	case has("2160", "4k", "uhd"):
		band = "2160p"
	case has("1080"):
		band = "1080p"
	case has("720"):
		band = "720p"
	}
	switch {
	case has("cam", "workprint"):
		return "cam"
	case has("telesync", "telecine") || n == "ts" || n == "tc":
		return "telesync"
	case has("remux"):
		if band == "2160p" {
			return "2160p_remux"
		}
		return "1080p_remux"
	case has("bluray", "br-disk", "brdisk", "bdrip"):
		if band == "" {
			return "sd_dvd"
		}
		return band + "_bluray"
	case has("web"):
		if band == "" {
			return "sd_tv"
		}
		return band + "_web"
	case has("hdtv", "raw-hd", "rawhd"):
		switch band {
		case "2160p":
			return "2160p_web"
		case "":
			return "sd_tv"
		}
		return band + "_tv"
	case has("dvd", "regional"):
		return "sd_dvd"
	case has("sdtv"):
		return "sd_tv"
	}
	return ""
}
