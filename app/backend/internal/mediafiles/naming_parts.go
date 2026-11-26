package mediafiles

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// The parts that keep their own rename, colon and character rules.
const (
	PartMovies = "movies"
	PartShows  = "shows"
	PartMusic  = "music"
)

// For gives s with part's rename, colon and character rules in
// RenameOnImport, Colon and Illegal, the fields every renderer reads.
// Movies (or an unknown part) keep s as it is.
func (s NamingSettings) For(part string) NamingSettings {
	switch part {
	case PartShows:
		s.RenameOnImport = s.ShowRenameOnImport
		s.Colon, s.Illegal = cmp.Or(s.ShowColon, s.Colon), cmp.Or(s.ShowIllegal, s.Illegal)
	case PartMusic:
		s.RenameOnImport = s.MusicRenameOnImport
		s.Colon, s.Illegal = cmp.Or(s.MusicColon, s.Colon), cmp.Or(s.MusicIllegal, s.Illegal)
	}
	return s
}

// Validate says what is wrong with s, in plain words.
func (s NamingSettings) Validate() error {
	s.fillShows() // show patterns left empty take their default
	if s.AdoptScanLimit != 0 && (s.AdoptScanLimit < 10 || s.AdoptScanLimit > 100000) {
		return errors.New("the folders read per scan must be from 10 to 100000")
	}
	if s.MusicAdoptScanLimit != 0 && (s.MusicAdoptScanLimit < 10 || s.MusicAdoptScanLimit > 100000) {
		return errors.New("the artist folders read per music scan must be from 10 to 100000")
	}
	for _, p := range []struct{ name, pat string }{{"file pattern", s.FilePattern}, {"folder pattern", s.FolderPattern},
		{"show folder pattern", s.ShowFolderPattern}, {"season folder pattern", s.SeasonFolderPattern}, {"specials folder", s.SpecialsFolder},
		{"episode pattern", s.EpisodePattern}, {"daily episode pattern", s.DailyPattern}, {"anime episode pattern", s.AnimePattern}} {
		if strings.TrimSpace(p.pat) == "" {
			return fmt.Errorf("the %s is empty", p.name)
		}
		if len(p.pat) > MaxPattern {
			return fmt.Errorf("the %s is too long (%d characters at most)", p.name, MaxPattern)
		}
		if strings.ContainsAny(p.pat, `/\`) {
			return fmt.Errorf("the %s holds a folder separator; it names one folder or one file", p.name)
		}
		words, err := wordsOf(p.pat)
		if err != nil {
			return fmt.Errorf("the %s: %w", p.name, err)
		}
		show := false
		for w := range words {
			show = show || showWords[w]
			if musicWords[w] {
				return fmt.Errorf("the %s cannot use music words such as {%s}", p.name, w)
			}
		}
		hasName := words["title"] || words["sort_title"] || words["title_year"] || (p.name != "folder pattern" && p.name != "show folder pattern" && words["release"])
		switch p.name {
		case "file pattern", "folder pattern", "show folder pattern":
			if show {
				return fmt.Errorf("the %s cannot use episode words such as {season}", p.name)
			}
			if !hasName {
				return fmt.Errorf("the %s must hold {title} or {sort_title}", p.name)
			}
		case "season folder pattern":
			if !words["season"] && !words["season_padded"] {
				return errors.New("the season folder pattern must hold {season} or {season_padded}")
			}
		case "specials folder":
			if len(words) > 0 {
				return errors.New("the specials folder is a plain name, without words in braces")
			}
		default:
			need := map[string]string{"episode pattern": "episode", "daily episode pattern": "air_date", "anime episode pattern": "absolute"}[p.name]
			if !hasName {
				return fmt.Errorf("the %s must hold {title} or {sort_title}", p.name)
			}
			if !words[need] && !words["episode"] && !words["release"] {
				return fmt.Errorf("the %s must hold {%s} or {episode}", p.name, need)
			}
		}
	}
	if err := s.validateMusic(); err != nil {
		return err
	}
	switch s.MultiEpisodeStyle {
	case MultiRange, MultiShortRange, MultiEach, MultiEachDash:
	default:
		return errors.New("multi_episode_style must be range, short_range, each or each_dash")
	}
	if s.AirDateStyle != AirDateDash && s.AirDateStyle != AirDateDot && s.AirDateStyle != AirDateSpace {
		return errors.New("air_date_style must be dash, dot or space")
	}
	if s.AbsoluteDigits < 2 || s.AbsoluteDigits > 4 {
		return errors.New("the running number must have 2 to 4 digits")
	}
	for _, r := range [][3]string{{"", s.Colon, s.Illegal}, {"show_", s.ShowColon, s.ShowIllegal}, {"music_", s.MusicColon, s.MusicIllegal}} {
		switch r[1] {
		case ColonSmart, ColonDash, ColonSpaceDash, ColonSpaceDashSpace, ColonRemove:
		default:
			return fmt.Errorf("%scolon must be smart, dash, space_dash, space_dash_space or remove", r[0])
		}
		if r[2] != IllegalReplace && r[2] != IllegalRemove {
			return fmt.Errorf("%sillegal must be replace or remove", r[0])
		}
	}
	if s.TitleLanguage != "" && !metadata.IsTextLanguage(s.TitleLanguage) {
		return fmt.Errorf("unknown title language %q", s.TitleLanguage)
	}
	return nil
}

// NamingFromJSON reads naming settings kept as JSON (an undo record). One
// kept before each part had its own rules gives every part the
// shared ones, as ReadNaming does.
func NamingFromJSON(b []byte) (NamingSettings, error) {
	var s NamingSettings
	if err := json.Unmarshal(b, &s); err != nil {
		return s, err
	}
	var has map[string]json.RawMessage
	_ = json.Unmarshal(b, &has)
	if _, ok := has["show_colon"]; !ok {
		s.ShowColon, s.ShowIllegal, s.ShowRenameOnImport = s.Colon, s.Illegal, s.RenameOnImport
	}
	if _, ok := has["music_colon"]; !ok {
		s.MusicColon, s.MusicIllegal, s.MusicRenameOnImport = s.Colon, s.Illegal, s.RenameOnImport
	}
	return s, nil
}
