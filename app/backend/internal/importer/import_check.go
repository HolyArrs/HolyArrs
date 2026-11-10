package importer

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func guardPlanWith(plan []mediafiles.PlanStep, lib string, cands []Candidate, rules FileRules, base map[string]int64) (*SpaceError, map[string]int64) {
	var lowSpace *SpaceError
	reserved := map[string]int64{}
	mine := map[string]int64{}
	for k, n := range base {
		reserved[k] = n
	}
	for i := range plan {
		if plan[i].TargetPath != "" && (!under(lib, plan[i].TargetPath) || !insideReal(lib, filepath.Dir(plan[i].TargetPath))) {
			plan[i].Method, plan[i].TargetPath = "refuse", ""
			plan[i].Why = append(plan[i].Why, "the target would be outside the Library folder")
		}
		if plan[i].Method == "hardlink" {
			if why := PathTooLong(plan[i].TargetPath); why != "" {
				plan[i].Method, plan[i].TargetPath = "refuse", ""
				plan[i].Why = append([]string{why}, plan[i].Why...)
			}
		}
		if _, isLink := LinkedFile(plan[i].SourceFile); plan[i].Method == "hardlink" && !rules.SkipFreeSpaceCheck && !isLink {
			dir := existingDir(filepath.Dir(plan[i].TargetPath))
			key := diskID(dir)
			if key == "" {
				key = dir
			}
			size := sizeOf(cands, plan[i].SourceFile)
			if err := checkSpaceReserved(dir, size, reserved[key], rules.MinFreeMB); err != nil {
				errors.As(err, &lowSpace)
				plan[i].Method, plan[i].TargetPath = "refuse", ""
				plan[i].Why = append([]string{err.Error()}, plan[i].Why...)
				continue
			}
			if !sameDiskOf(plan[i].SourceFile, dir) {
				reserved[key] += size
				mine[key] += size
			}
		}
	}
	return lowSpace, mine
}

func (im *Importer) guard(plan []mediafiles.PlanStep, lib string, cands []Candidate, rules FileRules) (*SpaceError, func()) {
	im.spaceMu.Lock()
	defer im.spaceMu.Unlock()
	if im.booked == nil {
		im.booked = map[string]int64{}
	}
	low, mine := guardPlanWith(plan, lib, cands, rules, im.booked)
	for k, n := range mine {
		im.booked[k] += n
	}
	return low, func() {
		im.spaceMu.Lock()
		defer im.spaceMu.Unlock()
		for k, n := range mine {
			if im.booked[k] -= n; im.booked[k] <= 0 {
				delete(im.booked, k)
			}
		}
	}
}

var sameDiskOf = sameDisk

func sameDisk(src, dir string) bool {
	a, b := diskID(src), diskID(dir)
	return a != "" && a == b
}

// MaxPathLen is the longest path the system allows.
var MaxPathLen = func() int {
	if runtime.GOOS == "windows" {
		return 259
	}
	return 4095
}()

// PathTooLong returns a message when p is longer than MaxPathLen, or an empty string.
func PathTooLong(p string) string {
	if len(p) > MaxPathLen {
		return fmt.Sprintf("the new path would be too long (%d characters, the limit here is %d)", len(p), MaxPathLen)
	}
	return ""
}

func planPaths(plan []mediafiles.PlanStep) []map[string]string {
	out := []map[string]string{}
	for _, st := range plan {
		out = append(out, map[string]string{"from": st.SourceFile, "to": st.TargetPath, "method": st.Method})
	}
	return out
}

func placedSome(rs []mediafiles.PlaceResult) bool {
	for _, r := range rs {
		if r.Outcome == "placed" {
			return true
		}
	}
	return false
}

func hasMain(plan []mediafiles.PlanStep) bool {
	for _, st := range plan {
		if st.Method == "hardlink" {
			return true
		}
	}
	return false
}

func (im *Importer) lowSpace(ctx context.Context, se *SpaceError, slotID, titleID, dlID int64, by domain.Actor) {
	e := &domain.Event{Actor: by, Kind: domain.EventFreeSpaceLow, TitleID: &titleID, SlotID: &slotID, DownloadID: &dlID, Summary: "Import refused: " + se.Error(),
		Data: map[string]any{"folder": se.Dir, "free": se.Free, "need": se.Need, "keep": se.Keep}, GroupKey: "slot:" + strconv.FormatInt(slotID, 10)}
	if im.Store.AddEvent(ctx, e) == nil {
		im.publish("event", e)
	}
	if im.OnLowSpace != nil {
		im.OnLowSpace(ctx, se.Dir, se.Free, se.Need)
	}
}
