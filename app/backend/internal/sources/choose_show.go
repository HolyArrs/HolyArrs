package sources

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// matchedByID says a stored Release was matched by the id its Source gave.
func matchedByID(rel domain.Release) bool {
	reasons, _ := rel.Verdict["reasons"].([]any)
	for _, r := range reasons {
		if m, _ := r.(map[string]any); m["rule"] == sourceIDRule && m["effect"] == "pass" {
			return true
		}
	}
	return false
}

// heldShowMiss says a waiting show Release no longer names the Slot's show
// (its year is not the show's), unless it was matched by the Source's id.
func (s *Searcher) heldShowMiss(ctx context.Context, sl mediafiles.Slot, rel domain.Release) bool {
	if matchedByID(rel) {
		return false
	}
	db := s.Store.Read()
	titleID, err := mediafiles.ItemTitleID(ctx, db, sl.ItemID)
	if err != nil {
		return false
	}
	var ti mediafiles.Title
	if err := mediafiles.LoadTitle(ctx, db, titleID, &ti); err != nil || ti.Kind != "show" {
		return false
	}
	p := releasename.Parse(rel.Name, releasename.Hints{Size: rel.Size, Show: true})
	return mediafiles.MatchRelease(p, []mediafiles.TitleRef{showSet{ti: ti}.ref()}).Title == nil
}

// showSearchSlots returns the episodes a Release that matched none can
// still be forced onto, and why it needs force. It applies to a Release
// found by a season search of a show, in two cases. Either the Release has
// one of the show's names and only its year kept it from matching, or it
// holds one of the show's names as whole words with only a site's lead word,
// a release tag or its own group beside them, and it names a season or
// episode. The Slots are those of the episodes its numbers cover, in the
// season and Version searched. A Release naming another season gets none,
// and neither does one with a loose name that names no season or episode.
// skipped says which episodes were left out for a protected copy or a copy
// as good. wider is set when the Release holds the season searched and
// another one. Anything unclear returns none.
func (s *Searcher) showSearchSlots(ctx context.Context, rel domain.Release) (out []int64, skipped, why []string, wider bool) {
	if rel.JobID == nil {
		return nil, nil, nil, false
	}
	db := s.Store.Read()
	var job domain.Job
	if err := database.Get(ctx, db, "jobs", *rel.JobID, &job); err != nil || job.Kind != jobs.KindSearchSeason || job.Subject == nil || job.Subject.Type != "part" {
		return nil, nil, nil, false
	}
	vid, _ := job.Params["version_id"].(float64)
	var part mediafiles.Part
	var ti mediafiles.Title
	var ver mediafiles.Version
	if mediafiles.LoadPart(ctx, db, job.Subject.ID, &part) != nil || part.Number == nil || mediafiles.LoadTitle(ctx, db, part.TitleID, &ti) != nil ||
		mediafiles.LoadVersion(ctx, db, int64(vid), &ver) != nil || ver.TitleID != ti.ID {
		return nil, nil, nil, false
	}
	set, err := s.loadShow(ctx, ti, ver)
	if err != nil {
		return nil, nil, nil, false
	}
	p := releasename.Parse(rel.Name, releasename.Hints{Size: rel.Size, Show: true})
	lib := []mediafiles.TitleRef{set.ref()}
	if mediafiles.MatchRelease(p, lib).Title != nil {
		return nil, nil, nil, false
	}
	if mediafiles.MatchTitle(p, lib).Title != nil {
		why = append(why, "its year did not match "+ti.Name+"; it was found searching for it")
	} else if p.Show != nil && looseShowName(p.Title.Value, p.Group.Value, lib[0], s.otherTitleRefs(ctx, ti.ID)) {
		why = append(why, fmt.Sprintf("its name %q only loosely matched %s; it was found searching for it", rel.Name, ti.Name))
	} else {
		return nil, nil, nil, false
	}
	refs := make([]mediafiles.EpisodeRef, len(set.eps))
	slots := map[int64]*mediafiles.Slot{}
	for i, e := range set.eps {
		refs[i] = e.ref
		slots[e.it.ID] = e.sl
	}
	held, miss := mediafiles.MatchEpisodes(p, set.order(), refs)
	if miss != "" {
		return nil, nil, nil, false
	}
	jr := mediafiles.ReleaseFacts{Name: rel.Name, Parse: p, Size: rel.Size, Protocol: rel.Protocol}
	if slices.ContainsFunc(held, func(e mediafiles.EpisodeRef) bool { return e.Season != *part.Number }) {
		return nil, nil, nil, slices.ContainsFunc(held, func(e mediafiles.EpisodeRef) bool { return e.Season == *part.Number })
	}
	for _, e := range held {
		sl := slots[e.ItemID]
		if sl == nil || !canTakePick(sl.State) && !StuckChoice(*sl, time.Now()) || turnedDownFor(rel, sl.ID) {
			continue
		}
		if slotHasProtectedCopy(ctx, db, sl.ID) {
			skipped = append(skipped, e.Label()+" has a protected copy")
			continue
		}
		if c, err := s.loadSlot(ctx, sl.ID); err == nil && c.cur != nil && !mediafiles.Gains(jr, c.pr.Body, c.cur) {
			skipped = append(skipped, e.Label()+" already has a copy as good")
			continue
		}
		out = append(out, sl.ID)
	}
	return out, skipped, why, false
}

