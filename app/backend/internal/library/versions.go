package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"golang.org/x/text/language"
)

// Errors from changing Versions.
var (
	ErrVersionLabel    = errors.New("a Version needs a label of at most 40 characters that the title does not use yet")
	ErrMainVersion     = errors.New("the main Version cannot be removed")
	ErrVersionHasFiles = errors.New("this Version still has files; remove them first")
	ErrTooManyVersions = errors.New("at most 5 more Versions at once")
	ErrVersionBusy     = errors.New("this Version has a download or import under way; wait for it to finish or stop it first")
	// ErrVersionFolder says the folder a Version would get is another
	// title's folder in the same Library folder.
	ErrVersionFolder = errors.New("another title in that Library folder already uses the folder this Version would get; choose another label or Library folder")
)

// VersionInput is one more wanted copy of a title, such as "4K". Zero
// values take the main Version's Pick rules and Library folder.
type VersionInput struct {
	Label           string   `json:"label"`
	FolderName      string   `json:"folder_name,omitempty"`
	PickRulesID     int64    `json:"pick_rules_id"`
	LibraryFolderID int64    `json:"library_folder_id"`
	KeepUpToDate    *bool    `json:"keep_up_to_date"`
	Edition         string   `json:"edition"`
	AudioLanguages  []string `json:"audio_languages"`
}

// VersionChange is an edit of one Version.
type VersionChange struct {
	Label          *string   `json:"label"`
	PickRulesID    *int64    `json:"pick_rules_id"`
	KeepUpToDate   *bool     `json:"keep_up_to_date"`
	Edition        *string   `json:"edition"`
	AudioLanguages *[]string `json:"audio_languages"`
}

// cleanLabel checks a Version label, which also names a folder ("<show>
// [label]"): at most 40 bytes of plain text, no path or wildcard
// characters, no brackets, no control, direction or zero-width
// characters, no trailing dot, and not a name Windows keeps for devices.
func CleanLabel(l string) (string, error) {
	l = strings.TrimSpace(l)
	if l == "" || len(l) > 40 || !utf8.ValidString(l) || strings.ContainsAny(l, `/\:*?"<>|[]`) || strings.HasSuffix(l, ".") {
		return "", ErrVersionLabel
	}
	for _, r := range l {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			// Cf holds the direction marks and overrides and the zero-width characters.
			return "", ErrVersionLabel
		}
	}
	if reservedName(l) {
		return "", ErrVersionLabel
	}
	return l, nil
}

// reservedName is a device name Windows keeps (CON, NUL, COM1, LPT1, ...),
// also with an extension.
func reservedName(l string) bool {
	base, _, _ := strings.Cut(strings.ToUpper(l), ".")
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9'
}

// labelTaken reports whether the title has another Version with this label (any case).
func labelTaken(tx *sql.Tx, titleID, exceptID int64, label string) (bool, error) {
	var n int
	err := tx.QueryRow("SELECT count(*) FROM versions WHERE title_id = ? AND id != ? AND lower(label) = lower(?)", titleID, exceptID, label).Scan(&n)
	return n > 0, err
}

// checkVersionFolder finds and checks a Library folder for a new Version.
// A show's Version needs a show Library folder; every other title a movie one.
func CheckVersionFolder(ctx context.Context, db database.Querier, id int64, titleKind string, downloadPaths []string) (mediafiles.LibraryFolder, error) {
	var lf mediafiles.LibraryFolder
	if err := database.Get(ctx, db, "library_folders", id, &lf); errors.Is(err, database.ErrNotFound) {
		return lf, ErrNoFolder
	} else if err != nil {
		return lf, err
	}
	if titleKind == "show" {
		if lf.Kind != "show" {
			return lf, ErrNotShowFolder
		}
	} else if lf.Kind != "movie" {
		return lf, ErrNotMovieFolder
	}
	if h := CheckFolder(lf.Path, downloadPaths); !h.Usable() {
		return lf, &FolderError{Health: h}
	}
	return lf, nil
}

