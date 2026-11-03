package importer

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
)

// unpackTimeout bounds a system tool run.
const unpackTimeout = 2 * time.Hour

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
			case errors.Is(err, unrar.ErrEncrypted):
				return nil, noop, whyPassword
			}
			return nil, noop, whyDamaged
		}
		s := set{first: first}
		for _, e := range entries {
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
			if err = extractWithTool(ctx, s.first, dest); errors.Is(err, errNoTool) {
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
			case s.tool:
				return fail(whyToolFails)
			}
			return fail(whyDamaged)
		}
	}
	extra, err := Scan(tmp)
	if err != nil {
		return fail(whyDamaged)
	}
	return extra, cleanup, ""
}

// extractWithTool unpacks first into dest with a system unrar or 7z, run
// without a shell, with a time limit.
func extractWithTool(ctx context.Context, first, dest string) error {
	ctx, cancel := context.WithTimeout(ctx, unpackTimeout)
	defer cancel()
	if p, err := lookTool("unrar"); err == nil {
		return runTool(ctx, p, []string{"x", "-y", "-o+", "-p-", "-idq", "--", first, dest + string(filepath.Separator)})
	}
	for _, n := range []string{"7z", "7zz", "7za"} {
		if p, err := lookTool(n); err == nil {
			return runTool(ctx, p, []string{"x", "-y", "-bd", "-aoa", "-o" + dest, "--", first})
		}
	}
	return errNoTool
}

// unpackStep is the plan line that tells why the RAR files were not unpacked.
func unpackStep(root, why string) domain.PlanStep {
	return domain.PlanStep{SourceFile: root, TargetSlotIDs: []int64{}, Method: "refuse", Why: []string{why}}
}
