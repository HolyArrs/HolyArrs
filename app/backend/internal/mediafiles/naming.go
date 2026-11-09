// Naming turns a movie's facts into folder and file names through
// the naming patterns in Settings > Where files go. Imports and renames use
// the same functions, so a rename preview is exactly what a rename does.
//
// A pattern is plain text with words in braces, such as
// "{title} ({year}) {quality}". The words are listed in Words.

package mediafiles

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// Default patterns (the same shape Radarr names movies by default).
const (
	DefaultFilePattern   = "{title} ({year}) {quality}"
	DefaultFolderPattern = "{title} ({year})"
)

// Default show patterns (the same shape Sonarr names episodes; the show
// folder keeps the year, as movie folders do, so same-named shows differ).
const (
	DefaultShowFolderPattern   = "{title} ({year})"
	DefaultSeasonFolderPattern = "Season {season}"
	DefaultSpecialsFolder      = "Specials"
	DefaultEpisodePattern      = "{title} - {episode} - {episode_title} {quality}"
	DefaultDailyPattern        = "{title} - {air_date} - {episode_title} {quality}"
	DefaultAnimePattern        = "{title} - {episode} - {absolute} - {episode_title} {quality}"
)

// What a colon in a name becomes.
const (
	ColonSmart          = "smart"            // ": " becomes " - ", any other ":" becomes "-"
	ColonDash           = "dash"             // ":" becomes "-"
	ColonSpaceDash      = "space_dash"       // ":" becomes " -"
	ColonSpaceDashSpace = "space_dash_space" // ":" becomes " - "
	ColonRemove         = "remove"           // ":" is dropped
)

// What other characters a file system cannot hold become.
const (
	IllegalReplace = "replace" // / \ * become "-", " becomes ', ? < > | are dropped
	IllegalRemove  = "remove"  // all of them are dropped
)

// Words are the words a pattern may use, with what each gives.
var Words = map[string]string{
	"title":      "the movie or show name",
	"sort_title": "the movie or show name with a leading The, A or An moved to the end",
	"year":       "the year the movie or show came out",
	"quality":    "resolution and source, such as 1080p BluRay",
	"resolution": "such as 1080p",
	"source":     "such as BluRay or WEB",
	"edition":    "such as Director's Cut, when the release name says so",
	"group":      "the release group",
	"tmdb":       "the TMDB id",
	"imdb":       "the IMDb id",
	"release":    "the original release name",
	// Show words: only the season folder and episode patterns use them.
	"season":        "the season number, such as 1",
	"episode":       "season and episode, such as S01E02, or S01E02-E03 for a file holding several",
	"episode_title": "the episode name; several are joined with +",
	"air_date":      "the day the episode aired, such as 2026-03-14",
	"absolute":      "the running number, such as 012, or 012-013 for several",
	"season_padded": "the season number with two digits, such as 01",
}

// showWords may appear only in show patterns.
var showWords = map[string]bool{"season_padded": true, "season": true, "episode": true, "episode_title": true, "air_date": true, "absolute": true}