// addVersionTx adds a Version and one Slot per Item of the title. The Slot
// state follows the title's Release gate. When another Version of the title
// already sits in the same Library folder, the folder name gets the label in
// brackets so the copies never share a folder.
func AddVersionTx(ctx context.Context, tx *sql.Tx, t mediafiles.Title, in VersionInput, now time.Time, by domain.Actor) (mediafiles.Version, error) {
	var v mediafiles.Version
	label, err := CleanLabel(in.Label)
	if err != nil {
		return v, err
	}
	if taken, err := labelTaken(tx, t.ID, 0, label); err != nil {
		return v, err
	} else if taken {
		return v, ErrVersionLabel
	}
	mains, err := database.Select[mediafiles.Version](ctx, tx, "versions", "title_id = ? ORDER BY is_main DESC, id LIMIT 1", t.ID)
	if err != nil {
		return v, err
	}
	if len(mains) == 0 {
		return v, database.ErrNotFound
	}
	main := mains[0]
	edition, audio, err := copyChoices(t.Kind, in.Edition, in.AudioLanguages)
	if err != nil {
		return v, fmt.Errorf("%w: %v", ErrCopyChoice, err)
	}
	if in.PickRulesID == 0 {
		in.PickRulesID = main.PickRulesID
	} else if err := PickRulesFor(tx, &in.PickRulesID, t.Kind); err != nil {
		return v, err
	}
	if in.LibraryFolderID == 0 {
		in.LibraryFolderID = main.LibraryFolderID
	}
	keep := t.KeepUpToDate
	if in.KeepUpToDate != nil {
		keep = *in.KeepUpToDate
	}
	folder := main.FolderName
	var shared int
	if err := tx.QueryRow("SELECT count(*) FROM versions WHERE title_id = ? AND library_folder_id = ?", t.ID, in.LibraryFolderID).Scan(&shared); err != nil {
		return v, err
	}
	if shared > 0 {
		folder += " [" + label + "]"
	}
	if in.FolderName != "" {
		folder = strings.TrimSpace(in.FolderName)
		if !filepath.IsLocal(folder) || strings.ContainsAny(folder, `/\:*?"<>|`) || strings.HasPrefix(folder, ".") || strings.HasSuffix(folder, ".") || folder != in.FolderName || len(folder) > 200 || reservedName(folder) || strings.IndexFunc(folder, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) >= 0 {
			return v, ErrVersionFolder
		}
	}
	// Every Version gets a separate folder within a Library folder.
	var clash int
	if err := tx.QueryRow("SELECT count(*) FROM versions WHERE library_folder_id = ? AND lower(folder_name) = lower(?)",
		in.LibraryFolderID, folder).Scan(&clash); err != nil {
		return v, err
	}
	if clash > 0 {
		return v, ErrVersionFolder
	}
	v = mediafiles.Version{TitleID: t.ID, Label: label, PickRulesID: in.PickRulesID, LibraryFolderID: in.LibraryFolderID,
		FolderName: folder, KeepUpToDate: keep, Edition: edition, AudioLanguages: audio}
	if _, err := database.Insert(tx, "versions", &v, by); err != nil {
		return v, err
	}
	items, err := database.Select[mediafiles.Item](ctx, tx, "items", "title_id = ? ORDER BY id", t.ID)
	if err != nil {
		return v, err
	}
	state := mediafiles.SlotNotOutYet
	if GateOpen(t.GateDriver, t.GateOpensAt, now) {
		state = mediafiles.SlotWanted
	}
	reason := gateReason(t.GateDriver, t.GateOpensAt)
	var wait time.Duration
	if t.Kind == "show" {
		defs, err := ReadShowDefaults(ctx, tx)
		if err != nil {
			return v, err
		}
		wait = time.Duration(defs.WaitMinutes) * time.Minute
	}
	for _, it := range items {
		if it.Kind == "episode" {
			// Each episode opens on its own air date, as in the main Version.
			open, r := EpisodeGate(it.AirDate, wait, now)
			state, reason = mediafiles.SlotNotOutYet, r
			if open {
				state = mediafiles.SlotWanted
			}
		}
		sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: state, LastReason: &reason}
		if _, err := database.Insert(tx, "slots", sl, by); err != nil {
			return v, err
		}
	}
	return v, nil
}

