package library

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// OldProfile is a Sonarr or Radarr quality profile as Pick rules read it,
// from an import or from a push through the arr API.
type OldProfile struct {
	// Qualities are the allowed qualities by name with groups opened up,
	// and CutoffQualities the cutoff's (several when it is a group).
	Qualities       []string
	CutoffQualities []string
	// Groups are the allowed groups' qualities, one list per group.
	Groups          [][]string
	UpgradeAllowed  bool
	MinScore        int
	CutoffScore     int
	MinUpgradeScore int
	// Language is the wanted language by name (Radarr), and LanguageCode
	// its HolyArrs code ("" when none fits).
	Language     string
	LanguageCode string
	// FourK starts from the 4K preset instead of Standard.
	FourK bool
	// Sizes is the old app's size table (shared by all its profiles);
	// empty leaves the size limits out.
	Sizes []OldQualitySize
}

// OldQualitySize is one row of an old app's size table, in MB a minute;
// Max 0 means no largest size and Preferred 0 no preferred size.
type OldQualitySize struct {
	Quality   string  `json:"quality"`
	Min       float64 `json:"min"`
	Max       float64 `json:"max"`
	Preferred float64 `json:"preferred,omitempty"`
}

// OldSizeLimits turns an old app's size table into per-minute limits per
// ladder step. Qualities sharing a step take the widest range.
func OldSizeLimits(rows []OldQualitySize) map[string]mediafiles.SizeLimit {
	const mib = float64(1 << 20)
	const noMax = int64(1) << 40 // a smallest size with no largest one
	out := map[string]mediafiles.SizeLimit{}
	open := map[string]bool{}
	for _, r := range rows {
		step := QualityStep(r.Quality)
		if !slices.Contains(mediafiles.Ladder, step) {
			continue
		}
		lo, hi, pref := int64(max(0, r.Min)*mib), int64(max(0, r.Max)*mib), int64(max(0, r.Preferred)*mib)
		l, seen := out[step]
		l.Preferred = max(l.Preferred, pref)
		switch {
		case !seen:
			l = mediafiles.SizeLimit{Min: lo, Max: hi, Preferred: pref, PerMinute: true}
			open[step] = hi == 0
		default:
			l.Min = min(l.Min, lo)
			if hi == 0 {
				open[step] = true
			}
			l.Max = max(l.Max, hi)
		}
		out[step] = l
	}
	for step, l := range out {
		switch {
		case open[step] && l.Min == 0:
			l.Max = 0
		case open[step] || l.Max < l.Min:
			l.Max = max(noMax, l.Min)
		}
		if l.Preferred > 0 {
			l.Preferred = max(l.Min, l.Preferred)
			if l.Max > 0 {
				l.Preferred = min(l.Max, l.Preferred)
			}
		}
		out[step] = l
	}
	return out
}

// ClampPoints keeps points inside the Pick rules bound.
func ClampPoints(n int) int { return max(-TrashMaxPoints, min(TrashMaxPoints, n)) }

