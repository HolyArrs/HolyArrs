package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// ImportedTrait is one old-app format brought as a trait, and its points.
type ImportedTrait struct {
	SourceID string               `json:"source_id"` // "<app>:<server>:<format id>"
	Name     string               `json:"name"`
	Specs    []mediafiles.OwnSpec `json:"specs"`
	Points   int                  `json:"points"`
}

// ImportRulesInput is an old app's quality profile as Pick rules: Body
// without its traits, and the formats it scores. ProfileKey names the
// profile across imports ("<app>:<server>:<profile id>").
type ImportRulesInput struct {
	App        string
	ProfileKey string
	Name       string
	Kinds      []string
	Body       mediafiles.PickRulesBody
	Traits     []ImportedTrait
}

// ImportRulesResult says what an import made: the Pick rules, whether they
// are new, and the traits it added (new ones only).
type ImportRulesResult struct {
	PickRules mediafiles.PickRules
	Created   bool
	NewTraits []int64
	Updated   int
}

// ImportPickRules stores an old app's profile as Pick rules. Each format
// becomes an imported trait, updated in place when the same format comes
// again (by its SourceID), and the Pick rules made by an earlier import of
// the same profile are updated (keeping their tags) instead of copied.
func ImportPickRules(ctx context.Context, st *database.Store, in ImportRulesInput, by domain.Actor) (ImportRulesResult, error) {
	var res ImportRulesResult
	keys := make([]string, len(in.Traits))
	err := st.Write(ctx, func(tx *sql.Tx) error {
		res.NewTraits, res.Updated = nil, 0
		for i, it := range in.Traits {
			rows, err := database.Select[mediafiles.OwnTrait](ctx, tx, "own_traits", "source = 'import' AND source_id = ?", it.SourceID)
			if err != nil {
				return err
			}
			var old mediafiles.OwnTrait
			if len(rows) > 0 {
				old = rows[0]
			}
			name, err := freeTraitName(tx, it.Name, in.App, old.ID)
			if err != nil {
				return err
			}
			if old.ID != 0 {
				old.Name, old.Match, old.Terms, old.Specs = name, "name", []string{}, it.Specs
				if err := database.Update(tx, "own_traits", &old, by); err != nil {
					return err
				}
				keys[i] = mediafiles.OwnTraitKey(old.ID)
				res.Updated++
				continue
			}
			t := mediafiles.OwnTrait{Name: name, Match: "name", Terms: []string{}, Source: "import", SourceID: it.SourceID, Specs: it.Specs}
			id, err := database.Insert(tx, "own_traits", &t, by)
			if err != nil {
				return err
			}
			keys[i] = mediafiles.OwnTraitKey(id)
			res.NewTraits = append(res.NewTraits, id)
		}
		return nil
	})
	if err != nil {
		return res, err
	}
	reload(ctx, st)

	b := in.Body
	b.Traits = make([]mediafiles.TraitPoints, 0, len(in.Traits))
	for i, it := range in.Traits {
		b.Traits = append(b.Traits, mediafiles.TraitPoints{Trait: keys[i], Points: it.Points})
	}
	b.Synced = &mediafiles.SyncedProfile{From: "import", App: in.App, ProfileID: in.ProfileKey, Name: in.Name, At: time.Now().UTC().Format(time.RFC3339)}
	var prev []mediafiles.PickRules
	prev, err = database.Select[mediafiles.PickRules](ctx, st.Read(), "pick_rules",
		"json_extract(body, '$.synced.from') = 'import' AND json_extract(body, '$.synced.profile_id') = ?", in.ProfileKey)
	if err != nil {
		return res, err
	}
	if len(prev) > 0 {
		p := prev[0]
		b.Tags = p.Body.Tags
		res.PickRules, err = UpdatePickRules(ctx, st, p.ID, PickRulesInput{Name: p.Name, Kinds: p.Kinds, Body: b}, by)
		return res, err
	}
	for n := 1; n < 20; n++ {
		name := in.Name
		switch {
		case n == 2:
			name = in.Name + " (" + in.App + ")"
		case n > 2:
			name = fmt.Sprintf("%s (%s %d)", in.Name, in.App, n-1)
		}
		res.PickRules, err = CreatePickRules(ctx, st, PickRulesInput{Name: name, Kinds: in.Kinds, Body: b}, by)
		if !errors.Is(err, ErrPickRulesName) {
			res.Created = err == nil
			return res, err
		}
	}
	return res, err
}
