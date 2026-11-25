package library

import (
	"context"
	"errors"
	"path"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// ErrNoFilePoints says the Slot is not this title's or holds no file.
var ErrNoFilePoints = errors.New("no such Slot on this Title")

// SlotPoints is how the Pick rules score the file a Slot holds, and
// whether the Slot counts it good enough.
type SlotPoints struct {
	mediafiles.FilePoints
	GoodEnough bool `json:"good_enough"`
	// MeetsVersion is false when the file misses the Version's edition or
	// audio languages, which keeps it short of good enough on its own.
	MeetsVersion bool `json:"meets_version"`
	// Names names the own traits among Traits ("own:7": "Remux tier 1").
	Names map[string]string `json:"names"`
}

// SlotPoints scores the file a title's Slot holds.
func (s *Service) SlotPoints(ctx context.Context, titleID, slotID int64) (SlotPoints, error) {
	db := s.Store.Read()
	var sl mediafiles.Slot
	if err := database.Get(ctx, db, "slots", slotID, &sl); err != nil || sl.FileID == nil {
		return SlotPoints{}, ErrNoFilePoints
	}
	var it mediafiles.Item
	if err := database.Get(ctx, db, "items", sl.ItemID, &it); err != nil || it.TitleID != titleID {
		return SlotPoints{}, ErrNoFilePoints
	}
	var f mediafiles.File
	if err := mediafiles.LoadFile(ctx, db, *sl.FileID, &f); err != nil {
		return SlotPoints{}, ErrNoFilePoints
	}
	var v mediafiles.Version
	if err := database.Get(ctx, db, "versions", sl.VersionID, &v); err != nil {
		return SlotPoints{}, err
	}
	var pr mediafiles.PickRules
	if err := database.Get(ctx, db, "pick_rules", v.PickRulesID, &pr); err != nil {
		return SlotPoints{}, err
	}
	var t mediafiles.Title
	if err := database.Get(ctx, db, "titles", titleID, &t); err != nil {
		return SlotPoints{}, err
	}
	out := SlotPoints{
		FilePoints:   ScoreSlotFile(f, v, t, pr.Body),
		GoodEnough:   sl.GoodEnough,
		MeetsVersion: fileMeetsVersionChoices(v, f),
		Names:        map[string]string{},
	}
	for _, ts := range out.Traits {
		if id, ok := mediafiles.OwnTraitID(ts.Trait); ok {
			var ot mediafiles.OwnTrait
			if database.Get(ctx, db, "own_traits", id, &ot) == nil {
				out.Names[ts.Trait] = ot.Name
			}
		}
	}
	return out, nil
}

// ScoreSlotFile scores a file in a Version of Title t the way a search
// scores the copy it holds.
func ScoreSlotFile(f mediafiles.File, v mediafiles.Version, t mediafiles.Title, rules mediafiles.PickRulesBody) mediafiles.FilePoints {
	name := strings.TrimSuffix(path.Base(f.RelativePath), path.Ext(f.RelativePath))
	if f.ReleaseName != nil && *f.ReleaseName != "" {
		name = *f.ReleaseName
	}
	audio := mediafiles.FileLanguages(f.MediaInfo, v.AudioLanguages)
	return mediafiles.ScoreTitleFile(mediafiles.JudgeName(name, f.MediaInfo), f.Size, audio, t, rules)
}
