package importer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"syscall"
	"time"

	"github.com/CoderLuii/holyarrs/internal/archive"
	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/parse"
	"github.com/CoderLuii/holyarrs/internal/unrar"
)

// Plain reasons shown on the Slot and in the import plan.
const (
	whyPassword = "Packed in password-protected archive files; they cannot be unpacked"
	whyDamaged  = "The archive files look damaged or incomplete, so they could not be unpacked"
	whyUnsafe   = "An archive file holds a path that is not safe, so it was not unpacked"
	whyNoSpace  = "Not enough free space to unpack the archive files next to the download"
	whySafely   = "The archive files could not be unpacked safely"
	whyTooBig   = "The archive files would unpack to more than is allowed"
	whyOldRar   = "Packed in a RAR format from before RAR 1.5, which HolyArrs cannot unpack"
	whyDiskFull = "The disk filled up while the archive files were being unpacked; nothing was imported"
)

const (
	// unpackTimeout bounds a system tool run.
	unpackTimeout = 2 * time.Hour
	// unpackMargin is how far past the checked total a tool may write
	// before it is stopped.
	unpackMargin = int64(16) << 20
)

var (
	// maxUnpack is the most a RAR set may unpack to.
	maxUnpack = int64(200) << 30
)

// extractCompressed is the built-in decoder; tests replace it.
var extractCompressed = unrar.ExtractCompressed

// diskFull says a write failed because the disk is full (ENOSPC, or
// ERROR_DISK_FULL and ERROR_HANDLE_DISK_FULL on Windows).
func diskFull(err error) bool {
	var en syscall.Errno
	if !errors.As(err, &en) {
		return false
	}
	return en == syscall.ENOSPC || (runtime.GOOS == "windows" && (en == 112 || en == 39))
}

// unpackRars handles a download that holds no media but a RAR set. It
// unpacks into a folder next to the download (never inside it, so a seeding
// torrent stays whole) and returns the unpacked files as Candidates plus a
// cleanup that removes the folder; cleanup is always safe to call. The RAR
// files themselves are only read. When it cannot unpack, why says so in
// plain words. A dry run unpacks nothing.
func unpackRars(ctx context.Context, downloadID int64, root string, cands []Candidate, rules FileRules, dryRun bool) (extra []Candidate, cleanup func(), why string) {
	noop := func() {}
	if dryRun {
		return nil, noop, ""
	}
	var firsts, others []string
	for _, c := range cands {
		// A video already there means nothing to unpack, but a scene set's
		// Sample clip beside the RARs does not count.
		if IsMedia(filepath.Ext(c.Path)) {
			rel, err := filepath.Rel(root, c.Path)
			if err != nil {
				rel = filepath.Base(c.Path)
			}
			if !isSample(rel, parse.Parse(filepath.Base(c.Path), parse.Hints{Size: c.Size})) {
				return nil, noop, ""
			}
		}
		if unrar.IsVolume(c.Path) && unrar.IsFirstVolume(c.Path) {
			firsts = append(firsts, c.Path)
		} else if k, first := archive.Of(c.Path); k != "" && first {
			others = append(others, c.Path)
		}
	}
	if len(firsts) == 0 {
		if len(others) == 0 {
			return nil, noop, ""
		}
		// 7-Zip, zip or tar instead of RAR (the kinds Unpackerr reads).
		return unpackOthers(ctx, downloadID, root, others, rules)
	}
	sort.Strings(firsts)

	type set struct {
		first    string
		tool, pw bool // needs a system tool; is password protected
	}
	var sets []set
	var total int64
	for _, first := range firsts {
		entries, err := unrar.List(first)
		if err != nil {
			log.Printf("importer: reading RAR set %s: %v", first, err)
			switch {
			case errors.Is(err, unrar.ErrOldFormat):
				return nil, noop, whyOldRar
			case errors.Is(err, unrar.ErrUnsafePath):
				return nil, noop, whyUnsafe
			case errors.Is(err, unrar.ErrUnrecognised):
				return nil, noop, whySafely
			case errors.Is(err, unrar.ErrEncrypted):
				return nil, noop, whyPassword
			}
			return nil, noop, whyDamaged
		}
		s := set{first: first}
		for _, e := range entries {
			if e.Link {
				return nil, noop, whySafely
			}
			// The sizes come from the file's own headers: add them without overflow.
			if e.Size < 0 || e.Size > maxUnpack-total {
				return nil, noop, whyTooBig
			}
			total += e.Size
			s.tool = s.tool || e.Compressed || e.Encrypted
			s.pw = s.pw || e.Encrypted
		}
		sets = append(sets, s)
	}

	// Next to the torrent's own folder, not inside it.
	parent := filepath.Dir(firsts[0])
	if filepath.Clean(parent) != filepath.Clean(root) {
		parent = filepath.Dir(parent)
	}
	if !rules.SkipFreeSpaceCheck {
		if err := CheckSpaceMin(parent, total, rules.MinFreeMB); err != nil {
			return nil, noop, whyNoSpace
		}
	}
	// What a tool may write: the checked total, a small margin, and never
	// more than the disk holds beyond its floor.
	limit := total + unpackMargin
	if free := freeSpaceOf(existingDir(parent)); free >= 0 && !rules.SkipFreeSpaceCheck {
		limit = min(limit, max(free-int64(rules.MinFreeMB)*mib, 0))
	}
	tmp := filepath.Join(parent, fmt.Sprintf(".holyarrs-unpack-%d", downloadID))
	cleanup = func() { _ = os.RemoveAll(tmp) }
	fail := func(why string) ([]Candidate, func(), string) {
		cleanup()
		return nil, noop, why
	}
	_ = os.RemoveAll(tmp) // left by a run that was cut off
	for i, s := range sets {
		dest := filepath.Join(tmp, fmt.Sprintf("set%d", i))
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return fail(whyDamaged)
		}
		var err error
		switch {
		case s.pw:
			return fail(whyPassword)
		case s.tool:
			// Compressed: the built-in Go decoder is the only path (Luis: Go
			// reader only; no system unrar or 7z, the image has no tools).
			dctx, dcancel := context.WithTimeout(ctx, unpackTimeout)
			err = extractCompressed(dctx, s.first, dest, limit)
			dcancel()
		default:
			_, err = unrar.Extract(s.first, dest)
		}
		if err != nil {
			log.Printf("importer: unpacking RAR set %s: %v", s.first, err)
			switch {
			case diskFull(err):
				return fail(whyDiskFull)
			case errors.Is(err, unrar.ErrUnsafePath):
				return fail(whyUnsafe)
			case errors.Is(err, unrar.ErrEncrypted):
				return fail(whyPassword)
			case errors.Is(err, unrar.ErrUnrecognised), errors.Is(err, unrar.ErrTooBig):
				return fail(whySafely)
			}
			return fail(whyDamaged)
		}
		// Whatever wrote here, only plain files and folders may be in it.
		if err := checkTree(dest); err != nil {
			log.Printf("importer: unpacked RAR set %s: %v", s.first, err)
			return fail(whySafely)
		}
	}
	extra, err := Scan(tmp)
	if err != nil {
		return fail(whyDamaged)
	}
	return extra, cleanup, ""
}

