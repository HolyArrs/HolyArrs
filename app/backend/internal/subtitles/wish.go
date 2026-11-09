package subtitles

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Hearing-impaired choices in a wish.
const (
	HINormal = "normal" // a normal subtitle; a hearing-impaired one also counts
	HIOnly   = "hi"     // only a hearing-impaired subtitle
	HIEither = "either" // either kind
)

// MaxWishLanguages caps the languages in one wish.
const MaxWishLanguages = 10

// Want is one language in a wish.
type Want struct {
	Language        string `json:"language"`
	HearingImpaired string `json:"hearing_impaired"`
	Forced          bool   `json:"forced"`
}

// Errors a wish can be refused with.
var (
	ErrMusic        = errors.New("music has no subtitles")
	ErrBadLanguage  = errors.New("unknown language")
	ErrTooManyWants = fmt.Errorf("a wish names at most %d languages", MaxWishLanguages)
	ErrNotFound     = errors.New("not found")
)

// CleanWants checks a wish and returns it in stored form: two-letter codes, a
// hearing-impaired choice and no language listed twice with the same forced
// switch. Nil becomes an empty list, meaning no subtitles wanted.
func CleanWants(ws []Want) ([]Want, error) {
	if len(ws) > MaxWishLanguages {
		return nil, ErrTooManyWants
	}
	out := []Want{}
	seen := map[string]bool{}
	for _, w := range ws {
		code := NormLanguage(w.Language)
		if code == "" {
			return nil, fmt.Errorf("%w: %q", ErrBadLanguage, w.Language)
		}
		switch w.HearingImpaired {
		case "":
			w.HearingImpaired = HINormal
		case HINormal, HIOnly, HIEither:
		default:
			return nil, fmt.Errorf("hearing_impaired must be %s, %s or %s", HINormal, HIOnly, HIEither)
		}
		key := fmt.Sprintf("%s/%v", code, w.Forced)
		if seen[key] {
			return nil, fmt.Errorf("%s (forced %v) is named twice", code, w.Forced)
		}
		seen[key] = true
		w.Language = code
		out = append(out, w)
	}
	return out, nil
}

// Wish is one stored wish row, for a title, one of its Versions or an episode.
type Wish struct {
	domain.Meta
	TitleID   int64  `db:"title_id" json:"title_id"`
	VersionID *int64 `db:"version_id" json:"version_id,omitempty"`
	ItemID    *int64 `db:"item_id" json:"item_id,omitempty"`
	Languages []Want `db:"languages,json" json:"languages"`
}

// Places where a resolved wish was found.
const (
	FromItem    = "episode"
	FromVersion = "version"
	FromTitle   = "title"
	FromDefault = "default"
)

// Resolved is the wish that applies to one Item in one Version.
type Resolved struct {
	Wants []Want `json:"languages"`
	From  string `json:"from"`
}

// titleKind reads a title's kind and refuses music.
func titleKind(ctx context.Context, db database.Querier, titleID int64) (string, error) {
	var kind string
	if err := db.QueryRowContext(ctx, "SELECT kind FROM titles WHERE id = ?", titleID).Scan(&kind); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	if kind == "artist" {
		return kind, ErrMusic
	}
	return kind, nil
}

// WishFor resolves the wish for an Item in a Version: the episode's own, else
// the Version's, else the title's, else the default.
func WishFor(ctx context.Context, db *sql.DB, titleID, versionID, itemID int64) (Resolved, error) {
	if _, err := titleKind(ctx, db, titleID); err != nil {
		return Resolved{}, err
	}
	rows, err := database.Select[Wish](ctx, db, "subtitle_wishes", "title_id = ? AND (item_id = ? OR version_id = ? OR (item_id IS NULL AND version_id IS NULL))",
		titleID, itemID, versionID)
	if err != nil {
		return Resolved{}, err
	}
	var item, ver, title *Wish
	for i := range rows {
		w := &rows[i]
		switch {
		case w.ItemID != nil:
			item = w
		case w.VersionID != nil:
			ver = w
		default:
			title = w
		}
	}
	for _, x := range []struct {
		w    *Wish
		from string
	}{{item, FromItem}, {ver, FromVersion}, {title, FromTitle}} {
		if x.w != nil {
			return Resolved{Wants: nonNil(x.w.Languages), From: x.from}, nil
		}
	}
	st := ReadSettings(ctx, db)
	return Resolved{Wants: nonNil(st.Languages), From: FromDefault}, nil
}

func nonNil(ws []Want) []Want {
	if ws == nil {
		return []Want{}
	}
	return ws
}

// Wishes lists a title's own wish rows for the title, its Versions and episodes.
func Wishes(ctx context.Context, db *sql.DB, titleID int64) ([]Wish, error) {
	if _, err := titleKind(ctx, db, titleID); err != nil {
		return nil, err
	}
	ws, err := database.Select[Wish](ctx, db, "subtitle_wishes", "title_id = ? ORDER BY item_id IS NOT NULL, version_id IS NOT NULL, id", titleID)
	if ws == nil {
		ws = []Wish{}
	}
	return ws, err
}

// SetWish stores the wish of a title (versionID and itemID 0), one of its
// Versions or one of its episodes. inherit deletes the row instead so the place
// inherits again. The Version or episode must belong to the title.
func SetWish(ctx context.Context, st *database.Store, titleID, versionID, itemID int64, wants []Want, inherit bool, by domain.Actor) error {
	if versionID != 0 && itemID != 0 {
		return errors.New("a wish is for a title, a Version or an episode, not two at once")
	}
	if _, err := titleKind(ctx, st.Read(), titleID); err != nil {
		return err
	}
	clean, err := CleanWants(wants)
	if err != nil && !inherit {
		return err
	}
	return st.Write(ctx, func(tx *sql.Tx) error {
		var vid, iid any
		if versionID != 0 {
			vid = versionID
			if err := owned(ctx, tx, "versions", versionID, titleID); err != nil {
				return err
			}
		}
		if itemID != 0 {
			iid = itemID
			var kind string
			if err := tx.QueryRowContext(ctx, "SELECT kind FROM items WHERE id = ? AND title_id = ?", itemID, titleID).Scan(&kind); err != nil {
				return fmt.Errorf("episode %d is not part of title %d: %w", itemID, titleID, ErrNotFound)
			}
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM subtitle_wishes WHERE title_id = ? AND version_id IS ? AND item_id IS ?", titleID, vid, iid); err != nil {
			return err
		}
		if inherit {
			return nil
		}
		w := &Wish{TitleID: titleID, Languages: clean}
		if versionID != 0 {
			w.VersionID = &versionID
		}
		if itemID != 0 {
			w.ItemID = &itemID
		}
		_, err := database.Insert(tx, "subtitle_wishes", w, by)
		return err
	})
}

func owned(ctx context.Context, tx *sql.Tx, table string, id, titleID int64) error {
	var n int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM "+table+" WHERE id = ? AND title_id = ?", id, titleID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%s %d is not part of title %d: %w", table[:len(table)-1], id, titleID, ErrNotFound)
	}
	return nil
}

// now is time.Now in UTC, swapped out in tests.
var now = func() time.Time { return time.Now().UTC() }

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
