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
// in lib, so a hyphenated title is never cut short.
func withoutLeadGroup(base string, h releasename.Hints, lib []mediafiles.TitleRef) (releasename.Result, mediafiles.Match, bool) {
	i := strings.IndexByte(base, '-')
	if i <= 0 || strings.ContainsAny(base[:i], ". _[]()") {
		return releasename.Result{}, mediafiles.Match{}, false
	}
	p := releasename.Parse(base[i+1:], h)
	m := mediafiles.MatchTitle(p, lib)
	if m.Title == nil {
		return releasename.Result{}, mediafiles.Match{}, false
	}
	if p.Group.Value == "" {
		p.Group = releasename.Field[string]{Value: base[:i], Conf: releasename.Likely}
	}
	return p, m, true
}
