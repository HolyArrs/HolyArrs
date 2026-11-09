package health

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// Folder is the outcome of checking one Library or download folder.
type Folder struct {
	Role  string // "movie library", "show library", "music library", "downloads"
	Path  string
	State string // ok, missing, read_only, no_hardlink, low_space, slow
	Owner string // uid:gid, empty when unknown
	Free  int64  // bytes, -1 when unknown
	// Reason is library.CheckFolder's reason, when it gave one.
	Reason string
	// Budget is the time limit of the run that marked the folder slow
	// (3 seconds at start-up, 20 in holyarrs doctor); 0 when unknown.
	Budget time.Duration
}

// label is the summary word for each state.
var doctorLabel = map[string]string{
	"ok":          "ok",
	"missing":     "missing",
	"read_only":   "not writable",
	"no_hardlink": "without hardlinks",
	"low_space":   "low on space",
	"slow":        "not checked (slow to answer)",
}

// Check turns the outcome into a report line with its fix.
func (f Folder) Check(uid, gid int) Check {
	text := f.Role + " " + f.Path
	var facts []string
	if f.Owner != "" {
		facts = append(facts, "owner "+f.Owner)
	}
	switch f.State {
	case "ok", "no_hardlink", "low_space":
		facts = append(facts, "writable")
		if f.Free >= 0 {
			facts = append(facts, Size(f.Free)+" free")
		}
	}
	if f.Role != "downloads" && f.State == "ok" {
		facts = append(facts, "hardlinks from downloads work")
	}
	if len(facts) > 0 {
		text += ": " + strings.Join(facts, ", ")
	}
	c := Check{Section: "Library and download folders", Status: StatusOK, Text: text}
	switch f.State {
	case "missing":
		c.Status, c.Text = StatusFail, f.Role+" "+f.Path+": does not exist inside the container"
		c.Fix = "add the volume for " + f.Path + " to the container, at the same path the Library or download client uses"
	case "read_only":
		c.Status, c.Text = StatusFail, f.Role+" "+f.Path+": not writable"
		if f.Owner != "" {
			c.Text += " (owner " + f.Owner + ")"
		}
		c.Fix = writeFix(f.Path, uid, gid, f.Owner, nil)
	case "no_hardlink":
		c.Status = StatusWarn
		c.Text += "; no hardlinks from downloads, files will be copied"
		c.Fix = "put downloads and the Library under one mount on one disk (for example /data/downloads and /data/media) so files are hardlinked, not copied"
	case "low_space":
		c.Status = StatusWarn
		c.Fix = "free up space on the disk that holds " + f.Path
	case "slow":
		limit := "in time"
		if f.Budget > 0 {
			limit = fmt.Sprintf("within %d seconds", int(f.Budget.Round(time.Second)/time.Second))
		}
		c.Status, c.Text = StatusWarn, f.Role+" "+f.Path+": did not answer "+limit+" and was skipped (often a disk waking from sleep or a slow network share)"
		c.Fix = "wake the disk or check the share, then run holyarrs doctor again"
		if f.Budget > 0 && f.Budget < doctorFolderBudget {
			c.Fix = "run holyarrs doctor to check it without the start-up time limit"
		}
	}
	return c
}

// CheckFolders checks every Library folder and download folder (savePath
// plus each Downloader's path map) with library.CheckFolder, hardlinks
// from the download folders included. The checks run side by side; one
// that is not done within budget is marked slow and left behind. db may
// be nil (only savePath is checked then).
func CheckFolders(ctx context.Context, db database.Lister, savePath string, budget time.Duration) ([]Folder, error) {
	var libs [][2]string
	downloads := []string{}
	if savePath != "" {
		downloads = append(downloads, savePath)
	}
	if db != nil {
		var err error
		if libs, err = readFolders(ctx, db); err != nil {
			return nil, err
		}
		if downloads, err = library.DownloadPaths(ctx, db, savePath); err != nil {
			return nil, err
		}
	}
	out := make([]Folder, 0, len(libs)+len(downloads))
	for _, l := range libs {
		out = append(out, Folder{Role: l[1] + " library", Path: l[0]})
	}
	for _, d := range downloads {
		out = append(out, Folder{Role: "downloads", Path: d})
	}
	type done struct {
		i int
		f Folder
	}
	ch := make(chan done, len(out))
	for i, f := range out {
		go func() {
			var links []string
			if f.Role != "downloads" {
				links = downloads
			}
			h := library.CheckFolder(f.Path, links)
			f.State, f.Reason, f.Free = h.State, h.Reason, h.FreeSpace
			if f.State == "ok" && h.Reason != "" {
				f.State = "low_space"
			}
			if st, err := os.Stat(f.Path); err == nil {
				f.Owner = ownerOf(st)
			}
			ch <- done{i, f}
		}()
	}
	timer := time.NewTimer(budget)
	defer timer.Stop()
	got := make([]bool, len(out))
	for n := 0; n < len(out); n++ {
		select {
		case d := <-ch:
			out[d.i], got[d.i] = d.f, true
		case <-timer.C:
			n = len(out)
		case <-ctx.Done():
			n = len(out)
		}
	}
	for i := range out {
		if !got[i] {
			out[i].State, out[i].Free, out[i].Budget = "slow", -1, budget
		}
	}
	return out, nil
}

// Summary is the one start-up line, such as "folders: 4 ok, 1 not writable".
func Summary(fs []Folder) string {
	order := []string{"ok", "missing", "read_only", "no_hardlink", "low_space", "slow"}
	n := map[string]int{}
	for _, f := range fs {
		n[f.State]++
	}
	var parts []string
	for _, s := range order {
		if n[s] > 0 || s == "ok" {
			parts = append(parts, fmt.Sprintf("%d %s", n[s], doctorLabel[s]))
		}
	}
	return "folders: " + strings.Join(parts, ", ")
}

// Startup checks the folders within budget and returns the summary line
// and one warning line per problem folder, each with its fix.
func DoctorStartup(ctx context.Context, db database.Lister, savePath string, budget time.Duration) (string, []string) {
	fs, err := CheckFolders(ctx, db, savePath, budget)
	if err != nil {
		return "folders: not checked: " + err.Error(), nil
	}
	uid, gid := os.Geteuid(), os.Getegid()
	var warns []string
	for _, f := range fs {
		if f.State == "ok" {
			continue
		}
		c := f.Check(uid, gid)
		line := "WARN " + c.Text
		if c.Fix != "" {
			line += ": " + c.Fix
		}
		warns = append(warns, line)
	}
	return Summary(fs), warns
}

// UnsafeFolderWarnings returns one warning per saved Library folder that is
// a filesystem or drive root or overlaps the config folder configDir. New
// folders like these are refused; this catches rows saved before that.
func UnsafeFolderWarnings(ctx context.Context, db database.Lister, configDir string) []string {
	if db == nil {
		return nil
	}
	libs, err := readFolders(ctx, db)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range libs {
		if library.UnsafeFolder(l[0], configDir) {
			out = append(out, l[1]+" library "+l[0]+": is a whole drive or overlaps the config folder "+configDir+"; move the Library to its own folder")
		}
	}
	return out
}
