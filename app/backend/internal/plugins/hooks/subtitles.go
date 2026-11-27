package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// Subtitles is the hook a subtitle-site plugin answers on (sources.subtitles):
// it offers subtitles for a video and makes the one HolyArrs picks, for
// example by transcribing the audio.
const Subtitles = "subtitles"

func init() { Scopes[Subtitles] = "sources.subtitles" }

// SubtitleFile says which video a job is about: its id, and where it sits
// for the plugin when it is inside a folder the owner granted it (that
// folder's name in the manifest and the path inside it, with forward
// slashes). A file outside its folders has no Folder or Path. The full path
// on the server is never sent.
type SubtitleFile struct {
	FileID int64  `json:"file_id"`
	Folder string `json:"folder,omitempty"`
	Path   string `json:"path,omitempty"`
}

// SubtitleAsk is one search (op "search"): the title, the release and the
// languages wanted, as two-letter codes.
type SubtitleAsk struct {
	Kind        string   `json:"kind"` // movie or episode
	Title       string   `json:"title,omitempty"`
	Year        int      `json:"year,omitempty"`
	Season      int      `json:"season,omitempty"`
	Episode     int      `json:"episode,omitempty"`
	IMDb        string   `json:"imdb,omitempty"`
	TMDB        string   `json:"tmdb,omitempty"`
	TVDB        string   `json:"tvdb,omitempty"`
	ReleaseName string   `json:"release_name,omitempty"`
	Languages   []string `json:"languages"`
	SubtitleFile
}

// SubtitleOffer is one subtitle the plugin can make. Key is its own id for
// it, handed back unchanged when HolyArrs asks for it.
type SubtitleOffer struct {
	Key             string `json:"key"`
	Language        string `json:"language"`
	HearingImpaired bool   `json:"hearing_impaired,omitempty"`
	Forced          bool   `json:"forced,omitempty"`
	Format          string `json:"format,omitempty"`
}

// SubtitleMake asks for one offered subtitle (op "make").
type SubtitleMake struct {
	Key      string `json:"key"`
	Language string `json:"language"`
	SubtitleFile
}

// SubtitleMade is the made subtitle: the file and its format (srt, vtt,
// ass, ssa or zip).
type SubtitleMade struct {
	Format string `json:"format,omitempty"`
	Body   []byte `json:"body"`
}

const (
	// subtitleSearchWait is how long a search waits for the plugin's offers.
	// A search asks every site in turn, so a plugin only looks, it never
	// makes anything here.
	subtitleSearchWait = 30 * time.Second
	// SubtitleMakeWait is how long HolyArrs waits for a made subtitle.
	// Transcribing a film takes minutes, so this runs in a job of its own.
	SubtitleMakeWait = time.Hour
	subtitleOffers   = 20
	subtitleMax      = 8 << 20 // a subtitle site's download cap
)

// SearchSubtitles asks plugin what it can offer for ask. Offers with no key
// or language are left out, and at most 20 are kept.
func SearchSubtitles(ctx context.Context, b *Broker, plugin string, ask SubtitleAsk) ([]SubtitleOffer, error) {
	raw, err := b.Call(ctx, plugin, Subtitles, "search", ask, subtitleSearchWait)
	if err != nil {
		return nil, err
	}
	var in []SubtitleOffer
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, errors.New("the plugin's answer could not be read")
		}
	}
	out := []SubtitleOffer{}
	for _, o := range in {
		if len(out) == subtitleOffers {
			break
		}
		if !cleanField(o.Key, 200) || !cleanField(o.Language, 16) || o.Language == "" || o.Key == "" {
			continue
		}
		o.Format = strings.ToLower(o.Format)
		if !cleanField(o.Format, 8) {
			o.Format = ""
		}
		out = append(out, o)
	}
	return out, nil
}

// MakeSubtitle asks plugin for one offered subtitle and waits up to an hour.
func MakeSubtitle(ctx context.Context, b *Broker, plugin string, ask SubtitleMake) (SubtitleMade, error) {
	raw, err := b.Call(ctx, plugin, Subtitles, "make", ask, SubtitleMakeWait)
	if err != nil {
		return SubtitleMade{}, err
	}
	var m SubtitleMade
	if err := json.Unmarshal(raw, &m); err != nil {
		return SubtitleMade{}, errors.New("the plugin's answer could not be read")
	}
	if len(m.Body) == 0 {
		return SubtitleMade{}, errors.New("the plugin sent an empty subtitle")
	}
	if len(m.Body) > subtitleMax {
		return SubtitleMade{}, errors.New("the plugin's answer is too large")
	}
	m.Format = strings.ToLower(m.Format)
	if !cleanField(m.Format, 8) {
		m.Format = ""
	}
	return m, nil
}

// cleanField says whether s is valid UTF-8 of at most n bytes with no
// control characters.
func cleanField(s string, n int) bool {
	if len(s) > n || !utf8.ValidString(s) {
		return false
	}
	return !strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f })
}
