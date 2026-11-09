// Package library manages Library folders, Titles, Versions and Slots: adding
// and editing movies, details refreshes and the Release gate.
package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// Errors from adding a movie.
var (
	ErrNoFolder        = errors.New("no movie Library folder")
	ErrNotMovieFolder  = errors.New("that Library folder is not for movies")
	ErrNeverAdd        = errors.New("this movie is on the Never-add list")
	ErrBadGate         = errors.New("release_gate must be announced, in_cinemas or released")
	ErrBadDelay        = errors.New("gate_delay_days must be between 0 and 365")
	ErrNoMetadataIDs   = errors.New("the movie has no TMDB id")
	ErrUnknownWhyAdded = errors.New("unknown why_added")
)

// AlreadyAddedError says the movie is already in the Library.
type AlreadyAddedError struct{ TitleID int64 }

func (e *AlreadyAddedError) Error() string {
	return fmt.Sprintf("this movie is already in the Library (Title %d)", e.TitleID)
}

// FolderError says the chosen Library folder cannot take files now.
type FolderError struct{ Health Health }

func (e *FolderError) Error() string {
	return fmt.Sprintf("the Library folder is not usable (%s): %s", e.Health.State, e.Health.Reason)
}

// AddOptions are the choices made when adding a movie. Zero values take the
// defaults: the first movie Library folder, its default Pick rules (else
// Standard), kept up to date, the Release gate settings, no search.
type AddOptions struct {
	LibraryFolderID int64  `json:"library_folder_id"`
	PickRulesID     int64  `json:"pick_rules_id"`
	KeepUpToDate    *bool  `json:"keep_up_to_date"`
	ReleaseGate     string `json:"release_gate"`
	GateDelayDays   *int   `json:"gate_delay_days"`
	SearchOnAdd     bool   `json:"search_on_add"`
	// Aliases are the movie's other titles (an anime's AniList titles).
	Aliases []string `json:"-"`
	// MoreVersions are extra wanted copies (for example 4K) next to the main one.
	MoreVersions []VersionInput `json:"more_versions"`
	// WhyAdded records why the movie was added (by_hand when empty).
	WhyAdded string `json:"-"`
	// Adopt keeps a movie folder found on disk as it is.
	Adopt *Adoption `json:"-"`
}

// Adoption is a movie folder found on disk that the person confirmed:
// the Version takes the folder's name as it is, and its main video (if
// any) becomes an adopted File. Nothing on disk changes.
type Adoption struct {
	FolderName string
	File       string // relative to the Library folder; "" when there is none
	Size       int64
	MediaInfo  map[string]any
}

// GateDefaults are the Release gate settings for new movies.
type GateDefaults struct {
	Gate             string `json:"release_gate"`
	DelayDays        int    `json:"gate_delay_days"`
	BulkRespectsGate bool   `json:"bulk_search_respects_gate"`
}

// ReadGateDefaults reads the Release gate settings (open choice 1; the
// recommended "released" + 0 days, bulk searches respect the gate, when unset).
func ReadGateDefaults(ctx context.Context, db database.Lister) (GateDefaults, error) {
	g := GateDefaults{Gate: GateReleased, BulkRespectsGate: true}
	rows, err := db.QueryContext(ctx, "SELECT key, value FROM settings WHERE grp = 'what_to_pick' AND key IN ('release_gate', 'gate_delay_days', 'bulk_search_respects_gate')")
	if err != nil {
		return g, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return g, err
		}
		switch k {
		case "release_gate":
			if ValidGate(v) {
				g.Gate = v
			}
		case "gate_delay_days":
			if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 365 {
				g.DelayDays = n
			}
		case "bulk_search_respects_gate":
			g.BulkRespectsGate = v != "false"
		}
	}
	return g, rows.Err()
}

