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

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer/archive"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer/unrar"
)

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
	unpackTimeout = 2 * time.Hour
	// unpackMargin is spare room kept free on top of the unpacked size.
	unpackMargin = int64(16) << 20
)

var (
	// maxUnpack refuses archives that would unpack to more than 200 GiB, which guards against archive bombs.
	maxUnpack = int64(200) << 30
)

var extractCompressed = unrar.ExtractCompressed

func diskFull(err error) bool {
	var en syscall.Errno
	if !errors.As(err, &en) {
		return false
	}
	return en == syscall.ENOSPC || (runtime.GOOS == "windows" && (en == 112 || en == 39))
}

func unpackRars(ctx context.Context, downloadID int64, root string, cands []Candidate, rules FileRules, dryRun bool) (extra []Candidate, cleanup func(), why string) {
	noop := func() {}
	if dryRun {
		return nil, noop, ""
	}
	var firsts, others []string
	for _, c := range cands {
		if IsMedia(filepath.Ext(c.Path)) {
			rel, err := filepath.Rel(root, c.Path)
			if err != nil {
				rel = filepath.Base(c.Path)
			}
			if !isSample(rel, releasename.Parse(filepath.Base(c.Path), releasename.Hints{Size: c.Size})) {
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
		return unpackOthers(ctx, downloadID, root, others, rules)
	}
	sort.Strings(firsts)

	type set struct {
		first    string
		tool, pw bool
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
			if e.Size < 0 || e.Size > maxUnpack-total {
				return nil, noop, whyTooBig
			}
			total += e.Size
			s.tool = s.tool || e.Compressed || e.Encrypted
			s.pw = s.pw || e.Encrypted
		}
		sets = append(sets, s)
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
	limit := total + unpackMargin
	whyLimit := whyTooBig
	if free := freeSpaceOf(existingDir(parent)); free >= 0 && !rules.SkipFreeSpaceCheck {
		if room := max(free-int64(rules.MinFreeMB)*mib, 0); room < limit {
			limit, whyLimit = room, whyNoSpace
		}
	}
	tmp := filepath.Join(parent, fmt.Sprintf(".holyarrs-unpack-%d", downloadID))
	cleanup = func() { _ = os.RemoveAll(tmp) }
	fail := func(why string) ([]Candidate, func(), string) {
		cleanup()
		return nil, noop, why
	}
	_ = os.RemoveAll(tmp)
	for i, s := range sets {
		dest := filepath.Join(tmp, fmt.Sprintf("set%d", i))
		if err := os.MkdirAll(dest, 0o777); err != nil {
			return fail(whyDamaged)
		}
		var err error
		switch {
		case s.pw:
			return fail(whyPassword)
		case s.tool:
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
			case errors.Is(err, unrar.ErrTooBig):
				return fail(whyLimit)
			case errors.Is(err, unrar.ErrUnrecognised):
				return fail(whySafely)
			}
			return fail(whyDamaged)
		}
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

func dirSize(dir string) int64 {
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

func unpackStep(root, why string) mediafiles.PlanStep {
	return mediafiles.PlanStep{SourceFile: root, TargetSlotIDs: []int64{}, Method: "refuse", Why: []string{why}}
}

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
	whyLimit := whyTooBig
	if free := freeSpaceOf(existingDir(parent)); free >= 0 && !rules.SkipFreeSpaceCheck {
		if room := max(free-int64(rules.MinFreeMB)*mib, 0); room < limit {
			limit, whyLimit = room, whyNoSpace
		}
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
		if err := os.MkdirAll(dest, 0o777); err != nil {
			return fail(whyDamaged)
		}
		left := limit - dirSize(tmp)
		if left <= 0 {
			return fail(whyLimit)
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
			case errors.Is(err, unrar.ErrTooBig):
				return fail(whyLimit)
			case errors.Is(err, unrar.ErrUnrecognised):
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

var extractOther = archive.Extract
