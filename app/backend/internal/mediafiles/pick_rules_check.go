package mediafiles

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// FieldError is a check failure on one field of a request. Field is a dotted
// path such as "name" or "body.protocol.prefer"; Text says what is wrong in
// plain words.
type FieldError struct {
	Field string
	Text  string
}

func (e *FieldError) Error() string { return e.Text }

func fieldErr(field, format string, args ...any) error {
	return &FieldError{Field: field, Text: fmt.Sprintf(format, args...)}
}

const (
	pickMaxText   = 200
	pickMaxNested = 64
	pickMaxPoints = 10000
	pickMaxWait   = 10080
	pickMaxAge    = 36500
)

var (
	videoTraits       = traitSet("hevc", "ddp_audio", "web_dl", "upscaled", "dubbed_only", "hardcoded_subs", "3d", "cam", "telesync", "remux", "av1", "dv", "hdr10", "hdr10plus", "atmos")
	musicTraits       = traitSet("cd_source", "web_source", "vinyl_source", "log_cue", "hires", "mp3", "aac", "opus", "vorbis", "flac", "alac", "single_image", "lossy_transcode", "deluxe")
	checkSafetyTraits = traitSet("sample", "executable", "password_protected", "disc_image")
	pickLangs         = traitSet("en", "fr", "de", "it", "es", "ru", "ja", "ko")
)

func traitSet(values ...string) map[string]bool {
	m := make(map[string]bool, len(values))
	for _, v := range values {
		m[v] = true
	}
	return m
}

func inPickPoints(n int) bool { return n >= -pickMaxPoints && n <= pickMaxPoints }

