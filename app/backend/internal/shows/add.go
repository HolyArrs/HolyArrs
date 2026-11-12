package shows

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// ShowAddOptions are the choices made when adding a show. Empty values take
// the show settings: the first show Library folder, its default Pick rules
// (else Standard), kept up to date.
type ShowAddOptions struct {
	LibraryFolderID int64  `json:"library_folder_id"`
	PickRulesID     int64  `json:"pick_rules_id"`
	KeepUpToDate    *bool  `json:"keep_up_to_date"`
	Seasons         string `json:"seasons"`        // all, future, latest, none
	NewSeasons      string `json:"new_seasons"`    // all, none
	EpisodeOrder    string `json:"episode_order"`  // aired, daily, absolute
	Source          string `json:"details_source"` // tmdb, tvmaze, tvdb
	// SearchOnAdd searches the whole show after the folder scan, the same
	// search POST /titles/{id}/search starts for a show.
	SearchOnAdd bool `json:"search_on_add"`
	// MoreVersions are extra wanted copies (for example 4K) next to the main one.
	MoreVersions []library.VersionInput `json:"more_versions"`
	// WhyAdded records why the show was added (by_hand when empty).
	WhyAdded string `json:"-"`
	// Adopt keeps a show folder found on disk as it is, with its files.
	Adopt *ShowAdoption `json:"-"`
	// SeasonNumbers, when not nil, are exactly the seasons kept up to
	// date (Seasons is then none): the arr API's per-season choice.
	SeasonNumbers []int `json:"-"`
	// FirstSeason keeps only the show's first season up to date: season 1,
	// else the lowest numbered season after it, else Specials.
	FirstSeason bool `json:"-"`
	// Specials, when set, says whether Specials start kept up to date with
	// the other seasons, in place of the keep_specials default.
	Specials *bool `json:"-"`
	// KeepEpisode, when set, narrows the episodes that start kept up to
	// date: one it says no to starts not kept, in the same write as the add,
	// so the scan and search after it never see it kept. The arr API uses it
	// for the old apps' episode choices (the first episode only, recent
	// episodes).
	KeepEpisode func(season, episode int, aired *time.Time) bool `json:"-"`
}

// ShowAlreadyAddedError says the show is already in the Library.
type ShowAlreadyAddedError struct{ TitleID int64 }

func (e *ShowAlreadyAddedError) Error() string {
	return fmt.Sprintf("this show is already in the Library (Title %d)", e.TitleID)
}

// ErrShowNeverAdd says the show is on the Never-add list.
var ErrShowNeverAdd = errors.New("this show is on the Never-add list")

// LookupShow finds one show by "<kind>:<id>" for the given details source.
// The id may belong to another source (a TMDB id while adding with
// TVMaze): that source is asked first for the show's other ids.
func (s *Service) LookupShow(ctx context.Context, id, source string) (metadata.Show, error) {
	kind, v, ok := splitShowID(id)
	if !ok {
		return metadata.Show{}, metadata.ErrShowNotFound
	}
	p, err := s.showSource(source)
	if err != nil {
		return metadata.Show{}, err
	}
	ids := map[string]string{kind: v}
	if kind != source {
		if other, err := s.showSource(kind); err == nil {
			if sh, err := other.Show(ctx, id); err == nil {
				for k, x := range sh.IDs {
					ids[k] = x
				}
			} else if !errors.Is(err, metadata.ErrShowNotFound) && !errors.Is(err, metadata.ErrNoTVDBKey) {
				return metadata.Show{}, err
			}
		}
	}
	sh, err := lookupShow(ctx, p, source, ids)
	if err != nil {
		return sh, err
	}
	// Keep the ids learnt on the way that the source did not repeat.
	if sh.IDs == nil {
		sh.IDs = map[string]string{}
	}
	for k, x := range ids {
		if sh.IDs[k] == "" {
			sh.IDs[k] = x
		}
	}
	return sh, nil
}

