package mediafiles

import (
	"fmt"
	"path"
	"strings"
	"unicode/utf8"
)

// Default music patterns: Artist/Album (Year)/NN - Title, and D-NN - Title
// on an album of more than one disc (the names albums got before patterns).
const (
	DefaultArtistFolderPattern   = "{artist}"
	DefaultAlbumFolderPattern    = "{album} ({year})"
	DefaultTrackPattern          = "{track} - {track_title}"
	DefaultMultiDiscTrackPattern = "{disc}-{track} - {track_title}"
	DefaultDiscFolderPattern     = "Disc {disc}"
)

// MusicMaxComponent is the most bytes one album folder or track file name
// may take: under the 255 most file systems allow, with room to spare.
const MusicMaxComponent = 240

// musicWords may appear only in music patterns ({year} and {edition} are
// shared with movies).
var musicWords = map[string]bool{"artist": true, "artist_sort": true, "album": true, "album_type": true, "disc": true, "track": true,
	"track_title": true, "track_artist": true, "format": true}

func init() {
	for k, v := range map[string]string{
		"artist":       "the artist name",
		"artist_sort":  "the artist name with a leading The moved to the end (Beatles, The)",
		"album":        "the album name",
		"album_type":   "the album kind, such as Album or EP",
		"disc":         "the disc number, such as 2",
		"track":        "the track number on its disc, two digits, such as 03",
		"track_title":  "the track name",
		"track_artist": "the artist (planned: a track's own artist on a various-artists album, once track credits are read)",
		"format":       "the file kind, such as FLAC or MP3",
	} {
		Words[k] = v
	}
}

// TrackFacts are what an artist folder, album folder and track file name
// are made from.
type TrackFacts struct {
	Artist      string
	Album       string
	AlbumType   string // album, ep, single, ...
	Edition     string // the Pressing's or album's disambiguation, such as deluxe
	Year        int    // the album's release year; 0: unknown
	Disc        int
	Discs       int // discs on the album's chosen Pressing
	Track       int
	Title       string // the track name
	TrackArtist string // "" takes the artist
	Ext         string // with the dot
}

func (s *NamingSettings) fillMusic() {
	for _, f := range []struct {
		p   *string
		def string
	}{{&s.ArtistFolderPattern, DefaultArtistFolderPattern}, {&s.AlbumFolderPattern, DefaultAlbumFolderPattern},
		{&s.TrackPattern, DefaultTrackPattern}, {&s.MultiDiscTrackPattern, DefaultMultiDiscTrackPattern}, {&s.DiscFolderPattern, DefaultDiscFolderPattern}} {
		if strings.TrimSpace(*f.p) == "" {
			*f.p = f.def
		}
	}
}

// validateMusic says what is wrong with the music patterns.
func (s NamingSettings) validateMusic() error {
	for _, p := range []struct{ name, pat, need, alt string }{
		{"artist folder pattern", s.ArtistFolderPattern, "artist", "artist_sort"},
		{"album folder pattern", s.AlbumFolderPattern, "album", ""},
		{"track pattern", s.TrackPattern, "track", ""},
		{"multi-disc track pattern", s.MultiDiscTrackPattern, "disc", ""},
		{"disc folder pattern", s.DiscFolderPattern, "disc", ""},
	} {
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
		for w := range words {
			if !musicWords[w] && w != "year" && w != "edition" {
				return fmt.Errorf("the %s cannot use {%s}; music patterns use music words such as {artist}, {album} and {track}", p.name, w)
			}
		}
		if !words[p.need] && (p.alt == "" || !words[p.alt]) {
			if p.alt != "" {
				return fmt.Errorf("the %s must hold {%s} or {%s}", p.name, p.need, p.alt)
			}
			return fmt.Errorf("the %s must hold {%s}", p.name, p.need)
		}
		if p.name == "multi-disc track pattern" && !words["track"] {
			return fmt.Errorf("the %s must hold {track}", p.name)
		}
	}
	return nil
}

