package shows

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

var (
	reTVPack         = regexp.MustCompile(`(?i)\b(pack|collection)\b`)
	reTVSeason       = regexp.MustCompile(`(?i)^(season[s]?\s*\d|s\d{1,2}\b|specials$)`)
	reTVPackSuffix   = regexp.MustCompile(`(?i)\s+(?:complete(?:\s+series)?(?:\s*\[[^]]*\])*\s*$|seasons?\s+\d+(?:\s*-\s*\d+)?(?:\s|$))`)
	reTVQualityBlock = regexp.MustCompile(`(?i)\s*[\[(][^\])]*(?:\d{3,4}p|web-dl|bluray|hdtv|hevc|x26[45])[^\])]*[\])]`)
	reTVRange        = regexp.MustCompile(`\s*\(((?:19|20)\d{2})-\d{2,4}\)\s*$`)
	reTVPackIndex    = regexp.MustCompile(`^\d{1,2}[a-z]?[.\-]\s+`)
)

var reTVLeadingTag = regexp.MustCompile(`^\s*\[[^\]]+\]\s*`)
var reTVEpisodeRange = regexp.MustCompile(`(?i)[ ._-]+EP?\d{2,4}\s*-\s*\d{2,4}\b.*$`)

func cleanTVPackName(s string) string {
	// A list of entries is not a title: use its first recognizable episode.
	parts := strings.Split(s, ",")
	if len(parts) > 1 && tvAuxiliary(strings.TrimSpace(parts[0])) {
		for _, part := range parts[1:] {
			p := releasename.Parse(strings.TrimSpace(part), releasename.Hints{Show: true})
			if p.IsShow() && len(p.Show.Episodes) > 0 && p.Title.Value != "" {
				s = part
				break
			}
		}
	}
	for reTVLeadingTag.MatchString(s) {
		trimmed := reTVLeadingTag.ReplaceAllString(s, "")
		if strings.TrimSpace(trimmed) == "" {
			break
		}
		s = trimmed
	}
	s = reTVEpisodeRange.ReplaceAllString(s, "")
	// Separators in the title are spaces; keep parenthesized qualifiers intact.
	at := strings.IndexAny(s, "([")
	if at < 0 {
		at = len(s)
	}
	s = strings.NewReplacer(".", " ", "_", " ").Replace(s[:at]) + s[at:]
	s = reTVQualityBlock.ReplaceAllString(s, "")
	s = reTVRange.ReplaceAllString(s, " ($1)")
	if at := reTVPackSuffix.FindStringIndex(s); at != nil && at[0] > 0 {
		s = s[:at[0]]
	}
	// Removing a season suffix can expose a preceding Complete marker.
	s = strings.TrimSpace(s)
	if at := reTVPackSuffix.FindStringIndex(s); at != nil && at[0] > 0 {
		s = s[:at[0]]
	}
	return strings.TrimSpace(s)
}

func tvAuxiliary(name string) bool {
	switch strings.ToLower(name) {
	case "extras", "featurettes", "bonus", "samples", "sample", "trailers", "soundtrack", "subs", "subtitles":
		return true
	}
	return false
}

func tvLookupName(rel string) string {
	name := path.Base(rel)
	if strings.Contains(rel, "/") {
		name = reTVPackIndex.ReplaceAllString(name, "")
	}
	return name
}

func tvExtraPath(rel string) bool {
	parts := strings.Split(rel, "/")
	if len(parts) < 3 {
		return false
	}
	for _, part := range parts[1 : len(parts)-1] {
		if tvAuxiliary(part) {
			return true
		}
	}
	return false
}

func tvHasMainVideo(videos []library.AdoptVideo) bool {
	for _, v := range videos {
		if !tvExtraPath(v.Path) {
			return true
		}
	}
	return false
}

// Only explicit collection wrappers split. Seasons and specials stay with
// their show, and a wrapper holding files is left for manual inspection.
// This costs one directory listing per wrapper, not one lookup per episode.
func tvFolders(ctx context.Context, root string, ents []os.DirEntry, known map[string]int64, fold bool) ([]string, error) {
	var out []string
	var walk func(string, []os.DirEntry, int) error
	walk = func(parent string, entries []os.DirEntry, depth int) error {
		for _, e := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !library.ScanWants(e, "") || parent != "" && tvAuxiliary(e.Name()) {
				continue
			}
			rel := path.Join(parent, e.Name())
			if depth < 8 && reTVPack.MatchString(e.Name()) && known[library.FolderKey(rel, fold)] == 0 {
				children, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(rel)))
				if err != nil {
					return fmt.Errorf("read collection %s: %w", rel, err)
				}
				dirs, split := 0, true
				for _, child := range children {
					if child.Type()&os.ModeSymlink != 0 {
						continue
					}
					if !child.IsDir() {
						split = false
						continue
					}
					if !library.ScanWants(child, "") || tvAuxiliary(child.Name()) {
						continue
					}
					if reTVSeason.MatchString(child.Name()) {
						split = false
					}
					dirs++
				}
				if split && dirs >= 2 {
					if err := walk(rel, children, depth+1); err != nil {
						return err
					}
					continue
				}
			}
			out = append(out, rel)
		}
		return nil
	}
	if err := walk("", ents, 0); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// Validate every component, including intermediate links, before adopting a
// nested folder. Files stay at their existing paths under the Library root.
func validTVFolder(root, rel string) bool {
	if rel == "" || strings.Contains(rel, `\`) || !filepath.IsLocal(rel) || path.Clean(rel) != rel {
		return false
	}
	cur := root
	for _, part := range strings.Split(rel, "/") {
		if strings.HasPrefix(part, ".") {
			return false
		}
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	return true
}