// SetGateDefaults stores the Release gate settings.
func SetGateDefaults(ctx context.Context, s *database.Store, g GateDefaults, by domain.Actor) error {
	if !ValidGate(g.Gate) {
		return ErrBadGate
	}
	if g.DelayDays < 0 || g.DelayDays > 365 {
		return ErrBadDelay
	}
	return s.Write(ctx, func(tx *sql.Tx) error {
		now := database.FormatTime(time.Now())
		for k, v := range map[string]string{"release_gate": g.Gate, "gate_delay_days": strconv.Itoa(g.DelayDays),
			"bulk_search_respects_gate": strconv.FormatBool(g.BulkRespectsGate)} {
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

// TitleByTMDB finds a movie Title by TMDB id; 0 when there is none.
func TitleByTMDB(ctx context.Context, db database.Querier, tmdbID string) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx, "SELECT id FROM titles WHERE kind = 'movie' AND state != 'removed' AND json_extract(source_ids, '$.tmdb') = ?", tmdbID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// OnNeverAdd reports whether a TMDB id is on the Never-add list.
func OnNeverAdd(ctx context.Context, db database.Querier, tmdbID string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, "SELECT count(*) FROM never_add WHERE kind = 'movie' AND json_extract(source_ids, '$.tmdb') = ?", tmdbID).Scan(&n)
	return n > 0, err
}

// AddMovie adds a looked-up movie as a Title with its single Item, a main
// Version and one Slot. The Slot starts not_out_yet or wanted, from the
// Release gate and the movie's dates. It is refused when the movie is
// already in the Library, on the Never-add list, or the Library folder is
// not usable. Queuing the follow-up Jobs is the caller's part.
func AddMovie(ctx context.Context, s *database.Store, m metadata.Movie, opt AddOptions, downloadPaths []string, now time.Time, by domain.Actor) (int64, error) {
	tmdbID := m.IDs["tmdb"]
	if tmdbID == "" {
		return 0, ErrNoMetadataIDs
	}
	if opt.WhyAdded == "" {
		opt.WhyAdded = "by_hand"
	}
	switch opt.WhyAdded {
	case "by_hand", "list", "series_follow", "ask", "import", "migration":
	default:
		return 0, ErrUnknownWhyAdded
	}
	defs, err := ReadGateDefaults(ctx, s.Read())
	if err != nil {
		return 0, err
	}
	gate, delay := defs.Gate, defs.DelayDays
	if opt.ReleaseGate != "" {
		if !ValidGate(opt.ReleaseGate) {
			return 0, ErrBadGate
		}
		gate = opt.ReleaseGate
	}
	if opt.GateDelayDays != nil {
		if *opt.GateDelayDays < 0 || *opt.GateDelayDays > 365 {
			return 0, ErrBadDelay
		}
		delay = *opt.GateDelayDays
	}
	keep := opt.KeepUpToDate == nil || *opt.KeepUpToDate

	// Pick the Library folder and check it on disk before taking the writer.
	db := s.Read()
	var lf mediafiles.LibraryFolder
	if opt.LibraryFolderID == 0 {
		fs, err := database.Select[mediafiles.LibraryFolder](ctx, db, "library_folders", "kind = 'movie' ORDER BY id LIMIT 1")
		if err != nil {
			return 0, err
		}
		if len(fs) == 0 {
			return 0, ErrNoFolder
		}
		lf = fs[0]
	} else if err := database.Get(ctx, db, "library_folders", opt.LibraryFolderID, &lf); errors.Is(err, database.ErrNotFound) {
		return 0, ErrNoFolder
	} else if err != nil {
		return 0, err
	}
	if lf.Kind != "movie" {
		return 0, ErrNotMovieFolder
	}
	if h := CheckFolder(lf.Path, downloadPaths); !h.Usable() {
		return 0, &FolderError{Health: h}
	}

	if len(opt.MoreVersions) > 5 {
		return 0, ErrTooManyVersions
	}
	for _, mv := range opt.MoreVersions {
		if _, err := CleanLabel(mv.Label); err != nil {
			return 0, err
		}
		if mv.LibraryFolderID != 0 && mv.LibraryFolderID != lf.ID {
			if _, err := CheckVersionFolder(ctx, db, mv.LibraryFolderID, "movie", downloadPaths); err != nil {
				return 0, err
			}
		}
	}

	ns, err := mediafiles.ReadNaming(ctx, db)
	if err != nil {
		return 0, err
	}
	if opt.Adopt != nil && opt.Adopt.File != "" {
		opt.Adopt.MediaInfo = importer.InspectFacts(filepath.Join(lf.Path, filepath.FromSlash(opt.Adopt.File)))
		if edition := importer.ParsedEdition(path.Base(opt.Adopt.File)); edition != "" {
			opt.Adopt.MediaInfo["edition"], opt.Adopt.MediaInfo["edition_source"] = edition, "name"
		}
	}
	ds := Dates{Cinema: m.CinemaDate, Digital: m.DigitalDate, Physical: m.PhysicalDate}
	driver, opens := Gate(gate, delay, ds)
	aliases := append([]string{}, opt.Aliases...)
	state := mediafiles.SlotNotOutYet
	if GateOpen(driver, opens, now) {
		state = mediafiles.SlotWanted
	}
	unlock := s.LockFiles()
	defer unlock()
	var titleID int64
	err = s.Write(ctx, func(tx *sql.Tx) error {
		if id, err := TitleByTMDB(ctx, tx, tmdbID); err != nil {
			return err
		} else if id != 0 {
			return &AlreadyAddedError{TitleID: id}
		}
		if on, err := OnNeverAdd(ctx, tx, tmdbID); err != nil {
			return err
		} else if on {
			return ErrNeverAdd
		}
		prID := opt.PickRulesID
		if prID == 0 && lf.DefaultPickRules != nil {
			prID = *lf.DefaultPickRules
		}
		if prID == 0 {
			if err := tx.QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.GoodEverydayName).Scan(&prID); err != nil {
				return fmt.Errorf("pick rules: %w", err)
			}
		} else if err := PickRulesFor(tx, &prID, "movie"); err != nil {
			return err
		}
		t := &mediafiles.Title{Kind: "movie", Aliases: aliases, MetadataSource: "tmdb", KeepUpToDate: keep, NewPartsPolicy: "all",
			ReleaseGate: gate, GateDelayDays: delay, MusicTypes: []string{}, WhyAdded: opt.WhyAdded, Artwork: []string{},
			State: "active", GateDriver: driver, GateOpensAt: opens}
		applyMovie(t, m, now)
		if _, err := database.Insert(tx, "titles", t, by); err != nil {
			return err
		}
		if err := syncMovieCollection(ctx, tx, t.ID, m); err != nil {
			return err
		}
		it := &mediafiles.Item{TitleID: t.ID, Kind: "movie", KeepUpToDate: keep}
		if m.Runtime > 0 {
			rt := m.Runtime
			it.Runtime = &rt
		}
		if _, err := database.Insert(tx, "items", it, by); err != nil {
			return err
		}
		year := 0
		if t.Year != nil {
			year = *t.Year
		}
		folderName := mediafiles.Folder(ns, mediafiles.NameFacts{Title: t.Name, Year: year, TMDB: t.SourceIDs["tmdb"], IMDB: t.SourceIDs["imdb"]})
		if opt.Adopt != nil {
			folderName = opt.Adopt.FolderName
		}
		v := &mediafiles.Version{TitleID: t.ID, Label: "Main", PickRulesID: prID, LibraryFolderID: lf.ID,
			FolderName: folderName, KeepUpToDate: keep, IsMain: true}
		if _, err := database.Insert(tx, "versions", v, by); err != nil {
			return err
		}
		reason := gateReason(driver, opens)
		sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: state, LastReason: &reason}
		if opt.Adopt != nil && opt.Adopt.File != "" {
			sl.State = mediafiles.SlotInLibrary
			why := "adopted from " + opt.Adopt.FolderName
			sl.LastReason = &why
		}
		if _, err := database.Insert(tx, "slots", sl, by); err != nil {
			return err
		}
		if opt.Adopt != nil && opt.Adopt.File != "" {
			if err := adoptFile(ctx, tx, *opt.Adopt, sl.ID, v.ID, prID, keep, now, by); err != nil {
				return err
			}
		}
		for _, mv := range opt.MoreVersions {
			if _, err := AddVersionTx(ctx, tx, *t, mv, now, by); err != nil {
				return err
			}
		}
		titleID = t.ID
		return nil
	})
	return titleID, err
}

// applyMovie copies looked-up details onto a Title (not its gate choice).
func applyMovie(t *mediafiles.Title, m metadata.Movie, now time.Time) {
	t.Name, t.SortName = m.Name, SortName(m.Name)
	ids := map[string]string{}
	for k, v := range t.SourceIDs {
		ids[k] = v
	}
	for k, v := range m.IDs {
		ids[k] = v
	}
	t.SourceIDs = ids
	t.Year, t.Overview, t.OriginalLanguage = nil, nil, nil
	if m.Year > 0 {
		y := m.Year
		t.Year = &y
	}
	if m.Overview != "" {
		o := m.Overview
		t.Overview = &o
	}
	if m.OriginalLanguage != "" {
		l := m.OriginalLanguage
		t.OriginalLanguage = &l
	}
	t.CinemaDate, t.DigitalDate, t.PhysicalDate = m.CinemaDate, m.DigitalDate, m.PhysicalDate
	t.Aliases = MergeAliases(t.Aliases, m.Name, m.OtherNames)
	t.Status = titleStatus(Stage(DatesOf(*t), now))
	ApplyDetails(t, m.Backdrop, m.Logo, m.Genres, m.Rating, m.People, m.Runtime)
	t.Directors, t.Studio = append([]string{}, m.Directors...), m.Studio
}

// applyDetails copies the screen details; a fresh lookup replaces them all.
func ApplyDetails(t *mediafiles.Title, backdrop, logo string, genres []string, rating string, people []string, runtime int) {
	t.Backdrop, t.Logo, t.Rating, t.RuntimeMinutes = backdrop, logo, rating, max(runtime, 0)
	t.Genres = append([]string{}, genres...)
	t.People = append([]string{}, people...)
}

// gateReason is the Slot reason that explains the gate.
func gateReason(driver string, opens *time.Time) string {
	switch {
	case driver == "announced":
		return "release gate: any time"
	case opens == nil:
		return "release gate: no release dates known yet"
	}
	what := map[string]string{"cinema": "cinema date", "digital": "digital date", "physical": "physical date",
		"cinema_plus_90": "cinema date + 90 days"}[driver]
	return fmt.Sprintf("release gate: searching from %s (%s)", opens.Format("2006-01-02"), what)
}

func SortName(n string) string {
	c := releasename.CleanName(n)
	for _, a := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(c, a) {
			return strings.TrimPrefix(c, a)
		}
	}
	return c
}

