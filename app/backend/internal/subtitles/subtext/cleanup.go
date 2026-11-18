package subtext

import (
	"errors"
	"strings"
)

// CleanupOptions are the cleanup steps for one language. Every step is off
// until the owner turns it on, so a saved subtitle stays as the site sent it.
type CleanupOptions struct {
	RemoveHI     bool `json:"remove_hi"`
	RemoveTags   bool `json:"remove_tags"`
	RemoveEmoji  bool `json:"remove_emoji"`
	OCRFixes     bool `json:"ocr_fixes"`
	CommonFixes  bool `json:"common_fixes"`
	FixUppercase bool `json:"fix_uppercase"`
	RemoveAds    bool `json:"remove_ads"`
	FixOverlaps  bool `json:"fix_overlaps"`
}

// Any reports whether at least one step is on.
func (o CleanupOptions) Any() bool { return o != (CleanupOptions{}) }

// CleanupReport counts what a cleanup did. Steps counts the cues each step
// touched, keyed by the step's option name, such as remove_ads.
type CleanupReport struct {
	Removed int            `json:"removed"`
	Changed int            `json:"changed"`
	Steps   map[string]int `json:"steps,omitempty"`
}

// Any reports whether the cleanup changed anything.
func (r CleanupReport) Any() bool { return r.Removed > 0 || r.Changed > 0 }

func (r *CleanupReport) count(step string, n int) {
	if n == 0 {
		return
	}
	if r.Steps == nil {
		r.Steps = map[string]int{}
	}
	r.Steps[step] += n
}

// OverlapGapMS is the gap left before the next cue when an overlap is fixed,
// so players never show two cues at once.
const OverlapGapMS = 50

// ErrLooseText refuses a cleanup when the parsed cues do not hold every line of
// the file, such as a cue with a blank line inside it. Writing those cues back
// would lose the lines the parser passed over.
var ErrLooseText = errors.New("the subtitle has text outside its cues, so it is left as it is")

// textStep is one cleanup step that works on the text of a single cue.
type textStep struct {
	name string
	on   func(CleanupOptions) bool
	fix  func(text, lang string) string
}

// Tags go first so the later steps see plain words, and hearing-impaired
// lines go before the case fix so speaker names in capitals are already gone.
var textSteps = []textStep{
	{"remove_tags", func(o CleanupOptions) bool { return o.RemoveTags }, removeTags},
	{"remove_emoji", func(o CleanupOptions) bool { return o.RemoveEmoji }, removeEmoji},
	{"remove_hi", func(o CleanupOptions) bool { return o.RemoveHI }, removeHI},
	{"ocr_fixes", func(o CleanupOptions) bool { return o.OCRFixes }, ocrFixes},
	{"common_fixes", func(o CleanupOptions) bool { return o.CommonFixes }, commonFixes},
	{"fix_uppercase", func(o CleanupOptions) bool { return o.FixUppercase }, fixUppercase},
}

// Cleanup runs the steps o turns on over cues, which must be in time order as
// ParseCues gives them. lang is the subtitle's language code; steps that only
// suit some languages, such as the English OCR fixes, look at it. A cue left
// with no text is removed.
func Cleanup(cues []Cue, o CleanupOptions, lang string) ([]Cue, CleanupReport) {
	var rep CleanupReport
	var ads map[int]bool
	if o.RemoveAds {
		ads = adCues(cues, lang)
	}
	out := make([]Cue, 0, len(cues))
	changed := make([]bool, 0, len(cues))
	for i, c := range cues {
		if ads[i] {
			rep.Removed++
			rep.count("remove_ads", 1)
			continue
		}
		text := c.Text
		for _, s := range textSteps {
			if !s.on(o) {
				continue
			}
			if t := s.fix(text, lang); t != text {
				text = t
				rep.count(s.name, 1)
			}
		}
		if blank(text) {
			// A cue that was empty to begin with is dropped by WriteSRT anyway
			// and is not counted as cleanup's doing.
			if !blank(c.Text) {
				rep.Removed++
			}
			continue
		}
		changed = append(changed, text != c.Text)
		c.Text = text
		out = append(out, c)
	}
	if o.FixOverlaps {
		rep.count("fix_overlaps", fixOverlaps(out, changed))
	}
	for _, ch := range changed {
		if ch {
			rep.Changed++
		}
	}
	return out, rep
}

// CleanupSRT cleans SRT text with o. It returns text unchanged when every step
// is off or when no step changed anything, so a file is only rewritten when
// cleanup did something. Text that cannot be read whole as cues is refused and
// left as it is.
func CleanupSRT(text string, o CleanupOptions, lang string) (string, CleanupReport, error) {
	if !o.Any() {
		return text, CleanupReport{}, nil
	}
	cues, err := ParseCues(text, SRT)
	if err != nil {
		return text, CleanupReport{}, err
	}
	if !holdsAllLines(text, cues) {
		return text, CleanupReport{}, ErrLooseText
	}
	out, rep := Cleanup(cues, o, lang)
	if !rep.Any() {
		return text, rep, nil
	}
	return WriteSRT(out), rep, nil
}

// RemoveAds drops the ad and credit cues from cues and returns how many went.
// It is for cues that are not saved as they are, such as the source of a
// translation, so the translator never spends lines on a site's credits.
func RemoveAds(cues []Cue, lang string) ([]Cue, int) {
	ads := adCues(cues, lang)
	if len(ads) == 0 {
		return cues, 0
	}
	out := make([]Cue, 0, len(cues)-len(ads))
	for i, c := range cues {
		if !ads[i] {
			out = append(out, c)
		}
	}
	return out, len(ads)
}

// holdsAllLines reports whether cues carry at least as many text lines as the
// SRT text has outside its numbers and times. The parser passes over lines
// after a blank line inside a cue; rewriting those cues would drop them.
func holdsAllLines(text string, cues []Cue) bool {
	inText := 0
	for line := range strings.Lines(text) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, _, ok := cueTimes(line); ok {
			continue
		}
		if _, ok := digits(line); ok {
			continue // a cue number, or a line of dialogue that is only a number
		}
		inText++
	}
	inCues := 0
	for _, c := range cues {
		for l := range strings.Lines(c.Text) {
			if strings.TrimSpace(l) != "" {
				inCues++
			}
		}
	}
	return inCues >= inText
}

// fixOverlaps ends each cue a little before the next one starts when the two
// overlap, marks the cues it moved in changed and returns how many it moved.
// Cues that start together are left alone, since some subtitles show two
// lines at once on purpose, one at the top and one at the bottom.
func fixOverlaps(cues []Cue, changed []bool) int {
	n := 0
	for i := 0; i+1 < len(cues); i++ {
		next := cues[i+1].StartMS
		if cues[i].EndMS <= next || next <= cues[i].StartMS {
			continue
		}
		end := next - OverlapGapMS
		if end <= cues[i].StartMS {
			end = next
		}
		cues[i].EndMS = end
		changed[i] = true
		n++
	}
	return n
}

// baseLang turns a language code such as pt-br into its base, pt.
func baseLang(lang string) string {
	lang = strings.ToLower(lang)
	if i := strings.IndexAny(lang, "-_"); i >= 0 {
		return lang[:i]
	}
	return lang
}
