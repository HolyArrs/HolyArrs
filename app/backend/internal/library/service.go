package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Job kinds run by the library.
const (
	KindRefreshTitle = "refresh_title"         // one Title's details
	KindScanTitle    = "scan_title"            // look at a Title's Version folders
	KindMoveTitle    = "move_title"            // move a Version to another Library folder (dry run first)
	KindCheckFolders = "check_library_folders" // health of every Library folder
	// KindSearchSlot is the Source search for one Slot (internal/source).
	KindSearchSlot = "search_slot"
)

// Service runs the library's Jobs and the work behind the API.
type Service struct {
	Store    *database.Store
	Queue    *jobs.Queue
	Metadata metadata.Provider
	// Shows are the show details sources (tmdb, tvmaze, tvdb); nil has none.
	Shows metadata.ShowSources
	// Artists is MusicBrainz (or a mirror); nil has no music details.
	Artists metadata.ArtistProvider
	Live    *webserver.LiveHub
	// Artwork caches posters; nil keeps no pictures.
	Artwork *ArtworkCache
	// ArtistImages reads Deezer's artist profiles (top songs, explicit
	// marks) and Wikipedia bios; nil reads none.
	ArtistImages *metadata.ArtistImages
	// ImageServer is the metadata server's picture store (<base>/v1/img):
	// the only source of artist photos, wide backgrounds and logos.
	ImageServer string
	// SavePath is the Downloader save folder named at start, for health checks.
	SavePath string
	// AnimeNames gives an anime's other titles (AniList romaji, English,
	// native) by kind (movie, show) and TMDB id, kept as the Title's aliases
	// so Library search and release matching find them; nil or none: no
	// aliases.
	AnimeNames func(ctx context.Context, kind, tmdbID, name string) []string
	// Now is the clock (time.Now when nil).
	Now func() time.Time

	// checkedPaths is the Downloader folder list of the last full folder
	// check; a timed check with another list runs the full test again.
	checkedMu    sync.Mutex
	checkedOnce  bool
	checkedPaths string
}

// animeAliases are the anime's other titles, or an empty list.
func (s *Service) AnimeAliases(ctx context.Context, kind, tmdbID, name string) []string {
	if s.AnimeNames == nil || tmdbID == "" {
		return []string{}
	}
	if names := s.AnimeNames(ctx, kind, tmdbID, name); len(names) > 0 {
		return names
	}
	return []string{}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Register adds the library's Job handlers to the queue.
func (s *Service) Register(q *jobs.Queue) {
	q.Handle(KindRefreshTitle, s.handleRefreshTitle)
	q.Handle(KindRefreshTitles, s.handleRefreshTitles)
	s.showParts().RegisterJobs(q)
	q.Handle(KindRefreshArtists, s.handleRefreshArtists)
	q.Handle(KindArtistPictures, s.handleArtistPictures)
	q.Handle(KindScanTitle, s.handleScan)
	q.Handle(KindMoveTitle, s.handleMove)
	q.Handle(KindCheckFolders, s.handleCheckFolders)
	q.Handle(KindRename, s.handleRename)
	q.Handle(KindCleanHolding, s.handleCleanHolding)
	q.Handle(KindScanLibrary, s.handleScanLibrary)
	q.Handle(KindAdopt, s.handleAdopt)
	q.Handle(KindRescan, s.handleRescan)
	q.Handle(KindRecheckPickRules, s.handleRecheckPickRules)
}

func (s *Service) Publish(topic string, v any) {
	if s.Live != nil {
		s.Live.Publish(topic, v)
	}
}

func (s *Service) Emit(ctx context.Context, e *domain.Event) {
	if err := s.Store.AddEvent(ctx, e); err != nil {
		log.Printf("library: event %s: %v", e.Kind, err)
		return
	}
	s.Publish("event", e)
}

func JobActor(r *jobs.Run) domain.Actor { return domain.Actor{Kind: "job", ID: r.Job.ID} }

// downloadPaths lists the Downloader folders for hardlink checks.
func (s *Service) DownloadFolders(ctx context.Context) []string {
	ps, err := DownloadPaths(ctx, s.Store.Read(), s.SavePath)
	if err != nil {
		log.Printf("library: downloader paths: %v", err)
	}
	return ps
}

// Add adds a movie and queues its details refresh, which is followed by a
// folder scan and, when asked, a search (Radarr's order for a new movie).
func (s *Service) Add(ctx context.Context, m metadata.Movie, opt AddOptions, by domain.Actor) (int64, int64, error) {
	if slices.Contains(m.Genres, "Animation") {
		opt.Aliases = s.AnimeAliases(ctx, "movie", m.IDs["tmdb"], m.Name)
	}
	id, err := AddMovie(ctx, s.Store, m, opt, s.DownloadFolders(ctx), s.now(), by)
	if err != nil {
		return 0, 0, err
	}
	var t mediafiles.Title
	_ = database.Get(ctx, s.Store.Read(), "titles", id, &t)
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventTitleAdded, TitleID: &id, Summary: "Added " + t.Name,
		Data: map[string]any{"why_added": t.WhyAdded, "release_gate": t.ReleaseGate, "gate_delay_days": t.GateDelayDays,
			"gate_driver": t.GateDriver, "search_on_add": opt.SearchOnAdd}, GroupKey: "title:" + strconv.FormatInt(id, 10)})
	then := []any{KindScanTitle}
	if opt.SearchOnAdd {
		then = append(then, KindSearchSlot)
	}
	jobID, _, err := s.Queue.Enqueue(ctx, KindRefreshTitle, &domain.Subject{Type: "title", ID: id},
		map[string]any{"then": then, "new": true}, "refresh_title:"+strconv.FormatInt(id, 10), by)
	return id, jobID, err
}