// adoptFile records an adopted main File for a new Slot and says whether
// it is good enough; below good enough and kept up to date, the Slot looks
// for a better copy, as after an import.
func adoptFile(ctx context.Context, tx *sql.Tx, a Adoption, slotID, versionID, pickRulesID int64, keep bool, now time.Time, by domain.Actor) error {
	ext := path.Ext(a.File)
	release := strings.TrimSuffix(path.Base(a.File), ext)
	p := releasename.Parse(release, releasename.Hints{Size: a.Size})
	f := &mediafiles.File{SlotIDs: []int64{slotID}, VersionID: versionID, RelativePath: a.File, Size: a.Size, AddedAt: now,
		Quality:   mediafiles.Quality{Resolution: p.Resolution.Value, Source: p.Source.Value, Codec: p.Codec.Value, HDR: p.HDR.Value},
		Languages: InspectedLanguages(a.MediaInfo), MediaInfo: map[string]any{"step": mediafiles.StepFor(p)}, ReleaseName: &release, Origin: "adopted",
		PlacedBy: "none", Role: "main", State: "present"}
	if f.Languages == nil {
		f.Languages = []string{}
	}
	for k, value := range a.MediaInfo {
		f.MediaInfo[k] = value
	}
	if g := p.Group.Value; g != "" {
		f.Group = &g
	}
	fileID, err := database.Insert(tx, "files", f, by)
	if err != nil {
		return err
	}
	var pr mediafiles.PickRules
	if err := database.Get(ctx, tx, "pick_rules", pickRulesID, &pr); err != nil {
		return err
	}
	ge := mediafiles.ReachesGoodEnough(release, pr.Body)
	if _, err := tx.Exec("UPDATE slots SET file_id = ?, good_enough = ? WHERE id = ?", fileID, ge, slotID); err != nil {
		return err
	}
	if !ge && keep {
		return mediafiles.MoveSlotTx(tx, slotID, mediafiles.SlotBetterWanted, "adopted below good enough, so a better copy is sought", by)
	}
	return nil
}
