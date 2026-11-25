// Naming turns a movie's facts into folder and file names through
// the naming patterns in Settings > Where files go. Imports and renames use
// the same functions, so a rename preview is exactly what a rename does.
//
// A pattern is plain text with words in braces, such as
// "{title} ({year}) {quality}". The words are listed in Words.

package mediafiles

import (
	"cmp"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

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
	"title":       "the movie or show name",
	"sort_title":  "the movie or show name with a leading The, A or An moved to the end",
	"title_first": "the first letter of the sort title, such as M for The Matrix; 0-9 for a digit, # for anything else",
	"year":        "the year the movie or show came out",
	"quality":     "resolution and source, such as 1080p BluRay",
	"resolution":  "such as 1080p",
	"source":      "such as BluRay or WEB",
	"audio":       "the audio codec, such as EAC3 or TrueHD",
	"hdr":         "the HDR kind, such as DolbyVision or HDR10+; empty for SDR",
	"edition":     "such as Director's Cut, when the release name says so",
	"group":       "the release group",
	"tmdb":        "the TMDB id; {tmdb-{tmdb}} keeps the outer braces, as Plex reads them",
	"tvdb":        "the TVDB id (shows)",
	"imdb":        "the IMDb id",
	"release":     "the original release name",
	// Words that name things the way Radarr and Sonarr do, so their
	// patterns (and the TRaSH guides') give the same names.
	"title_year":      "the name with the year, such as Dune (2024)",
	"quality_full":    "the quality as Radarr names it, with Proper or Repack, such as Bluray-1080p Proper",
	"quality_title":   "the quality as Radarr names it, such as Bluray-1080p",
	"proper":          "the word Proper or Repack when the release is a fixed one; empty otherwise",
	"video_codec":     "the video codec, such as x265 or AV1",
	"audio_channels":  "the audio channels, such as 5.1",
	"bit_depth":       "the video bit depth when the release name says it, such as 10",
	"hdr_type":        "the HDR kind as Radarr names it, such as DV HDR10; empty for SDR",
	"audio_languages": "the audio languages, such as [JA+EN]; empty when English alone",
	"three_d":         "3D when the release is 3D; empty otherwise",
	"french_version":  "the French dub: VFF (France), VFQ (Québec), VFI (international) or VF2 (both), from the release name, its .nfo or the audio tracks; empty when none says. Next to {release} it is only added when the release name leaves it out",
	"scored_tags":     "always empty: accepted so patterns with Custom Formats still save",
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
	// GroupByGenre puts each movie folder in a folder of its first genre:
	// Movies/Drama/Heat (1995)/. Off (the default) keeps one folder per
	// movie at the top.
	GroupByGenre bool `json:"group_by_genre"`
	// GroupByLetter puts each movie folder in a folder of its first
	// letter: Movies/H/Heat (1995)/. It wins over GroupByGenre.
	GroupByLetter bool `json:"group_by_letter"`
	// ShowGroupByLetter does the same for show folders: Shows/O/The Office/
	// sits under O, as the sort title starts with Office.
	ShowGroupByLetter bool `json:"show_group_by_letter"`
	// Colon, Illegal and RenameOnImport above are for movies; shows and
	// music keep their own, as Sonarr, Radarr and Lidarr each do.
	// Use For to get the settings one part names files with.
	ShowRenameOnImport  bool   `json:"show_rename_on_import"`
	ShowColon           string `json:"show_colon"`
	ShowIllegal         string `json:"show_illegal"`
	MusicRenameOnImport bool   `json:"music_rename_on_import"`
	MusicColon          string `json:"music_colon"`
	MusicIllegal        string `json:"music_illegal"`
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
	// TitleLanguage is the app language whose TMDB name new movie and show
	// names use, when one was kept with the title's details; "" keeps the
	// stored name (as Radarr's movie title language does). Names already on
	// disk change only when renamed.
	TitleLanguage string `json:"title_language"`
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
// unless changed: each costs a few lookups paced seconds apart, so a part
// finishes in a few minutes.
const DefaultMusicAdoptScanLimit = 50

// NamingDefaults are the settings before anyone changes them.
func NamingDefaults() NamingSettings {
	s := NamingSettings{FilePattern: DefaultFilePattern, FolderPattern: DefaultFolderPattern, Colon: ColonSmart, Illegal: IllegalReplace, RenameOnImport: true,
		ShowColon: ColonSmart, ShowIllegal: IllegalReplace, ShowRenameOnImport: true, MusicColon: ColonSmart, MusicIllegal: IllegalReplace, MusicRenameOnImport: true,
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
	// A part's colon and character rules left empty take the movie ones.
	s.ShowColon, s.ShowIllegal = cmp.Or(s.ShowColon, s.Colon), cmp.Or(s.ShowIllegal, s.Illegal)
	s.MusicColon, s.MusicIllegal = cmp.Or(s.MusicColon, s.Colon), cmp.Or(s.MusicIllegal, s.Illegal)
	s.fillMusic()
}

// Literal braces: "{tmdb-{tmdb}}" keeps its outer braces, as Sonarr's
// "{tmdb-{TmdbId}}" does. They stand in as these bytes while a pattern renders.
const litOpen, litShut = '', ''

// literalBraces marks a { that opens text holding another {word} (and the }
// that closes it) as literal.
func literalBraces(pat string) string {
	b := []byte(pat)
	depth := 0
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case '{':
			next, shut := strings.IndexByte(pat[i+1:], '{'), strings.IndexByte(pat[i+1:], '}')
			if next >= 0 && (shut < 0 || next < shut) {
				b[i] = litOpen
				depth++
			} else if shut >= 0 {
				i += shut + 1
			}
		case '}':
			if depth > 0 {
				b[i] = litShut
				depth--
			}
		}
	}
	return string(b)
}

