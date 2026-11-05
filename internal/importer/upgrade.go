package importer

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

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/judge"
	"github.com/CoderLuii/holyarrs/internal/mediainfo"
	"github.com/CoderLuii/holyarrs/internal/parse"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// HoldingDir is the folder inside each Library folder where a replaced
// File is moved (M1 choice 8: kept there, not deleted at once). Clearing it
// after 7 days belongs to the clean-up job of stage 5.
const HoldingDir = ".holyarrs-replaced"

// upgrade is what an Import knows about the File it may replace and about
// the Release's name.
type upgrade struct {
	rules   domain.PickRulesBody
	old     *domain.File // the File in place, when there is one
	oldName string
	relName string
	lie     string // why the files are refused: lower than the name said, or than the File in place
	facts   map[string]map[string]any
	copyWhy string
}

func plainStep(s string) string { return strings.ReplaceAll(s, "_", " ") }

// checkUpgrade compares each main file of the plan with the Release name
// and with the File in place (pick-rules.md 4.3, never downgrade). A file
// whose own name parses to a lower step than either is refused.
// minutes is the movie's runtime (0: unknown) for the size floors.
func checkUpgrade(ctx context.Context, db *sql.DB, sl domain.Slot, ver domain.Version, d domain.Download, plan []domain.PlanStep, fakes fakeRules, minutes int) (upgrade, error) {
	u := upgrade{facts: map[string]map[string]any{}}
	var pr domain.PickRules
	if err := store.Get(ctx, db, "pick_rules", ver.PickRulesID, &pr); err != nil {
		return u, err
	}
	u.rules = pr.Body
	if d.ReleaseID != nil {
		_ = db.QueryRowContext(ctx, "SELECT name FROM releases WHERE id = ?", *d.ReleaseID).Scan(&u.relName)
	}
	if sl.FileID != nil {
		var f domain.File
		if err := store.Get(ctx, db, "files", *sl.FileID, &f); err == nil && f.State == "present" {
			u.old = &f
			u.oldName = strings.TrimSuffix(filepath.Base(f.RelativePath), filepath.Ext(f.RelativePath))
			if f.ReleaseName != nil && *f.ReleaseName != "" {
				u.oldName = *f.ReleaseName
			}
		}
	}
	var claimed int64
	if d.ReleaseID != nil {
		_ = db.QueryRowContext(ctx, "SELECT COALESCE(size, 0) FROM releases WHERE id = ?", *d.ReleaseID).Scan(&claimed)
	}
	mismatch := fakes.claimMismatch(claimed, d.FilesSeen)
	for i := range plan {
		if plan[i].Method != "hardlink" {
			continue
		}
		if u.old != nil && u.old.Protected {
			u.copyWhy = "the copy in place is protected; choose another Version for an upgrade"
			plan[i].Method, plan[i].TargetPath = "refuse", ""
			plan[i].Why = append(plan[i].Why, u.copyWhy)
			continue
		}
		facts := InspectFacts(plan[i].SourceFile)
		u.facts[plan[i].SourceFile] = facts
		if sampleByLength(facts, minutes) {
			plan[i].Method, plan[i].TargetPath = "refuse", ""
			plan[i].Why = append(plan[i].Why, "a sample, not the full video (it plays "+lengthWords(factInt(facts, "runtime_seconds"))+")")
			continue
		}
		why := copyRefusal(ver, filepath.Base(plan[i].SourceFile), u.relName, d.ConfirmedEdition, facts)
		if why == "" && u.old != nil {
			if edition, _ := u.old.MediaInfo["edition"].(string); edition != "" && EditionKey(edition) != EditionKey(fmt.Sprint(facts["edition"])) {
				why = "the new edition differs from the copy in place; choose another Version"
			}
		}
		if why != "" {
			u.copyWhy = why
			plan[i].Method, plan[i].TargetPath = "refuse", ""
			plan[i].Why = append(plan[i].Why, why)
			continue
		}
		why, err := fakes.fakeFileOf(plan[i].SourceFile, false, minutes, u.relName)
		if err != nil {
			// Not the Release's fault: the import waits for a person.
			plan[i].Method, plan[i].TargetPath = "refuse", ""
			plan[i].Why = append(plan[i].Why, unreadable(err))
			continue
		}
		if why == "" {
			why = mismatch
		}
		if why != "" {
			u.lie = why
			plan[i].Method, plan[i].TargetPath = "refuse", ""
			plan[i].Why = append(plan[i].Why, why)
			continue
		}
		name := strings.TrimSuffix(filepath.Base(plan[i].SourceFile), filepath.Ext(plan[i].SourceFile))
		why = lieOf(name, u.relName, u.old != nil, u.oldName, u.rules, facts)
		if why != "" {
			u.lie = why
			plan[i].Method, plan[i].TargetPath = "refuse", ""
			plan[i].Why = append(plan[i].Why, why)
			continue
		}
		if u.old != nil {
			id := u.old.ID
			plan[i].ReplacesFileID = &id
		}
	}
	return u, nil
}

