// Package lists keeps Outside lists: TMDB and Trakt lists whose movies or
// shows are added to the Library on a schedule (model.md 2.25). Titles on
// the Never-add list are skipped, and every added title has why_added = list.
package lists

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"slices"
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
	ErrTitleKind  = errors.New("title_kind must be movie or show")
	ErrSeasons    = errors.New("seasons must be all, future, latest, first or none, and new_seasons all or none")
	ErrRulesKind  = errors.New("those Pick rules are not for this kind of list: pick rules made for its movies or shows")
	ErrNoTraktKey = errors.New("add your Trakt key (the client id of your Trakt app) in Outside lists settings first")
)

// Input is what a person sets on a list. Zero values take the defaults:
// a movie list, the first Library folder of its kind, its Pick rules, kept
// up to date, the Release gate default, no search on add, on, every 12
// hours. A show list keeps all seasons up to date, and new ones, unless
// told otherwise; the Release gate is for movies only.
type Input struct {
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	TitleKind       string `json:"title_kind"`
	Ref             string `json:"ref"`
	LibraryFolderID int64  `json:"library_folder_id"`
	PickRulesID     int64  `json:"pick_rules_id"`
	KeepUpToDate    *bool  `json:"keep_up_to_date"`
	SearchOnAdd     bool   `json:"search_on_add"`
	ReleaseGate     string `json:"release_gate"`
	Seasons         string `json:"seasons"`
	NewSeasons      string `json:"new_seasons"`
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

// SeasonsFirst keeps only the first season of a show from a list up to
// date; the other choices are the ones a show is added with by hand.
const SeasonsFirst = "first"

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
	// A change that leaves title_kind out keeps what the list adds.
	kind := in.TitleKind
	if kind == "" {
		kind = l.TitleKind
	}
	if kind == "" {
		kind = "movie"
	}
	seasons, newSeasons, gate := "", "", in.ReleaseGate
	switch kind {
	case "movie":
	case "show":
		seasons, newSeasons, gate = in.Seasons, in.NewSeasons, ""
		if seasons == "" {
			seasons = library.SeasonsAll
		}
		if newSeasons == "" || seasons == library.SeasonsFuture {
			newSeasons = "all" // future only keeps every season to come, as when adding by hand
		}
		if (!library.ValidSeasonChoice(seasons) && seasons != SeasonsFirst) || (newSeasons != "all" && newSeasons != "none") {
			return ErrSeasons
		}
	default:
		return ErrTitleKind
	}
	every := in.SyncEveryHours
	if every == 0 {
		every = 12
	}
	if every < 6 || every > 168 {
		return ErrEvery
	}
	if gate != "" && !library.ValidGate(gate) {
		return library.ErrBadGate
	}
	noFolder, wrongFolder := library.ErrNoFolder, library.ErrNotMovieFolder
	if kind == "show" {
		noFolder, wrongFolder = library.ErrNoShowFolder, library.ErrNotShowFolder
	}
	folder := in.LibraryFolderID
	if folder == 0 {
		if err := tx.QueryRow("SELECT id FROM library_folders WHERE kind = ? ORDER BY id LIMIT 1", kind).Scan(&folder); errors.Is(err, sql.ErrNoRows) {
			return noFolder
		} else if err != nil {
			return err
		}
	}
	var lf domain.LibraryFolder
	if err := store.Get(ctx, tx, "library_folders", folder, &lf); errors.Is(err, store.ErrNotFound) {
		return noFolder
	} else if err != nil {
		return err
	}
	if lf.Kind != kind {
		return wrongFolder
	}
	var rules *int64
	if in.PickRulesID != 0 {
		var pr domain.PickRules
		if err := store.Get(ctx, tx, "pick_rules", in.PickRulesID, &pr); errors.Is(err, store.ErrNotFound) {
			return library.ErrNoPickRules
		} else if err != nil {
			return err
		}
		if !slices.Contains(pr.Kinds, kind) {
			return ErrRulesKind
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
	l.Name, l.Kind, l.TitleKind, l.Ref, l.LibraryFolderID, l.PickRulesID = name, in.Kind, kind, ref, folder, rules
	l.KeepUpToDate = in.KeepUpToDate == nil || *in.KeepUpToDate
	l.Enabled = in.Enabled == nil || *in.Enabled
	l.SearchOnAdd, l.ReleaseGate, l.SyncEveryHours = in.SearchOnAdd, gate, every
	l.Seasons, l.NewSeasons = seasons, newSeasons
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

// Edit changes a list. A new reference or kind (of list or of title)
// forgets the last sync.
func Edit(ctx context.Context, s *store.Store, id int64, in Input, by domain.Actor) (domain.OutsideList, error) {
	var l domain.OutsideList
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := store.Get(ctx, tx, "outside_lists", id, &l); err != nil {
			return err
		}
		kind, ref, titleKind := l.Kind, l.Ref, l.TitleKind
		if err := fill(ctx, tx, in, &l); err != nil {
			return err
		}
		if kind != l.Kind || ref != l.Ref || titleKind != l.TitleKind {
			l.LastSyncedAt, l.LastResult, l.LastError = nil, "", ""
		}
		return store.Update(tx, "outside_lists", &l, by)
	})
	return l, err
}

// Remove deletes a list. Titles it added stay in the Library.
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
		s += fmt.Sprintf(", %d without an id to look up", r.Skipped)
	}
	return s
}