func wordsOf(pat string) (map[string]bool, error) {
	pat = literalBraces(pat)
	if strings.Count(pat, string(litOpen)) != strings.Count(pat, string(litShut)) {
		return nil, errors.New("a { is unclosed")
	}
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
		t := parseTok(rest[open+1 : open+end])
		if t.word == "" {
			return nil, fmt.Errorf("unknown word {%s}: use a word from the list, such as {title}, {year} or {quality}", rest[open+1:open+end])
		}
		if t.badFmt != "" {
			return nil, fmt.Errorf("{%s}: use :00 to pad a number or :30 to cut a name", rest[open+1:open+end])
		}
		out[t.word] = true
		rest = rest[open+end+1:]
	}
}

// NameFacts are what a name is made from.
type NameFacts struct {
	Title   string
	Year    int
	Step    string // quality step, such as 1080p_bluray
	Edition string
	Audio   string // the audio codec, such as EAC3; "" when not known
	HDR     string // the HDR kinds, such as DolbyVision; "" for SDR
	Group   string
	Genre   string // the first genre; only GroupByGenre uses it
	TMDB    string
	TVDB    string
	IMDB    string
	Release string // the release name, without extension
	Ext     string // with the dot
	// Facts the release name gives for the Radarr- and Sonarr-style words;
	// "" (or nil) when it does not say.
	Proper     string // Proper or Repack
	VideoCodec string // x265, h264, AV1
	Channels   string // 5.1
	BitDepth   string // 10
	ThreeD     string // 3D
	WebRip     bool
	Languages  []string // audio language codes, such as ja and en
	// FrenchVersion is the French dub (VFF, VFQ, VFI, VF2) the release
	// name tags, or else the one its .nfo or audio tracks show; "" when
	// none says. FrenchInName says the release name tags it.
	FrenchVersion string
	FrenchInName  bool
}

// TitleNamer gives a title's kept TMDB name in an app language ("" when
// none was kept); set once at start. It never asks outside.
var TitleNamer func(kind, tmdbID, lang string) string

// inLanguage swaps f's title for its name in s's title language, when set
// and kept; kind is "movie" or "show".
func inLanguage(s NamingSettings, kind string, f NameFacts) NameFacts {
	if s.TitleLanguage == "" || s.TitleLanguage == "en" || f.TMDB == "" || TitleNamer == nil {
		return f
	}
	if n := strings.TrimSpace(TitleNamer(kind, f.TMDB, s.TitleLanguage)); n != "" {
		f.Title = n
	}
	return f
}

// MaxPattern is the longest pattern: room for the TRaSH guides' longest
// (about 330 characters); the name it gives is still cut to MaxComponent.
const MaxPattern = 600

// MaxComponent is the most bytes one rendered file or folder name may
// take; most file systems stop at 255.
const MaxComponent = 200

// FileName renders a file name (with its extension) from the file pattern,
// at most MaxComponent bytes with the extension kept.
func FileName(s NamingSettings, f NameFacts) string {
	f = inLanguage(s, "movie", f)
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
	f = inLanguage(s, "movie", f)
	name := capName(render(s, s.FolderPattern, f), MaxComponent)
	if name == "" {
		return "Unnamed"
	}
	return name
}

