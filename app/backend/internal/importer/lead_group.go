package importer

import (
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// withoutLeadGroup reads a file name again without a leading "group-", the
// way scene files inside RAR sets are named
// ("bravery-the.agency.2024.s01e02.1080p.bluray.x264.mkv"). It is used only
// after the full name matched nothing, and the rest must still name a Title
// in lib, so a hyphenated title is never cut short. A group with a hyphen of
// its own ("d-z0n3-title...") is tried one hyphen further, up to three.
func withoutLeadGroup(base string, h releasename.Hints, lib []mediafiles.TitleRef) (releasename.Result, mediafiles.Match, bool) {
	end := 0
	for try := 0; try < 3; try++ {
		i := strings.IndexByte(base[end:], '-')
		if i < 0 {
			break
		}
		end += i
		group := base[:end]
		if end == 0 || strings.ContainsAny(group, ". _[]()") || len(group) > 20 {
			break
		}
		p := releasename.Parse(base[end+1:], h)
		if m := mediafiles.MatchTitle(p, lib); m.Title != nil {
			if p.Group.Value == "" {
				p.Group = releasename.Field[string]{Value: group, Conf: releasename.Likely}
			}
			return p, m, true
		}
		end++
	}
	return releasename.Result{}, mediafiles.Match{}, false
}