// lieOf says why a file is refused: its own name parses to a lower
// resolution than the Release name said, or to a lower step than the copy
// in place (oldName, when hasOld). "" means it may be placed.
func lieOf(name, relName string, hasOld bool, oldName string, rules domain.PickRulesBody, facts map[string]any) string {
	fStep, fIdx := judge.StepOf(name, rules)
	fp := parse.Parse(name, parse.Hints{}).Resolution
	if real := (mediainfo.Info{Width: factInt(facts, "width"), Height: factInt(facts, "height")}).Resolution(); real > 0 {
		// The picture size read from the file beats any name: a 720p file
		// named 1080p is caught. The Release name's claim counts first,
		// else the file's own name.
		claim := fp
		if relName != "" {
			if rp := parse.Parse(relName, parse.Hints{}).Resolution; rp.Conf == parse.Sure && rp.Value > 0 {
				claim = rp
			}
		}
		if claim.Conf == parse.Sure && claim.Value > real {
			return fmt.Sprintf("file is %dp, name said %dp", real, claim.Value)
		}
	} else if relName != "" {
		// The resolution in the file's own name against the Release name's.
		rp := parse.Parse(relName, parse.Hints{}).Resolution
		fr, rr := fp.Value, rp.Value
		if fp.Conf == parse.Sure && rp.Conf == parse.Sure && fr > 0 && rr > fr {
			return fmt.Sprintf("file is %dp, name said %dp", fr, rr)
		}
	}
	// Only a file name that parses with confidence is compared with the
	// copy in place; a plainer name than its Release is not a lie.
	sure := parse.Parse(name, parse.Hints{}).Source.Conf == parse.Sure
	if fIdx >= 0 && sure && hasOld {
		if oStep, oIdx := judge.StepOf(oldName, rules); oIdx > fIdx {
			return fmt.Sprintf("file is %s, lower than the copy in place (%s)", plainStep(fStep), plainStep(oStep))
		}
	}
	return ""
}

// lossyOverLossless says why a track file may not replace the copy in
// place: a lossy file over a lossless copy (#43), whatever its Release
// said. Only the kinds of file are compared, by extension (and the copy in
// place also by its Release name): bit rates in bare track names are not
// known well enough to rank one lossy copy over another.
func lossyOverLossless(newPath, relName string, old domain.File, oldPath string, rules domain.PickRulesBody) string {
	ext := strings.ToLower(filepath.Ext(newPath))
	newKind := audioKind(ext, newPath)
	if newKind == "lossless" {
		return ""
	}
	oldKind := audioKind(strings.ToLower(path.Ext(old.RelativePath)), oldPath)
	switch oldKind {
	case "unknown":
		// An .m4a/.mka whose codec cannot be read counts as lossless: its
		// stored name ("[M4A]" from an older adopt) may only be its
		// extension, so it is never replaced by what may be lossy.
		oldKind = "lossless"
	case "":
		// Any other file: its stored name says.
		if step, _ := judge.StepOf(fileName(old), rules); step == "lossless" || step == "lossless_hires" {
			oldKind = "lossless"
		}
	}
	if oldKind != "lossless" {
		return ""
	}
	if newKind == "unknown" {
		// Over a lossless copy the file itself must say it is lossless: a
		// Release name may lie (#43).
		return fmt.Sprintf("the format of the file (%s) could not be read, so it may not replace the lossless copy in place", strings.TrimPrefix(ext, "."))
	}
	if newKind == "" {
		// A plain track file (no known kind): only its Release name says.
		if step, _ := judge.StepOf(relName, rules); !strings.HasPrefix(step, "lossy_") {
			return ""
		}
	}
	return fmt.Sprintf("file is lossy (%s), lower than the lossless copy in place", strings.TrimPrefix(ext, "."))
}

// audioKind is "lossless" or "lossy" for a file by its extension, or for
// an .m4a/.mka/.mp4 by the codec read from it ("unknown" when it cannot be
// read); "" for any other file.
func audioKind(ext, p string) string {
	switch {
	case losslessExts[ext]:
		return "lossless"
	case lossyExts[ext]:
		return "lossy"
	case ext == ".m4a" || ext == ".mka" || ext == ".mp4":
		if p == "" {
			return "unknown"
		}
		if l, known := fileLossless(p); known {
			if l {
				return "lossless"
			}
			return "lossy"
		}
		return "unknown"
	}
	return ""
}

