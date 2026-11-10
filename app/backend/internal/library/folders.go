package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// MinFreeSpace is the free space under which a Library folder gets a
// warning, and the least a copy must leave free.
const MinFreeSpace = importer.MinFreeSpace

// Health is the outcome of checking one Library folder.
type Health struct {
	State     string `json:"health"` // ok, missing, read_only, no_hardlink
	Reason    string `json:"reason"`
	FreeSpace int64  `json:"free_space"` // bytes, -1 when unknown
}

// Usable reports whether files can be placed in the folder at all. A folder
// without hardlinks still works: files are copied.
func (h Health) Usable() bool { return h.State == "ok" || h.State == "no_hardlink" }

// CheckFolder looks at a Library folder: it must exist, take writes, and
// take hardlinks from each Downloader folder in downloadPaths. Free space
// under MinFreeSpace adds a warning. Its probe files are removed again.
func CheckFolder(path string, downloadPaths []string) Health {
	h := Health{State: "ok", FreeSpace: -1}
	st, err := os.Stat(path)
	if err != nil || !st.IsDir() {
		h.State, h.Reason = "missing", "the folder does not exist or is not a folder"
		return h
	}
	probe, err := os.CreateTemp(path, ".holyarrs-check-*")
	if err != nil {
		h.State, h.Reason = "read_only", "HolyArrs cannot write to the folder"
		return h
	}
	_ = probe.Close()
	_ = os.Remove(probe.Name())
	var reasons []string
	for _, dp := range downloadPaths {
		if dp == "" {
			continue
		}
		if ok, why := canLink(dp, path); !ok {
			h.State = "no_hardlink"
			reasons = append(reasons, why)
		}
	}
	h.FreeSpace = importer.FreeSpace(path)
	if h.FreeSpace >= 0 && h.FreeSpace < MinFreeSpace {
		reasons = append(reasons, lowSpaceNote)
	}
	h.Reason = strings.Join(reasons, "; ")
	return h
}

// canLink tries a hardlink from a probe file in from into to.
func canLink(from, to string) (bool, string) {
	src, err := os.CreateTemp(from, ".holyarrs-link-*")
	if err != nil {
		return false, fmt.Sprintf("cannot write in the Downloader folder %s to test hardlinks", from)
	}
	_ = src.Close()
	defer os.Remove(src.Name())
	dst := filepath.Join(to, filepath.Base(src.Name()))
	if err := os.Link(src.Name(), dst); err != nil {
		return false, fmt.Sprintf("no hardlinks from %s (files will be copied)", from)
	}
	_ = os.Remove(dst)
	return true, ""
}

// DownloadPaths lists the folders Downloaders save into, as HolyArrs sees
// them: extra (the save path set at start) plus every path map's own side.
func DownloadPaths(ctx context.Context, db database.Lister, extra ...string) ([]string, error) {
	dls, err := database.Select[domain.Downloader](ctx, db, "downloaders", "state != 'disabled'")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, p := range extra {
		add(p)
	}
	for _, d := range dls {
		for _, m := range d.PathMap {
			add(m.Ours)
		}
	}
	return out, nil
}

// Folder errors.
var (
	ErrFolderPath   = errors.New("the path must be an absolute folder path, not a whole drive and not inside or around the config folder")
	ErrFolderKind   = errors.New("kind must be movie, show or music")
	ErrFolderTaken  = errors.New("that folder, or one inside or around it, is already a Library folder")
	ErrFolderInUse  = errors.New("titles still use this Library folder; move them first")
	ErrFolderName   = errors.New("another Library folder of this kind already has that name")
	ErrFolderAbsent = errors.New("the folder does not exist")
	ErrNoPickRules  = errors.New("no Pick rules with that id")
)

// FolderInput is what a person sets on a Library folder.
type FolderInput struct {
	Path             string `json:"path"`
	Kind             string `json:"kind"`
	Label            string `json:"label"`
	DefaultPickRules *int64 `json:"default_pick_rules_id"`
}

// FullPath reports whether p is a full folder path HolyArrs can use: a
// Linux container path (/data/media/movies, on any host) or a full path of
// the host it runs on. A path Git Bash mangled ("C:/Program Files/Git/data/...",
// made when a container is set up without MSYS_NO_PATHCONV=1) never is.
func FullPath(p string) bool {
	slash := strings.ToLower(filepath.ToSlash(p))
	if len(slash) > 2 && slash[1] == ':' && strings.HasPrefix(slash[2:], "/program files/git/") {
		return false
	}
	if path.IsAbs(filepath.ToSlash(p)) && filepath.VolumeName(p) == "" {
		return true
	}
	return filepath.IsAbs(p)
}