// NamingSettings are the naming settings.
type NamingSettings struct {
	FilePattern   string `json:"file_pattern"`
	FolderPattern string `json:"folder_pattern"`
	Colon         string `json:"colon"`
	Illegal       string `json:"illegal"`
	// RenameOnImport names imported files by the file pattern; off keeps
	// the release's own file name.
	RenameOnImport bool `json:"rename_on_import"`
	// Show patterns: the show folder (when a show is added), the season
	// folder (SeasonFolders off puts episodes straight in the show folder),
	// the folder for season 0, and the episode file for each episode order.
	ShowFolderPattern   string `json:"show_folder_pattern"`
	SeasonFolders       bool   `json:"season_folders"`
	SeasonFolderPattern string `json:"season_folder_pattern"`
	SpecialsFolder      string `json:"specials_folder"`
	EpisodePattern      string `json:"episode_pattern"`
	DailyPattern        string `json:"daily_pattern"`
	AnimePattern        string `json:"anime_pattern"`
	// MultiEpisodeStyle is how {episode} writes a file holding several
	// episodes (MultiRange and the others below).
	MultiEpisodeStyle string `json:"multi_episode_style"`
	// AirDateStyle is how {air_date} separates year, month and day.
	AirDateStyle string `json:"air_date_style"`
	// AbsoluteDigits is how many digits {absolute} has at least (2 to 4).
	AbsoluteDigits int `json:"absolute_digits"`
	// AdoptScanLimit is how many folders one Import a library scan reads;
	// a bigger library is scanned in parts ("scan the rest"). 0 means the
	// default.
	AdoptScanLimit int `json:"adopt_scan_limit"`
	// MusicAdoptScanLimit is how many artist folders one music scan reads
	// (each costs MusicBrainz lookups at one a second); 0 means the default.
	MusicAdoptScanLimit int `json:"music_adopt_scan_limit"`
	// Music patterns: the artist folder (when an artist is added), the
	// album folder, the track file, the track file on an album of more than
	// one disc, and the disc folder (DiscFolders on puts each disc of such
	// an album in its own folder).
	ArtistFolderPattern   string `json:"artist_folder_pattern"`
	AlbumFolderPattern    string `json:"album_folder_pattern"`
	TrackPattern          string `json:"track_pattern"`
	MultiDiscTrackPattern string `json:"multi_disc_track_pattern"`
	DiscFolders           bool   `json:"disc_folders"`
	DiscFolderPattern     string `json:"disc_folder_pattern"`
	// MusicReadTags lets matching read a track file's own tags (title,
	// track and disc numbers) when its name is not enough.
	MusicReadTags bool `json:"music_read_tags"`
}

// How {episode} writes a file holding episodes 2 to 4 of season 1.
const (
	MultiRange      = "range"       // S01E02-E04
	MultiShortRange = "short_range" // S01E02-04
	MultiEach       = "each"        // S01E02E03E04
	MultiEachDash   = "each_dash"   // S01E02-E03-E04
)

// How {air_date} writes 14 March 2026.
const (
	AirDateDash  = "dash"  // YYYY-MM-DD
	AirDateDot   = "dot"   // 2026.03.14
	AirDateSpace = "space" // 2026 03 14
)

// DefaultAdoptScanLimit is the folders one scan reads unless changed.
const DefaultAdoptScanLimit = 5000

// DefaultMusicAdoptScanLimit is the artist folders one music scan reads
// unless changed.
const DefaultMusicAdoptScanLimit = 200

// NamingDefaults are the settings before anyone changes them.
func NamingDefaults() NamingSettings {
	s := NamingSettings{FilePattern: DefaultFilePattern, FolderPattern: DefaultFolderPattern, Colon: ColonSmart, Illegal: IllegalReplace, RenameOnImport: true,
		AdoptScanLimit: DefaultAdoptScanLimit, MusicAdoptScanLimit: DefaultMusicAdoptScanLimit, SeasonFolders: true, MusicReadTags: true}
	s.fillShows()
	return s
}