func splitShowID(id string) (kind, v string, ok bool) {
	for i := 0; i < len(id); i++ {
		if id[i] == ':' {
			kind, v = id[:i], id[i+1:]
			return kind, v, v != "" && (kind == "tmdb" || kind == "tvdb" || kind == "tvmaze" || kind == "imdb")
		}
	}
	return "", "", false
}

// AddShow adds a show by id with its season choice, new-season policy,
// episode order and details source. Seasons, episodes, a main Version and
// one Slot per episode are written at once; each Slot starts not out yet
// or wanted from its air date. A folder scan Job follows.
func (s *Service) AddShow(ctx context.Context, id string, opt ShowAddOptions, by domain.Actor) (int64, int64, error) {
	db := s.Store.Read()
	defs, err := library.ReadShowDefaults(ctx, db)
	if err != nil {
		return 0, 0, err
	}
	if opt.SeasonNumbers != nil || opt.FirstSeason {
		opt.Seasons = library.SeasonsNone
	}
	if opt.Specials != nil {
		defs.KeepSpecials = *opt.Specials
	}
	if opt.Seasons == "" {
		opt.Seasons = defs.SeasonChoice
	}
	if opt.NewSeasons == "" {
		opt.NewSeasons = defs.NewSeasons
	}
	if opt.EpisodeOrder == "" {
		opt.EpisodeOrder = defs.EpisodeOrder
	}
	if opt.Source == "" {
		opt.Source = defs.Source
	}
	if opt.WhyAdded == "" {
		opt.WhyAdded = "by_hand"
	}
	switch {
	case !library.ValidSeasonChoice(opt.Seasons):
		return 0, 0, library.ErrBadSeasonChoice
	case opt.NewSeasons != "all" && opt.NewSeasons != "none":
		return 0, 0, library.ErrBadNewSeasons
	case !library.ValidOrder(opt.EpisodeOrder):
		return 0, 0, library.ErrBadOrder
	case !metadata.ValidShowSource(opt.Source):
		return 0, 0, metadata.ErrUnknownSource
	}
	switch opt.WhyAdded {
	case "by_hand", "list", "series_follow", "ask", "import", "migration":
	default:
		return 0, 0, library.ErrUnknownWhyAdded
	}
	keep := opt.KeepUpToDate == nil || *opt.KeepUpToDate

	var lf mediafiles.LibraryFolder
	if opt.LibraryFolderID == 0 {
		fs, err := database.Select[mediafiles.LibraryFolder](ctx, db, "library_folders", "kind = 'show' ORDER BY id LIMIT 1")
		if err != nil {
			return 0, 0, err
		}
		if len(fs) == 0 {
			return 0, 0, library.ErrNoShowFolder
		}
		lf = fs[0]
	} else if err := database.Get(ctx, db, "library_folders", opt.LibraryFolderID, &lf); errors.Is(err, database.ErrNotFound) {
		return 0, 0, library.ErrNoShowFolder
	} else if err != nil {
		return 0, 0, err
	}
	if lf.Kind != "show" {
		return 0, 0, library.ErrNotShowFolder
	}
	if h := library.CheckFolder(lf.Path, s.DownloadFolders(ctx)); !h.Usable() {
		return 0, 0, &library.FolderError{Health: h}
	}
	if len(opt.MoreVersions) > 5 {
		return 0, 0, library.ErrTooManyVersions
	}
	for _, mv := range opt.MoreVersions {
		if _, err := library.CleanLabel(mv.Label); err != nil {
			return 0, 0, err
		}
		if mv.LibraryFolderID != 0 && mv.LibraryFolderID != lf.ID {
			if _, err := library.CheckVersionFolder(ctx, db, mv.LibraryFolderID, "show", s.DownloadFolders(ctx)); err != nil {
				return 0, 0, err
			}
		}
	}
	sh, err := s.LookupShow(ctx, id, opt.Source)
	if err != nil {
		return 0, 0, err
	}
	s.addAirTimes(ctx, &sh)
	aliases := s.AnimeAliases(ctx, "show", sh.IDs["tmdb"], sh.Name)
	ns, err := mediafiles.ReadNaming(ctx, db)
	if err != nil {
		return 0, 0, err
	}
	var adoptFiles []library.AdoptEpisodeFile
	if opt.Adopt != nil {
		adoptFiles = mapEpisodeFiles(library.ListVideosN(lf.Path, opt.Adopt.FolderName, maxShowVideos), sh, opt.EpisodeOrder)
	}
	now := s.Clock()
	var t mediafiles.Title
	var sync ShowSync
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		if existing, err := library.ShowByIDs(ctx, tx, sh.IDs); err != nil {
			return err
		} else if existing != 0 {
			return &ShowAlreadyAddedError{TitleID: existing}
		}
		if on, err := library.ShowOnNeverAdd(ctx, tx, sh.IDs); err != nil {
			return err
		} else if on {
			return ErrShowNeverAdd
		}
		prID := opt.PickRulesID
		if prID == 0 && lf.DefaultPickRules != nil {
			prID = *lf.DefaultPickRules
		}
		if prID == 0 {
			if err := tx.QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.GoodEverydayName).Scan(&prID); err != nil {
				return fmt.Errorf("pick rules: %w", err)
			}
		} else if err := library.PickRulesFor(tx, &prID, "show"); err != nil {
			return err
		}
		order, choice := opt.EpisodeOrder, opt.Seasons
		var from *time.Time
		if choice == library.SeasonsFuture {
			f := now
			from = &f
			opt.NewSeasons = "all" // new seasons air later: wanted
		}
		t = mediafiles.Title{Kind: "show", Aliases: aliases, MetadataSource: opt.Source, EpisodeOrder: &order, KeepUpToDate: keep,
			NewPartsPolicy: opt.NewSeasons, ReleaseGate: library.GateReleased, MusicTypes: []string{}, WhyAdded: opt.WhyAdded, Artwork: []string{},
			State: "active", GateDriver: "none", SeasonChoice: &choice, FutureFrom: from, RefreshedAt: &now}
		applyShow(&t, sh)
		if _, err := database.Insert(tx, "titles", &t, by); err != nil {
			return err
		}
		year := 0
		if t.Year != nil {
			year = *t.Year
		}
		folder := mediafiles.ShowFolder(ns, mediafiles.NameFacts{Title: t.Name, Year: year, TMDB: t.SourceIDs["tmdb"], IMDB: t.SourceIDs["imdb"]})
		if opt.Adopt != nil {
			folder = opt.Adopt.FolderName
		} else {
			// A folder for this show already on disk under a slightly different
			// name is used as is, so the scan after the add finds its files.
			taken := map[string]bool{}
			if rows, err := tx.QueryContext(ctx, "SELECT folder_name FROM versions WHERE library_folder_id = ?", lf.ID); err == nil {
				for rows.Next() {
					var n string
					if rows.Scan(&n) == nil {
						taken[strings.ToLower(n)] = true
					}
				}
				rows.Close()
			}
			folder = library.ExistingFolder(lf.Path, folder, t.Name, year, func(d string) bool { return taken[strings.ToLower(d)] })
		}
		v := &mediafiles.Version{TitleID: t.ID, Label: "Main", PickRulesID: prID, LibraryFolderID: lf.ID, FolderName: folder, KeepUpToDate: keep, IsMain: true}
		if _, err := database.Insert(tx, "versions", v, by); err != nil {
			return err
		}
		var err error
		sync, err = syncShowTx(ctx, tx, t, sh, defs, choice, now, by)
		if err != nil {
			return err
		}
		// Seasons named one by one start kept up to date with all their
		// episodes, in the same write, so the scan and search after the
		// add already see them.
		if keep {
			ts, who := database.FormatTime(now), library.ActorText(by)
			if opt.FirstSeason {
				var first sql.NullInt64
				if err := tx.QueryRow("SELECT COALESCE(MIN(CASE WHEN number >= 1 THEN number END), MIN(number)) FROM parts WHERE title_id = ? AND kind = 'season'", t.ID).Scan(&first); err != nil {
					return err
				}
				if first.Valid {
					opt.SeasonNumbers = append(opt.SeasonNumbers, int(first.Int64))
				}
			}
			for _, n := range opt.SeasonNumbers {
				if _, err := tx.Exec("UPDATE parts SET keep_up_to_date = 1, updated_at = ?, updated_by = ? WHERE title_id = ? AND kind = 'season' AND number = ?", ts, who, t.ID, n); err != nil {
					return err
				}
				if _, err := tx.Exec("UPDATE items SET keep_up_to_date = 1, updated_at = ?, updated_by = ? WHERE part_id IN (SELECT id FROM parts WHERE title_id = ? AND kind = 'season' AND number = ?)", ts, who, t.ID, n); err != nil {
					return err
				}
			}
		}
		if keep && opt.KeepEpisode != nil {
			if err := narrowEpisodesTx(ctx, tx, t.ID, opt.KeepEpisode, by); err != nil {
				return err
			}
		}
		// More Versions after the episodes, so each gets one Slot per episode.
		for _, mv := range opt.MoreVersions {
			if _, err := library.AddVersionTx(ctx, tx, t, mv, now, by); err != nil {
				return err
			}
		}
		if opt.Adopt == nil {
			return nil
		}
		return adoptEpisodesTx(ctx, tx, t.ID, v.ID, prID, keep, adoptFiles, opt.Adopt, now, by)
	})
	if err != nil {
		return 0, 0, err
	}
	s.CachePoster(ctx, t.ID, sh.Poster, by)
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventTitleAdded, TitleID: &t.ID, Summary: "Added " + t.Name,
		Data: map[string]any{"why_added": t.WhyAdded, "seasons": opt.Seasons, "new_seasons": opt.NewSeasons, "episode_order": opt.EpisodeOrder,
			"details_source": opt.Source, "episodes": sync.EpisodesAdded, "wanted": countWanted(ctx, s.Store.Read(), t.ID)},
		GroupKey: library.GroupKey(t.ID)})
	var params map[string]any
	if opt.SearchOnAdd {
		params = map[string]any{"then": []any{library.StepSearchShow}}
	}
	jobID, _, err := s.Queue.Enqueue(ctx, library.KindScanTitle, &domain.Subject{Type: "title", ID: t.ID}, params, "scan_title:"+strconv.FormatInt(t.ID, 10), by)
	return t.ID, jobID, err
}