// CheckPickRules checks a Pick rules name, kinds and body: the rules a data
// pack and the editing API share. It answers a *FieldError.
func CheckPickRules(name string, kinds []string, b PickRulesBody) error {
	if strings.TrimSpace(name) == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > pickMaxText {
		return fieldErr("name", "The name must hold 1 to 200 characters.")
	}
	if len(kinds) == 0 || len(kinds) > 3 {
		return fieldErr("kinds", "Pick one to three kinds: movie, show or music.")
	}
	music := false
	seenKinds := map[string]bool{}
	for _, kind := range kinds {
		if seenKinds[kind] || (kind != "movie" && kind != "show" && kind != "music") {
			return fieldErr("kinds", "Unknown or repeated kind %q; kinds are movie, show and music.", kind)
		}
		seenKinds[kind] = true
		music = music || kind == "music"
	}
	if music && len(kinds) != 1 {
		return fieldErr("kinds", "Music Pick rules cannot also be for movies or shows.")
	}
	ladder, traits := Ladder, videoTraits
	if music {
		ladder, traits = MusicLadder, musicTraits
	}
	if len(b.Steps) == 0 || len(b.Steps) > pickMaxNested {
		return fieldErr("body.steps", "List 1 to 64 quality steps.")
	}
	steps := map[string]bool{}
	allowed, goodEnoughAllowed := false, false
	for _, s := range b.Steps {
		if !slices.Contains(ladder, s.Quality) || steps[s.Quality] {
			return fieldErr("body.steps", "Unknown or repeated quality step %q.", s.Quality)
		}
		steps[s.Quality] = true
		allowed = allowed || s.Allowed
		goodEnoughAllowed = goodEnoughAllowed || (s.Quality == b.GoodEnoughAt && s.Allowed)
	}
	if !allowed {
		return fieldErr("body.steps", "Allow at least one quality step.")
	}
	if !steps[b.GoodEnoughAt] || !goodEnoughAllowed {
		return fieldErr("body.good_enough_at", "Good enough must be one of the allowed quality steps.")
	}
	if len(b.Traits) > pickMaxNested {
		return fieldErr("body.traits", "List at most 64 traits.")
	}
	seen := map[string]bool{}
	// An own trait is checked by its key's form; one deleted later
	// simply never matches.
	known := func(t string) bool { _, own := OwnTraitID(t); return traits[t] || own }
	for _, tp := range b.Traits {
		if !known(tp.Trait) || seen[tp.Trait] {
			return fieldErr("body.traits", "Unknown or repeated trait %q.", tp.Trait)
		}
		if !inPickPoints(tp.Points) {
			return fieldErr("body.traits", "Points for %q must be from -10000 to 10000.", tp.Trait)
		}
		seen[tp.Trait] = true
	}
	for _, l := range []struct {
		field  string
		values []string
		safety bool
	}{{"body.must_have", b.MustHave, false}, {"body.never", b.Never, true}} {
		if len(l.values) > pickMaxNested {
			return fieldErr(l.field, "List at most 64 traits.")
		}
		seen := map[string]bool{}
		for _, trait := range l.values {
			ok := known(trait) || (l.safety && checkSafetyTraits[trait])
			if !ok || seen[trait] {
				return fieldErr(l.field, "Unknown or repeated trait %q.", trait)
			}
			seen[trait] = true
		}
	}
	if !inPickPoints(b.MinPoints) {
		return fieldErr("body.min_points", "Minimum points must be from -10000 to 10000.")
	}
	if !inPickPoints(b.UpgradeMinPoints) {
		return fieldErr("body.upgrade_min_points", "Upgrade points must be from -10000 to 10000.")
	}
	if b.GoodEnoughPoints != nil && !inPickPoints(*b.GoodEnoughPoints) {
		return fieldErr("body.good_enough_points", "Good enough points must be from -10000 to 10000.")
	}
	if len(b.SizeLimits) > pickMaxNested {
		return fieldErr("body.size_limits", "List at most 64 size limits.")
	}
	for step, limit := range b.SizeLimits {
		if step != "*" && !steps[step] {
			return fieldErr("body.size_limits", "A size limit names %q, which is not a listed quality step.", step)
		}
		if limit.Min < 0 || limit.Max < limit.Min || limit.Max > 1<<50 {
			return fieldErr("body.size_limits", "The size limit for %q needs 0 <= smallest <= largest <= 1 PiB.", step)
		}
	}
	if b.Protocol.Prefer != "torrent" && b.Protocol.Prefer != "usenet" && b.Protocol.Prefer != "either" {
		return fieldErr("body.protocol.prefer", "Prefer must be torrent, usenet or either.")
	}
	if b.Protocol.WaitMinutesTorrent < 0 || b.Protocol.WaitMinutesTorrent > pickMaxWait {
		return fieldErr("body.protocol.wait_minutes_torrent", "The torrent wait must be from 0 to 10080 minutes.")
	}
	if b.Protocol.WaitMinutesUsenet < 0 || b.Protocol.WaitMinutesUsenet > pickMaxWait {
		return fieldErr("body.protocol.wait_minutes_usenet", "The Usenet wait must be from 0 to 10080 minutes.")
	}
	if len(b.Language.Wanted) > 32 {
		return fieldErr("body.language.wanted", "List at most 32 languages.")
	}
	seenLang := map[string]bool{}
	for _, lang := range b.Language.Wanted {
		if !pickLangs[lang] || seenLang[lang] {
			return fieldErr("body.language.wanted", "Unknown or repeated language %q.", lang)
		}
		seenLang[lang] = true
	}
	if b.Order != "quality_then_points" && b.Order != "points_then_quality" {
		return fieldErr("body.order", "Order must be quality_then_points or points_then_quality.")
	}
	if !slices.Contains([]string{"take_any", "same_group_only", "prefer_only", "ignore"}, b.ProperPolicy.Mode) {
		return fieldErr("body.proper_policy.mode", "The fixed re-release mode must be take_any, same_group_only, prefer_only or ignore.")
	}
	if b.ProperPolicy.MaxAgeDays < 0 || b.ProperPolicy.MaxAgeDays > pickMaxAge {
		return fieldErr("body.proper_policy.max_age_days", "The fixed re-release age must be from 0 to 36500 days.")
	}
	return nil
}
