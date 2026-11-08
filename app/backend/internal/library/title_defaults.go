package library

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// ErrBadFolder says a Movies and TV default names a Library folder that is
// gone or holds another kind.
var ErrBadFolder = errors.New("that Library folder is gone or is not for this kind")

// KindDefaults are what a new movie or show gets when the add leaves a field
// out. 0 means the first folder of that kind and the folder's own Pick rules.
type KindDefaults struct {
	LibraryFolderID int64 `json:"library_folder_id"`
	PickRulesID     int64 `json:"pick_rules_id"`
	KeepUpToDate    bool  `json:"keep_up_to_date"`
	SearchOnAdd     bool  `json:"search_on_add"`
}

// TitleDefaults are the Movies and TV defaults.
type TitleDefaults struct {
	Movie KindDefaults `json:"movie"`
	Show  KindDefaults `json:"show"`
}

// BuiltInTitleDefaults are the values before anything is saved: keep up to
// date on for both, search when added on for movies and off for shows.
func BuiltInTitleDefaults() TitleDefaults {
	return TitleDefaults{Movie: KindDefaults{KeepUpToDate: true, SearchOnAdd: true}, Show: KindDefaults{KeepUpToDate: true}}
}

// TitleDefaultKeys are the settings keys (group what_to_pick).
var TitleDefaultKeys = []string{"movie_library_folder_id", "movie_pick_rules_id", "movie_keep_up_to_date", "movie_search_on_add",
	"show_library_folder_id", "show_pick_rules_id", "show_keep_up_to_date", "show_search_on_add"}

func (d *TitleDefaults) field(key string) (*KindDefaults, string) {
	if len(key) > 6 && key[:6] == "movie_" {
		return &d.Movie, key[6:]
	}
	return &d.Show, key[5:]
}

// ReadTitleDefaults reads the Movies and TV defaults and which keys were saved.
func ReadTitleDefaults(ctx context.Context, db database.Lister) (TitleDefaults, map[string]bool, error) {
	d := BuiltInTitleDefaults()
	saved := map[string]bool{}
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM settings WHERE grp = 'what_to_pick' AND key IN
		('movie_library_folder_id', 'movie_pick_rules_id', 'movie_keep_up_to_date', 'movie_search_on_add',
		'show_library_folder_id', 'show_pick_rules_id', 'show_keep_up_to_date', 'show_search_on_add')`)
	if err != nil {
		return d, saved, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return d, saved, err
		}
		kd, name := d.field(k)
		switch name {
		case "library_folder_id":
			kd.LibraryFolderID, _ = strconv.ParseInt(v, 10, 64)
		case "pick_rules_id":
			kd.PickRulesID, _ = strconv.ParseInt(v, 10, 64)
		case "keep_up_to_date":
			kd.KeepUpToDate = v == "true"
		case "search_on_add":
			kd.SearchOnAdd = v == "true"
		}
		saved[k] = true
	}
	return d, saved, rows.Err()
}

func checkKindDefaults(tx *sql.Tx, kind string, kd KindDefaults) error {
	if kd.LibraryFolderID != 0 {
		var k string
		err := tx.QueryRow("SELECT kind FROM library_folders WHERE id = ?", kd.LibraryFolderID).Scan(&k)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && k != kind) {
			return ErrBadFolder
		} else if err != nil {
			return err
		}
	}
	if kd.PickRulesID != 0 {
		id := kd.PickRulesID
		return PickRulesFor(tx, &id, kind)
	}
	return nil
}

// SetTitleDefaults checks and stores the Movies and TV defaults named in
// keys (TitleDefaultKeys; nil stores them all). A key left out stays as it
// is, unsaved when it was never saved, and a kind with no key is not checked.
func SetTitleDefaults(ctx context.Context, s *database.Store, d TitleDefaults, keys []string, by domain.Actor) error {
	if keys == nil {
		keys = TitleDefaultKeys
	}
	return s.Write(ctx, func(tx *sql.Tx) error {
		for _, kind := range []string{"movie", "show"} {
			if !slices.ContainsFunc(keys, func(k string) bool { return strings.HasPrefix(k, kind+"_") }) {
				continue
			}
			kd := d.Movie
			if kind == "show" {
				kd = d.Show
			}
			if err := checkKindDefaults(tx, kind, kd); err != nil {
				return err
			}
		}
		now := database.FormatTime(time.Now())
		for _, k := range keys {
			if !slices.Contains(TitleDefaultKeys, k) {
				continue
			}
			kd, name := d.field(k)
			v := map[string]string{"library_folder_id": strconv.FormatInt(kd.LibraryFolderID, 10), "pick_rules_id": strconv.FormatInt(kd.PickRulesID, 10),
				"keep_up_to_date": strconv.FormatBool(kd.KeepUpToDate), "search_on_add": strconv.FormatBool(kd.SearchOnAdd)}[name]
			_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('what_to_pick', ?, ?, ?, ?, ?, ?)
				ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				k, v, now, ActorText(by), now, ActorText(by))
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// UsableDefault says a saved folder or Pick rules id still exists and fits
// kind; a gone one falls back to the built-in choice (0).
func UsableDefault(ctx context.Context, db database.Querier, table, kind string, id int64) bool {
	if id == 0 {
		return false
	}
	var n int
	switch table {
	case "library_folders":
		_ = db.QueryRowContext(ctx, "SELECT count(*) FROM library_folders WHERE id = ? AND kind = ?", id, kind).Scan(&n)
	case "pick_rules":
		_ = db.QueryRowContext(ctx, `SELECT count(*) FROM pick_rules WHERE id = ? AND kinds NOT LIKE '%"music"%'`, id).Scan(&n)
	}
	return n == 1
}
