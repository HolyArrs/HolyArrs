package importer

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// episodeName gives the name an episode file from a download is recorded
// and named by, and p (the file name's parse) with the quality the grabbed
// release adds. Fansub groups often name the file without its quality
// ("[EMBER] Show - 01.mkv") while the release they posted gives it
// ("[EMBER] Show S01E01 [1080p] [HEVC WEBRip]"), so reading the file name
// alone recorded such a file with no resolution and named it SD TV. What the
// file name says for sure still wins.
//
// A download of a one-episode release with a single video in it keeps the
// release name as the file's name, as Sonarr keeps the scene name: the
// Library judges and names a file by it later. A pack keeps each file's own
// name, because the pack's name does not say which episode a file holds.
func (t ShowTarget) episodeName(source string, p releasename.Result) (string, releasename.Result) {
	name := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	if t.ReleaseName == "" {
		return name, p
	}
	rel := releasename.Parse(t.ReleaseName, releasename.Hints{Show: true})
	fill(&p.Resolution, rel.Resolution, func(v int) bool { return v > 0 })
	fill(&p.Source, rel.Source, func(v string) bool { return v != "" })
	fill(&p.Codec, rel.Codec, func(v string) bool { return v != "" })
	fill(&p.HDR, rel.HDR, func(v []string) bool { return len(v) > 0 })
	fill(&p.Group, rel.Group, func(v string) bool { return v != "" })
	if t.oneVideo && rel.IsShow() && !mediafiles.IsPack(rel) {
		name = t.ReleaseName
	}
	return name, p
}

// fill takes the release's value when the file name gives none, or only a
// guess where the release is sure ("[EMBER]" alone reads as a likely
// web_dl).
func fill[T any](f *releasename.Field[T], rel releasename.Field[T], has func(T) bool) {
	if has(rel.Value) && (!has(f.Value) || f.Conf != releasename.Sure && rel.Conf == releasename.Sure) {
		*f = rel
	}
}

// withInspectedResolution gives p the resolution the file inspection read
// when no name gives one, as JudgeName does for files in the Library.
func withInspectedResolution(p releasename.Result, facts map[string]any) releasename.Result {
	r, _ := facts["resolution"].(string)
	if n, err := strconv.Atoi(strings.TrimSuffix(r, "p")); err == nil && n > 0 && p.Resolution.Value == 0 {
		p.Resolution = releasename.Field[int]{Value: n, Conf: releasename.Sure}
	}
	return p
}

// videoFiles counts the videos among cands that are not samples.
func videoFiles(cands []Candidate) int {
	n := 0
	for _, c := range cands {
		if videoExt[strings.ToLower(filepath.Ext(c.Path))] && !isSample(c.Path, releasename.Parse(filepath.Base(c.Path), releasename.Hints{Size: c.Size, Show: true})) {
			n++
		}
	}
	return n
}