// fillShows gives empty show patterns their default (settings saved
// before shows had patterns).
func (s *NamingSettings) fillShows() {
	for _, f := range []struct {
		p   *string
		def string
	}{{&s.ShowFolderPattern, DefaultShowFolderPattern}, {&s.SeasonFolderPattern, DefaultSeasonFolderPattern}, {&s.SpecialsFolder, DefaultSpecialsFolder},
		{&s.EpisodePattern, DefaultEpisodePattern}, {&s.DailyPattern, DefaultDailyPattern}, {&s.AnimePattern, DefaultAnimePattern},
		{&s.MultiEpisodeStyle, MultiRange}, {&s.AirDateStyle, AirDateDash}} {
		if strings.TrimSpace(*f.p) == "" {
			*f.p = f.def
		}
	}
	if s.AbsoluteDigits == 0 {
		s.AbsoluteDigits = 3
	}
	s.fillMusic()
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
		if len(p.pat) > 200 {
			return fmt.Errorf("the %s is too long (200 characters at most)", p.name)
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
		hasName := words["title"] || words["sort_title"] || (p.name != "folder pattern" && p.name != "show folder pattern" && words["release"])
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
	switch s.Colon {
	case ColonSmart, ColonDash, ColonSpaceDash, ColonSpaceDashSpace, ColonRemove:
	default:
		return fmt.Errorf("colon must be smart, dash, space_dash, space_dash_space or remove")
	}
	if s.Illegal != IllegalReplace && s.Illegal != IllegalRemove {
		return fmt.Errorf("illegal must be replace or remove")
	}
	return nil
}

func wordsOf(pat string) (map[string]bool, error) {
	out := map[string]bool{}
	for rest := pat; ; {
		open, close := strings.IndexByte(rest, '{'), strings.IndexByte(rest, '}')
		if open < 0 && close < 0 {
			return out, nil
		}
		if open < 0 || (close >= 0 && close < open) {
			return nil, errors.New("a } is unopened")
		}
		end := strings.IndexByte(rest[open:], '}')
		if end < 0 {
			return nil, errors.New("a { is unclosed")
		}
		w := rest[open+1 : open+end]
		if _, ok := Words[w]; !ok {
			return nil, fmt.Errorf("unknown word {%s}", w)
		}
		out[w] = true
		rest = rest[open+end+1:]
	}
}

// NameFacts are what a name is made from.
type NameFacts struct {
	Title   string
	Year    int
	Step    string // quality step, such as 1080p_bluray
	Edition string
	Group   string
	TMDB    string
	IMDB    string
	Release string // the release name, without extension
	Ext     string // with the dot
}

// MaxComponent is the most bytes one rendered file or folder name may
// take; most file systems stop at 255.
const MaxComponent = 200

// FileName renders a file name (with its extension) from the file pattern,
// at most MaxComponent bytes with the extension kept.
func FileName(s NamingSettings, f NameFacts) string {
	ext := strings.ToLower(f.Ext)
	name := capName(render(s, s.FilePattern, f), MaxComponent-len(ext))
	if name == "" {
		name = "Unnamed"
	}
	return name + ext
}

// Folder renders a movie folder name from the folder pattern, at most
// MaxComponent bytes.
func Folder(s NamingSettings, f NameFacts) string {
	name := capName(render(s, s.FolderPattern, f), MaxComponent)
	if name == "" {
		return "Unnamed"
	}
	return name
}

// capName cuts name to at most n bytes on a UTF-8 boundary and trims the
// spaces and dashes the cut leaves at the end.
func capName(name string, n int) string {
	if len(name) <= n {
		return name
	}
	if n < 0 {
		n = 0
	}
	for n > 0 && !utf8.RuneStart(name[n]) {
		n--
	}
	return strings.TrimRight(name[:n], " -_.")
}

// CleanName makes one name safe on every file system with the default rules.
func CleanName(name string) string { return clean(NamingDefaults(), name) }

// CleanWith makes one name safe with s's colon and character rules.
func CleanWith(s NamingSettings, name string) string { return clean(s, name) }

func render(s NamingSettings, pat string, f NameFacts) string { return renderVals(s, pat, values(f)) }

func values(f NameFacts) map[string]string {
	res, src := splitStep(f.Step)
	quality := strings.TrimSpace(res + " " + src)
	year := ""
	if f.Year > 0 {
		year = fmt.Sprint(f.Year)
	}
	return map[string]string{"title": f.Title, "sort_title": sortTitle(f.Title), "year": year, "quality": quality,
		"resolution": res, "source": src, "edition": f.Edition, "group": f.Group, "tmdb": f.TMDB, "imdb": f.IMDB, "release": f.Release}
}

func renderVals(s NamingSettings, pat string, vals map[string]string) string {
	var b strings.Builder
	for rest := pat; rest != ""; {
		open := strings.IndexByte(rest, '{')
		end := -1
		if open >= 0 {
			end = strings.IndexByte(rest[open:], '}')
		}
		if open < 0 || end < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:open])
		b.WriteString(vals[rest[open+1:open+end]])
		rest = rest[open+end+1:]
	}
	out := b.String()
	// Words with no value leave empty brackets and lone dashes behind.
	for _, e := range []string{"()", "[]", "( )", "[ ]"} {
		out = strings.ReplaceAll(out, e, "")
	}
	out = clean(s, out)
	// An empty word between two dashes leaves "- -" behind.
	for strings.Contains(out, "- -") {
		out = strings.ReplaceAll(out, "- -", "-")
	}
	for {
		t := strings.Trim(out, " -_.")
		t = strings.TrimSuffix(strings.TrimSpace(t), " -")
		if t == out {
			break
		}
		out = t
	}
	return clean(s, out)
}

