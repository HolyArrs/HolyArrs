package sources

import (
	"context"
	"fmt"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// episodeSet builds the show search for one episode Slot.
func (s *Searcher) episodeSet(ctx context.Context, c slotCtx, out bool) (showSet, *epSlot, error) {
	set, err := s.loadShow(ctx, c.ti, c.ver)
	if err != nil {
		return set, nil, err
	}
	set.single = true
	if set.window = s.packWindow(ctx, c.ti); set.window > 0 {
		if set.packed, err = packFetchingItems(ctx, s.Store.Read(), c.ti.ID); err != nil {
			return set, nil, err
		}
	}
	set.target[c.sl.ID] = true
	if !out {
		set.notOut[c.sl.ID] = true
	}
	for i := range set.eps {
		if set.eps[i].sl != nil && set.eps[i].sl.ID == c.sl.ID {
			return set, &set.eps[i], nil
		}
	}
	return set, nil, fmt.Errorf("episode Slot %d is not in its show", c.sl.ID)
}

// feedShow judges a feed check's releases for one show, per Version. Each
// release is matched to the episodes it holds and chosen or held for every
// looking Slot it fills, best first. It returns how many were chosen, held
// and newly stored.
func (s *Searcher) feedShow(ctx context.Context, by domain.Actor, jobID *int64, cs []slotCtx, fs []found, storedID map[string]int64,
	keyOf func(found) string) (int, int, int, error) {
	chosen, held, stored := 0, 0, 0
	byVer := map[int64][]slotCtx{}
	var order []int64
	for _, c := range cs {
		if len(byVer[c.ver.ID]) == 0 {
			order = append(order, c.ver.ID)
		}
		byVer[c.ver.ID] = append(byVer[c.ver.ID], c)
	}
	rules := s.showRules(ctx)
	for _, vid := range order {
		group := byVer[vid]
		set, err := s.loadShow(ctx, group[0].ti, group[0].ver)
		if err != nil {
			return chosen, held, stored, err
		}
		if set.window = s.packWindow(ctx, group[0].ti); set.window > 0 {
			if set.packed, err = packFetchingItems(ctx, s.Store.Read(), group[0].ti.ID); err != nil {
				return chosen, held, stored, err
			}
		}
		kept := map[int64]bool{}
		for _, e := range set.eps {
			if e.sl != nil && e.kept {
				kept[e.sl.ID] = true
			}
		}
		heldNow := map[int64]int64{}
		cands := append([]found(nil), fs...)
		for _, c := range group {
			if !kept[c.sl.ID] {
				continue
			}
			set.target[c.sl.ID] = true
			if c.sl.State == mediafiles.SlotHeld && c.sl.HeldReleaseID != nil {
				heldNow[c.sl.ID] = *c.sl.HeldReleaseID
				dup := false
				for _, f := range cands {
					dup = dup || f.relID == *c.sl.HeldReleaseID
				}
				if f, ok := s.storedFound(ctx, *c.sl.HeldReleaseID); ok && !dup {
					cands = append(cands, f)
				}
			}
		}
		if len(set.target) == 0 {
			continue
		}
		for k := range cands {
			if id := storedID[keyOf(cands[k])]; id > 0 && cands[k].relID == 0 {
				cands[k].relID = id
			}
		}
		rels, vs, err := s.judgeShow(ctx, set, cands, false, false, rules)
		if err != nil {
			return chosen, held, stored, err
		}
		picks := s.pickShow(ctx, cands, vs, time.Now().UTC())
		first := int64(0)
		for id := range set.target {
			if first == 0 || id < first {
				first = id
			}
		}
		ids, err := s.storeAll(ctx, by, jobID, "feed", first, cands, rels, vs, false)
		if err != nil {
			return chosen, held, stored, err
		}
		for k := range cands {
			if cands[k].relID == 0 {
				storedID[keyOf(cands[k])] = ids[k]
				stored++
			}
		}
		keep := picks[:0]
		for _, pk := range picks {
			same := pk.held
			for _, id := range pk.slots {
				same = same && heldNow[id] == ids[pk.idx]
			}
			if !same {
				keep = append(keep, pk)
			}
		}
		if beforeSettle != nil {
			beforeSettle(first)
		}
		c1, h1, _ := s.settlePicks(ctx, by, jobID, set.ti.ID, set.labels(), cands, vs, ids, keep, "Feed check")
		chosen, held = chosen+c1, held+h1
	}
	return chosen, held, stored, nil
}
