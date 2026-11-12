// Package fakesite is the lab's subtitle site: a small HTTP server with
// made-up subtitles, used by tests and by cmd/holyarrs-fakesubs in the lab.
// Nothing it serves comes from a real site.
package fakesite

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// Sub is one subtitle the site offers.
type Sub struct {
	ID              string  `json:"id"`
	Kind            string  `json:"-"` // movie or episode
	Language        string  `json:"language"`
	HearingImpaired bool    `json:"hearing_impaired"`
	Forced          bool    `json:"forced"`
	Release         string  `json:"release"`
	Title           string  `json:"title"`
	Year            int     `json:"year,omitempty"`
	Season          int     `json:"season,omitempty"`
	Episode         int     `json:"episode,omitempty"`
	ByID            bool    `json:"by_id"`
	Uploader        string  `json:"uploader,omitempty"`
	Trusted         bool    `json:"trusted"`
	FPS             float64 `json:"fps,omitempty"`
	Corrected       bool    `json:"corrected"`
	Downloads       int     `json:"downloads"`
	Format          string  `json:"format"`
	Text            string  `json:"-"`
	Zipped          bool    `json:"-"` // served inside a zip
	// Echoed marks a subtitle invented from the query in Echo mode. Its release
	// name is derived from the one asked for.
	Echoed bool `json:"echoed,omitempty"`
	// AITranslated and MachineTranslated are the machine-made flags the
	// OpenSubtitles fake sends.
	AITranslated      bool `json:"-"`
	MachineTranslated bool `json:"-"`
}

// Site serves a catalog. With Echo set it also answers unknown titles with
// made-up subtitles built from the query, which the lab relies on.
type Site struct {
	mu       sync.Mutex
	Subs     []Sub
	Echo     bool
	Searches int
	Fail     int // when set, every call answers with this status
	// Key, when set, must arrive on every call as X-Api-Key. A wrong or missing
	// key gets a 401, like a site that needs an account.
	Key string
}

// Cue returns the text of a made-up subtitle with one cue at 1 s saying line.
func Cue(line string, startMS int) string {
	f := func(ms int) string {
		return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
	}
	return "1\r\n" + f(startMS) + " --> " + f(startMS+1500) + "\r\n" + line + "\r\n"
}

// Lab returns the lab catalog: the open movies of the lab library with made-up
// text, one of them two seconds out of sync and one packed in a zip.
func Lab() *Site {
	return &Site{Echo: true, Subs: []Sub{
		{ID: "bbb-en", Kind: "movie", Language: "en", Release: "Big.Buck.Bunny.2008.1080p.BluRay.x264-LAB", Title: "Big Buck Bunny", Year: 2008, Trusted: true, Format: "srt", Text: Cue("A big rabbit wakes up.", 1000), Downloads: 40},
		{ID: "bbb-es", Kind: "movie", Language: "es", Release: "Big.Buck.Bunny.2008.1080p.BluRay.x264-LAB", Title: "Big Buck Bunny", Year: 2008, Format: "srt", Text: Cue("Un gran conejo despierta.", 3000), Downloads: 9, Zipped: true},
		{ID: "ed-en", Kind: "movie", Language: "en", Release: "Elephants.Dream.2006.720p.WEB.x264-LAB", Title: "Elephants Dream", Year: 2006, Format: "srt", Text: Cue("The machine never sleeps.", 1000)},
	}}
}

// ServeHTTP answers /api/search and /api/download/{id}.
func (s *Site) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Fail != 0 {
		http.Error(w, "failing on purpose", s.Fail)
		return
	}
	if s.Key != "" && r.Header.Get("X-Api-Key") != s.Key {
		http.Error(w, "this site needs your key", http.StatusUnauthorized)
		return
	}
	switch {
	case r.URL.Path == "/api/search":
		s.Searches++
		s.search(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/download/"):
		s.download(w, strings.TrimPrefix(r.URL.Path, "/api/download/"))
	default:
		http.NotFound(w, r)
	}
}

func fold(v string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(v) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// labRelease is the release name the lab site gives a made-up subtitle: the
// asked release with the lab's own group. It shares the release parts without
// echoing the name, since an echo would score as an exact match.
func labRelease(asked string) string {
	if asked == "" {
		return ""
	}
	if i := strings.LastIndex(asked, "-"); i > 0 {
		asked = asked[:i]
	}
	return asked + "-HOLYARRSLAB"
}

func (s *Site) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind, title := q.Get("kind"), q.Get("title")
	season, _ := strconv.Atoi(q.Get("season"))
	episode, _ := strconv.Atoi(q.Get("episode"))
	langs := strings.Split(q.Get("lang"), ",")
	out := []Sub{}
	for _, sub := range s.Subs {
		if sub.Kind != kind || fold(sub.Title) != fold(title) || !slices.Contains(langs, sub.Language) {
			continue
		}
		if kind == "episode" && sub.Season != season {
			continue // sites answer for the asked season, so episodes may differ
		}
		out = append(out, sub)
	}
	if s.Echo && title != "" {
		year, _ := strconv.Atoi(q.Get("year"))
		for _, l := range langs {
			if l == "" || slices.ContainsFunc(out, func(x Sub) bool { return x.Language == l }) {
				continue // a language the catalog or an earlier echo already has
			}
			id := fmt.Sprintf("echo-%s-%s-%d-%d-%s", fold(title), kind, season, episode, l)
			sub := Sub{ID: id, Kind: kind, Language: l, Title: title, Year: year, Season: season, Episode: episode, ByID: q.Get("tmdb") != "" || q.Get("tvdb") != "", Release: labRelease(q.Get("release")), Echoed: true,
				Uploader: "lab", Trusted: true, Format: "srt", Text: Cue("Made-up line for "+title+".", 1000)}
			s.Subs = append(s.Subs, sub)
			out = append(out, sub)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"results": out})
}

func (s *Site) download(w http.ResponseWriter, id string) {
	for _, sub := range s.Subs {
		if sub.ID != id {
			continue
		}
		if !sub.Zipped {
			w.Header().Set("Content-Type", "application/x-subrip")
			_, _ = w.Write([]byte(sub.Text))
			return
		}
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		f, _ := zw.Create(sub.ID + "." + sub.Format)
		_, _ = f.Write([]byte(sub.Text))
		_ = zw.Close()
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(buf.Bytes())
		return
	}
	http.NotFound(w, nil)
}