// dirSize adds up the sizes of what is under dir, links not followed.
func dirSize(dir string) int64 {
	// A folder or file that cannot be read counts as over every limit, so the
	// watch stops the tool rather than missing what it wrote.
	var n int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			fi, err := d.Info()
			if err != nil {
				return err
			}
			n += fi.Size()
		}
		return nil
	})
	if err != nil {
		return math.MaxInt64
	}
	return n
}

// checkTree refuses a folder that holds anything but plain files and
// folders (a symlink, a junction, a device, a pipe) or a path that
// resolves outside it.
func checkTree(root string) error {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if t := d.Type(); !t.IsDir() && !t.IsRegular() {
			return fmt.Errorf("%s is not a plain file or folder", p)
		}
		if real, err := filepath.EvalSymlinks(p); err != nil || (real != realRoot && !under(realRoot, real)) {
			return fmt.Errorf("%s resolves outside the unpack folder", p)
		}
		return nil
	})
}

// unpackStep is the plan line that tells why the archives were not unpacked.
func unpackStep(root, why string) domain.PlanStep {
	return domain.PlanStep{SourceFile: root, TargetSlotIDs: []int64{}, Method: "refuse", Why: []string{why}}
}

// unpackOthers unpacks 7-Zip, zip and tar archives (internal/archive) next
// to the download, as unpackRars does a RAR set. Their unpacked size is not
// known before they are read, so the free-space check counts the archives
// themselves and the output may grow to maxUnpack, never past the disk's
// floor; the reasons are the RAR ones.
func unpackOthers(ctx context.Context, downloadID int64, root string, firsts []string, rules FileRules) ([]Candidate, func(), string) {
	noop := func() {}
	sort.Strings(firsts)
	var total int64
	for _, f := range firsts {
		if fi, err := os.Stat(f); err == nil {
			total += fi.Size()
		}
	}
	parent := filepath.Dir(firsts[0])
	if filepath.Clean(parent) != filepath.Clean(root) {
		parent = filepath.Dir(parent)
	}
	if !rules.SkipFreeSpaceCheck {
		if err := CheckSpaceMin(parent, total, rules.MinFreeMB); err != nil {
			return nil, noop, whyNoSpace
		}
	}
	limit := maxUnpack
	if free := freeSpaceOf(existingDir(parent)); free >= 0 && !rules.SkipFreeSpaceCheck {
		limit = min(limit, max(free-int64(rules.MinFreeMB)*mib, 0))
	}
	tmp := filepath.Join(parent, fmt.Sprintf(".holyarrs-unpack-%d", downloadID))
	cleanup := func() { _ = os.RemoveAll(tmp) }
	fail := func(why string) ([]Candidate, func(), string) {
		cleanup()
		return nil, noop, why
	}
	_ = os.RemoveAll(tmp)
	for i, first := range firsts {
		dest := filepath.Join(tmp, fmt.Sprintf("set%d", i))
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return fail(whyDamaged)
		}
		// One limit for the whole download: what earlier archives wrote counts.
		left := limit - dirSize(tmp)
		if left <= 0 {
			return fail(whyTooBig)
		}
		k, _ := archive.Of(first)
		actx, cancel := context.WithTimeout(ctx, unpackTimeout)
		err := extractOther(actx, first, k, dest, left)
		cancel()
		if err != nil {
			log.Printf("importer: unpacking %s: %v", first, err)
			switch {
			case diskFull(err):
				return fail(whyDiskFull)
			case errors.Is(err, unrar.ErrEncrypted):
				return fail(whyPassword)
			case errors.Is(err, unrar.ErrUnsafePath):
				return fail(whyUnsafe)
			case errors.Is(err, unrar.ErrUnrecognised), errors.Is(err, unrar.ErrTooBig):
				return fail(whySafely)
			}
			return fail(whyDamaged)
		}
		if err := checkTree(dest); err != nil {
			log.Printf("importer: unpacked %s: %v", first, err)
			return fail(whySafely)
		}
	}
	extra, err := Scan(tmp)
	if err != nil {
		return fail(whyDamaged)
	}
	return extra, cleanup, ""
}

// extractOther is archive.Extract; tests replace it.
var extractOther = archive.Extract
