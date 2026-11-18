package sources

import (
	"context"
	"slices"
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
// still be forced onto, and the show's name. It applies only to a year
// miss: the Release was found by a season search of a show, its name is
// one of that show's and only its year kept it from matching. The Slots
// are those of the episodes its numbers cover, in the season and Version
// searched; a Release naming another season gets none. skipped says which
// episodes were left out for a protected copy or a copy as good; wider is
// set when the Release holds the season searched and another one. Anything
// unclear returns none.
func (s *Searcher) showSearchSlots(ctx context.Context, rel domain.Release) (out []int64, skipped []string, show string, wider bool) {
	if rel.JobID == nil {
		return nil, nil, "", false
	}
	db := s.Store.Read()
	var job domain.Job
	if err := database.Get(ctx, db, "jobs", *rel.JobID, &job); err != nil || job.Kind != jobs.KindSearchSeason || job.Subject == nil || job.Subject.Type != "part" {
		return nil, nil, "", false
	}
	vid, _ := job.Params["version_id"].(float64)
	var part mediafiles.Part
	var ti mediafiles.Title
	var ver mediafiles.Version
	if mediafiles.LoadPart(ctx, db, job.Subject.ID, &part) != nil || part.Number == nil || mediafiles.LoadTitle(ctx, db, part.TitleID, &ti) != nil ||
		mediafiles.LoadVersion(ctx, db, int64(vid), &ver) != nil || ver.TitleID != ti.ID {
		return nil, nil, "", false
	}
	set, err := s.loadShow(ctx, ti, ver)
	if err != nil {
		return nil, nil, "", false
	}
	p := releasename.Parse(rel.Name, releasename.Hints{Size: rel.Size, Show: true})
	lib := []mediafiles.TitleRef{set.ref()}
	if mediafiles.MatchRelease(p, lib).Title != nil || mediafiles.MatchTitle(p, lib).Title == nil {
		return nil, nil, "", false
	}
	refs := make([]mediafiles.EpisodeRef, len(set.eps))
	slots := map[int64]*mediafiles.Slot{}
	for i, e := range set.eps {
		refs[i] = e.ref
		slots[e.it.ID] = e.sl
	}
	held, why := mediafiles.MatchEpisodes(p, set.order(), refs)
	if why != "" {
		return nil, nil, "", false
	}
	jr := mediafiles.ReleaseFacts{Name: rel.Name, Parse: p, Size: rel.Size, Protocol: rel.Protocol}
	if slices.ContainsFunc(held, func(e mediafiles.EpisodeRef) bool { return e.Season != *part.Number }) {
		return nil, nil, "", slices.ContainsFunc(held, func(e mediafiles.EpisodeRef) bool { return e.Season == *part.Number })
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
	return out, skipped, ti.Name, false
}
