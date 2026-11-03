// Package lists keeps Outside lists: TMDB and Trakt lists whose movies are
// added to the Library on a schedule (model.md 2.25). Movies on the
// Never-add list are skipped, and every added movie has why_added = list.
package lists

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Kinds of Outside list, and the Job kind that reads them.
const (
	KindTMDB  = "tmdb_list"
	KindTrakt = "trakt_list"
	KindSync  = "sync_outside_lists"
)

// Errors from saving a list.
var (
	ErrName       = errors.New("a list needs a name of at most 80 characters")
	ErrNameTaken  = errors.New("another Outside list has that name")
	ErrKind       = errors.New("kind must be tmdb_list or trakt_list")
	ErrRef        = errors.New("for TMDB give the list number or its link; for Trakt give user/list, user/watchlist or the link")
	ErrEvery      = errors.New("sync_every_hours must be between 6 and 168")
	ErrNoTraktKey = errors.New("add your Trakt key (the client id of your Trakt app) in Outside lists settings first")
)

// Input is what a person sets on a list. Zero values take the defaults:
// the first movie Library folder, its Pick rules, kept up to date, the
// Release gate default, no search on add, on, every 12 hours.
type Input struct {
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Ref             string `json:"ref"`
	LibraryFolderID int64  `json:"library_folder_id"`
	PickRulesID     int64  `json:"pick_rules_id"`
	KeepUpToDate    *bool  `json:"keep_up_to_date"`
	SearchOnAdd     bool   `json:"search_on_add"`
	ReleaseGate     string `json:"release_gate"`
	Enabled         *bool  `json:"enabled"`
	SyncEveryHours  int    `json:"sync_every_hours"`
}

var (
	tmdbRef   = regexp.MustCompile(`^(?:https?://(?:www\.)?themoviedb\.org/list/)?(\d{1,12})(?:[-/?#].*)?$`)
	traktURL  = regexp.MustCompile(`^https?://(?:www\.|app\.)?trakt\.tv/users/([A-Za-z0-9._-]+)/(?:lists/([A-Za-z0-9._-]+)|(watchlist))/?(?:[?#].*)?$`)
	traktPair = regexp.MustCompile(`^([A-Za-z0-9._-]+)/([A-Za-z0-9._-]+)$`)
)

// NormalRef turns what a person pasted into the stored list reference.
func NormalRef(kind, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	switch kind {
	case KindTMDB:
		if m := tmdbRef.FindStringSubmatch(ref); m != nil {
			return m[1], nil
		}
	case KindTrakt:
		if m := traktURL.FindStringSubmatch(ref); m != nil {
			return m[1] + "/" + m[2] + m[3], nil
		}
		if m := traktPair.FindStringSubmatch(ref); m != nil {
			return ref, nil
		}
	default:
		return "", ErrKind
	}
	return "", ErrRef
}

