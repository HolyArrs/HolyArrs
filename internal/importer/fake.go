package importer

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/judge"
	"github.com/CoderLuii/holyarrs/internal/parse"
)

// fakeRules are the import-time fake checks (4.3.5). With
// refuse_tiny_releases on, an empty file is always a fake. With
// import_size_floors on (the default), a file under the judge's fixed
// floors (1 MB a minute of runtime, 20 MB for a video of unknown length,
// 512 KB a track) is a fake, and so is a download whose media files
// together hold far less than the size its Release was judged at.
// import_min_video_mb and import_min_track_kb, when above 0, replace the
// fixed floor for a video or a track (to raise or lower it).
type fakeRules struct {
	on       bool  // refuse_tiny_releases
	floors   bool  // import_size_floors
	minVideo int64 // replaces the per-minute video floor when > 0
	minTrack int64 // replaces the track floor when > 0
}

// claimedShare: media holding less than this share of the judged size is a fake.
const claimedShare = 10

// oldContainer are video containers no modern HD Release comes in; a file
// in one under a Release name that says 720p or more is a known fake.
var oldContainer = map[string]bool{".wmv": true, ".asf": true}

// refuseFakes reads the settings: refuse_tiny_releases and
// import_size_floors (on unless "false"), import_min_video_mb and
// import_min_track_kb.
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

// fakeFileOf says why a file to be placed is a fake: far too small for a
// video of minutes length (0: unknown) or a track, or a video in an old
// container under an HD Release name. "" means it may be placed. A file
// that cannot be read returns the error: that is never the Release's fault.
func (fr fakeRules) fakeFileOf(path string, music bool, minutes int, relName string) (string, error) {
	if !fr.on {
		return "", nil
	}
	// The file on disk, not the size the Downloader listed.
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
			least = judge.MinTrack
		}
		if size < least {
			return fmt.Sprintf("the file is %d KB, too small to be a track; likely a fake", size>>10), nil
		}
		return "", nil
	}
	least := fr.minVideo
	if least == 0 && fr.floors {
		least = judge.MinVideo
		if minutes > 0 {
			least = int64(minutes) * judge.MinVideoPerMinute
		}
	}
	if size < least {
		return fmt.Sprintf("the file is %s, too small to be the full video; likely a fake", sizeText(size)), nil
	}
	if oldContainer[ext] && relName != "" {
		if r := parse.Parse(relName, parse.Hints{}).Resolution; r.Value >= 720 {
			return fmt.Sprintf("a %s file where the Release said %dp; likely a fake", ext, r.Value), nil
		}
	}
	return "", nil
}

// claimMismatch says why a download is a fake when its video and audio
// files (as the Downloader listed them) hold far less than the size its
// Release was judged at: a Release said to be 4 GB that holds a 3 MB
// video. "" means the sizes agree well enough or are not known.
func (fr fakeRules) claimMismatch(claimed int64, seen []domain.SeenFile) string {
	if !fr.on || !fr.floors || claimed <= 0 || len(seen) == 0 {
		return ""
	}
	var held int64
	for _, f := range seen {
		if IsMedia(filepath.Ext(strings.ReplaceAll(f.Name, `\`, "/"))) && f.Size > 0 {
			held += f.Size
		}
	}
	if held <= 0 || held >= claimed/claimedShare {
		return ""
	}
	return fmt.Sprintf("the Release said %s but holds %s of video or audio; likely a fake", sizeText(claimed), sizeText(held))
}

// unreadable is the plan reason for a file whose size could not be read:
// the import waits for a person, the Release is not blamed.
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