// fileLossless reads an audio file's codec from its headers: known is false
// when it could not be read.
func fileLossless(p string) (lossless, known bool) {
	info, err := mediainfo.Inspect(p)
	if err != nil {
		return false, false
	}
	return info.Lossless()
}

var (
	lossyExts    = map[string]bool{".mp3": true, ".mp2": true, ".aac": true, ".ogg": true, ".oga": true, ".opus": true, ".wma": true, ".mpc": true}
	losslessExts = map[string]bool{".flac": true, ".wav": true, ".aiff": true, ".aif": true, ".ape": true, ".wv": true, ".dsf": true, ".dff": true, ".alac": true, ".tak": true}
)

// renameFile is os.Rename; tests replace it to force the copy fallback.
var renameFile = os.Rename

// crashAt lets tests stop an Import at a stage ("held", "placed") as if the
// process died there.
var crashAt func(stage string) error

// holdPath picks a free place in the Library folder's holding folder for
// rel. Both ends must stay inside the Library folder.
func holdPath(libPath, rel string, now time.Time) (string, error) {
	from := filepath.Join(libPath, filepath.FromSlash(rel))
	to := filepath.Join(libPath, HoldingDir, now.UTC().Format("2006-01-02"), filepath.FromSlash(rel))
	if !under(libPath, from) || !under(filepath.Join(libPath, HoldingDir), to) ||
		!insideReal(libPath, filepath.Dir(from)) || !insideReal(libPath, filepath.Dir(to)) {
		return "", fmt.Errorf("%q is outside the Library folder", rel)
	}
	base := to
	for n := 1; ; n++ {
		if _, err := os.Lstat(to); errors.Is(err, os.ErrNotExist) {
			return to, nil
		}
		to = fmt.Sprintf("%s.%d", base, n)
	}
}

// HoldPath is a free place for rel in the Library folder's holding folder,
// in the day folder of now. Both ends stay inside the Library folder.
func HoldPath(libPath, rel string, now time.Time) (string, error) { return holdPath(libPath, rel, now) }

// moveFile renames from to to; when that fails (another disk) it copies
// and then removes the original.
func moveFile(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o777); err != nil {
		return err
	}
	err := renameFile(from, to)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return err
	}
	if cerr := copyFile(from, to); cerr != nil {
		return fmt.Errorf("rename failed (%v), copy failed (%v)", err, cerr)
	}
	return os.Remove(from)
}

// holdAside moves a File into the Library folder's holding folder and
// returns where it went ("" when it was already gone).
func holdAside(libPath, rel string, now time.Time) (string, error) {
	to, err := holdPath(libPath, rel, now)
	if err != nil {
		return "", err
	}
	if err := moveFile(filepath.Join(libPath, filepath.FromSlash(rel)), to); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return to, nil
}

// reject ends an Import whose files lied: the Import is rejected, the
// Release turned down with the reason. A Slot without a File is rejected;
// one whose File stays in place goes back to in_library, and on to
// better_wanted when that File is below good enough and kept up to date.
// It returns the Slot's new state.
func (im *Importer) reject(ctx context.Context, imp *domain.Import, sl domain.Slot, d domain.Download, why string, by domain.Actor,
	hasFile, keepLooking bool) (domain.SlotState, error) {
	to := domain.SlotRejected
	if hasFile {
		to = domain.SlotInLibrary
	}
	imp.State, imp.Mode = "rejected", "auto"
	err := im.Store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := store.Insert(tx, "imports", imp, by); err != nil {
			return err
		}
		if d.ReleaseID != nil {
			if _, err := tx.Exec(`UPDATE releases SET state = 'turned_down', verdict = json_set(COALESCE(verdict, '{}'), '$.turned_down_reason', ?),
				updated_at = ?, updated_by = ? WHERE id = ?`, why, store.FormatTime(time.Now()), actorText(by), *d.ReleaseID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("UPDATE slots SET active_download_id = NULL WHERE id = ?", sl.ID); err != nil {
			return err
		}
		if sl.State == domain.SlotDownloading {
			if err := store.MoveSlotTx(tx, sl.ID, domain.SlotImporting, "importing", by); err != nil {
				return err
			}
		}
		if err := store.MoveSlotTx(tx, sl.ID, to, why, by); err != nil {
			return err
		}
		if hasFile && keepLooking && !sl.GoodEnough {
			to = domain.SlotBetterWanted
			return store.MoveSlotTx(tx, sl.ID, to, why+"; the copy in place stays and a better one is sought", by)
		}
		return nil
	})
	return to, err
}

func actorText(a domain.Actor) string {
	b, _ := json.Marshal(a)
	return string(b)
}

// InsideReal reports whether p stays inside root's real path once the
// symlinks in the part of p that exists are followed (root itself counts).
func InsideReal(root, p string) bool { return insideReal(root, p) }