func splitStep(step string) (res, src string) {
	r, s, ok := strings.Cut(step, "_")
	if !ok {
		return "", strings.ToUpper(step)
	}
	switch s {
	case "bluray":
		s = "BluRay"
	case "remux":
		s = "Remux"
	default:
		s = strings.ToUpper(s)
	}
	if r == "sd" {
		r = "SD"
	}
	return r, s
}

func sortTitle(t string) string {
	for _, a := range []string{"The ", "A ", "An "} {
		if rest, ok := strings.CutPrefix(t, a); ok && rest != "" {
			return rest + ", " + strings.TrimSpace(a)
		}
	}
	return t
}

func clean(s NamingSettings, name string) string {
	switch s.Colon {
	case ColonDash:
		name = strings.ReplaceAll(name, ":", "-")
	case ColonSpaceDash:
		name = strings.ReplaceAll(name, ":", " -")
	case ColonSpaceDashSpace:
		name = strings.ReplaceAll(name, ":", " - ")
	case ColonRemove:
		name = strings.ReplaceAll(name, ":", "")
	default:
		name = strings.ReplaceAll(name, ": ", " - ")
		name = strings.ReplaceAll(name, ":", "-")
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\' || r == '*':
			if s.Illegal == IllegalRemove {
				return -1
			}
			return '-'
		case r == '"':
			if s.Illegal == IllegalRemove {
				return -1
			}
			return '\''
		case strings.ContainsRune("?<>|", r):
			return -1
		case unicode.IsSpace(r):
			return ' '
		case r < 32 || r == 127:
			return -1
		case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
			return -1 // bidi controls: they reorder what a name shows
		case r >= 0x200B && r <= 0x200F, r == 0x2060, r == 0xFEFF:
			return -1 // zero-width characters: they hide in a name
		}
		return r
	}, name)
	name = strings.Join(strings.Fields(name), " ")
	name = strings.TrimRight(name, ". ")
	// Windows keeps CON, PRN, AUX, NUL, COM1-9 and LPT1-9 for devices, with
	// or without an extension: such a name gets a suffix.
	stem, ext, dot := strings.Cut(name, ".")
	if reserved(strings.ToUpper(strings.TrimRight(stem, " "))) {
		name = strings.TrimRight(stem, " ") + "_"
		if dot {
			name += "." + ext
		}
	}
	return name
}