func musicVals(f TrackFacts) map[string]string {
	year := ""
	if f.Year > 0 {
		year = fmt.Sprint(f.Year)
	}
	ta := f.TrackArtist
	if strings.TrimSpace(ta) == "" {
		ta = f.Artist
	}
	track := ""
	if f.Track > 0 {
		track = fmt.Sprintf("%02d", f.Track)
	}
	disc := ""
	if f.Disc > 0 {
		disc = fmt.Sprint(f.Disc)
	}
	kind := f.AlbumType
	switch strings.ToLower(kind) {
	case "ep":
		kind = "EP"
	case "":
	default:
		kind = strings.ToUpper(kind[:1]) + kind[1:]
	}
	return map[string]string{"artist": f.Artist, "artist_sort": theLast(f.Artist), "album": f.Album, "album_type": kind, "year": year,
		"edition": f.Edition, "disc": disc, "track": track, "track_title": f.Title, "track_artist": ta,
		"format": strings.ToUpper(strings.TrimPrefix(f.Ext, "."))}
}

// theLast moves a leading "The " to the end: "The Beatles" sorts as
// "Beatles, The".
func theLast(n string) string {
	if rest, ok := strings.CutPrefix(n, "The "); ok && strings.TrimSpace(rest) != "" {
		return rest + ", The"
	}
	return n
}

// fit renders pat and, when the name is longer than n bytes, shortens the
// value of key (the album or track name) so the rest (the year, the track
// number) stays whole; capName cuts what is still too long.
func fit(s NamingSettings, pat string, vals map[string]string, key string, n int) string {
	out := renderVals(s, pat, vals)
	for i := 0; len(out) > n && i < 6 && vals[key] != ""; i++ {
		v := vals[key]
		keep := len(v) - (len(out) - n)
		if keep <= 0 {
			vals[key] = ""
		} else {
			for keep > 0 && !utf8.RuneStart(v[keep]) {
				keep--
			}
			vals[key] = strings.TrimRight(v[:keep], " .-_")
		}
		out = renderVals(s, pat, vals)
	}
	return capName(out, n)
}

// ArtistFolder names an artist folder from the artist folder pattern.
func ArtistFolder(s NamingSettings, f TrackFacts) string {
	s.fillMusic()
	name := fit(s, s.ArtistFolderPattern, musicVals(f), "artist", MusicMaxComponent)
	if name == "" {
		return "Unnamed"
	}
	return name
}

// AlbumFolder names an album folder from the album folder pattern; an
// album name that cleans to nothing is "Unknown album".
func AlbumFolder(s NamingSettings, f TrackFacts) string {
	s.fillMusic()
	if clean(s, f.Album) == "" {
		f.Album = "Unknown album"
	}
	name := fit(s, s.AlbumFolderPattern, musicVals(f), "album", MusicMaxComponent)
	if name == "" {
		return "Unknown album"
	}
	return name
}

// DiscFolder names the folder of one disc ("" unless disc folders are on
// and the album has more than one disc).
func DiscFolder(s NamingSettings, f TrackFacts) string {
	s.fillMusic()
	if !s.DiscFolders || f.Discs < 2 || f.Disc < 1 {
		return ""
	}
	return capName(renderVals(s, s.DiscFolderPattern, musicVals(f)), MusicMaxComponent)
}

// TrackFile names a track file (with its extension): the multi-disc
// pattern on an album of more than one disc without disc folders, the
// track pattern otherwise.
func TrackFile(s NamingSettings, f TrackFacts) string {
	s.fillMusic()
	pat := s.TrackPattern
	if f.Discs > 1 && !s.DiscFolders {
		pat = s.MultiDiscTrackPattern
	}
	ext := strings.ToLower(f.Ext)
	name := fit(s, pat, musicVals(f), "track_title", MusicMaxComponent-len(ext))
	if name == "" {
		name = "Unnamed"
	}
	return name + ext
}

// TrackPath is where a track file goes inside the Library folder, with
// forward slashes: the artist folder, the album folder, the disc folder
// (when on) and the file. An import and a rename both use it, so a rename
// preview is what a rename does.
func TrackPath(s NamingSettings, artistFolder string, f TrackFacts) string {
	return path.Join(artistFolder, AlbumFolder(s, f), DiscFolder(s, f), TrackFile(s, f))
}
