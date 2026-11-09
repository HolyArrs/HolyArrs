package library

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

var (
	// ErrPickRulesInUse says Pick rules are still used, so they cannot be
	// removed, lose a kind something uses, or change ladder.
	ErrPickRulesInUse = errors.New("these Pick rules are in use")
	// ErrPresetLocked says a ready-made rule keeps its name and kinds and
	// cannot be removed; only its body may change.
	ErrPresetLocked = errors.New("ready-made Pick rules keep their name and kinds and cannot be removed")
	// ErrPickRulesName says another rule already has this name.
	ErrPickRulesName = errors.New("other Pick rules already have this name")
)

// PickRulesInput is a new or changed Pick rule.
type PickRulesInput struct {
	Name  string                   `json:"name"`
	Kinds []string                 `json:"kinds"`
	Body  mediafiles.PickRulesBody `json:"body"`
}

// Use counts what uses one Pick rule. Requests counts the ask settings and
// TitleDefaults the saved Movies and TV defaults.
type Use struct {
	Versions       int `json:"versions"`
	LibraryFolders int `json:"library_folders"`
	OutsideLists   int `json:"outside_lists"`
	Wishlist       int `json:"wishlist"`
	TitleDefaults  int `json:"title_defaults"`
}

// Total is every use added up.
func (u Use) Total() int {
	return u.Versions + u.LibraryFolders + u.OutsideLists + u.Wishlist + u.TitleDefaults
}

// askRuleKeys and defaultRuleKeys are the settings that name Pick rules, with the kind each is for.
var askRuleKeys = map[string]string{"ask_pick_rules_id": "movie", "ask_show_pick_rules_id": "show", "ask_music_pick_rules_id": "music"}
var defaultRuleKeys = map[string]string{"movie_pick_rules_id": "movie", "show_pick_rules_id": "show"}

// pickRulesUse counts the uses of id and the kinds they need.
func pickRulesUse(ctx context.Context, q Reader, id int64) (Use, map[string]bool, error) {
	var u Use
	kinds := map[string]bool{}
	rows, err := q.QueryContext(ctx, `SELECT t.kind, count(*) FROM versions v JOIN titles t ON t.id = v.title_id WHERE v.pick_rules_id = ? GROUP BY t.kind`, id)
	if err != nil {
		return u, nil, err
	}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			rows.Close()
			return u, nil, err
		}
		if k == "artist" {
			k = "music"
		}
		u.Versions += n
		kinds[k] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return u, nil, err
	}
	rows, err = q.QueryContext(ctx, `SELECT kind FROM library_folders WHERE default_pick_rules_id = ?`, id)
	if err != nil {
		return u, nil, err
	}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			rows.Close()
			return u, nil, err
		}
		u.LibraryFolders++
		kinds[k] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return u, nil, err
	}
	// Outside lists add movies or shows: each list counts with its own kind.
	rows, err = q.QueryContext(ctx, `SELECT title_kind FROM outside_lists WHERE pick_rules_id = ?`, id)
	if err != nil {
		return u, nil, err
	}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			rows.Close()
			return u, nil, err
		}
		u.OutsideLists++
		kinds[k] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return u, nil, err
	}
	rows, err = q.QueryContext(ctx, `SELECT grp, key FROM settings WHERE value = ? AND key IN
		('ask_pick_rules_id', 'ask_show_pick_rules_id', 'ask_music_pick_rules_id', 'movie_pick_rules_id', 'show_pick_rules_id')`, strconv.FormatInt(id, 10))
	if err != nil {
		return u, nil, err
	}
	for rows.Next() {
		var grp, k string
		if err := rows.Scan(&grp, &k); err != nil {
			rows.Close()
			return u, nil, err
		}
		if kind, ok := askRuleKeys[k]; ok && grp == "this_app" {
			u.Wishlist++
			kinds[kind] = true
		} else if kind, ok := defaultRuleKeys[k]; ok && grp == "what_to_pick" {
			u.TitleDefaults++
			kinds[kind] = true
		}
	}
	rows.Close()
	return u, kinds, rows.Err()
}

// PickRulesUse counts what uses one Pick rule.
func PickRulesUse(ctx context.Context, st *database.Store, id int64) (Use, error) {
	u, _, err := pickRulesUse(ctx, st.Read(), id)
	return u, err
}

func nameTaken(tx *sql.Tx, name string, not int64) error {
	var n int
	if err := tx.QueryRow("SELECT count(*) FROM pick_rules WHERE lower(name) = lower(?) AND id != ?", strings.TrimSpace(name), not).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrPickRulesName
	}
	return nil
}

// CreatePickRules checks and stores new Pick rules.
func CreatePickRules(ctx context.Context, st *database.Store, in PickRulesInput, by domain.Actor) (mediafiles.PickRules, error) {
	in.Name = strings.TrimSpace(in.Name)
	p := mediafiles.PickRules{Name: in.Name, Kinds: in.Kinds, Revision: 1, Body: in.Body}
	if err := mediafiles.CheckPickRules(in.Name, in.Kinds, in.Body); err != nil {
		return p, err
	}
	err := st.Write(ctx, func(tx *sql.Tx) error {
		if err := nameTaken(tx, in.Name, 0); err != nil {
			return err
		}
		if err := checkOwnKeys(tx, in.Body); err != nil {
			return err
		}
		_, err := database.Insert(tx, "pick_rules", &p, by)
		return err
	})
	return p, err
}