// OldProfileRules builds the Pick rules body an old app's profile asks
// for, without its traits: its allowed qualities as steps, its cutoff as
// Good enough, its score settings as the points thresholds and its
// language. notes say what did not carry over.
func OldProfileRules(p OldProfile) (mediafiles.PickRulesBody, []string) {
	body := mediafiles.GoodEveryday()
	if p.FourK {
		body = mediafiles.FourK()
	}
	body.Traits = nil
	var notes []string
	// The old apps keep size limits per quality and refuse no traits of
	// their own, so the ready-made rules' limits and refusals are left out.
	body.SizeLimits = OldSizeLimits(p.Sizes)
	if len(body.SizeLimits) == 0 {
		notes = append(notes, "its size limits are not brought; set them under Pick rules")
	}
	discs := slices.ContainsFunc(p.Qualities, func(q string) bool {
		q = strings.ToLower(q)
		return strings.Contains(q, "br-disk") || strings.Contains(q, "brdisk")
	})
	steps, good := profileSteps(p.Qualities, p.CutoffQualities)
	body.Never = slices.DeleteFunc(slices.Clone(body.Never), func(t string) bool {
		// A step it allows is not also refused as a trait.
		return t == "hardcoded_subs" || t == "3d" || t == "disc_image" && discs || (t == "cam" || t == "telesync") && slices.Contains(steps, t)
	})
	body.MinPoints = ClampPoints(p.MinScore)
	g := ClampPoints(p.CutoffScore)
	if len(steps) > 0 && good != "" {
		for i := range body.Steps {
			body.Steps[i].Allowed = slices.Contains(steps, body.Steps[i].Quality)
		}
		keepProfileOrder(body.Steps, steps)
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
	body.UpgradeMinPoints = ClampPoints(max(p.MinUpgradeScore, 1))
	body.Order = "quality_then_points"
	// The old apps rank a group's qualities equal and let the score decide.
	body.EqualSteps = groupSteps(p.Groups)
	notes = append(notes, qualityNotes(p.Qualities)...)
	// Sonarr profiles and Radarr's "Any" take every language; "Original"
	// is what the ready-made rules already ask.
	every := mediafiles.LanguageRule{Wanted: mediafiles.PickLanguages(), OriginalOK: true}
	switch lang := strings.ToLower(strings.TrimSpace(p.Language)); lang {
	case "original":
		body.Language = mediafiles.LanguageRule{Wanted: []string{}, OriginalOK: true}
	case "", "any", "unknown":
		body.Language = every
	default:
		want := body
		want.Language = mediafiles.LanguageRule{Wanted: []string{p.LanguageCode}}
		if p.LanguageCode != "" && mediafiles.CheckPickRules("x", []string{"movie"}, want) == nil {
			body = want
		} else {
			body.Language = every
			notes = append(notes, "its language "+clipWords(p.Language, 40)+" has no match here, so any language is taken")
		}
	}
	return body, notes
}

// groupSteps lists, per group, the steps its qualities fall on; only
// groups spanning more than one step are kept.
func groupSteps(groups [][]string) [][]string {
	var out [][]string
	for _, g := range groups {
		var steps []string
		for _, q := range g {
			if s := QualityStep(q); s != "" && !slices.Contains(steps, s) {
				steps = append(steps, s)
			}
		}
		if len(steps) > 1 {
			out = append(out, steps)
		}
	}
	return out
}

// clipWords cuts s to n characters.
func clipWords(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > n {
		return string([]rune(s)[:n])
	}
	return s
}

// qualityNotes says where a profile's qualities do not line up with the
// ladder here: a quality with no step.
func qualityNotes(qualities []string) []string {
	var lost []string
	for _, q := range qualities {
		if slices.Index(mediafiles.Ladder, QualityStep(q)) < 0 {
			lost = append(lost, clipWords(q, 40))
		}
	}
	if len(lost) > 0 {
		return []string{"no step here for " + strings.Join(lost, ", ")}
	}
	return nil
}

// keepProfileOrder ranks the allowed steps among themselves in the
// profile's order (low to high), so a profile that puts web above Blu-ray
// keeps doing so. Steps not allowed keep their places.
func keepProfileOrder(body []mediafiles.Step, order []string) {
	var at []int
	for i, s := range body {
		if s.Allowed {
			at = append(at, i)
		}
	}
	var want []string
	for _, q := range order {
		if slices.ContainsFunc(body, func(s mediafiles.Step) bool { return s.Allowed && s.Quality == q }) {
			want = append(want, q)
		}
	}
	if len(want) != len(at) {
		return
	}
	for k, i := range at {
		body[i].Quality = want[k]
	}
}

// profileSteps maps a profile's allowed qualities to ladder steps, and its
// cutoff to the best allowed step it names (else the best allowed step).
func profileSteps(qualities, cutoff []string) (steps []string, good string) {
	for _, q := range qualities {
		if s := QualityStep(q); s != "" && !slices.Contains(steps, s) {
			steps = append(steps, s)
		}
	}
	// The best is the last named in the profile's own order (low to high).
	best := func(names []string) string {
		top := -1
		for _, q := range names {
			if i := slices.Index(steps, QualityStep(q)); i > top {
				top = i
			}
		}
		if top < 0 {
			return ""
		}
		return steps[top]
	}
	if good = best(cutoff); good == "" {
		good = best(qualities)
	}
	return steps, good
}

// QualityStep maps an old app's quality name ("WEBDL-1080p", "Bluray-2160p
// Remux", "HDTV-720p", "DVD") to a ladder step; "" when none fits.
func QualityStep(name string) string {
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