// MovieFolder is the folder of a movie inside its Library folder: Folder,
// under a folder of its first letter or genre when that grouping is on.
func MovieFolder(s NamingSettings, f NameFacts) string {
	if g := MovieGroupFolder(s, f.Title, f.Genre); g != "" {
		return g + "/" + Folder(s, f)
	}
	return Folder(s, f)
}

// MovieGroupFolder is the folder a movie sits in under its Library
// folder: its first letter when GroupByLetter is on, else its genre when
// GroupByGenre is on; "" when neither applies.
func MovieGroupFolder(s NamingSettings, title, genre string) string {
	if s.GroupByLetter {
		return LetterFolder(title)
	}
	return GenreFolder(s, genre)
}

// LetterFolder is the folder a title sits in when grouped by first letter:
// the first letter of its sort title without accents, 0-9 or #.
func LetterFolder(title string) string {
	return titleFirst(title)
}

// ShowFolderIn is a show's folder inside its Library folder: ShowFolder,
// under the first letter when ShowGroupByLetter is on.
func ShowFolderIn(s NamingSettings, f NameFacts) string {
	if l := LetterFolder(f.Title); s.ShowGroupByLetter && l != "" {
		return l + "/" + ShowFolder(s, f)
	}
	return ShowFolder(s, f)
}

