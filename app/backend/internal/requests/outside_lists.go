package requests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// Kinds of Outside list, and the Job kind that reads them.
const (
	ListTMDB        = "tmdb_list"
	ListTrakt       = "trakt_list"
	OutsideListSync = "sync_outside_lists"
)

// SeasonsFirst is the seasons choice that keeps only season one of a show.
// Library has no such choice, so lists add it on top of the library ones.
const SeasonsFirst = "first"

// Errors returned when a list is saved or read.
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

// Patterns for the list links and short forms people paste.
var (
	tmdbRef   = regexp.MustCompile(`^(?:https?://(?:www\.)?themoviedb\.org/list/)?(\d{1,12})(?:[-/?#].*)?$`)
	traktURL  = regexp.MustCompile(`^https?://(?:www\.|app\.)?trakt\.tv/users/([A-Za-z0-9._-]+)/(?:lists/([A-Za-z0-9._-]+)|(watchlist))/?(?:[?#].*)?$`)
	traktPair = regexp.MustCompile(`^([A-Za-z0-9._-]+)/([A-Za-z0-9._-]+)$`)
)

// OutsideListInput is what a person sets on a list. Zero values take defaults: a movie
// list, the first Library folder of its kind, no Pick rules, kept up to date,
// no search on add, enabled, synced every 12 hours. A show list takes all
// seasons and all new seasons. The Release gate applies to movies only.
type OutsideListInput struct {
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

// NormalListRef turns what a person pasted into the stored list reference. Trakt
// lists are refused while the Trakt lists feature is off.
func NormalListRef(kind, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	switch kind {
	case ListTMDB:
		if m := tmdbRef.FindStringSubmatch(ref); m != nil {
			return m[1], nil
		}
	case ListTrakt:
		if !startup.FeatureOn(startup.TraktLists) {
			return "", ErrKind
		}
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

// AddOutsideList saves a new Outside list. Each then function runs in the same
// transaction with the new id, so callers can save related rows atomically.
func AddOutsideList(ctx context.Context, s *database.Store, in OutsideListInput, by domain.Actor, then ...func(tx *sql.Tx, id int64) error) (domain.OutsideList, error) {
	var l domain.OutsideList
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := fill(ctx, tx, in, &l); err != nil {
			return err
		}
		id, err := database.Insert(tx, "outside_lists", &l, by)
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

// EditOutsideList changes a list. A new reference or kind clears the last sync, since
// the old result no longer describes what the list reads.
func EditOutsideList(ctx context.Context, s *database.Store, id int64, in OutsideListInput, by domain.Actor) (domain.OutsideList, error) {
	var l domain.OutsideList
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "outside_lists", id, &l); err != nil {
			return err
		}
		kind, ref, titleKind := l.Kind, l.Ref, l.TitleKind
		if err := fill(ctx, tx, in, &l); err != nil {
			return err
		}
		if kind != l.Kind || ref != l.Ref || titleKind != l.TitleKind {
			l.LastSyncedAt, l.LastResult, l.LastError = nil, "", ""
		}
		return database.Update(tx, "outside_lists", &l, by)
	})
	return l, err
}

// RemoveOutsideList deletes a list. Titles it added stay in the Library.
func RemoveOutsideList(ctx context.Context, s *database.Store, id int64) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec(removeOutsideListSQL, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return database.ErrNotFound
		}
		return nil
	})
}

// GetOutsideList returns one list by id.
func GetOutsideList(ctx context.Context, db database.Querier, id int64) (domain.OutsideList, error) {
	var l domain.OutsideList
	err := database.Get(ctx, db, "outside_lists", id, &l)
	return l, err
}

// AllOutsideLists returns every Outside list ordered by name, never nil.
func AllOutsideLists(ctx context.Context, db database.Lister) ([]domain.OutsideList, error) {
	out, err := database.Select[domain.OutsideList](ctx, db, "outside_lists", "1=1 ORDER BY name COLLATE NOCASE")
	if out == nil {
		out = []domain.OutsideList{}
	}
	return out, err
}

// fill checks an OutsideListInput and writes the result into l. It runs inside the
// caller's transaction so folder and name checks match what gets saved.
func fill(ctx context.Context, tx *sql.Tx, in OutsideListInput, l *domain.OutsideList) error {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 80 {
		return ErrName
	}
	ref, err := NormalListRef(in.Kind, in.Ref)
	if err != nil {
		return err
	}
	// An edit that leaves title_kind out keeps what the list already adds.
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
			newSeasons = "all" // "future" already means every new season, as when adding by hand
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
		if err := tx.QueryRow(fillSQL, kind).Scan(&folder); errors.Is(err, sql.ErrNoRows) {
			return noFolder
		} else if err != nil {
			return err
		}
	}
	var lf mediafiles.LibraryFolder
	if err := database.Get(ctx, tx, "library_folders", folder, &lf); errors.Is(err, database.ErrNotFound) {
		return noFolder
	} else if err != nil {
		return err
	}
	if lf.Kind != kind {
		return wrongFolder
	}
	var rules *int64
	if in.PickRulesID != 0 {
		var pr mediafiles.PickRules
		if err := database.Get(ctx, tx, "pick_rules", in.PickRulesID, &pr); errors.Is(err, database.ErrNotFound) {
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
	if err := tx.QueryRow(fillSQL2, name, l.ID).Scan(&taken); err != nil {
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

// due reports whether a list is enabled and its sync interval has passed.
func due(l domain.OutsideList, now time.Time) bool {
	return l.Enabled && (l.LastSyncedAt == nil || !now.Before(l.LastSyncedAt.Add(time.Duration(l.SyncEveryHours)*time.Hour)))
}

// describe words a sync result for the list's last_result field.
func describe(r OutsideListResult) string {
	s := fmt.Sprintf("%d on the list, %d added, %d already in the Library, %d on the Never-add list", r.Found, r.Added, r.Already, r.NeverAdd)
	if r.Failed > 0 {
		s += fmt.Sprintf(", %d could not be added", r.Failed)
	}
	if r.Skipped > 0 {
		s += fmt.Sprintf(", %d without an id to look up", r.Skipped)
	}
	return s
}
