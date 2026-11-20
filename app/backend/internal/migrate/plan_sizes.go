package migrate

import (
	"fmt"
	"maps"
	"math"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// usedProfiles are the quality profiles a title or an import list of the
// app uses. Only those get Pick rules of their own for their sizes; one
// nothing uses keeps its ready-made match, so no unused rules are made.
func usedProfiles(s *Snapshot) map[int64]bool {
	used := map[int64]bool{}
	for _, t := range s.Titles {
		used[t.ProfileID] = true
	}
	for _, l := range s.ImportLists {
		for _, k := range []string{"link.qualityProfileId", "qualityProfileId"} {
			if id := num(anyKey(l.Settings, k)); id != 0 {
				used[id] = true
			}
		}
	}
	return used
}

// appSizes are a Sonarr's or Radarr's size table as limits per quality
// step; nil when it has none or the owner left sizes behind.
func (b *builder) appSizes(s *Snapshot) map[string]mediafiles.SizeLimit {
	if b.noSizes || s.Kind != Sonarr && s.Kind != Radarr || len(s.QualitySizes) == 0 {
		return nil
	}
	rows := make([]library.OldSize, len(s.QualitySizes))
	for i, q := range s.QualitySizes {
		rows[i] = library.OldSize{Name: q.Name, Min: q.Min, Max: q.Max}
	}
	if l := library.OldSizeLimits(rows); len(l) > 0 {
		return l
	}
	return nil
}

// presetBody is the ready-made Pick rules a Sonarr or Radarr profile
// matched (matchPreset).
func presetBody(preset string) mediafiles.PickRulesBody {
	if preset == mediafiles.FourKName {
		return mediafiles.FourK()
	}
	return mediafiles.GoodEveryday()
}

// sizesDiffer says whether limits change the sizes of a step body allows.
func sizesDiffer(body mediafiles.PickRulesBody, limits map[string]mediafiles.SizeLimit) bool {
	for _, st := range body.Steps {
		l, ok := limits[st.Quality]
		if !st.Allowed || !ok {
			continue
		}
		cur, ok := body.SizeLimits[st.Quality]
		if !ok {
			cur, ok = body.SizeLimits["*"]
		}
		if !ok || cur != l {
			return true
		}
	}
	return false
}

// sizedRules plans a Sonarr or Radarr profile as a copy of the ready-made
// Pick rules it matched, with its app's size limits per step.
func (b *builder) sizedRules(s *Snapshot, p Profile, preset string, limits map[string]mediafiles.SizeLimit) *RulesPlan {
	body := presetBody(preset)
	body.SizeLimits = maps.Clone(body.SizeLimits)
	maps.Copy(body.SizeLimits, limits)
	name := clip(b.named(p.Name), maxName)
	return &RulesPlan{App: string(s.Kind), Profile: name, Preset: name, Name: name, ProfileKey: profileKey(s, p),
		Kinds: appKinds(s.Kind), Body: &body, CopyOf: preset, Sizes: true}
}

// sizedItem plans a profile as a copy of the ready-made Pick rules it
// matched with its app's sizes, and its titles onto that copy.
func (b *builder) sizedItem(s *Snapshot, p Profile, i int, key, preset, why string, limits map[string]mediafiles.SizeLimit, leftOut map[string]string) {
	rp := b.sizedRules(s, p, preset, limits)
	b.ownRulesName(rp, leftOut)
	b.presets[b.presetKey(s.Kind, p.ID)] = rp.Name
	b.imported[rp.Name] = true
	b.snapRules = append(b.snapRules, rp)
	b.add(GroupRules, Item{Key: key, Name: b.named(p.Name), From: b.from("quality_profiles", i),
		Detail: fmt.Sprintf("becomes Pick rules %s, a copy of %s with the old app's sizes", rp.Name, preset),
		Why:    why + "; sizes from the old app: " + sizeWords(*rp.Body), Rules: rp})
}

// profileKey names an old app's profile across imports.
func profileKey(s *Snapshot, p Profile) string {
	return fmt.Sprintf("%s:%s:%d", s.Kind, instanceLabel(s.Address), p.ID)
}

// appKinds are the title kinds Pick rules made from an app's profile serve.
func appKinds(k Kind) []string {
	return []string{map[Kind]string{Sonarr: "show", Radarr: "movie"}[k]}
}

// sizeWords says the size limits of the steps body allows ("1080p web
// 0.23 to 7.62 GB an hour, 1080p Blu-ray up to 9.08 GB an hour").
func sizeWords(body mediafiles.PickRulesBody) string {
	var out []string
	for _, st := range body.Steps {
		if l, ok := body.SizeLimits[st.Quality]; ok && st.Allowed {
			out = append(out, stepWords(st.Quality)+" "+limitWords(l))
		}
	}
	return strings.Join(out, ", ")
}

// sizesNote says the old app's sizes Pick rules took (the same words as
// sizedItem's, so the page shows both the same way).
func sizesNote(body mediafiles.PickRulesBody) string {
	return "sizes from the old app: " + sizeWords(body)
}

// stepWords names a ladder step ("1080p_bluray" is "1080p Blu-ray").
func stepWords(step string) string {
	words := map[string]string{"cam": "CAM", "telesync": "telesync", "sd_tv": "SD TV", "sd_dvd": "DVD",
		"tv": "TV", "web": "web", "bluray": "Blu-ray", "remux": "Remux"}
	if w, ok := words[step]; ok {
		return w
	}
	band, kind, _ := strings.Cut(step, "_")
	return band + " " + words[kind]
}

// limitWords says a per-minute limit in GB an hour.
func limitWords(l mediafiles.SizeLimit) string {
	gbh := func(n int64) string {
		return strconv.FormatFloat(math.Round(float64(n)*60/float64(int64(1)<<30)*100)/100, 'f', -1, 64)
	}
	open := l.Max == 0 || l.Max >= library.NoSizeMax
	switch {
	case l.Min == 0 && open:
		return "any size"
	case l.Min == 0:
		return "up to " + gbh(l.Max) + " GB an hour"
	case open:
		return "from " + gbh(l.Min) + " GB an hour"
	}
	return gbh(l.Min) + " to " + gbh(l.Max) + " GB an hour"
}

// sizeRows accounts for each row of an app's size table: brought onto the
// Pick rules made from its profiles, or the same as the ready-made ones
// they matched.
func (b *builder) sizeRows(s *Snapshot) {
	b.cover("quality_sizes")
	for i, q := range s.QualitySizes {
		from := b.from("quality_sizes", i)
		switch {
		case library.QualityStep(q.Name) == "":
			b.not(GroupRules, from, q.Name, "no HolyArrs quality step matches it")
		case b.noSizes:
			b.not(GroupRules, from, q.Name, "you left size limits behind; titles take the sizes of their Pick rules")
		case len(s.QualityProfiles) == 0:
			b.not(GroupRules, from, q.Name, "no quality profile came over to carry it")
		default:
			b.p.Mapped = append(b.p.Mapped, from)
		}
	}
}
