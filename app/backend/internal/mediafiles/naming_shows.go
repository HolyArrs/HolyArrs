package mediafiles

import (
	"fmt"
	"path"
	"strings"
)

// Episode orders a show can follow (titles.episode_order).
const (
	OrderAired    = "aired"
	OrderDaily    = "daily"
	OrderAbsolute = "absolute"
)

// EpisodeFacts are what an episode file name is made from: the show's
// facts plus the episodes one file holds (one, or several in a
// multi-episode file, all of one season).
type EpisodeFacts struct {
	NameFacts
	Order    string // aired, daily or absolute
	Season   int
	Episodes []int
	Absolute []int    // running numbers, 0 when unknown
	AirDate  string   // YYYY-MM-DD of the first episode
	Titles   []string // episode names
}

// episodeCode is S01E02; a file holding several episodes is written in
// the multi-episode style (S01E02-E03 by default).
func episodeCode(season int, eps []int, style string) string {
	if len(eps) == 0 {
		return fmt.Sprintf("S%02d", season)
	}
	out := fmt.Sprintf("S%02dE%02d", season, eps[0])
	if len(eps) == 1 {
		return out
	}
	last := eps[len(eps)-1]
	switch style {
	case MultiShortRange:
		out += fmt.Sprintf("-%02d", last)
	case MultiEach, MultiEachDash:
		sep := "E"
		if style == MultiEachDash {
			sep = "-E"
		}
		for _, e := range eps[1:] {
			out += fmt.Sprintf("%s%02d", sep, e)
		}
	default:
		out += fmt.Sprintf("-E%02d", last)
	}
	return out
}

func absoluteCode(abs []int, digits int) string {
	var ns []int
	for _, n := range abs {
		if n > 0 {
			ns = append(ns, n)
		}
	}
	if len(ns) == 0 {
		return ""
	}
	if digits < 2 || digits > 4 {
		digits = 3
	}
	out := fmt.Sprintf("%0*d", digits, ns[0])
	if len(ns) > 1 {
		out += fmt.Sprintf("-%0*d", digits, ns[len(ns)-1])
	}
	return out
}

// airDate writes a YYYY-MM-DD day in the air date style.
func airDate(day, style string) string {
	switch style {
	case AirDateDot:
		return strings.ReplaceAll(day, "-", ".")
	case AirDateSpace:
		return strings.ReplaceAll(day, "-", " ")
	}
	return day
}

// episodeTitles joins the distinct episode names with " + ".
func episodeTitles(ts []string) string {
	var out []string
	for _, t := range ts {
		t = strings.TrimSpace(t)
		if t == "" || strings.EqualFold(t, "TBA") {
			continue
		}
		dup := false
		for _, o := range out {
			dup = dup || o == t
		}
		if !dup {
			out = append(out, t)
		}
	}
	return strings.Join(out, " + ")
}

// EpisodeFile renders an episode file name (with its extension) from the
// pattern of the show's episode order, at most MaxComponent bytes.
func EpisodeFile(s NamingSettings, f EpisodeFacts) string {
	s.fillShows()
	pat := s.EpisodePattern
	switch {
	case f.Order == OrderDaily && f.AirDate != "" && f.Season > 0:
		pat = s.DailyPattern
	case f.Order == OrderAbsolute && absoluteCode(f.Absolute, s.AbsoluteDigits) != "":
		pat = s.AnimePattern
	}
	vals := values(f.NameFacts)
	vals["season"] = fmt.Sprint(f.Season)
	vals["season_padded"] = fmt.Sprintf("%02d", f.Season)
	vals["episode"] = episodeCode(f.Season, f.Episodes, s.MultiEpisodeStyle)
	vals["episode_title"] = episodeTitles(f.Titles)
	vals["air_date"] = airDate(f.AirDate, s.AirDateStyle)
	vals["absolute"] = absoluteCode(f.Absolute, s.AbsoluteDigits)
	ext := strings.ToLower(f.Ext)
	name := capName(renderVals(s, pat, vals), MaxComponent-len(ext))
	if name == "" {
		name = "Unnamed"
	}
	return name + ext
}

// SeasonFolder names the folder of one season ("" when season folders are
// off); season 0 goes to the specials folder.
func SeasonFolder(s NamingSettings, season int) string {
	s.fillShows()
	if !s.SeasonFolders {
		return ""
	}
	if season == 0 {
		return capName(clean(s, s.SpecialsFolder), MaxComponent)
	}
	return capName(renderVals(s, s.SeasonFolderPattern, map[string]string{"season": fmt.Sprint(season), "season_padded": fmt.Sprintf("%02d", season)}), MaxComponent)
}

// ShowFolder renders a show folder name from the show folder pattern.
func ShowFolder(s NamingSettings, f NameFacts) string {
	s.fillShows()
	name := capName(render(s, s.ShowFolderPattern, f), MaxComponent)
	if name == "" {
		return "Unnamed"
	}
	return name
}

// EpisodePath is where an episode file goes inside the Library folder,
// with forward slashes: the show folder, the season folder, the file. An
// import and a rename both use it, so a rename preview is what a rename does.
func EpisodePath(s NamingSettings, showFolder string, f EpisodeFacts) string {
	return path.Join(showFolder, SeasonFolder(s, f.Season), EpisodeFile(s, f))
}
