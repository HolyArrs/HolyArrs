package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// ShowByIDs finds a show Title sharing any of ids; 0 when there is none.
func ShowByIDs(ctx context.Context, db database.Querier, ids map[string]string) (int64, error) {
	for _, k := range []string{"tvdb", "tmdb", "tvmaze", "imdb"} {
		v := ids[k]
		if v == "" {
			continue
		}
		var id int64
		err := db.QueryRowContext(ctx, "SELECT id FROM titles WHERE kind = 'show' AND state != 'removed' AND json_extract(source_ids, '$.'||?) = ?", k, v).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
	}
	return 0, nil
}

// ShowOnNeverAdd says whether a show with any of ids is on the Never-add list.
func ShowOnNeverAdd(ctx context.Context, db database.Querier, ids map[string]string) (bool, error) {
	return showOnNeverAdd(ctx, db, ids)
}

func showOnNeverAdd(ctx context.Context, db database.Querier, ids map[string]string) (bool, error) {
	for _, k := range []string{"tvdb", "tmdb", "tvmaze", "imdb"} {
		if ids[k] == "" {
			continue
		}
		var n int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM never_add WHERE kind = 'show' AND json_extract(source_ids, '$.'||?) = ?", k, ids[k]).Scan(&n); err != nil || n > 0 {
			return n > 0, err
		}
	}
	return false, nil
}

// stepSearchWanted is the chain step that searches everything one title
// still wants (its kept episodes, seasons or albums), the way the missing
// search would, now.
const stepSearchWanted = "search_wanted"

// ScanThenSearch scans the title's folder, then searches all it still
// wants. An approved request for a show or an artist uses it, so what was
// asked for is looked for at once, not at the next missing search.
func (s *Service) ScanThenSearch(ctx context.Context, titleID int64, by domain.Actor) error {
	_, _, err := s.Queue.Enqueue(ctx, KindScanTitle, &domain.Subject{Type: "title", ID: titleID},
		map[string]any{"then": []any{stepSearchWanted}}, "scan_then_search:"+strconv.FormatInt(titleID, 10), by)
	return err
}

// searchWanted queues the missing search for this one title.
func (s *Service) searchWanted(ctx context.Context, titleID int64, by domain.Actor) error {
	_, _, err := s.Queue.Enqueue(ctx, jobs.KindSearchMissing, &domain.Subject{Type: "title", ID: titleID},
		map[string]any{"title_id": titleID}, "search_wanted:"+strconv.FormatInt(titleID, 10), by)
	return err
}

func Deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// mountWarning says a Library folder looks like a dropped mount: it is
// empty while the Library knows files in it. Scanning it would find
// nothing to adopt.
func MountWarning(ctx context.Context, db Reader, lf mediafiles.LibraryFolder, entries int) string {
	if entries > 0 {
		return ""
	}
	var n int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM files WHERE state = 'present' AND version_id IN (SELECT id FROM versions WHERE library_folder_id = ?)`, lf.ID).Scan(&n)
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("the Library folder is empty while the Library knows %d files in it; is a drive or network share not mounted?", n)
}

// folderTaken says whether a folder of a Library folder already belongs to
// a title in the Library, comparing names as its filesystem does.
func FolderTaken(ctx context.Context, db Reader, lf mediafiles.LibraryFolder, folder string) (bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT folder_name FROM versions WHERE library_folder_id = ? AND lower(folder_name) = lower(?)
		AND title_id IN (SELECT id FROM titles WHERE state != 'removed')`, lf.ID, folder)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	fold := FoldsCase(lf.Path)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if sameFolderName(name, folder, fold) {
			return true, nil
		}
	}
	return false, rows.Err()
}

// episodeLabel is "S01E02" for a Slot's episode.
func EpisodeLabel(ctx context.Context, db database.Querier, slotID int64) string {
	var se, ep sql.NullInt64
	_ = db.QueryRowContext(ctx, `SELECT json_extract(i.numbers, '$.season'), json_extract(i.numbers, '$.episode') FROM items i
		JOIN slots s ON s.item_id = i.id WHERE s.id = ?`, slotID).Scan(&se, &ep)
	return fmt.Sprintf("S%02dE%02d", se.Int64, ep.Int64)
}