// SeasonVersion is one season and Version of a show to search.
type SeasonVersion struct{ PartID, VersionID int64 }

// ShowSeasonsToSearch lists, for a whole-show search, every season with an
// episode kept up to date that is looking for a copy, in a show and
// Version kept up to date.
func ShowSeasonsToSearch(ctx context.Context, db database.Lister, titleID int64) ([]SeasonVersion, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT p.id, s.version_id FROM parts p JOIN items i ON i.part_id = p.id JOIN slots s ON s.item_id = i.id
		JOIN titles t ON t.id = p.title_id JOIN versions v ON v.id = s.version_id
		WHERE p.title_id = ? AND t.keep_up_to_date = 1 AND v.keep_up_to_date = 1 AND p.keep_up_to_date = 1 AND i.keep_up_to_date = 1 AND i.dropped_at IS NULL
		AND s.state IN ('wanted', 'better_wanted', 'failed', 'rejected', 'stalled') ORDER BY p.number, s.version_id`, titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []SeasonVersion
	for rows.Next() {
		var x SeasonVersion
		if err := rows.Scan(&x.PartID, &x.VersionID); err != nil {
			return nil, err
		}
		list = append(list, x)
	}
	return list, rows.Err()
}

// searchShow is the whole-show search after a new show's scan: an
// automatic search of each season, queued the way the missing search and
// a failed download queue one (no by_hand, so the release gate holds).
func (s *Service) searchShow(ctx context.Context, titleID int64, by domain.Actor) error {
	list, err := ShowSeasonsToSearch(ctx, s.Store.Read(), titleID)
	if err != nil {
		return err
	}
	for _, x := range list {
		if _, _, err := s.Queue.Enqueue(ctx, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: x.PartID},
			map[string]any{"version_id": x.VersionID}, fmt.Sprintf("search_season:%d:%d", x.PartID, x.VersionID), by); err != nil {
			return err
		}
	}
	return nil
}

func countWanted(ctx context.Context, db database.Querier, titleID int64) int {
	var n int
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM slots WHERE state = 'wanted' AND item_id IN (SELECT id FROM items WHERE title_id = ?)", titleID).Scan(&n)
	return n
}

// SetSeason switches Keep up to date for one season and all its episodes.
func (s *Service) SetSeason(ctx context.Context, titleID int64, number int, keep bool, by domain.Actor) error {
	var t mediafiles.Title
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "titles", titleID, &t); err != nil {
			return err
		}
		if t.Kind != "show" {
			return library.ErrNotShow
		}
		var partID int64
		if err := tx.QueryRow("SELECT id FROM parts WHERE title_id = ? AND kind = 'season' AND number = ?", titleID, number).Scan(&partID); errors.Is(err, sql.ErrNoRows) {
			return library.ErrNoSuchSeason
		} else if err != nil {
			return err
		}
		ts, who := database.FormatTime(s.Clock()), library.ActorText(by)
		if _, err := tx.Exec("UPDATE parts SET keep_up_to_date = ?, updated_at = ?, updated_by = ? WHERE id = ?", keep, ts, who, partID); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE items SET keep_up_to_date = ?, updated_at = ?, updated_by = ? WHERE part_id = ?", keep, ts, who, partID)
		return err
	})
	if err != nil {
		return err
	}
	word := map[bool]string{true: "on", false: "off"}[keep]
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventSeasonChanged, TitleID: &titleID,
		Summary: fmt.Sprintf("%s season %d: Keep up to date %s", t.Name, number, word), Data: map[string]any{"season": number, "keep_up_to_date": keep},
		GroupKey: library.GroupKey(titleID)})
	return nil
}

// KeepEpisodes keeps single episodes of one season up to date. A season
// not kept yet is switched on with only these episodes; in a kept season
// the episodes are switched on too.
func (s *Service) KeepEpisodes(ctx context.Context, titleID int64, number int, episodes []int, by domain.Actor) error {
	if len(episodes) == 0 {
		return nil
	}
	var t mediafiles.Title
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "titles", titleID, &t); err != nil {
			return err
		}
		if t.Kind != "show" {
			return library.ErrNotShow
		}
		var partID int64
		var keep bool
		if err := tx.QueryRow("SELECT id, keep_up_to_date FROM parts WHERE title_id = ? AND kind = 'season' AND number = ?", titleID, number).Scan(&partID, &keep); errors.Is(err, sql.ErrNoRows) {
			return library.ErrNoSuchSeason
		} else if err != nil {
			return err
		}
		ts, who := database.FormatTime(s.Clock()), library.ActorText(by)
		if !keep {
			if _, err := tx.Exec("UPDATE parts SET keep_up_to_date = 1, updated_at = ?, updated_by = ? WHERE id = ?", ts, who, partID); err != nil {
				return err
			}
			if _, err := tx.Exec("UPDATE items SET keep_up_to_date = 0, updated_at = ?, updated_by = ? WHERE part_id = ? AND kind = 'episode' AND keep_up_to_date = 1", ts, who, partID); err != nil {
				return err
			}
		}
		args := []any{ts, who, partID}
		for _, n := range episodes {
			args = append(args, n)
		}
		_, err := tx.Exec("UPDATE items SET keep_up_to_date = 1, updated_at = ?, updated_by = ? WHERE part_id = ? AND kind = 'episode' AND json_extract(numbers, '$.episode') IN ("+
			strings.TrimSuffix(strings.Repeat("?,", len(episodes)), ",")+")", args...)
		return err
	})
	if err != nil {
		return err
	}
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventSeasonChanged, TitleID: &titleID,
		Summary: fmt.Sprintf("%s season %d: Keep up to date on for %d episodes", t.Name, number, len(episodes)), Data: map[string]any{"season": number, "episodes": episodes},
		GroupKey: library.GroupKey(titleID)})
	return nil
}

// KeepFuture switches a show in the Library to Future seasons only: what it
// keeps stays kept, episodes still to air (and their seasons) are kept up
// to date, and seasons found later are wanted.
func (s *Service) KeepFuture(ctx context.Context, titleID int64, by domain.Actor) error {
	var t mediafiles.Title
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "titles", titleID, &t); err != nil {
			return err
		}
		if t.Kind != "show" {
			return library.ErrNotShow
		}
		now := s.Clock()
		choice := library.SeasonsFuture
		t.KeepUpToDate, t.NewPartsPolicy, t.SeasonChoice, t.FutureFrom = true, "all", &choice, &now
		if err := database.Update(tx, "titles", &t, by); err != nil {
			return err
		}
		ts, who := database.FormatTime(now), library.ActorText(by)
		ahead := "(air_date IS NULL OR air_date > ?)"
		if _, err := tx.Exec("UPDATE items SET keep_up_to_date = 1, updated_at = ?, updated_by = ? WHERE title_id = ? AND kind = 'episode' AND keep_up_to_date = 0 AND "+ahead,
			ts, who, titleID, ts); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE parts SET keep_up_to_date = 1, updated_at = ?, updated_by = ? WHERE title_id = ? AND kind = 'season' AND number > 0 AND keep_up_to_date = 0
			AND id IN (SELECT part_id FROM items WHERE title_id = ? AND kind = 'episode' AND `+ahead+`)`, ts, who, titleID, titleID, ts)
		return err
	})
	if err != nil {
		return err
	}
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventSeasonChanged, TitleID: &titleID,
		Summary: t.Name + ": Future seasons only", Data: map[string]any{"season_choice": library.SeasonsFuture}, GroupKey: library.GroupKey(titleID)})
	return nil
}