// looseLead and looseTail are the words a release may carry before or after
// the show's name without being another title: a site's call to action
// before it and a release tag after it. Any other word beside the name (a
// subtitle, a country, "Soundtrack", a second show) makes it another title.
var (
	looseLead = map[string]bool{"watch": true, "download": true, "stream": true, "streaming": true, "free": true, "online": true, "full": true, "hd": true}
	looseTail = map[string]bool{"complete": true, "uncut": true, "dubbed": true, "subbed": true, "multi": true, "dual": true, "remastered": true, "repack": true, "proper": true, "batch": true}
)

// looseShowName says whether a release title is one of the show's names with
// extra words only at the edges, and only safe ones: a site's lead word
// ("Watch"), a release tag after the name ("Uncut"), or the release's own
// group. A word in between, or any other word beside the name, makes it
// another title ("Copper Valley Chronicles", "Copper Valley UK"). A name of
// one word is never loosened, and a title whose name or extra words belong
// to another library title (others) is refused. "&", "and" and a stray "amp"
// (left by an escaped "&amp;") are left out on both sides, because release
// names drop or spell them in any of these ways. A title with no extra words
// is not loose.
func looseShowName(title, group string, t mediafiles.TitleRef, others []mediafiles.TitleRef) bool {
	have := looseWords(title)
	grp := looseWords(group)
	for _, n := range append([]string{t.Name}, t.Aliases...) {
		for _, form := range releasename.NameForms(n) {
			want := looseWords(form)
			if len(want) < 2 {
				continue
			}
			for i := 0; i+len(want) <= len(have); i++ {
				if !slices.Equal(have[i:i+len(want)], want) {
					continue
				}
				lead, tail := have[:i], have[i+len(want):]
				if len(lead)+len(tail) == 0 || !safeEdge(lead, looseLead, grp) || !safeEdge(tail, looseTail, grp) {
					continue
				}
				if !otherTitle(have, lead, tail, others) {
					return true
				}
			}
		}
	}
	return false
}

// safeEdge says whether every word of an edge is a safe word or part of the
// release's group.
func safeEdge(words []string, safe map[string]bool, group []string) bool {
	if len(words) > 0 && slices.Equal(words, group) {
		return true
	}
	return !slices.ContainsFunc(words, func(w string) bool { return !safe[w] })
}

// otherTitle says whether the whole release title, or one of its edges, is
// the name of another library title.
func otherTitle(have, lead, tail []string, others []mediafiles.TitleRef) bool {
	for _, o := range others {
		for _, n := range append([]string{o.Name}, o.Aliases...) {
			for _, form := range releasename.NameForms(n) {
				w := looseWords(form)
				if len(w) > 0 && (slices.Equal(w, have) || slices.Equal(w, lead) || slices.Equal(w, tail)) {
					return true
				}
			}
		}
	}
	return false
}

// looseWords are the clean words of a name with "and" and "amp" left out.
func looseWords(name string) []string {
	return slices.DeleteFunc(strings.Fields(releasename.CleanName(name)), func(w string) bool { return w == "and" || w == "amp" })
}

// otherTitleRefs are the other active Titles of the library, so a loose name
// is not read as one of them. A failed read returns none.
func (s *Searcher) otherTitleRefs(ctx context.Context, id int64) []mediafiles.TitleRef {
	ts, err := database.Select[mediafiles.Title](ctx, s.Store.Read(), "titles", "state = 'active' AND id != ?", id)
	if err != nil {
		return nil
	}
	return titleRefs(ts)
}