// AddVersion adds one more wanted copy (for example 4K) to a title that is
// already in the Library.
func AddVersion(ctx context.Context, s *database.Store, titleID int64, in VersionInput, downloadPaths []string, now time.Time, by domain.Actor) (mediafiles.Version, error) {
	unlock := s.LockFiles()
	defer unlock()
	var v mediafiles.Version
	if _, err := CleanLabel(in.Label); err != nil {
		return v, err
	}
	if in.LibraryFolderID != 0 {
		var kind string
		if err := s.Read().QueryRowContext(ctx, "SELECT kind FROM titles WHERE id = ?", titleID).Scan(&kind); errors.Is(err, sql.ErrNoRows) {
			return v, database.ErrNotFound
		} else if err != nil {
			return v, err
		}
		if _, err := CheckVersionFolder(ctx, s.Read(), in.LibraryFolderID, kind, downloadPaths); err != nil {
			return v, err
		}
	}
	err := s.Write(ctx, func(tx *sql.Tx) error {
		var t mediafiles.Title
		if err := database.Get(ctx, tx, "titles", titleID, &t); err != nil {
			return err
		}
		if t.State == "removed" {
			return ErrAlreadyRemoved
		}
		var err error
		v, err = AddVersionTx(ctx, tx, t, in, now, by)
		return err
	})
	return v, err
}

// versionOf reads a Version and checks it belongs to the title.
func versionOf(ctx context.Context, q database.Querier, titleID, versionID int64) (mediafiles.Version, error) {
	var v mediafiles.Version
	if err := database.Get(ctx, q, "versions", versionID, &v); err != nil {
		return v, err
	}
	if v.TitleID != titleID {
		return v, database.ErrNotFound
	}
	return v, nil
}

// EditVersion changes one Version's label, Pick rules or Keep up to date.
// Moving a Version to another Library folder is the move Job's work.
func EditVersion(ctx context.Context, s *database.Store, titleID, versionID int64, c VersionChange, by domain.Actor) (mediafiles.Version, error) {
	unlock := s.LockFiles()
	defer unlock()
	var v mediafiles.Version
	err := s.Write(ctx, func(tx *sql.Tx) error {
		var err error
		if v, err = versionOf(ctx, tx, titleID, versionID); err != nil {
			return err
		}
		var t mediafiles.Title
		if err := database.Get(ctx, tx, "titles", titleID, &t); err != nil {
			return err
		}
		if t.State == "removed" {
			return ErrAlreadyRemoved
		}
		if c.Label != nil {
			l, err := CleanLabel(*c.Label)
			if err != nil {
				return err
			}
			if taken, err := labelTaken(tx, titleID, v.ID, l); err != nil {
				return err
			} else if taken {
				return ErrVersionLabel
			}
			v.Label = l
		}
		if c.PickRulesID != nil {
			if err := PickRulesFor(tx, c.PickRulesID, t.Kind); err != nil {
				return err
			}
			v.PickRulesID = *c.PickRulesID
		}
		if c.KeepUpToDate != nil {
			v.KeepUpToDate = *c.KeepUpToDate
		}
		if c.Edition != nil || c.AudioLanguages != nil {
			edition, audio := v.Edition, v.AudioLanguages
			if c.Edition != nil {
				edition = *c.Edition
			}
			if c.AudioLanguages != nil {
				audio = *c.AudioLanguages
			}
			var err error
			if v.Edition, v.AudioLanguages, err = copyChoices(t.Kind, edition, audio); err != nil {
				return fmt.Errorf("%w: %v", ErrCopyChoice, err)
			}
		}
		if err := database.Update(tx, "versions", &v, by); err != nil {
			return err
		}
		_, err = goodEnoughTx(ctx, tx, v, t.KeepUpToDate && v.KeepUpToDate, by)
		return err
	})
	return v, err
}

