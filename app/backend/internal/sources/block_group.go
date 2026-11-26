package sources

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// ErrNoGroup: the Release name shows no release group to block.
var ErrNoGroup = errors.New("this release name shows no release group to block")

// ReleaseGroup is the release group read from a Release name, or "".
func ReleaseGroup(rel domain.Release) string {
	music := rel.Parsed != nil && rel.Parsed["music"] != nil
	p := releasename.Parse(rel.Name, releasename.Hints{Music: music})
	if p.Group.Conf == releasename.Unknown {
		return ""
	}
	return strings.TrimSpace(p.Group.Value)
}

// BlockGroup adds the Release's group to the blocked groups of the Pick
// rules its Slots use (only slotID's when above 0), and returns the group.
func (s *Searcher) BlockGroup(ctx context.Context, releaseID, slotID int64, by domain.Actor) (string, error) {
	var rel domain.Release
	if err := database.Get(ctx, s.Store.Read(), "releases", releaseID, &rel); err != nil {
		return "", ErrNoRelease
	}
	group := ReleaseGroup(rel)
	if group == "" {
		return "", ErrNoGroup
	}
	slots := rel.MatchedSlotIDs
	if slotID > 0 {
		slots = []int64{slotID}
	}
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		var done []int64
		for _, sid := range slots {
			var sl mediafiles.Slot
			var ver mediafiles.Version
			if mediafiles.LoadSlot(ctx, tx, sid, &sl) != nil || mediafiles.LoadVersion(ctx, tx, sl.VersionID, &ver) != nil {
				continue
			}
			if slices.Contains(done, ver.PickRulesID) {
				continue
			}
			done = append(done, ver.PickRulesID)
			var pr mediafiles.PickRules
			if err := mediafiles.LoadPickRules(ctx, tx, ver.PickRulesID, &pr); err != nil {
				continue
			}
			list, added := mediafiles.AddBlockedGroup(pr.Body.BlockedGroups, group)
			if !added {
				continue
			}
			pr.Body.BlockedGroups = list
			pr.Revision++
			if err := database.Update(tx, "pick_rules", &pr, by); err != nil {
				return err
			}
		}
		return nil
	})
	return group, err
}
