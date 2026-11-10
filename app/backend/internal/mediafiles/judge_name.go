package mediafiles

import (
	"regexp"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

var inspectedRes = regexp.MustCompile(`^\d{3,4}p$`)

// JudgeName is the name a File's quality is judged by: its release name,
// plus the quality the old app recorded for it when the name gives no
// source, plus the resolution the file inspection read when there is still
// none. Files moved over from the old apps are often named without quality
// ("Show - S01E02 - Title"); without this they always looked below good
// enough and a better copy was sought for each.
func JudgeName(name string, mediaInfo map[string]any) string {
	p := releasename.Parse(name, releasename.Hints{})
	if rec, _ := mediaInfo[RecordedQualityKey].(string); rec != "" && p.Source.Value == "" {
		name += " " + rec
		p = releasename.Parse(name, releasename.Hints{})
	}
	r, _ := mediaInfo["resolution"].(string)
	if !inspectedRes.MatchString(r) || p.Resolution.Value > 0 {
		return name
	}
	return name + " " + r
}

// RecordedQualityKey is the media info key of the quality the old app
// recorded for a moved-over file ("WEBDL-1080p").
const RecordedQualityKey = "recorded_quality"

// OwnedReachesGoodEnough is ReachesGoodEnough for a file already in the
// Library. It reads the name with the inspected resolution (JudgeName). A
// file whose name says nothing about its source (web, Blu-ray, TV), as the
// old apps name episodes, gets the benefit of the doubt: it counts as good
// enough when its resolution reaches the good-enough step's, so moving a
// Library over never asks for a better copy of every file.
func OwnedReachesGoodEnough(name string, mediaInfo map[string]any, rules PickRulesBody) bool {
	judge := JudgeName(name, mediaInfo)
	if ReachesGoodEnough(judge, rules) {
		return true
	}
	p := releasename.Parse(judge, hintsFor(rules))
	if p.Music != nil || p.Source.Value != "" || p.Resolution.Value == 0 {
		return false
	}
	need := stepResolution(rules.GoodEnoughAt)
	return need > 0 && p.Resolution.Value >= need
}

// stepResolution is the resolution a video ladder step stands for
// ("1080p_web" is 1080; the SD steps are 480), 0 when it has none.
func stepResolution(step string) int {
	switch {
	case strings.HasPrefix(step, "2160p"):
		return 2160
	case strings.HasPrefix(step, "1080p"):
		return 1080
	case strings.HasPrefix(step, "720p"):
		return 720
	case strings.HasPrefix(step, "sd_"):
		return 480
	}
	return 0
}