// next queues the step after this one in a chain: refresh, then scan, then search.
func (s *Service) RunNextStep(ctx context.Context, r *jobs.Run, titleID int64) error {
	raw, _ := r.Job.Params["then"].([]any)
	if len(raw) == 0 {
		return nil
	}
	kind, _ := raw[0].(string)
	rest := raw[1:]
	by := JobActor(r)
	switch kind {
	case KindScanTitle:
		_, _, err := s.Queue.Enqueue(ctx, KindScanTitle, &domain.Subject{Type: "title", ID: titleID},
			map[string]any{"then": rest}, "scan_title:"+strconv.FormatInt(titleID, 10), by)
		return err
	case KindSearchSlot:
		return s.searchTitle(ctx, titleID, by)
	case StepSearchShow:
		return s.showParts().SearchShow(ctx, titleID, by)
	case stepSearchWanted:
		return s.searchWanted(ctx, titleID, by)
	}
	return fmt.Errorf("unknown next step %q", kind)
}

// searchTitle queues a search for each of the Title's Slots that is past its
// gate and looking (wanted or better_wanted). Slots not out yet are skipped.
func (s *Service) searchTitle(ctx context.Context, titleID int64, by domain.Actor) error {
	var kind string
	if err := s.Store.Read().QueryRowContext(ctx, "SELECT kind FROM titles WHERE id = ?", titleID).Scan(&kind); err != nil {
		return err
	}
	if kind == "artist" {
		return nil // albums are searched from stage 3.3
	}
	slots, err := database.Select[mediafiles.Slot](ctx, s.Store.Read(), "slots",
		"item_id IN (SELECT id FROM items WHERE title_id = ?) AND state IN ('wanted', 'better_wanted') ORDER BY id", titleID)
	if err != nil {
		return err
	}
	for _, sl := range slots {
		if _, _, err := s.Queue.Enqueue(ctx, KindSearchSlot, &domain.Subject{Type: "slot", ID: sl.ID}, nil,
			"search_slot:"+strconv.FormatInt(sl.ID, 10), by); err != nil {
			return err
		}
	}
	return nil
}

// versionFolder is a Version's folder on disk.
func versionFolder(ctx context.Context, db database.Querier, v mediafiles.Version) (string, mediafiles.LibraryFolder, error) {
	var lf mediafiles.LibraryFolder
	if err := database.Get(ctx, db, "library_folders", v.LibraryFolderID, &lf); err != nil {
		return "", lf, err
	}
	return filepath.Join(lf.Path, v.FolderName), lf, nil
}

var videoExt = map[string]bool{".mkv": true, ".mp4": true, ".m4v": true, ".avi": true, ".ts": true, ".m2ts": true, ".wmv": true, ".mov": true, ".webm": true}

