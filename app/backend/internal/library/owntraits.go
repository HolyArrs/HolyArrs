package library

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"slices"
	"sync"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// ErrOwnTraitName says another own trait already has this name.
var ErrOwnTraitName = errors.New("another trait already has this name")

// OwnTraitInput is a new or changed own trait.
type OwnTraitInput struct {
	Name  string   `json:"name"`
	Match string   `json:"match"`
	Terms []string `json:"terms"`
}

// OwnTraits lists every own trait by name.
func OwnTraits(ctx context.Context, st *database.Store) ([]mediafiles.OwnTrait, error) {
	return database.Select[mediafiles.OwnTrait](ctx, st.Read(), "own_traits", "1=1 ORDER BY lower(name), id")
}

// loadMu keeps two reloads from crossing: a slow one that read the table
// first must not replace what a later one stored.
var loadMu sync.Mutex

// LoadOwnTraits hands the stored own traits to the judge; the app calls it
// at start, and every change below calls it after saving.
func LoadOwnTraits(ctx context.Context, st *database.Store) error {
	loadMu.Lock()
	defer loadMu.Unlock()
	ts, err := OwnTraits(ctx, st)
	if err != nil {
		return err
	}
	mediafiles.SetOwnTraits(ts)
	return nil
}

// reload is LoadOwnTraits after a saved change: the change stands either
// way, so a failure is logged and tried once more instead of answered.
func reload(ctx context.Context, st *database.Store) {
	if err := LoadOwnTraits(ctx, st); err != nil {
		if err = LoadOwnTraits(context.WithoutCancel(ctx), st); err != nil {
			log.Printf("own traits: the judge keeps the old list until the next change or start: %v", err)
		}
	}
}

func ownNameTaken(tx *sql.Tx, name string, not int64) error {
	var n int
	if err := tx.QueryRow("SELECT count(*) FROM own_traits WHERE lower(name) = lower(?) AND id != ?", name, not).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrOwnTraitName
	}
	return nil
}

// CreateOwnTrait checks and stores a new own trait.
func CreateOwnTrait(ctx context.Context, st *database.Store, in OwnTraitInput, by domain.Actor) (mediafiles.OwnTrait, error) {
	t := mediafiles.OwnTrait{Name: in.Name, Match: in.Match, Terms: in.Terms}
	if err := mediafiles.CheckOwnTrait(&t); err != nil {
		return t, err
	}
	err := st.Write(ctx, func(tx *sql.Tx) error {
		if err := ownNameTaken(tx, t.Name, 0); err != nil {
			return err
		}
		_, err := database.Insert(tx, "own_traits", &t, by)
		return err
	})
	if err != nil {
		return t, err
	}
	reload(ctx, st)
	return t, nil
}

// UpdateOwnTrait changes an own trait; Pick rules that use it judge by the
// new terms from then on.
func UpdateOwnTrait(ctx context.Context, st *database.Store, id int64, in OwnTraitInput, by domain.Actor) (mediafiles.OwnTrait, error) {
	next := mediafiles.OwnTrait{Name: in.Name, Match: in.Match, Terms: in.Terms}
	if err := mediafiles.CheckOwnTrait(&next); err != nil {
		return next, err
	}
	var t mediafiles.OwnTrait
	err := st.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "own_traits", id, &t); err != nil {
			return err
		}
		if err := ownNameTaken(tx, next.Name, id); err != nil {
			return err
		}
		t.Name, t.Match, t.Terms = next.Name, next.Match, next.Terms
		return database.Update(tx, "own_traits", &t, by)
	})
	if err != nil {
		return t, err
	}
	reload(ctx, st)
	return t, nil
}

// DeleteOwnTrait removes an own trait and takes it out of every Pick rule's
// points, must-have and never lists (a must-have nobody can meet would
// otherwise turn every Release down). It answers how many Pick rules changed.
func DeleteOwnTrait(ctx context.Context, st *database.Store, id int64, by domain.Actor) (int, error) {
	key := mediafiles.OwnTraitKey(id)
	changed := 0
	err := st.Write(ctx, func(tx *sql.Tx) error {
		var t mediafiles.OwnTrait
		if err := database.Get(ctx, tx, "own_traits", id, &t); err != nil {
			return err
		}
		ps, err := database.Select[mediafiles.PickRules](ctx, tx, "pick_rules", "body LIKE ?", "%\""+key+"\"%")
		if err != nil {
			return err
		}
		for _, p := range ps {
			b := p.Body
			n := len(b.Traits) + len(b.MustHave) + len(b.Never)
			b.Traits = slices.DeleteFunc(b.Traits, func(tp mediafiles.TraitPoints) bool { return tp.Trait == key })
			b.MustHave = slices.DeleteFunc(b.MustHave, func(s string) bool { return s == key })
			b.Never = slices.DeleteFunc(b.Never, func(s string) bool { return s == key })
			if len(b.Traits)+len(b.MustHave)+len(b.Never) == n {
				continue
			}
			p.Body = b
			p.Revision++
			if err := database.Update(tx, "pick_rules", &p, by); err != nil {
				return err
			}
			changed++
		}
		_, err = tx.Exec("DELETE FROM own_traits WHERE id = ?", id)
		return err
	})
	if err != nil {
		return 0, err
	}
	reload(ctx, st)
	return changed, nil
}

// checkOwnKeys refuses Pick rules naming an own trait that does not exist,
// on the field of the list that holds it.
func checkOwnKeys(tx *sql.Tx, b mediafiles.PickRulesBody) error {
	traits := make([]string, 0, len(b.Traits))
	for _, tp := range b.Traits {
		traits = append(traits, tp.Trait)
	}
	for _, l := range []struct {
		field string
		keys  []string
	}{{"body.traits", traits}, {"body.must_have", b.MustHave}, {"body.never", b.Never}} {
		for _, k := range l.keys {
			id, ok := mediafiles.OwnTraitID(k)
			if !ok {
				continue
			}
			var n int
			if err := tx.QueryRow("SELECT count(*) FROM own_traits WHERE id = ?", id).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				return &mediafiles.FieldError{Field: l.field, Text: "A trait of yours that was removed (" + k + ") is still listed; take it out."}
			}
		}
	}
	return nil
}