// SetEpisode switches Keep up to date for one episode.
func (s *Service) SetEpisode(ctx context.Context, itemID int64, keep bool, by domain.Actor) (mediafiles.Item, error) {
	var it mediafiles.Item
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "items", itemID, &it); err != nil {
			return err
		}
		if it.Kind != "episode" {
			return database.ErrNotFound
		}
		it.KeepUpToDate = keep
		return database.Update(tx, "items", &it, by)
	})
	if err != nil {
		return it, err
	}
	var name string
	_ = s.Store.Read().QueryRowContext(ctx, "SELECT name FROM titles WHERE id = ?", it.TitleID).Scan(&name)
	label := fmt.Sprintf("S%02dE%02d", library.Deref(it.Numbers.Season), library.Deref(it.Numbers.Episode))
	word := map[bool]string{true: "on", false: "off"}[keep]
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventSeasonChanged, TitleID: &it.TitleID,
		Summary: fmt.Sprintf("%s %s: Keep up to date %s", name, label, word), Data: map[string]any{"item_id": it.ID, "keep_up_to_date": keep},
		GroupKey: library.GroupKey(it.TitleID)})
	return it, nil
}

// SeasonSummary is one season of a show with its episode counts.
type SeasonSummary struct {
	Number       int        `json:"number"`
	Name         string     `json:"name"`
	KeepUpToDate bool       `json:"keep_up_to_date"`
	Episodes     int        `json:"episodes"`
	Aired        int        `json:"aired"`
	InLibrary    int        `json:"in_library"`    // episodes with a file in the main Version
	Missing      int        `json:"missing"`       // aired, kept up to date, no file yet
	BetterWanted int        `json:"better_wanted"` // in the Library, a better copy is sought
	NotOutYet    int        `json:"not_out_yet"`
	Downloading  int        `json:"downloading"` // a copy is on the way (downloading, stalled, importing)
	Failed       int        `json:"failed"`      // the download failed or its import needs a decision
	NextAirDate  *time.Time `json:"next_air_date,omitempty"`
	Dropped      bool       `json:"dropped,omitempty"`
}

