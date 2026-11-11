package importer

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

type fakeRules struct {
	on       bool
	floors   bool
	minVideo int64
	minTrack int64
}

// claimedShare is the divisor for the smallest size a file may have compared with what its name claims.
const claimedShare = 10

var oldContainer = map[string]bool{".wmv": true, ".asf": true}

func (im *Importer) refuseFakes(ctx context.Context, db *sql.DB) fakeRules {
	get := func(k string) string {
		var v string
		_ = db.QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'what_to_pick' AND key = ?", k).Scan(&v)
		return v
	}
	r := fakeRules{on: get("refuse_tiny_releases") != "false", floors: get("import_size_floors") != "false" && !im.NoSizeFloors}
	if n, err := strconv.ParseInt(get("import_min_video_mb"), 10, 64); err == nil && n > 0 && n <= 10000 {
		r.minVideo = n << 20
	}
	if n, err := strconv.ParseInt(get("import_min_track_kb"), 10, 64); err == nil && n > 0 && n <= 100000 {
		r.minTrack = n << 10
	}
	return r
}

func (fr fakeRules) lengthMinutes(minutes int) int {
	if !fr.floors {
		return 0
	}
	return minutes
}

func (fr fakeRules) fakeFileOf(path string, music bool, minutes int, relName string) (string, error) {
	if !fr.on {
		return "", nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	size := fi.Size()
	if size == 0 {
		return "the file is empty; likely a fake", nil
	}
	ext := strings.ToLower(filepath.Ext(path))
	if music {
		least := fr.minTrack
		if least == 0 && fr.floors {
			least = mediafiles.MinTrack
		}
		if size < least {
			return fmt.Sprintf("the file is %d KB, too small to be a track; likely a fake", size>>10), nil
		}
		return "", nil
	}
	least := fr.minVideo
	if least == 0 && fr.floors {
		least = mediafiles.MinVideo
		if minutes > 0 {
			least = int64(minutes) * mediafiles.MinVideoPerMinute
		}
	}
	if size < least {
		return fmt.Sprintf("the file is %s, too small to be the full video; likely a fake", sizeText(size)), nil
	}
	if oldContainer[ext] && relName != "" {
		if r := releasename.Parse(relName, releasename.Hints{}).Resolution; r.Value >= 720 {
			return fmt.Sprintf("a %s file where the Release said %dp; likely a fake", ext, r.Value), nil
		}
	}
	return "", nil
}

// claimMismatch compares the Release's claimed size with the video or audio
// the download holds, measured from the files to import: a RAR set counts
// what it unpacked to, not only the sample beside it.
func (fr fakeRules) claimMismatch(claimed int64, cands []Candidate) string {
	if !fr.on || !fr.floors || claimed <= 0 || len(cands) == 0 {
		return ""
	}
	var held int64
	for _, c := range cands {
		if IsMedia(filepath.Ext(strings.ReplaceAll(c.Path, `\`, "/"))) && c.Size > 0 {
			held += c.Size
		}
	}
	if held <= 0 || held >= claimed/claimedShare {
		return ""
	}
	return fmt.Sprintf("the Release said %s but holds %s of video or audio; likely a fake", sizeText(claimed), sizeText(held))
}

func unreadable(err error) string {
	why := "could not read the file to check it"
	if os.IsNotExist(err) {
		why = "the file is not in the download folder any more"
	} else if os.IsPermission(err) {
		why = "HolyArrs may not read the file (permissions)"
	}
	return why
}

func sizeText(n int64) string {
	if n < 1<<20 {
		return fmt.Sprintf("%d KB", n>>10)
	}
	if n >= 1<<30 {
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	}
	return fmt.Sprintf("%d MB", n>>20)
}