// UnsafeFolder reports whether p, symlinks followed, is a filesystem or
// drive root or overlaps the data folder dataDir. AddFolder refuses such a
// path; the doctor warns about rows saved before this check.
func UnsafeFolder(p, dataDir string) bool { return importer.UnsafeFolder(p, dataDir) }

func overlaps(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if a == b {
		return true
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(a, strings.TrimSuffix(b, sep)+sep) || strings.HasPrefix(b, strings.TrimSuffix(a, sep)+sep)
}

// AddFolder adds a Library folder after checking it. then, when given,
// runs in the same transaction with the new id.
func AddFolder(ctx context.Context, s *database.Store, in FolderInput, downloadPaths []string, by domain.Actor, then ...func(tx *sql.Tx, id int64) error) (mediafiles.LibraryFolder, error) {
	var lf mediafiles.LibraryFolder
	if in.Path == "" || !filepath.IsAbs(in.Path) {
		return lf, ErrFolderPath
	}
	in.Path = filepath.Clean(in.Path)
	if UnsafeFolder(in.Path, s.Dir()) {
		return lf, ErrFolderPath
	}
	if in.Kind != "movie" && in.Kind != "show" && in.Kind != "music" {
		return lf, ErrFolderKind
	}
	h := CheckFolder(in.Path, downloadPaths)
	if h.State == "missing" {
		return lf, ErrFolderAbsent
	}
	if in.Label == "" {
		in.Label = filepath.Base(in.Path)
	}
	now := time.Now().UTC()
	lf = mediafiles.LibraryFolder{Path: in.Path, Kind: in.Kind, Label: in.Label, DefaultPickRules: in.DefaultPickRules,
		FreeSpace: max(h.FreeSpace, 0), Health: h.State, HealthReason: h.Reason, HealthCheckedAt: &now, SameDiskAs: []int64{}}
	err := s.Write(ctx, func(tx *sql.Tx) error {
		all, err := database.Select[mediafiles.LibraryFolder](ctx, tx, "library_folders", "")
		if err != nil {
			return err
		}
		for _, o := range all {
			if overlaps(o.Path, in.Path) {
				return ErrFolderTaken
			}
		}
		// Names are unique per kind, so a title's Folder menu can tell them apart.
		// A new folder whose name is taken gets a number; renaming to a taken name is refused.
		lf.Label, _ = freeLabel(all, in.Kind, in.Label, 0, true)
		if err := pickRulesExist(tx, in.DefaultPickRules); err != nil {
			return err
		}
		id, err := database.Insert(tx, "library_folders", &lf, by)
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
	return lf, err
}

// ErrNotVideoRules says music Pick rules were given for a movie or a show:
// they have no video steps, so they could never pick a copy.
var ErrNotVideoRules = errors.New("these Pick rules are for music; choose Pick rules for movies and shows")

// pickRulesFor checks that Pick rules exist and fit a title kind: music
// rules for an artist, movie and show rules for every other title.
func PickRulesFor(tx *sql.Tx, id *int64, titleKind string) error {
	if err := pickRulesExist(tx, id); err != nil || id == nil {
		return err
	}
	music := musicRulesTx(tx, *id)
	switch {
	case titleKind == "artist" && !music:
		return ErrNotMusicRules
	case titleKind != "artist" && music:
		return ErrNotVideoRules
	}
	return nil
}

func pickRulesExist(tx *sql.Tx, id *int64) error {
	if id == nil {
		return nil
	}
	var n int
	if err := tx.QueryRow("SELECT count(*) FROM pick_rules WHERE id = ?", *id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNoPickRules
	}
	return nil
}

// FolderChange is an edit of a Library folder. Its path and kind stay;
// moving Titles is a separate, dry-run-first Job.
type FolderChange struct {
	Label            *string `json:"label"`
	DefaultPickRules *int64  `json:"default_pick_rules_id"`
	CopiesExpected   *bool   `json:"copies_expected"`
}

// UpdateFolder changes a Library folder's label, default Pick rules and
// whether copying (no hardlinks) is expected there.
func UpdateFolder(ctx context.Context, s *database.Store, id int64, c FolderChange, by domain.Actor) (mediafiles.LibraryFolder, error) {
	var lf mediafiles.LibraryFolder
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "library_folders", id, &lf); err != nil {
			return err
		}
		if c.Label != nil && *c.Label != "" && *c.Label != lf.Label {
			all, err := database.Select[mediafiles.LibraryFolder](ctx, tx, "library_folders", "")
			if err != nil {
				return err
			}
			if _, ok := freeLabel(all, lf.Kind, *c.Label, id, false); !ok {
				return ErrFolderName
			}
			lf.Label = *c.Label
		}
		if c.DefaultPickRules != nil {
			if err := pickRulesExist(tx, c.DefaultPickRules); err != nil {
				return err
			}
			lf.DefaultPickRules = c.DefaultPickRules
		}
		if c.CopiesExpected != nil {
			lf.CopiesExpected = *c.CopiesExpected
		}
		return database.Update(tx, "library_folders", &lf, by)
	})
	return lf, err
}

