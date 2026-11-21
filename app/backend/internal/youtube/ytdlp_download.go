package youtube

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// progressMark starts each progress line yt-dlp prints for HolyArrs.
const progressMark = "HAYT "

// FormatFor is the yt-dlp format for a height cap: H.264 with AAC first so
// every browser plays the file, else the best video at or below the cap with
// AAC, else with any audio, and only then a single combined file.
func FormatFor(maxHeight int) string {
	if maxHeight <= 0 {
		maxHeight = 1080
	}
	h := "[height<=" + strconv.Itoa(maxHeight) + "]"
	return "bv*[vcodec^=avc1]" + h + "+ba[acodec^=mp4a]" +
		"/bv*" + h + "+ba[acodec^=mp4a]" +
		"/bv*" + h + "+ba" +
		"/b" + h
}

// Download fetches one video as MP4 with a JPEG thumbnail beside it, named
// outBase plus the extension, and returns the video's path. progress, when
// not nil, hears each progress line. A run that prints nothing for Stall is
// stopped. A failed run leaves no partial files behind.
func (r Runner) Download(ctx context.Context, videoID string, maxHeight int, outBase string, progress func(Progress)) (string, error) {
	if !reVideoID.MatchString(videoID) {
		return "", ErrNotYouTube
	}
	stall := r.Stall
	if stall <= 0 {
		stall = defaultStall
	}
	final := ""
	err := r.run(ctx, 0, stall, downloadArgs(r.Tools, videoID, maxHeight, outBase), func(line string, stderr bool) {
		if p, ok := parseProgress(line); ok {
			if progress != nil {
				progress(p)
			}
			return
		}
		if t := strings.TrimSpace(line); !stderr && filepath.IsAbs(t) {
			final = t
		}
	})
	if err != nil {
		removePartials(outBase)
		return "", err
	}
	if final == "" {
		final = outBase + ".mp4"
	}
	if _, err := os.Stat(final); err != nil {
		return "", errors.New("yt-dlp finished without a video file")
	}
	return final, nil
}

// downloadArgs are the arguments after the common ones. The output name is
// a yt-dlp template, so a "%" in a title is doubled.
func downloadArgs(t Tools, videoID string, maxHeight int, outBase string) []string {
	return []string{
		"-f", FormatFor(maxHeight),
		"--merge-output-format", "mp4",
		"--remux-video", "mp4",
		"--write-thumbnail", "--convert-thumbnails", "jpg",
		"--no-playlist",
		// --print makes yt-dlp quiet; --progress keeps the progress lines.
		"--progress",
		"--progress-template", "download:" + progressMark + "%(progress.downloaded_bytes)s %(progress.total_bytes)s %(progress.total_bytes_estimate)s",
		"--print", "after_move:filepath",
		"-o", strings.ReplaceAll(outBase, "%", "%%") + ".%(ext)s",
		"--", watchURL(videoID),
	}
}

// parseProgress reads "HAYT <done> <total> <estimate>"; yt-dlp prints NA for
// an unknown number.
func parseProgress(line string) (Progress, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), progressMark)
	if !ok {
		return Progress{}, false
	}
	f := strings.Fields(rest)
	num := func(i int) int64 {
		if i >= len(f) {
			return 0
		}
		v, err := strconv.ParseFloat(f[i], 64)
		if err != nil || v < 0 {
			return 0
		}
		return int64(v)
	}
	p := Progress{Done: num(0), Total: num(1)}
	if p.Total == 0 {
		p.Total = num(2)
	}
	return p, true
}

// removePartials deletes what an unfinished run of outBase left: .part
// files and their fragments, .ytdl state and .temp. files.
func removePartials(outBase string) {
	dir, base := filepath.Split(outBase)
	entries, err := os.ReadDir(filepath.Clean(dir))
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, base) {
			continue
		}
		rest := name[len(base):]
		if strings.HasSuffix(rest, ".part") || strings.Contains(rest, ".part-Frag") ||
			strings.HasSuffix(rest, ".ytdl") || strings.Contains(rest, ".temp.") {
			os.Remove(filepath.Join(dir, name))
		}
	}
}