func reserved(n string) bool {
	switch n {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(n) == 4 && (strings.HasPrefix(n, "COM") || strings.HasPrefix(n, "LPT")) && n[3] >= '1' && n[3] <= '9'
}

const group = "where_files_go"

// ReadNaming reads the naming settings; unset or broken ones keep their default.
func ReadNaming(ctx context.Context, db database.Lister) (NamingSettings, error) {
	s := NamingDefaults()
	err := database.EachSetting(ctx, db, group, func(k, v string) {
		switch k {
		case "file_pattern":
			s.FilePattern = v
		case "folder_pattern":
			s.FolderPattern = v
		case "colon":
			s.Colon = v
		case "illegal":
			s.Illegal = v
		case "rename_on_import":
			s.RenameOnImport = v != "false"
		case "adopt_scan_limit":
			if n, err := strconv.Atoi(v); err == nil {
				s.AdoptScanLimit = n
			}
		case "music_adopt_scan_limit":
			if n, err := strconv.Atoi(v); err == nil {
				s.MusicAdoptScanLimit = n
			}
		case "show_folder_pattern":
			s.ShowFolderPattern = v
		case "season_folders":
			s.SeasonFolders = v != "false"
		case "season_folder_pattern":
			s.SeasonFolderPattern = v
		case "specials_folder":
			s.SpecialsFolder = v
		case "episode_pattern":
			s.EpisodePattern = v
		case "daily_pattern":
			s.DailyPattern = v
		case "anime_pattern":
			s.AnimePattern = v
		case "multi_episode_style":
			s.MultiEpisodeStyle = v
		case "air_date_style":
			s.AirDateStyle = v
		case "absolute_digits":
			if n, err := strconv.Atoi(v); err == nil {
				s.AbsoluteDigits = n
			}
		case "artist_folder_pattern":
			s.ArtistFolderPattern = v
		case "album_folder_pattern":
			s.AlbumFolderPattern = v
		case "track_pattern":
			s.TrackPattern = v
		case "multi_disc_track_pattern":
			s.MultiDiscTrackPattern = v
		case "disc_folders":
			s.DiscFolders = v == "true"
		case "disc_folder_pattern":
			s.DiscFolderPattern = v
		case "music_read_tags":
			s.MusicReadTags = v != "false"
		}
	})
	if err != nil {
		return s, err
	}
	s.fillShows()
	if s.Validate() != nil {
		d := NamingDefaults()
		d.RenameOnImport = s.RenameOnImport
		d.SeasonFolders = s.SeasonFolders
		d.DiscFolders = s.DiscFolders
		d.MusicReadTags = s.MusicReadTags
		return d, nil
	}
	if s.AdoptScanLimit == 0 {
		s.AdoptScanLimit = DefaultAdoptScanLimit
	}
	if s.MusicAdoptScanLimit == 0 {
		s.MusicAdoptScanLimit = DefaultMusicAdoptScanLimit
	}
	return s, nil
}

// PutNamingSettings stores valid naming settings. then, when given, runs in the same
// transaction (the move tool marks its change there).
func PutNamingSettings(ctx context.Context, st *database.Store, s NamingSettings, by database.Actor, then ...func(tx *sql.Tx) error) error {
	s.fillShows()
	if err := s.Validate(); err != nil {
		return err
	}
	if s.AdoptScanLimit == 0 {
		s.AdoptScanLimit = DefaultAdoptScanLimit
	}
	if s.MusicAdoptScanLimit == 0 {
		s.MusicAdoptScanLimit = DefaultMusicAdoptScanLimit
	}
	return st.Write(ctx, func(tx *sql.Tx) error {
		now := time.Now()
		for k, v := range map[string]string{"file_pattern": s.FilePattern, "folder_pattern": s.FolderPattern, "colon": s.Colon,
			"illegal": s.Illegal, "rename_on_import": fmt.Sprint(s.RenameOnImport),
			"adopt_scan_limit": strconv.Itoa(s.AdoptScanLimit), "music_adopt_scan_limit": strconv.Itoa(s.MusicAdoptScanLimit), "show_folder_pattern": s.ShowFolderPattern, "season_folders": fmt.Sprint(s.SeasonFolders),
			"season_folder_pattern": s.SeasonFolderPattern, "specials_folder": s.SpecialsFolder, "episode_pattern": s.EpisodePattern,
			"daily_pattern": s.DailyPattern, "anime_pattern": s.AnimePattern, "multi_episode_style": s.MultiEpisodeStyle,
			"air_date_style": s.AirDateStyle, "absolute_digits": strconv.Itoa(s.AbsoluteDigits),
			"artist_folder_pattern": s.ArtistFolderPattern, "album_folder_pattern": s.AlbumFolderPattern, "track_pattern": s.TrackPattern,
			"multi_disc_track_pattern": s.MultiDiscTrackPattern, "disc_folders": fmt.Sprint(s.DiscFolders), "disc_folder_pattern": s.DiscFolderPattern,
			"music_read_tags": fmt.Sprint(s.MusicReadTags)} {
			if err := database.PutSetting(tx, group, k, v, by, now); err != nil {
				return err
			}
		}
		for _, f := range then {
			if err := f(tx); err != nil {
				return err
			}
		}
		return nil
	})
}

// FactsFrom builds NameFacts from a parsed release name: what an import knows.
func FactsFrom(title string, year int, ids map[string]string, p releasename.Result, release, ext string) NameFacts {
	f := NameFacts{Title: title, Year: year, Step: StepFor(p), Group: p.Group.Value, Release: release, Ext: ext,
		TMDB: ids["tmdb"], IMDB: ids["imdb"]}
	if len(p.Edition) > 0 {
		f.Edition = strings.Join(p.Edition, " ")
	}
	return f
}
