package downloads

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// aliasScores lifts a candidate already in the Library by its other names:
// a release named after the full title ("Marshals A Yellowstone Story")
// matches the Library's "Marshals" through its alias. Only titles the owner
// already has are lifted, so no new title is ever added on an alias alone.
func (a *Web) aliasScores(ctx context.Context, title string, cs []Candidate) {
	for i := range cs {
		if cs[i].Score >= sureScore {
			continue
		}
		id, err := a.libraryTitle(ctx, cs[i])
		if err != nil || id == 0 {
			continue
		}
		names, err := mediafiles.TitleOtherNames(ctx, a.Store.Read(), id)
		if err != nil {
			continue
		}
		for _, n := range names {
			if s := likeness(title, n); s > cs[i].Score {
				cs[i].Score = s
			}
		}
	}
}
