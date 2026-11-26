package mediafiles

import "time"

// TraitScore is one points trait that fits a file, with its points.
type TraitScore struct {
	Trait  string `json:"trait"`
	Points int    `json:"points"`
}

// FilePoints is how Pick rules score a file already in the Library: its
// quality step, the traits that fit it and their total, beside the good
// enough step and points.
type FilePoints struct {
	Step             string       `json:"step"`
	GoodEnoughAt     string       `json:"good_enough_at"`
	StepReached      bool         `json:"step_reached"`
	Points           int          `json:"points"`
	GoodEnoughPoints *int         `json:"good_enough_points"`
	Traits           []TraitScore `json:"traits"`
}

// ScoreFile scores a file the way a search scores the copy it holds
// (CurrentOfFile): judge is JudgeName of its release name, size its bytes
// and audio its audio languages (FileLanguages).
func ScoreFile(judge string, size int64, audio []string, rules PickRulesBody) FilePoints {
	return ScoreTitleFile(judge, size, audio, Title{}, rules)
}

// ScoreTitleFile is ScoreFile for a file of Title t: language and year
// tests read t's original language and year (CurrentOfTitleFile).
func ScoreTitleFile(judge string, size int64, audio []string, t Title, rules PickRulesBody) FilePoints {
	cur := CurrentOfTitleFile(judge, size, time.Time{}, audio, t, rules)
	out := FilePoints{Step: cur.Step, GoodEnoughAt: rules.GoodEnoughAt, Points: cur.Points, GoodEnoughPoints: rules.GoodEnoughPoints, Traits: []TraitScore{}}
	idx := stepIndex(rules, cur.Step)
	out.StepReached = idx >= 0 && idx >= stepIndex(rules, rules.GoodEnoughAt)
	if cur.parse != nil {
		_, hits := scoreTraits(*cur.parse, rules)
		for i, tp := range rules.Traits {
			if hits[i].ok && tp.Points != 0 {
				out.Traits = append(out.Traits, TraitScore{Trait: tp.Trait, Points: tp.Points})
			}
		}
		if n := len(deviceMisses(*cur.parse, rules.Devices)); n > 0 {
			out.Traits = append(out.Traits, TraitScore{Trait: "plays_on", Points: -DevicePenalty * n})
		}
	}
	return out
}
