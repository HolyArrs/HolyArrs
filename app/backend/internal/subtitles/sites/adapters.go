package sites

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

// The answer shapes below are our own reading of each site's public API.
// They are checked against replies we recorded in testdata, never against
// the live sites.

// gestdown serves show subtitles from Addic7ed as JSON. It covers shows only.
type gestdown struct{ c *client }

type gdShow struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	TVDbID int    `json:"tvDbId"`
}

type gdSub struct {
	SubtitleID      string `json:"subtitleId"`
	Version         string `json:"version"`
	Completed       bool   `json:"completed"`
	HearingImpaired bool   `json:"hearingImpaired"`
	Corrected       bool   `json:"corrected"`
	DownloadURI     string `json:"downloadUri"`
	Language        string `json:"language"`
	DownloadCount   int    `json:"downloadCount"`
}

func (g *gestdown) Search(ctx context.Context, q Query) ([]Candidate, error) {
	if q.Kind != "episode" || q.Title == "" || q.Season <= 0 || q.Episode <= 0 {
		return nil, nil
	}
	body, err := g.c.get(ctx, "/shows/search/"+url.PathEscape(q.Title), "application/json", maxSearchBody, true)
	if err != nil || body == nil {
		return nil, err
	}
	var shows struct {
		Shows []gdShow `json:"shows"`
	}
	if err := json.Unmarshal(body, &shows); err != nil {
		return nil, unreadable("show search", err)
	}
	var show *gdShow
	byID := false
	for i, s := range shows.Shows {
		if q.TVDB != "" && strconv.Itoa(s.TVDbID) == q.TVDB {
			show, byID = &shows.Shows[i], true
			break
		}
		if show == nil && sameName(s.Name, q.Title) {
			show = &shows.Shows[i]
		}
	}
	if show == nil {
		return nil, nil
	}
	var out []Candidate
	for _, lang := range q.Languages {
		name := langName(lang)
		if name == "" {
			continue
		}
		p := "/subtitles/get/" + url.PathEscape(show.ID) + "/" + strconv.Itoa(q.Season) + "/" + strconv.Itoa(q.Episode) + "/" + url.PathEscape(name)
		body, err := g.c.get(ctx, p, "application/json", maxSearchBody, true)
		if err != nil {
			return out, err
		}
		if body == nil {
			continue
		}
		var res struct {
			Matching []gdSub `json:"matchingSubtitles"`
			Episode  struct {
				Season int    `json:"season"`
				Number int    `json:"number"`
				Show   string `json:"show"`
			} `json:"episode"`
		}
		if err := json.Unmarshal(body, &res); err != nil {
			return out, unreadable("subtitle list", err)
		}
		for _, s := range res.Matching {
			if !s.Completed || s.SubtitleID == "" || s.DownloadURI == "" {
				continue
			}
			season, episode := res.Episode.Season, res.Episode.Number
			if season == 0 {
				season, episode = q.Season, q.Episode
			}
			out = append(out, Candidate{Site: g.c.cfg.Name, Key: s.SubtitleID, Language: lang, HearingImpaired: s.HearingImpaired,
				ReleaseNames: []string{show.Name + ".S" + two(season) + "E" + two(episode) + "." + s.Version}, Title: show.Name,
				Season: season, Episode: episode, MatchedByID: byID, Corrected: s.Corrected, Downloads: s.DownloadCount,
				Format: "srt", Link: s.DownloadURI})
		}
	}
	return out, nil
}

func (g *gestdown) Download(ctx context.Context, c Candidate) ([]byte, error) {
	return g.c.get(ctx, c.Link, "", maxDownloadBody, false)
}

// podnapisi answers a search with JSON and a download with a zip.
type podnapisi struct{ c *client }

type pnSub struct {
	ID       string   `json:"id"`
	Language string   `json:"language"`
	Flags    []string `json:"flags"`
	Releases []string `json:"releases"`
	Download string   `json:"download"`
	FPS      float64  `json:"fps"`
	Movie    struct {
		Title       string `json:"title"`
		Year        int    `json:"year"`
		Type        string `json:"type"`
		EpisodeInfo struct {
			Season  int `json:"season"`
			Episode int `json:"episode"`
		} `json:"episode_info"`
		Providers []string `json:"providers"`
	} `json:"movie"`
	Stats struct {
		Downloads int `json:"downloads"`
	} `json:"stats"`
	Author struct {
		Name    string `json:"name"`
		Trusted bool   `json:"trusted"`
	} `json:"author"`
}

