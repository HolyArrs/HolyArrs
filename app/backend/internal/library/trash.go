package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TrashResult says what applying a TRaSH Guides profile did.
type TrashResult struct {
	PickRules mediafiles.PickRules `json:"pick_rules"`
	Added     int                  `json:"added"`   // synced traits made
	Updated   int                  `json:"updated"` // synced traits already there, refreshed
	Scored    []TrashScored        `json:"scored"`
	Skipped   []TrashSkipped       `json:"skipped"`
}

// TrashScored is one synced trait and the points it now gives.
type TrashScored struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Points int    `json:"points"`
}

// ErrTrashProfile says the TRaSH profile asked for is not there.
var ErrTrashProfile = errors.New("no such TRaSH Guides profile")

// pickMaxTraits matches the Pick rules limit on listed traits.
const pickMaxTraits = 64

// ApplyTrash applies TRaSH profile profileID (of app sonarr or radarr) to
// Pick rules id: each format it can test becomes a synced trait (made once,
// updated in place on later syncs), the rules' points list gets the
// profile's scores in place of the ones a previous sync put there, and the
// minimum, good enough and upgrade points follow the profile. Points for
// your own traits and built-in traits stay as they are.
func ApplyTrash(ctx context.Context, st *database.Store, c *TrashGuides, id int64, app, profileID string, by domain.Actor) (TrashResult, error) {
	var res TrashResult
	d, err := c.Load(ctx, app)
	if err != nil {
		return res, err
	}
	prof, ok := d.Profile(profileID)
	if !ok {
		return res, ErrTrashProfile
	}
	plan := d.Build(prof)
	res.Skipped = plan.Skipped

	var p mediafiles.PickRules
	if err := database.Get(ctx, st.Read(), "pick_rules", id, &p); err != nil {
		return res, err
	}
	if mediafiles.IsMusicRules(p.Body) {
		return res, &mediafiles.FieldError{Field: "body", Text: "TRaSH Guides profiles are for shows and movies, not music."}
	}

	keys := make([]string, len(plan.Traits))
	err = st.Write(ctx, func(tx *sql.Tx) error {
		res.Added, res.Updated = 0, 0
		for i, pl := range plan.Traits {
			t := pl.Trait
			var old mediafiles.OwnTrait
			rows, err := database.Select[mediafiles.OwnTrait](ctx, tx, "own_traits", "source = ? AND source_id = ?", t.Source, t.SourceID)
			if err != nil {
				return err
			}
			if len(rows) > 0 {
				old = rows[0]
			}
			name, err := freeTraitName(tx, t.Name, app, old.ID)
			if err != nil {
				return err
			}
			if old.ID != 0 {
				old.Name, old.Match, old.Terms, old.Specs = name, t.Match, t.Terms, t.Specs
				if err := database.Update(tx, "own_traits", &old, by); err != nil {
					return err
				}
				keys[i] = mediafiles.OwnTraitKey(old.ID)
				res.Updated++
				continue
			}
			t.Name = name
			nid, err := database.Insert(tx, "own_traits", &t, by)
			if err != nil {
				return err
			}
			keys[i] = mediafiles.OwnTraitKey(nid)
			res.Added++
		}
		return nil
	})
	if err != nil {
		return res, err
	}
	reload(ctx, st)

	// The points list: keep what is not from TRaSH, put the profile's in.
	synced, err := syncedKeys(ctx, st)
	if err != nil {
		return res, err
	}
	b := p.Body
	kept := slices.DeleteFunc(slices.Clone(b.Traits), func(tp mediafiles.TraitPoints) bool { return synced[tp.Trait] })
	var add []mediafiles.TraitPoints
	for i, pl := range plan.Traits {
		add = append(add, mediafiles.TraitPoints{Trait: keys[i], Points: pl.Points})
	}
	if room := pickMaxTraits - len(kept); len(add) > room {
		// Keep the strongest scores; the rest are reported.
		sort.SliceStable(add, func(i, j int) bool { return abs(add[i].Points) > abs(add[j].Points) })
		for _, tp := range add[max(room, 0):] {
			res.Skipped = append(res.Skipped, TrashSkipped{Name: traitName(plan, keys, tp.Trait), Reason: "Pick rules hold at most 64 traits"})
		}
		add = add[:max(room, 0)]
	}
	b.Traits = append(kept, add...)
	for _, tp := range add {
		res.Scored = append(res.Scored, TrashScored{Key: tp.Trait, Name: traitName(plan, keys, tp.Trait), Points: tp.Points})
	}
	clamp := func(n int) int { return max(-TrashMaxPoints, min(TrashMaxPoints, n)) }
	b.MinPoints = clamp(prof.MinScore)
	g := clamp(prof.CutoffScore)
	b.GoodEnoughPoints = &g
	b.UpgradeMinPoints = clamp(prof.MinUpgradeScore)
	b.Synced = &mediafiles.SyncedProfile{From: "trash", App: app, ProfileID: prof.ID, Name: prof.Name, At: time.Now().UTC().Format(time.RFC3339)}
	res.PickRules, err = UpdatePickRules(ctx, st, id, PickRulesInput{Name: p.Name, Kinds: p.Kinds, Body: b}, by)
	return res, err
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func traitName(plan TrashPlan, keys []string, key string) string {
	if i := slices.Index(keys, key); i >= 0 {
		return plan.Traits[i].Trait.Name
	}
	return key
}

// syncedKeys lists the Pick rules keys of every synced trait.
func syncedKeys(ctx context.Context, st *database.Store) (map[string]bool, error) {
	ts, err := database.Select[mediafiles.OwnTrait](ctx, st.Read(), "own_traits", "source != ''")
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(ts))
	for _, t := range ts {
		m[mediafiles.OwnTraitKey(t.ID)] = true
	}
	return m, nil
}

// freeTraitName finds a name no other trait has: the format's own name,
// then with "(shows)" or "(movies)", then numbered.
func freeTraitName(tx *sql.Tx, name, app string, self int64) (string, error) {
	word := "shows"
	if app == "radarr" {
		word = "movies"
	}
	try := []string{name, name + " (" + word + ")"}
	for n := 2; n < 20; n++ {
		try = append(try, fmt.Sprintf("%s (%s %d)", name, word, n))
	}
	for _, t := range try {
		if utf8.RuneCountInString(t) > 60 {
			t = string([]rune(t)[:60])
		}
		t = strings.TrimSpace(t)
		err := ownNameTaken(tx, t, self)
		if err == nil {
			return t, nil
		}
		if !errors.Is(err, ErrOwnTraitName) {
			return "", err
		}
	}
	return "", ErrOwnTraitName
}