// ValidOrder reports whether o is an episode order HolyArrs follows.
func ValidOrder(o string) bool { return o == OrderAired || o == OrderDaily || o == OrderAbsolute }

// ShowDefaults are the show settings (What to pick).
type ShowDefaults struct {
	SeasonChoice string `json:"season_choice"` // all, future, latest, none
	NewSeasons   string `json:"new_seasons"`   // all, none
	EpisodeOrder string `json:"episode_order"` // aired, daily, absolute
	Source       string `json:"details_source"`
	KeepSpecials bool   `json:"keep_specials"`
	WaitMinutes  int    `json:"episode_wait_minutes"` // after the air time before an episode is wanted
}

// ReadShowDefaults reads the show settings; unset or odd values take the
// recommended defaults (all seasons, new seasons kept, aired order, TMDB,
// specials off, no wait).
func ReadShowDefaults(ctx context.Context, db database.Lister) (ShowDefaults, error) {
	d := ShowDefaults{SeasonChoice: SeasonsAll, NewSeasons: "all", EpisodeOrder: OrderAired, Source: "tmdb"}
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM settings WHERE grp = 'what_to_pick' AND key IN
		('show_season_choice', 'show_new_seasons', 'show_episode_order', 'show_details_source', 'keep_specials', 'episode_wait_minutes')`)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return d, err
		}
		switch k {
		case "show_season_choice":
			if ValidSeasonChoice(v) {
				d.SeasonChoice = v
			}
		case "show_new_seasons":
			if v == "all" || v == "none" {
				d.NewSeasons = v
			}
		case "show_episode_order":
			if ValidOrder(v) {
				d.EpisodeOrder = v
			}
		case "show_details_source":
			if metadata.ValidShowSource(v) {
				d.Source = v
			}
		case "keep_specials":
			d.KeepSpecials = v == "true"
		case "episode_wait_minutes":
			if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 10080 {
				d.WaitMinutes = n
			}
		}
	}
	return d, rows.Err()
}

// EpisodeGate says whether an episode that airs at air is out at now,
// counting wait after the air time, and why in one phrase. An episode with
// no air date is never out.
func EpisodeGate(air *time.Time, wait time.Duration, now time.Time) (open bool, reason string) {
	if air == nil {
		return false, "no air date yet"
	}
	from := air.Add(wait)
	if now.Before(from) {
		if wait > 0 {
			return false, fmt.Sprintf("airs %s UTC; wanted from %s UTC", air.UTC().Format("2006-01-02 15:04"), from.UTC().Format("2006-01-02 15:04"))
		}
		return false, "airs " + air.UTC().Format("2006-01-02 15:04") + " UTC"
	}
	return true, "aired " + air.UTC().Format("2006-01-02 15:04") + " UTC"
}

func EqStr(a, b *string) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

func EqInt(a, b *int) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

func EqTime(a, b *time.Time) bool {
	return (a == nil) == (b == nil) && (a == nil || a.Equal(*b))
}

// Season choices: which seasons are kept up to date when a show is added.
const (
	SeasonsAll    = "all"
	SeasonsFuture = "future"
	SeasonsLatest = "latest"
	SeasonsNone   = "none"
)

// Episode orders a show can follow.
const (
	OrderAired    = "aired"
	OrderDaily    = "daily"
	OrderAbsolute = "absolute"
)

// Errors about shows.
var (
	ErrBadSeasonChoice = errors.New("seasons must be all, future, latest or none")
	ErrBadNewSeasons   = errors.New("new_seasons must be all or none")
	ErrBadOrder        = errors.New("episode_order must be aired, daily or absolute")
	ErrBadWait         = errors.New("episode_wait_minutes must be between 0 and 10080")
	ErrNotShow         = errors.New("that Title is not a show")
	ErrShowOnly        = errors.New("that choice is for shows only")
	ErrMovieOnly       = errors.New("the Release gate is for movies; shows are searched once each episode has aired")
	ErrNoShowFolder    = errors.New("no show Library folder")
	ErrNotShowFolder   = errors.New("that Library folder is not for shows")
	ErrNoShowSources   = errors.New("show details sources are not set up")
	ErrNoSuchSeason    = errors.New("the show has no such season")
)

// ValidSeasonChoice reports whether c is a season choice.
func ValidSeasonChoice(c string) bool {
	return c == SeasonsAll || c == SeasonsFuture || c == SeasonsLatest || c == SeasonsNone
}

// activeStates are Slot states where a Download or File holds the episode.
const HoldingSlots = `file_id IS NOT NULL OR active_download_id IS NOT NULL OR held_release_id IS NOT NULL OR state IN ('searching', 'held', 'chosen', 'downloading', 'stalled', 'importing', 'needs_a_decision', 'in_library', 'better_wanted')`

// AdoptEpisodeFile is a video in a show folder and the episodes its name
// gives; Why says why a file holds no episode (it is left alone).
type AdoptEpisodeFile struct {
	Path     string   `json:"path"` // relative to the Library folder
	Size     int64    `json:"size"`
	Episodes []string `json:"episodes"` // S01E02, ...
	Why      string   `json:"why,omitempty"`
	Keys     [][2]int `json:"-"` // season, episode
}

// StepSearchShow is the chain step after a new show's scan when it was
// added with search when added.
const StepSearchShow = "search_show"

// ShowParts are the show steps the shared title code runs for a show
// Title: refresh, folder adoption, the episode gates, the whole-show
// search and the show Jobs. The shows package fills them in with UseShows.
type ShowParts interface {
	Refresh(ctx context.Context, titleID int64, by domain.Actor, jobID *int64) (RefreshOutcome, error)
	PlanAdopt(ctx context.Context, folderID int64, after string) (AdoptPlan, error)
	AdoptOne(ctx context.Context, lf mediafiles.LibraryFolder, c AdoptChoice, by domain.Actor) (AdoptResult, error)
	RecordFound(ctx context.Context, lf mediafiles.LibraryFolder, v mediafiles.Version, known map[string]bool, by domain.Actor) (int, error)
	OpenEpisodeGates(ctx context.Context) (int, error)
	SearchShow(ctx context.Context, titleID int64, by domain.Actor) error
	RegisterJobs(q *jobs.Queue)
}

// ErrNoShows says the program was built without the shows module.
var ErrNoShows = errors.New("shows are not available")

var newShowParts func(*Service) ShowParts

// UseShows sets how a Service gets its show steps. The shows package calls
// it when it is linked in, so every Service runs show Titles the same way.
func UseShows(f func(*Service) ShowParts) { newShowParts = f }

func (s *Service) showParts() ShowParts {
	if newShowParts == nil {
		return noShows{}
	}
	return newShowParts(s)
}

func (s *Service) openEpisodeGates(ctx context.Context) (int, error) {
	return s.showParts().OpenEpisodeGates(ctx)
}

type noShows struct{}

func (noShows) Refresh(context.Context, int64, domain.Actor, *int64) (RefreshOutcome, error) {
	return RefreshOutcome{}, ErrNoShows
}
func (noShows) PlanAdopt(context.Context, int64, string) (AdoptPlan, error) {
	return AdoptPlan{}, ErrNoShows
}
func (noShows) AdoptOne(context.Context, mediafiles.LibraryFolder, AdoptChoice, domain.Actor) (AdoptResult, error) {
	return AdoptResult{}, ErrNoShows
}
func (noShows) RecordFound(context.Context, mediafiles.LibraryFolder, mediafiles.Version, map[string]bool, domain.Actor) (int, error) {
	return 0, ErrNoShows
}
func (noShows) OpenEpisodeGates(context.Context) (int, error)         { return 0, nil }
func (noShows) SearchShow(context.Context, int64, domain.Actor) error { return ErrNoShows }
func (noShows) RegisterJobs(*jobs.Queue)                              {}