// UpdatePickRules changes Pick rules and bumps their revision. A ready-made
// rule keeps its name and kinds; a rule in use keeps every kind something
// uses and its ladder. then, when given, runs in the same transaction after
// the change.
func UpdatePickRules(ctx context.Context, st *database.Store, id int64, in PickRulesInput, by domain.Actor, then ...func(tx *sql.Tx) error) (mediafiles.PickRules, error) {
	in.Name = strings.TrimSpace(in.Name)
	var p mediafiles.PickRules
	if err := mediafiles.CheckPickRules(in.Name, in.Kinds, in.Body); err != nil {
		return p, err
	}
	err := st.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "pick_rules", id, &p); err != nil {
			return err
		}
		if p.Preset && (in.Name != p.Name || !sameKinds(in.Kinds, p.Kinds)) {
			return ErrPresetLocked
		}
		if err := nameTaken(tx, in.Name, id); err != nil {
			return err
		}
		if err := checkOwnKeys(tx, in.Body); err != nil {
			return err
		}
		u, used, err := pickRulesUse(ctx, tx, id)
		if err != nil {
			return err
		}
		if u.Total() > 0 {
			if mediafiles.IsMusicRules(p.Body) != mediafiles.IsMusicRules(in.Body) {
				return ErrPickRulesInUse
			}
			for k := range used {
				if !slices.Contains(in.Kinds, k) {
					return ErrPickRulesInUse
				}
			}
		}
		p.Name, p.Kinds, p.Body = in.Name, in.Kinds, in.Body
		p.Revision++
		if err := database.Update(tx, "pick_rules", &p, by); err != nil {
			return err
		}
		for _, f := range then {
			if err := f(tx); err != nil {
				return err
			}
		}
		return nil
	})
	return p, err
}

func sameKinds(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// DeletePickRules removes Pick rules nothing uses. Ready-made rules stay.
func DeletePickRules(ctx context.Context, st *database.Store, id int64, by domain.Actor) error {
	return st.Write(ctx, func(tx *sql.Tx) error {
		var p mediafiles.PickRules
		if err := database.Get(ctx, tx, "pick_rules", id, &p); err != nil {
			return err
		}
		if p.Preset {
			return ErrPresetLocked
		}
		u, _, err := pickRulesUse(ctx, tx, id)
		if err != nil {
			return err
		}
		if u.Total() > 0 {
			return ErrPickRulesInUse
		}
		_, err = tx.Exec("DELETE FROM pick_rules WHERE id = ?", id)
		return err
	})
}

// PickRulesUses counts what uses each Pick rule, for every rule at once: the
// same counts as PickRulesUse, from one grouped query per kind of use. A rule
// nothing uses has no entry.
func PickRulesUses(ctx context.Context, st *database.Store) (map[int64]Use, error) {
	q := st.Read()
	out := map[int64]Use{}
	count := func(query string, add func(u *Use, n int)) error {
		rows, err := q.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var n int
			if err := rows.Scan(&id, &n); err != nil {
				return err
			}
			u := out[id]
			add(&u, n)
			out[id] = u
		}
		return rows.Err()
	}
	if err := count(`SELECT v.pick_rules_id, count(*) FROM versions v JOIN titles t ON t.id = v.title_id GROUP BY v.pick_rules_id`, func(u *Use, n int) { u.Versions += n }); err != nil {
		return nil, err
	}
	if err := count(`SELECT default_pick_rules_id, count(*) FROM library_folders WHERE default_pick_rules_id IS NOT NULL GROUP BY default_pick_rules_id`,
		func(u *Use, n int) { u.LibraryFolders += n }); err != nil {
		return nil, err
	}
	if err := count(`SELECT pick_rules_id, count(*) FROM outside_lists WHERE pick_rules_id IS NOT NULL GROUP BY pick_rules_id`,
		func(u *Use, n int) { u.OutsideLists += n }); err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT grp, key, value FROM settings WHERE key IN
		('ask_pick_rules_id', 'ask_show_pick_rules_id', 'ask_music_pick_rules_id', 'movie_pick_rules_id', 'show_pick_rules_id')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var grp, k, v string
		if err := rows.Scan(&grp, &k, &v); err != nil {
			return nil, err
		}
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || strconv.FormatInt(id, 10) != v {
			continue // PickRulesUse matches the exact id text
		}
		u := out[id]
		if _, ok := askRuleKeys[k]; ok && grp == "this_app" {
			u.Wishlist++
		} else if _, ok := defaultRuleKeys[k]; ok && grp == "what_to_pick" {
			u.TitleDefaults++
		} else {
			continue
		}
		out[id] = u
	}
	return out, rows.Err()
}