func (p *podnapisi) Search(ctx context.Context, q Query) ([]Candidate, error) {
	if q.Title == "" {
		return nil, nil
	}
	v := url.Values{}
	v.Set("keywords", q.Title)
	if q.Year > 0 {
		v.Set("year", strconv.Itoa(q.Year))
	}
	if q.Kind == "episode" {
		v.Set("movie_type", "tv-series")
		v.Set("seasons", strconv.Itoa(q.Season))
		v.Set("episodes", strconv.Itoa(q.Episode))
	} else {
		v.Set("movie_type", "movie")
	}
	var out []Candidate
	for _, lang := range q.Languages {
		v.Set("language", lang)
		body, err := p.c.get(ctx, "/subtitles/search/advanced?"+v.Encode(), "application/json", maxSearchBody, true)
		if err != nil {
			return out, err
		}
		if body == nil {
			continue
		}
		var res struct {
			Data []pnSub `json:"data"`
		}
		if err := json.Unmarshal(body, &res); err != nil {
			return out, unreadable("search", err)
		}
		for _, s := range res.Data {
			if s.ID == "" || s.Download == "" {
				continue
			}
			c := Candidate{Site: p.c.cfg.Name, Key: s.ID, Language: lang, ReleaseNames: s.Releases, Title: s.Movie.Title, Year: s.Movie.Year,
				Uploader: s.Author.Name, Trusted: s.Author.Trusted, FPS: s.FPS, Downloads: s.Stats.Downloads, Link: s.Download}
			if s.Language != "" && !strings.EqualFold(s.Language, lang) {
				continue // the site answered in another language
			}
			for _, f := range s.Flags {
				switch f {
				case "hearing_impaired":
					c.HearingImpaired = true
				case "foreign_only":
					c.Forced = true
				}
			}
			if q.Kind == "episode" {
				c.Season, c.Episode = s.Movie.EpisodeInfo.Season, s.Movie.EpisodeInfo.Episode
			}
			for _, pr := range s.Movie.Providers {
				if (q.IMDb != "" && pr == "imdb:"+q.IMDb) || (q.TMDB != "" && pr == "tmdb:"+q.TMDB) {
					c.MatchedByID = true
				}
			}
			out = append(out, c)
		}
	}
	return out, nil
}

func (p *podnapisi) Download(ctx context.Context, c Candidate) ([]byte, error) {
	return p.c.get(ctx, c.Link, "", maxDownloadBody, false)
}

// fake is the lab's subtitle site, served by package fakesite.
type fake struct{ c *client }

type fakeResult struct {
	ID              string  `json:"id"`
	Language        string  `json:"language"`
	HearingImpaired bool    `json:"hearing_impaired"`
	Forced          bool    `json:"forced"`
	Release         string  `json:"release"`
	Title           string  `json:"title"`
	Year            int     `json:"year"`
	Season          int     `json:"season"`
	Episode         int     `json:"episode"`
	ByID            bool    `json:"by_id"`
	Uploader        string  `json:"uploader"`
	Trusted         bool    `json:"trusted"`
	FPS             float64 `json:"fps"`
	Corrected       bool    `json:"corrected"`
	Downloads       int     `json:"downloads"`
	Format          string  `json:"format"`
	Echoed          bool    `json:"echoed"`
}

func (f *fake) Search(ctx context.Context, q Query) ([]Candidate, error) {
	v := url.Values{}
	v.Set("kind", q.Kind)
	v.Set("title", q.Title)
	for k, x := range map[string]int{"year": q.Year, "season": q.Season, "episode": q.Episode} {
		if x > 0 {
			v.Set(k, strconv.Itoa(x))
		}
	}
	for k, x := range map[string]string{"imdb": q.IMDb, "tmdb": q.TMDB, "tvdb": q.TVDB} {
		if x != "" {
			v.Set(k, x)
		}
	}
	v.Set("lang", strings.Join(q.Languages, ","))
	if q.ReleaseName != "" {
		v.Set("release", q.ReleaseName)
	}
	body, err := f.c.get(ctx, "/api/search?"+v.Encode(), "application/json", maxSearchBody, false)
	if err != nil {
		return nil, err
	}
	var res struct {
		Results []fakeResult `json:"results"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, unreadable("search", err)
	}
	out := make([]Candidate, 0, len(res.Results))
	for _, r := range res.Results {
		if r.ID == "" {
			continue
		}
		var rel []string
		// The lab site marks subtitles it builds from our query as echoed. A release
		// name that equals ours there is just our own name sent back, so it never
		// counts as an exact release match.
		if r.Release != "" && !(r.Echoed && sameName(r.Release, q.ReleaseName)) {
			rel = []string{r.Release}
		}
		out = append(out, Candidate{Site: f.c.cfg.Name, Key: r.ID, Language: r.Language, HearingImpaired: r.HearingImpaired, Forced: r.Forced,
			ReleaseNames: rel, Title: r.Title, Year: r.Year, Season: r.Season, Episode: r.Episode, MatchedByID: r.ByID, Uploader: r.Uploader,
			Trusted: r.Trusted, FPS: r.FPS, Corrected: r.Corrected, Downloads: r.Downloads, Format: r.Format, Link: "/api/download/" + url.PathEscape(r.ID)})
	}
	return out, nil
}

func (f *fake) Download(ctx context.Context, c Candidate) ([]byte, error) {
	return f.c.get(ctx, c.Link, "", maxDownloadBody, false)
}

func two(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// sameName compares names ignoring case and every character but letters and digits.
func sameName(a, b string) bool { return fold(a) == fold(b) && fold(a) != "" }

func fold(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 127 {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// langNames maps two-letter codes to the English language names Gestdown expects.
var langNames = map[string]string{"en": "English", "es": "Spanish", "fr": "French", "de": "German", "it": "Italian", "pt": "Portuguese",
	"nl": "Dutch", "pl": "Polish", "ro": "Romanian", "ru": "Russian", "sv": "Swedish", "tr": "Turkish", "el": "Greek", "he": "Hebrew",
	"hu": "Hungarian", "cs": "Czech", "da": "Danish", "fi": "Finnish", "no": "Norwegian", "ar": "Arabic", "bg": "Bulgarian", "hr": "Croatian",
	"sr": "Serbian", "sl": "Slovenian", "ca": "Catalan", "eu": "Euskera", "gl": "Galego", "ja": "Japanese", "ko": "Korean", "zh": "Chinese (Simplified)"}

func langName(code string) string { return langNames[code] }