// freeLabel says whether label is free among the folders of kind (skipping
// the folder with id skip). With number, a taken label gets " 2", " 3" and so on.
func freeLabel(all []mediafiles.LibraryFolder, kind, label string, skip int64, number bool) (string, bool) {
	taken := func(l string) bool {
		for _, o := range all {
			if o.ID != skip && o.Kind == kind && strings.EqualFold(o.Label, l) {
				return true
			}
		}
		return false
	}
	if !taken(label) {
		return label, true
	}
	if !number {
		return "", false
	}
	for n := 2; n < 1000; n++ {
		if l := fmt.Sprintf("%s %d", label, n); !taken(l) {
			return l, true
		}
	}
	return label, true
}

// DeleteFolder removes a Library folder no Version uses. Nothing on disk is touched.
func DeleteFolder(ctx context.Context, s *database.Store, id int64) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow("SELECT count(*) FROM versions WHERE library_folder_id = ?", id).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrFolderInUse
		}
		res, err := tx.Exec("DELETE FROM library_folders WHERE id = ?", id)
		if err != nil {
			return err
		}
		if k, _ := res.RowsAffected(); k == 0 {
			return database.ErrNotFound
		}
		return nil
	})
}

// RecheckFolder checks one Library folder and stores the outcome. changed is
// true when its health or reason moved. quick only reads: a folder last
// found ok or without hardlinks keeps that health and its "last checked"
// time, and only its free space is read again (no probe files, so a
// sleeping disk is not written to). A folder never checked, or last found
// missing or not writable, gets the full test anyway.
func RecheckFolder(ctx context.Context, s *database.Store, id int64, downloadPaths []string, by domain.Actor, quick bool) (lf mediafiles.LibraryFolder, changed bool, err error) {
	if err := database.Get(ctx, s.Read(), "library_folders", id, &lf); err != nil {
		return lf, false, err
	}
	if quick && lf.HealthCheckedAt != nil && (lf.Health == "ok" || lf.Health == "no_hardlink") {
		if st, serr := os.Stat(lf.Path); serr == nil && st.IsDir() {
			free := importer.FreeSpace(lf.Path)
			reason := freeSpaceReason(lf.HealthReason, free)
			changed = reason != lf.HealthReason
			now := time.Now().UTC()
			err = s.Write(ctx, func(tx *sql.Tx) error {
				_, err := tx.Exec(`UPDATE library_folders SET health_reason = ?, free_space = ?, updated_at = ?, updated_by = ? WHERE id = ?`,
					reason, max(free, 0), database.FormatTime(now), ActorText(by), id)
				return err
			})
			lf.HealthReason, lf.FreeSpace = reason, max(free, 0)
			return lf, changed, err
		}
	}
	h := CheckFolder(lf.Path, downloadPaths)
	changed = h.State != lf.Health || h.Reason != lf.HealthReason
	now := time.Now().UTC()
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE library_folders SET health = ?, health_reason = ?, free_space = ?, health_checked_at = ?,
			updated_at = ?, updated_by = ? WHERE id = ?`,
			h.State, h.Reason, max(h.FreeSpace, 0), database.FormatTime(now), database.FormatTime(now), ActorText(by), id)
		return err
	})
	lf.Health, lf.HealthReason, lf.FreeSpace, lf.HealthCheckedAt = h.State, h.Reason, max(h.FreeSpace, 0), &now
	return lf, changed, err
}

// lowSpaceNote is the reason part CheckFolder adds under MinFreeSpace.
const lowSpaceNote = "under 100 MB free"

// freeSpaceReason is reason with the low-space part added or taken out for
// free bytes (-1: unknown, left out).
func freeSpaceReason(reason string, free int64) string {
	var parts []string
	for _, p := range strings.Split(reason, "; ") {
		if p != "" && p != lowSpaceNote {
			parts = append(parts, p)
		}
	}
	if free >= 0 && free < MinFreeSpace {
		parts = append(parts, lowSpaceNote)
	}
	return strings.Join(parts, "; ")
}

func ActorText(a domain.Actor) string {
	b, _ := json.Marshal(a)
	return string(b)
}
