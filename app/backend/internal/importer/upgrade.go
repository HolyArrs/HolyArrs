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

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// HoldingDir is the folder name where replaced files wait before removal.
const HoldingDir = ".holyarrs-replaced"

type upgrade struct {
	rules   mediafiles.PickRulesBody
	old     *mediafiles.File
	oldName string
	relName string
	lie     string
	facts   map[string]map[string]any
	copyWhy string
}

func plainStep(s string) string { return strings.ReplaceAll(s, "_", " ") }

func checkUpgrade(ctx context.Context, db *sql.DB, sl mediafiles.Slot, ver mediafiles.Version, d domain.Download, plan []mediafiles.PlanStep, fakes fakeRules, minutes int) (upgrade, error) {
	u := upgrade{facts: map[string]map[string]any{}}
	var pr mediafiles.PickRules
	if err := database.Get(ctx, db, "pick_rules", ver.PickRulesID, &pr); err != nil {
		return u, err
	}
	u.rules = pr.Body
	if d.ReleaseID != nil {
		_ = db.QueryRowContext(ctx, "SELECT name FROM releases WHERE id = ?", *d.ReleaseID).Scan(&u.relName)
	}
	if sl.FileID != nil {
		var f mediafiles.File
		if err := database.Get(ctx, db, "files", *sl.FileID, &f); err == nil && f.State == "present" {
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
		if sampleByLength(facts, fakes.lengthMinutes(minutes)) {
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

func lieOf(name, relName string, hasOld bool, oldName string, rules mediafiles.PickRulesBody, facts map[string]any) string {
	fStep, fIdx := mediafiles.StepOf(name, rules)
	fp := releasename.Parse(name, releasename.Hints{}).Resolution
	shown := mediafiles.Info{Width: factInt(facts, "width"), Height: factInt(facts, "height"), DisplayWidth: factInt(facts, "display_width")}
	if real := shown.Resolution(); real > 0 {
		claim := fp
		if relName != "" {
			if rp := releasename.Parse(relName, releasename.Hints{}).Resolution; rp.Conf == releasename.Sure && rp.Value > 0 {
				claim = rp
			}
		}
		if claim.Conf == releasename.Sure && claim.Value > real {
			return fmt.Sprintf("file is %dp, name said %dp", real, claim.Value)
		}
	} else if relName != "" {
		rp := releasename.Parse(relName, releasename.Hints{}).Resolution
		fr, rr := fp.Value, rp.Value
		if fp.Conf == releasename.Sure && rp.Conf == releasename.Sure && fr > 0 && rr > fr {
			return fmt.Sprintf("file is %dp, name said %dp", fr, rr)
		}
	}
	sure := releasename.Parse(name, releasename.Hints{}).Source.Conf == releasename.Sure
	if fIdx >= 0 && sure && hasOld {
		if oStep, oIdx := mediafiles.StepOf(oldName, rules); oIdx > fIdx {
			return fmt.Sprintf("file is %s, lower than the copy in place (%s)", plainStep(fStep), plainStep(oStep))
		}
	}
	return ""
}

func lossyOverLossless(newPath, relName string, old mediafiles.File, oldPath string, rules mediafiles.PickRulesBody) string {
	ext := strings.ToLower(filepath.Ext(newPath))
	newKind := audioKind(ext, newPath)
	if newKind == "lossless" {
		return ""
	}
	oldKind := audioKind(strings.ToLower(path.Ext(old.RelativePath)), oldPath)
	switch oldKind {
	case "unknown":
		oldKind = "lossless"
	case "":
		if step, _ := mediafiles.StepOf(fileName(old), rules); step == "lossless" || step == "lossless_hires" {
			oldKind = "lossless"
		}
	}
	if oldKind != "lossless" {
		return ""
	}
	if newKind == "unknown" {
		return fmt.Sprintf("the format of the file (%s) could not be read, so it may not replace the lossless copy in place", strings.TrimPrefix(ext, "."))
	}
	if newKind == "" {
		if step, _ := mediafiles.StepOf(relName, rules); !strings.HasPrefix(step, "lossy_") {
			return ""
		}
	}
	return fmt.Sprintf("file is lossy (%s), lower than the lossless copy in place", strings.TrimPrefix(ext, "."))
}

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

func fileLossless(p string) (lossless, known bool) {
	info, err := mediafiles.Inspect(p)
	if err != nil {
		return false, false
	}
	return info.Lossless()
}

var (
	lossyExts    = map[string]bool{".mp3": true, ".mp2": true, ".aac": true, ".ogg": true, ".oga": true, ".opus": true, ".wma": true, ".mpc": true}
	losslessExts = map[string]bool{".flac": true, ".wav": true, ".aiff": true, ".aif": true, ".ape": true, ".wv": true, ".dsf": true, ".dff": true, ".alac": true, ".tak": true}
)

var renameFile = os.Rename

var crashAt func(stage string) error

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

// HoldPath returns where a replaced file is held.
func HoldPath(libPath, rel string, now time.Time) (string, error) { return holdPath(libPath, rel, now) }

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

func (im *Importer) reject(ctx context.Context, imp *mediafiles.Import, sl mediafiles.Slot, d domain.Download, why string, by domain.Actor,
	hasFile, keepLooking bool) (mediafiles.SlotState, error) {
	to := mediafiles.SlotRejected
	if hasFile {
		to = mediafiles.SlotInLibrary
	}
	imp.State, imp.Mode = "rejected", "auto"
	err := im.Store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := database.Insert(tx, "imports", imp, by); err != nil {
			return err
		}
		if d.ReleaseID != nil {
			if _, err := tx.Exec(`UPDATE releases SET state = 'turned_down', verdict = json_set(COALESCE(verdict, '{}'), '$.turned_down_reason', ?),
				updated_at = ?, updated_by = ? WHERE id = ?`, why, database.FormatTime(time.Now()), actorText(by), *d.ReleaseID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("UPDATE slots SET active_download_id = NULL WHERE id = ?", sl.ID); err != nil {
			return err
		}
		if sl.State == mediafiles.SlotDownloading {
			if err := mediafiles.MoveSlotTx(tx, sl.ID, mediafiles.SlotImporting, "importing", by); err != nil {
				return err
			}
		}
		if err := mediafiles.MoveSlotTx(tx, sl.ID, to, why, by); err != nil {
			return err
		}
		if hasFile && keepLooking && !sl.GoodEnough {
			to = mediafiles.SlotBetterWanted
			return mediafiles.MoveSlotTx(tx, sl.ID, to, why+"; the copy in place stays and a better one is sought", by)
		}
		return nil
	})
	return to, err
}

func actorText(a domain.Actor) string {
	b, _ := json.Marshal(a)
	return string(b)
}

// InsideReal reports whether p is inside root after links are resolved.
func InsideReal(root, p string) bool { return insideReal(root, p) }