// fill checks an Input and turns it into a list row.
func fill(ctx context.Context, tx *sql.Tx, in Input, l *domain.OutsideList) error {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 80 {
		return ErrName
	}
	ref, err := NormalRef(in.Kind, in.Ref)
	if err != nil {
		return err
	}
	every := in.SyncEveryHours
	if every == 0 {
		every = 12
	}
	if every < 6 || every > 168 {
		return ErrEvery
	}
	if in.ReleaseGate != "" && !library.ValidGate(in.ReleaseGate) {
		return library.ErrBadGate
	}
	folder := in.LibraryFolderID
	if folder == 0 {
		if err := tx.QueryRow("SELECT id FROM library_folders WHERE kind = 'movie' ORDER BY id LIMIT 1").Scan(&folder); errors.Is(err, sql.ErrNoRows) {
			return library.ErrNoFolder
		} else if err != nil {
			return err
		}
	}
	var lf domain.LibraryFolder
	if err := store.Get(ctx, tx, "library_folders", folder, &lf); errors.Is(err, store.ErrNotFound) {
		return library.ErrNoFolder
	} else if err != nil {
		return err
	}
	if lf.Kind != "movie" {
		return library.ErrNotMovieFolder
	}
	var rules *int64
	if in.PickRulesID != 0 {
		var n int
		if err := tx.QueryRow("SELECT count(*) FROM pick_rules WHERE id = ?", in.PickRulesID).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return library.ErrNoPickRules
		}
		id := in.PickRulesID
		rules = &id
	}
	var taken int
	if err := tx.QueryRow("SELECT count(*) FROM outside_lists WHERE name = ? COLLATE NOCASE AND id != ?", name, l.ID).Scan(&taken); err != nil {
		return err
	}
	if taken > 0 {
		return ErrNameTaken
	}
	l.Name, l.Kind, l.Ref, l.LibraryFolderID, l.PickRulesID = name, in.Kind, ref, folder, rules
	l.KeepUpToDate = in.KeepUpToDate == nil || *in.KeepUpToDate
	l.Enabled = in.Enabled == nil || *in.Enabled
	l.SearchOnAdd, l.ReleaseGate, l.SyncEveryHours = in.SearchOnAdd, in.ReleaseGate, every
	return nil
}

// Add saves a new Outside list. then, when given, runs in the same
// transaction with the new id.
func Add(ctx context.Context, s *store.Store, in Input, by domain.Actor, then ...func(tx *sql.Tx, id int64) error) (domain.OutsideList, error) {
	var l domain.OutsideList
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := fill(ctx, tx, in, &l); err != nil {
			return err
		}
		id, err := store.Insert(tx, "outside_lists", &l, by)
		if err != nil {
			return err
		}
		for _, f := range then {
			if err := f(tx, id); err != nil {
				return err
			}
		}
		return nil
	})
	return l, err
}

// Edit changes a list. A new reference or kind forgets the last sync.
func Edit(ctx context.Context, s *store.Store, id int64, in Input, by domain.Actor) (domain.OutsideList, error) {
	var l domain.OutsideList
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := store.Get(ctx, tx, "outside_lists", id, &l); err != nil {
			return err
		}
		kind, ref := l.Kind, l.Ref
		if err := fill(ctx, tx, in, &l); err != nil {
			return err
		}
		if kind != l.Kind || ref != l.Ref {
			l.LastSyncedAt, l.LastResult, l.LastError = nil, "", ""
		}
		return store.Update(tx, "outside_lists", &l, by)
	})
	return l, err
}

// Remove deletes a list. Movies it added stay in the Library.
func Remove(ctx context.Context, s *store.Store, id int64) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec("DELETE FROM outside_lists WHERE id = ?", id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return store.ErrNotFound
		}
		return nil
	})
}

// Get reads one list.
func Get(ctx context.Context, db store.Querier, id int64) (domain.OutsideList, error) {
	var l domain.OutsideList
	err := store.Get(ctx, db, "outside_lists", id, &l)
	return l, err
}

// All lists every Outside list by name.
func All(ctx context.Context, db store.Lister) ([]domain.OutsideList, error) {
	out, err := store.Select[domain.OutsideList](ctx, db, "outside_lists", "1=1 ORDER BY name COLLATE NOCASE")
	if out == nil {
		out = []domain.OutsideList{}
	}
	return out, err
}

// due reports whether a list should be read now.
func due(l domain.OutsideList, now time.Time) bool {
	return l.Enabled && (l.LastSyncedAt == nil || !now.Before(l.LastSyncedAt.Add(time.Duration(l.SyncEveryHours)*time.Hour)))
}

func describe(r Result) string {
	s := fmt.Sprintf("%d on the list, %d added, %d already in the Library, %d on the Never-add list", r.Found, r.Added, r.Already, r.NeverAdd)
	if r.Failed > 0 {
		s += fmt.Sprintf(", %d could not be added", r.Failed)
	}
	if r.Skipped > 0 {
		s += fmt.Sprintf(", %d without a TMDB or IMDb id", r.Skipped)
	}
	return s
}
