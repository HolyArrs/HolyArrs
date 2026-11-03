package importer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/unrar"
)

// Plain reasons shown on the Slot and in the import plan.
const (
	whyNeedTool  = "Packed in compressed RAR files; install unrar to unpack them"
	whyPassword  = "Packed in password-protected RAR files; they cannot be unpacked"
	whyDamaged   = "The RAR files look damaged or incomplete, so they could not be unpacked"
	whyUnsafe    = "A RAR file holds a path that is not safe, so it was not unpacked"
	whyNoSpace   = "Not enough free space to unpack the RAR files next to the download"
	whyToolFails = "The RAR files could not be unpacked by the system tool"
	whySafely    = "The RAR files could not be unpacked safely"
	whyTooBig    = "The RAR files would unpack to more than is allowed"
)

const (
	// unpackTimeout bounds a system tool run.
	unpackTimeout = 2 * time.Hour
	// unpackMargin is how far past the checked total a tool may write
	// before it is stopped.
	unpackMargin = int64(16) << 20
)

var (
	// unpackPoll is how often the folder a tool writes into is measured.
	unpackPoll = time.Second
	// maxUnpack is the most a RAR set may unpack to.
	maxUnpack = int64(200) << 30
)

// lookTool and runTool are the system tool lookup and run; tests replace them.
var (
	lookTool = exec.LookPath
	runTool  = func(ctx context.Context, name string, args []string) error {
		return exec.CommandContext(ctx, name, args...).Run()
	}
)

var errNoTool = errors.New("no unrar or 7z on this system")

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
	var firsts []string
	for _, c := range cands {
		if IsMedia(filepath.Ext(c.Path)) {
			return nil, noop, ""
		}
		if unrar.IsVolume(c.Path) && unrar.IsFirstVolume(c.Path) {
			firsts = append(firsts, c.Path)
		}
	}
	if len(firsts) == 0 {
		return nil, noop, ""
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
			if err = extractWithTool(ctx, s.first, dest, limit); errors.Is(err, errNoTool) {
				return fail(whyNeedTool)
			}
		default:
			_, err = unrar.Extract(s.first, dest)
		}
		if err != nil {
			log.Printf("importer: unpacking RAR set %s: %v", s.first, err)
			switch {
			case errors.Is(err, unrar.ErrUnsafePath):
				return fail(whyUnsafe)
			case errors.Is(err, unrar.ErrUnrecognised), errors.Is(err, errTooMuch):
				return fail(whySafely)
			case s.tool:
				return fail(whyToolFails)
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

var errTooMuch = errors.New("the tool wrote more than the set was checked to hold")

// extractWithTool unpacks first into dest with a system unrar or 7z, run
// without a shell, with a time limit. Links are not asked for (no -ol for
// unrar, no -snl for 7z) and nothing is overwritten. While it runs the
// size of dest is measured, and the tool is stopped once it passes limit.
func extractWithTool(ctx context.Context, first, dest string, limit int64) error {
	var name string
	var args []string
	if p, err := lookTool("unrar"); err == nil {
		name, args = p, []string{"x", "-y", "-o-", "-p-", "-idq", "--", first, dest + string(filepath.Separator)}
	}
	for _, n := range []string{"7z", "7zz", "7za"} {
		if p, err := lookTool(n); name == "" && err == nil {
			name, args = p, []string{"x", "-y", "-bd", "-aos", "-o" + dest, "--", first}
		}
	}
	if name == "" {
		return errNoTool
	}
	ctx, cancel := context.WithTimeout(ctx, unpackTimeout)
	defer cancel()
	done := make(chan struct{})
	var over atomic.Bool
	go func() {
		tick := time.NewTicker(unpackPoll)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				if dirSize(dest) > limit {
					over.Store(true)
					cancel()
					return
				}
			}
		}
	}()
	err := runTool(ctx, name, args)
	close(done)
	if over.Load() || dirSize(dest) > limit {
		return errTooMuch
	}
	return err
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

// unpackStep is the plan line that tells why the RAR files were not unpacked.
func unpackStep(root, why string) domain.PlanStep {
	return domain.PlanStep{SourceFile: root, TargetSlotIDs: []int64{}, Method: "refuse", Why: []string{why}}
}