// RemoveVersion drops a Version that is not the main one, holds no file
// that is present or missing (only gone or replaced ones) and has no Slot
// with a download or import under way, nor a finished download still
// waiting to be imported (also one waiting in Needs a decision); its Slots
// go with it.
func RemoveVersion(ctx context.Context, s *database.Store, titleID, versionID int64, by domain.Actor) error {
	unlock := s.LockFiles()
	defer unlock()
	return s.Write(ctx, func(tx *sql.Tx) error {
		v, err := versionOf(ctx, tx, titleID, versionID)
		if err != nil {
			return err
		}
		if v.IsMain {
			return ErrMainVersion
		}
		var busy int
		err = tx.QueryRow(`SELECT count(*) FROM slots sl WHERE sl.version_id = ? AND (sl.state IN ('chosen', 'downloading', 'stalled', 'importing')
			OR EXISTS (SELECT 1 FROM downloads d, json_each(d.slot_ids) j WHERE j.value = sl.id
				AND (d.state IN ('sent', 'queued', 'downloading', 'stalled', 'paused')
					OR d.state IN ('finished', 'seeding') AND NOT EXISTS (SELECT 1 FROM imports im WHERE im.download_id = d.id AND im.state IN ('done', 'rejected')))))`, v.ID).Scan(&busy)
		if err != nil {
			return err
		}
		if busy > 0 {
			return ErrVersionBusy
		}
		var n int
		if err := tx.QueryRow("SELECT count(*) FROM files WHERE version_id = ? AND state IN ('present', 'missing')", v.ID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrVersionHasFiles
		}
		if _, err := tx.Exec("DELETE FROM files WHERE version_id = ?", v.ID); err != nil {
			return err
		}
		_, err = tx.Exec("DELETE FROM versions WHERE id = ?", v.ID)
		return err
	})
}

// Clock is the service's time now.
func (s *Service) Clock() time.Time { return s.now() }

// copyChoices validates explicit copy requirements. No choice is inferred for
// an existing Version. Base language codes match inspected stream metadata.
func copyChoices(kind, edition string, audio []string) (string, []string, error) {
	edition = strings.TrimSpace(edition)
	if edition != "" {
		if kind != "movie" {
			return "", nil, errors.New("editions apply to movie copies")
		}
		if _, err := CleanLabel(edition); err != nil || strings.ContainsAny(edition, "{}") {
			return "", nil, errors.New("edition needs a plain label of at most 40 characters")
		}
	}
	if len(audio) > 16 || len(audio) > 0 && kind != "movie" && kind != "show" {
		return "", nil, errors.New("choose up to 16 movie or show audio languages")
	}
	out := []string{}
	seen := map[string]bool{}
	for _, code := range audio {
		code = strings.TrimSpace(code)
		if len(code) < 2 || len(code) > 3 || strings.IndexFunc(code, func(r rune) bool { return r < 'A' || r > 'Z' && r < 'a' || r > 'z' }) >= 0 {
			return "", nil, errors.New("audio languages need base language codes, such as en or ja")
		}
		tag, err := language.Parse(strings.TrimSpace(code))
		base, confidence := tag.Base()
		if err != nil || tag.String() == "und" || base.String() == "und" || confidence == language.No || len(code) > 35 {
			return "", nil, errors.New("audio languages need recognized language codes, such as en or ja")
		}
		code = strings.ToLower(base.String())
		if !seen[code] {
			out = append(out, code)
			seen[code] = true
		}
	}
	return edition, out, nil
}

// CheckAudioLanguages checks audio languages wanted on a movie or show
// before anything is added (the same rules a Version edit uses).
func CheckAudioLanguages(kind string, audio []string) error {
	if _, _, err := copyChoices(kind, "", audio); err != nil {
		return fmt.Errorf("%w: %v", ErrCopyChoice, err)
	}
	return nil
}

// VersionEvent records a change to a title's Versions in History.
func (s *Service) VersionEvent(ctx context.Context, t mediafiles.Title, summary string, v mediafiles.Version, by domain.Actor) {
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventVersionChanged, TitleID: &t.ID, Summary: summary + " of " + t.Name,
		Data:     map[string]any{"version_id": v.ID, "label": v.Label, "pick_rules_id": v.PickRulesID, "library_folder_id": v.LibraryFolderID},
		GroupKey: GroupKey(t.ID)})
}
