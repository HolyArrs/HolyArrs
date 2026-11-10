package mediafiles

import (
	"regexp"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

var inspectedRes = regexp.MustCompile(`^\d{3,4}p$`)

// JudgeName is the name a File's quality is judged by: its release name,
// plus the resolution the file inspection read when the name gives none.
// Files moved over from the old apps are often named without quality
// ("Show - S01E02 - Title"); without this they always looked below good
// enough and a better copy was sought for each.
func JudgeName(name string, mediaInfo map[string]any) string {
	r, _ := mediaInfo["resolution"].(string)
	if !inspectedRes.MatchString(r) || releasename.Parse(name, releasename.Hints{}).Resolution.Value > 0 {
		return name
	}
	return name + " " + r
}