// Seasons summarises a show's seasons from its episodes and the main
// Version's Slots.
func Seasons(ctx context.Context, db database.Lister, titleID int64, now time.Time) ([]SeasonSummary, error) {
	parts, err := database.Select[mediafiles.Part](ctx, db, "parts", "title_id = ? AND kind = 'season' ORDER BY number", titleID)
	if err != nil {
		return nil, err
	}
	out := make([]SeasonSummary, 0, len(parts))
	index := map[int64]int{}
	for i, p := range parts {
		out = append(out, SeasonSummary{Number: library.Deref(p.Number), Name: p.Name, KeepUpToDate: p.KeepUpToDate, Dropped: p.DroppedAt != nil})
		index[p.ID] = i
	}
	if len(parts) == 0 {
		return out, nil
	}
	if grouped, err := library.HasEpisodeGroup(ctx, db, titleID); err != nil || grouped {
		if err != nil {
			return nil, err
		}
		return groupSeasons(ctx, db, titleID, now, parts)
	}
	// One query for every season of the show instead of one per season. The
	// main Version is looked up once (by the show's id), not once per episode.
	rows, err := db.QueryContext(ctx, `SELECT i.part_id, i.air_date, i.keep_up_to_date, COALESCE(s.state, '') FROM items i
		LEFT JOIN slots s ON s.item_id = i.id AND s.version_id = (SELECT id FROM versions WHERE title_id = ? ORDER BY is_main DESC, id LIMIT 1)
		WHERE i.part_id IN (SELECT id FROM parts WHERE title_id = ? AND kind = 'season')`, titleID, titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var part int64
		var air sql.NullString
		var keep bool
		var state string
		if err := rows.Scan(&part, &air, &keep, &state); err != nil {
			return nil, err
		}
		idx, ok := index[part]
		if !ok {
			continue
		}
		out[idx].tally(air, keep, state, now)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// tally counts one episode of a season: its air date, its Keep up to date
// switch and the main Version's Slot state.
func (ss *SeasonSummary) tally(air sql.NullString, keep bool, state string, now time.Time) {
	ss.Episodes++
	if air.Valid {
		if at, err := time.Parse(database.TimeLayout, air.String); err == nil {
			if !at.After(now) {
				ss.Aired++
			} else if ss.NextAirDate == nil || at.Before(*ss.NextAirDate) {
				a := at
				ss.NextAirDate = &a
			}
		}
	}
	switch mediafiles.SlotState(state) {
	case mediafiles.SlotInLibrary:
		ss.InLibrary++
	case mediafiles.SlotBetterWanted:
		ss.InLibrary++
		ss.BetterWanted++
	case mediafiles.SlotNotOutYet:
		ss.NotOutYet++
	case mediafiles.SlotChosen, mediafiles.SlotDownloading, mediafiles.SlotStalled, mediafiles.SlotImporting:
		ss.Downloading++
	case mediafiles.SlotFailed, mediafiles.SlotNeedsADecision:
		ss.Failed++
	case mediafiles.SlotWanted:
		if keep {
			ss.Missing++
		}
	}
}

// narrowEpisodesTx stops keeping up to date every kept episode of a show
// that keepEpisode says no to.
func narrowEpisodesTx(ctx context.Context, tx *sql.Tx, titleID int64, keepEpisode func(season, episode int, aired *time.Time) bool, by domain.Actor) error {
	its, err := database.Select[mediafiles.Item](ctx, tx, "items", "title_id = ? AND kind = 'episode' AND keep_up_to_date = 1", titleID)
	if err != nil {
		return err
	}
	for i := range its {
		it := &its[i]
		season, episode := 0, 0
		if it.Numbers.Season != nil {
			season = *it.Numbers.Season
		}
		if it.Numbers.Episode != nil {
			episode = *it.Numbers.Episode
		}
		if keepEpisode(season, episode, it.AirDate) {
			continue
		}
		it.KeepUpToDate = false
		if err := database.Update(tx, "items", it, by); err != nil {
			return err
		}
	}
	return nil
}