// handleScan looks at each Version folder of a Title and reports what is
// there: whether the folder exists, the video files in it, and which known
// Files are present or absent. It reads only; matching unknown files to the
// Title (adopting them) is the Library scan's work.
func (s *Service) handleScan(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil || r.Job.Subject.Type != "title" {
		return errors.New("scan_title needs a title subject")
	}
	titleID := r.Job.Subject.ID
	db := s.Store.Read()
	vs, err := database.Select[mediafiles.Version](ctx, db, "versions", "title_id = ? ORDER BY id", titleID)
	if err != nil {
		return err
	}
	var out []map[string]any
	for _, v := range vs {
		dir, lf, err := versionFolder(ctx, db, v)
		if err != nil {
			return err
		}
		res := map[string]any{"version": v.Label, "folder": dir, "exists": false}
		var videos []string
		if lf.Health == "missing" {
			res["note"] = "the Library folder is missing; nothing was looked at"
		} else if st, err := os.Stat(dir); err == nil && st.IsDir() {
			res["exists"] = true
			_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
				if err == nil && d.IsDir() && p != dir && NasSystemDir(d.Name()) {
					return filepath.SkipDir
				}
				if err == nil && !d.IsDir() && videoExt[strings.ToLower(filepath.Ext(p))] {
					rel, _ := filepath.Rel(dir, p)
					videos = append(videos, filepath.ToSlash(rel))
				}
				return nil
			})
		}
		files, err := database.Select[mediafiles.File](ctx, db, "files", "version_id = ? AND state = 'present' AND role = 'main'", v.ID)
		if err != nil {
			return err
		}
		present, absent := 0, 0
		for _, f := range files {
			// A File's path is relative to its Library folder, Version folder included.
			if _, err := os.Stat(filepath.Join(lf.Path, filepath.FromSlash(f.RelativePath))); err == nil {
				present++
			} else {
				absent++
			}
		}
		res["video_files"], res["known_present"], res["known_absent"] = len(videos), present, absent
		out = append(out, res)
	}
	r.SetResult(map[string]any{"versions": out})
	s.Emit(ctx, &domain.Event{Actor: JobActor(r), Kind: domain.EventFolderScanned, TitleID: &titleID, JobID: &r.Job.ID,
		Summary: "Looked at the folder", Data: map[string]any{"versions": out}, GroupKey: "title:" + strconv.FormatInt(titleID, 10)})
	return s.RunNextStep(ctx, r, titleID)
}

// handleCheckFolders checks every Library folder and records changes.
func (s *Service) handleCheckFolders(ctx context.Context, r *jobs.Run) error {
	fs, err := database.Select[mediafiles.LibraryFolder](ctx, s.Store.Read(), "library_folders", "1=1 ORDER BY id")
	if err != nil {
		return err
	}
	paths := s.DownloadFolders(ctx)
	// A timed run only reads, so disks that sleep are not woken by probe
	// writes. Run now, and a changed Downloader folder list, run the full
	// write and hardlink test.
	scheduled, _ := r.Job.Params["scheduled"].(bool)
	byHand, _ := r.Job.Params["by_hand"].(bool)
	key := strings.Join(paths, "|")
	s.checkedMu.Lock()
	quick := scheduled && !byHand && s.checkedOnce && s.checkedPaths == key
	s.checkedMu.Unlock()
	bad := 0
	for i, f := range fs {
		_ = r.Progress(ctx, float64(i)/float64(len(fs)), "checking "+f.Label)
		lf, changed, err := RecheckFolder(ctx, s.Store, f.ID, paths, JobActor(r), quick)
		if err != nil {
			return err
		}
		if lf.Health != "ok" {
			bad++
		}
		if changed {
			s.Emit(ctx, &domain.Event{Actor: JobActor(r), Kind: domain.EventFolderHealth, JobID: &r.Job.ID,
				Summary: fmt.Sprintf("Library folder %s: %s", lf.Label, lf.Health),
				Data:    map[string]any{"library_folder_id": lf.ID, "health": lf.Health, "reason": lf.HealthReason, "was": f.Health}})
			s.Publish("library_folder", lf)
		}
	}
	if !quick {
		s.checkedMu.Lock()
		s.checkedPaths, s.checkedOnce = key, true
		s.checkedMu.Unlock()
	}
	r.SetResult(map[string]any{"checked": len(fs), "not_ok": bad})
	return nil
}

// NeverAddList returns the Never-add list, newest first.
func NeverAddList(ctx context.Context, db database.Lister) ([]mediafiles.NeverAdd, error) {
	out, err := database.Select[mediafiles.NeverAdd](ctx, db, "never_add", "1=1 ORDER BY id DESC")
	if out == nil {
		out = []mediafiles.NeverAdd{}
	}
	return out, err
}

// AddNeverAdd puts a movie, or a show when n.Kind is show, on the
// Never-add list by its TMDB id.
func AddNeverAdd(ctx context.Context, s *database.Store, n mediafiles.NeverAdd, by domain.Actor) (mediafiles.NeverAdd, error) {
	if n.SourceIDs["tmdb"] == "" {
		return n, ErrNoMetadataIDs
	}
	if n.Kind != "show" {
		n.Kind = "movie"
	}
	err := s.Write(ctx, func(tx *sql.Tx) error {
		on, err := OnNeverAdd(ctx, tx, n.SourceIDs["tmdb"])
		if n.Kind == "show" {
			on, err = showOnNeverAdd(ctx, tx, n.SourceIDs)
		}
		if err != nil || on {
			return err
		}
		_, err = database.Insert(tx, "never_add", &n, by)
		return err
	})
	return n, err
}

// RemoveNeverAdd takes one entry off the Never-add list.
func RemoveNeverAdd(ctx context.Context, s *database.Store, id int64) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec("DELETE FROM never_add WHERE id = ?", id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return database.ErrNotFound
		}
		return nil
	})
}