// GenreFolder is the folder a movie of this genre sits in when
// GroupByGenre is on; "" when it is off or there is no genre.
func GenreFolder(s NamingSettings, genre string) string {
	g := capName(clean(s, strings.TrimSpace(genre)), MaxComponent)
	if !s.GroupByGenre || g == "" || g == "." || g == ".." {
		return ""
	}
	return g
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

func render(s NamingSettings, pat string, f NameFacts) string {
	vals := values(f)
	if f.FrenchInName && vals["french_version"] != "" {
		// {release} already says it: the word adds only a missing tag.
		if UsesWord(pat, "release") {
			vals["french_version"] = ""
		}
	}
	return renderVals(s, pat, vals)
}

// UsesWord says pattern p uses word w.
func UsesWord(p, w string) bool {
	words, err := wordsOf(p)
	return err == nil && words[w]
}

func values(f NameFacts) map[string]string {
	res, src := splitStep(f.Step)
	quality := strings.TrimSpace(res + " " + src)
	year := ""
	if f.Year > 0 {
		year = fmt.Sprint(f.Year)
	}
	qt := ""
	if f.Step != "" {
		qt = qualityTitle(f.Step, f.WebRip)
	}
	titleYear := f.Title
	if year != "" && f.Title != "" {
		titleYear += " (" + year + ")"
	}
	return map[string]string{"title": f.Title, "sort_title": sortTitle(f.Title), "title_first": titleFirst(f.Title), "year": year, "quality": quality,
		"resolution": res, "source": src, "audio": f.Audio, "hdr": f.HDR, "edition": f.Edition, "group": f.Group,
		"tmdb": f.TMDB, "tvdb": f.TVDB, "imdb": f.IMDB, "release": f.Release,
		"title_year": titleYear, "quality_title": qt, "quality_full": strings.TrimSpace(qt + " " + f.Proper),
		"proper": f.Proper, "video_codec": f.VideoCodec, "audio_channels": f.Channels, "bit_depth": f.BitDepth, "hdr_type": hdrType(f.HDR),
		"audio_languages": audioLanguages(f.Languages), "three_d": f.ThreeD, "french_version": f.FrenchVersion}
}

func renderVals(s NamingSettings, pat string, vals map[string]string) string {
	var b strings.Builder
	for rest := dropEmptyGroups(literalBraces(pat), vals); rest != ""; {
		open := strings.IndexByte(rest, '{')
		end := -1
		if open >= 0 {
			end = strings.IndexByte(rest[open:], '}')
		}
		if open < 0 || end < 0 {
			b.WriteString(rest)
			break
		}
		t := parseTok(rest[open+1 : open+end])
		lit, v := rest[:open], tokValue(t, vals)
		rest = rest[open+end+1:]
		if v == "" && t.pre == "" && t.suf == "" {
			// A word with no value takes one dot, dash or underscore the
			// pattern wrote beside it, never a character of another word's
			// value: "{audio}.{resolution}" without a codec gives "2160p".
			// It takes a dash after it only when nothing stands before it
			// to join, so " {edition}-{group}" keeps the group's dash.
			if n := len(lit); n > 0 && strings.IndexByte("._-", lit[n-1]) >= 0 {
				lit = lit[:n-1]
			} else if rest != "" && strings.IndexByte("._-", rest[0]) >= 0 && (rest[0] != '-' || opensName(b.String()+lit)) {
				rest = rest[1:]
			}
		}
		b.WriteString(lit)
		b.WriteString(strings.NewReplacer(string(litOpen), "", string(litShut), "").Replace(v))
	}
	out := b.String()
	// Words with no value leave empty brackets and lone dashes behind.
	for _, e := range []string{"()", "[]", "( )", "[ ]"} {
		out = strings.ReplaceAll(out, e, "")
	}
	out = strings.NewReplacer(string(litOpen), "{", string(litShut), "}").Replace(out)
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

// dropEmptyGroups removes a [...] or (...) whose words are all empty, with one
// space beside it, so "[imdbid-{imdb}]" without an id leaves nothing behind.
func dropEmptyGroups(pat string, vals map[string]string) string {
	for i := 0; i < len(pat); i++ {
		c := pat[i]
		if c == '{' {
			// A bracket inside a word's braces ({[Quality Full]}) is the word's own.
			if k := strings.IndexByte(pat[i:], '}'); k > 0 {
				i += k
			}
			continue
		}
		if c != '[' && c != '(' && c != litOpen {
			continue
		}
		shut := byte(']')
		if c == '(' {
			shut = ')'
		} else if c == litOpen {
			shut = litShut
		}
		j := strings.IndexAny(pat[i+1:], "[]()")
		if j < 0 || pat[i+1+j] != shut {
			continue
		}
		j += i + 1
		inner := pat[i+1 : j]
		if !strings.Contains(inner, "{") || !allEmpty(inner, vals) {
			continue
		}
		start, end := i, j+1
		if start > 0 && pat[start-1] == ' ' {
			start--
		} else if end < len(pat) && pat[end] == ' ' {
			end++
		} else if start > 0 && end < len(pat) && pat[end] == pat[start-1] && strings.IndexByte("._-", pat[end]) >= 0 {
			end++ // "{title}.{tvdb-{tvdb}}.{year}" keeps one dot
		}
		pat = pat[:start] + pat[end:]
		i = start - 1
	}
	return pat
}

// allEmpty reports whether every {word} in text has no value.
func allEmpty(text string, vals map[string]string) bool {
	for {
		open := strings.IndexByte(text, '{')
		if open < 0 {
			return true
		}
		end := strings.IndexByte(text[open:], '}')
		if end < 0 {
			return true
		}
		if tokValue(parseTok(text[open+1:open+end]), vals) != "" {
			return false
		}
		text = text[open+end+1:]
	}
}

// opensName reports whether text ends where a name or a bracket starts, so a
// separator after an empty word there has nothing to join.
func opensName(text string) bool {
	return text == "" || strings.IndexByte("[(", text[len(text)-1]) >= 0
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

// titleFirst is the first letter of the sort title with accents dropped, so
// "Élite" files under E next to other E titles.
func titleFirst(t string) string {
	r, _ := utf8.DecodeRuneInString(sortTitle(strings.TrimSpace(t)))
	switch {
	case r == utf8.RuneError:
		return ""
	case unicode.IsDigit(r):
		return "0-9"
	case !unicode.IsLetter(r):
		return "#"
	}
	base, _ := utf8.DecodeRuneInString(norm.NFD.String(string(r)))
	return string(unicode.ToUpper(base))
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

// FactsFrom builds NameFacts from a parsed release name: what an import knows.
func FactsFrom(title string, year int, ids map[string]string, p releasename.Result, release, ext string) NameFacts {
	f := NameFacts{Title: title, Year: year, Step: StepFor(p), Group: p.Group.Value, Release: release, Ext: ext,
		Audio: AudioLabel(p.Audio.Format), HDR: HDRLabel(strings.Join(p.HDR.Value, " ")),
		TMDB: ids["tmdb"], TVDB: ids["tvdb"], IMDB: ids["imdb"]}
	if len(p.Edition) > 0 {
		f.Edition = strings.Join(p.Edition, " ")
	}
	releaseFacts(&f, p)
	if v := releasename.FrenchVersion(release); v != "" {
		f.FrenchVersion, f.FrenchInName = v, true
	} else if v := releasename.FrenchVersion(p.Original); v != "" {
		// {release} lacks it here, so the word still adds it.
		f.FrenchVersion = v
	}
	return f
}

// AddFrenchVersion fills a French dub the release name leaves out (see
// NameFacts.FrenchVersion).
func (f *NameFacts) AddFrenchVersion(v string) {
	if f.FrenchVersion == "" {
		f.FrenchVersion = v
	}
}
