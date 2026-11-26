package sources

import (
	"context"
	"fmt"
	"slices"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Stranded counts the titles kept up to date with a copy (Version) that no
// automatic search would ask anyone for: of the Sources that search on their
// own for the title's kind, none is taken for the copy by the title's tags,
// the copy's Pick rules' tags or its Library folder's tags. It follows the
// rules Search goes by (copyOf, RulesTakeSource), so a title it counts gets
// "no Source takes this title's tags" on every search. The usual way to get
// there is tagging every Source while some titles have no tags: a title with
// no tags is served only by Sources with none, as in the old apps. A kind with
// no such Source at all is left out; Settings says so on its own.
// text is the Settings line, "" when no title is counted.
func Stranded(ctx context.Context, st *database.Store) (n int, text string, err error) {
	db := st.Read()
	srcs, err := database.Select[domain.Source](ctx, db, "sources", anyState)
	if err != nil {
		return 0, "", err
	}
	titles, err := database.Select[mediafiles.Title](ctx, db, "titles", "state = 'active' AND keep_up_to_date = 1")
	if err != nil || len(titles) == 0 {
		return 0, "", err
	}
	versions, err := database.Select[mediafiles.Version](ctx, db, "versions", "keep_up_to_date = 1")
	if err != nil {
		return 0, "", err
	}
	rules, err := database.Select[mediafiles.PickRules](ctx, db, "pick_rules", "1=1")
	if err != nil {
		return 0, "", err
	}
	folder, err := mediafiles.FolderSourceTags(ctx, db, 0)
	if err != nil {
		return 0, "", err
	}
	ruleTags := make(map[int64][]string, len(rules))
	for _, pr := range rules {
		ruleTags[pr.ID] = pr.Body.Tags
	}
	copies := map[int64][]mediafiles.Version{}
	for _, v := range versions {
		copies[v.TitleID] = append(copies[v.TitleID], v)
	}
	// The Sources an automatic search may ask for each kind, and the tags of
	// every Source of the kind (copyOf reads those for a folder's tags).
	usable, kinds := map[string][]domain.Source{}, map[string][][]string{}
	for _, kind := range []string{"movie", "show", "artist"} {
		kinds[kind] = kindTags(srcs, kind)
		for _, src := range srcs {
			if ofKind(src, kind) && src.State != "disabled" && src.Format != "rss" && src.UseFor.AutoSearch {
				usable[kind] = append(usable[kind], src)
			}
		}
	}
	for _, ti := range titles {
		asks := usable[ti.Kind]
		if len(asks) == 0 {
			continue
		}
		stuck := slices.ContainsFunc(copies[ti.ID], func(v mediafiles.Version) bool {
			cp := copyOf(ti.Tags, folder[ti.ID][v.ID], kinds[ti.Kind])
			rt := ruleTags[v.PickRulesID]
			return !slices.ContainsFunc(asks, func(src domain.Source) bool {
				return !(cp.only && len(src.Tags) == 0) && cp.takes(src.Tags, rt) && mediafiles.RulesTakeSource(src.Tags, rt)
			})
		})
		if stuck {
			n++
		}
	}
	switch {
	case n == 1:
		text = "1 title has no Source that searches for it: every Source it could use has a tag it does not share, or its Pick rules leave the Source out"
	case n > 1:
		text = fmt.Sprintf("%d titles have no Source that searches for them: every Source they could use has a tag they do not share, or their Pick rules leave the Source out", n)
	}
	return n, text, nil
}
