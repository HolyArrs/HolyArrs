package herovideo

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

// YouTube Music's own web API (the one its pages call): no key and no API
// points. A search for artists by name, then the artist's page and its
// full videos list.
const (
	musicClient  = "WEB_REMIX"
	musicVersion = "1.20251020.01.00"
	// artistsOnly is the search filter for artists.
	artistsOnly = "EgWKAQIgAWoMEA4QChADEAQQCRAF"
)

// Video labels YouTube Music gives (musicVideoType).
const (
	labelUserUpload = "MUSIC_VIDEO_TYPE_UGC"
)

// homeLanguage is the page language for the second search in an artist's
// home country: YouTube Music answers names in the page language, so a
// name in its own script (米津玄師, Кино, 아이유) only matches there.
var homeLanguage = map[string]string{
	"JP": "ja", "KR": "ko", "KP": "ko", "CN": "zh-CN", "TW": "zh-TW", "HK": "zh-HK", "MO": "zh-HK", "SG": "zh-CN",
	"RU": "ru", "BY": "ru", "KZ": "ru", "KG": "ru", "UA": "uk", "BG": "bg", "RS": "sr", "ME": "sr", "MK": "mk", "BA": "bs",
	"GR": "el", "CY": "el", "IL": "iw", "TH": "th", "VN": "vi", "IN": "hi", "PK": "ur", "BD": "bn", "LK": "si", "NP": "ne",
	"MM": "my", "KH": "km", "LA": "lo", "MN": "mn", "GE": "ka", "AM": "hy", "AZ": "az", "IR": "fa", "AF": "fa", "TR": "tr",
	"SA": "ar", "AE": "ar", "EG": "ar", "LB": "ar", "SY": "ar", "JO": "ar", "IQ": "ar", "KW": "ar", "QA": "ar", "BH": "ar",
	"OM": "ar", "YE": "ar", "LY": "ar", "SD": "ar", "PS": "ar", "MA": "ar", "DZ": "ar", "TN": "ar", "ET": "am",
	"ID": "id", "MY": "ms", "PH": "fil", "DE": "de", "AT": "de", "CH": "de", "FR": "fr", "BE": "fr", "ES": "es", "IT": "it",
	"PT": "pt-PT", "BR": "pt", "NL": "nl", "SE": "sv", "NO": "no", "DK": "da", "FI": "fi", "IS": "is", "PL": "pl", "CZ": "cs",
	"SK": "sk", "HU": "hu", "RO": "ro", "MD": "ro", "HR": "hr", "SI": "sl", "EE": "et", "LV": "lv", "LT": "lt", "AL": "sq",
	"MX": "es-419", "AR": "es-419", "CO": "es-419", "CL": "es-419", "PE": "es-419", "VE": "es-419", "EC": "es-419",
	"UY": "es-419", "PY": "es-419", "BO": "es-419", "CU": "es-419", "DO": "es-419", "PR": "es-419", "GT": "es-419",
}

// music calls one YouTube Music endpoint (search or browse). A 429 pauses
// the source (Retry-After when given); 5xx, 403 and network errors count
// toward its pause after failures.
func (s *Service) music(ctx context.Context, endpoint, gl, hl string, body map[string]any) (any, error) {
	if err := s.allow(ctx, srcMusic); err != nil {
		return nil, err
	}
	body["context"] = map[string]any{"client": map[string]any{"clientName": musicClient, "clientVersion": musicVersion, "hl": hl, "gl": gl}}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.o.Music+"/youtubei/v1/"+endpoint+"?prettyPrint=false", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", s.o.UserAgent)
	req.Header.Set("Origin", s.o.Music)
	resp, err := s.o.HTTP.Do(req)
	if err != nil {
		s.done(srcMusic, 0, err)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		s.backoff(srcMusic, retryAfter(resp))
		return nil, &StatusError{resp.StatusCode}
	}
	s.done(srcMusic, resp.StatusCode, nil)
	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{resp.StatusCode}
	}
	var out any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func retryAfter(resp *http.Response) time.Duration {
	n, err := strconv.Atoi(resp.Header.Get("Retry-After"))
	if err != nil || n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}

// findArtist answers the YouTube Music artist page (its browse ID) whose
// name matches the artist's (NFKC, case folded): searched in the US first,
// then, when the name misses, in the artist's home country and language.
// "" when neither search has the artist.
func (s *Service) findArtist(ctx context.Context, name, country string) (string, error) {
	id, err := s.searchArtist(ctx, name, "US", "en")
	if err != nil || id != "" {
		return id, err
	}
	c := strings.ToUpper(strings.TrimSpace(country))
	if len(c) != 2 || c == "US" || c[0] == 'X' { // MusicBrainz uses XW, XE... for regions
		return "", nil
	}
	hl := homeLanguage[c]
	if hl == "" {
		hl = "en"
	}
	return s.searchArtist(ctx, name, c, hl)
}

func (s *Service) searchArtist(ctx context.Context, name, gl, hl string) (string, error) {
	d, err := s.music(ctx, "search", gl, hl, map[string]any{"query": name, "params": artistsOnly})
	if err != nil {
		return "", err
	}
	for _, r := range searchArtists(d) {
		if imgstore.SameName(r.Name, name) {
			return r.ID, nil
		}
	}
	return "", nil
}

type musicArtist struct{ Name, ID string }

// searchArtists reads the artists of a search answer, in its order.
func searchArtists(d any) []musicArtist {
	var out []musicArtist
	each(d, "musicResponsiveListItemRenderer", func(it any) {
		be := dig(it, "navigationEndpoint", "browseEndpoint")
		id := str(dig(be, "browseId"))
		page := str(dig(be, "browseEndpointContextSupportedConfigs", "browseEndpointContextMusicConfig", "pageType"))
		if page != "MUSIC_PAGE_TYPE_ARTIST" || !channelID.MatchString(id) {
			return
		}
		out = append(out, musicArtist{Name: text(dig(it, "flexColumns", 0, "musicResponsiveListItemFlexColumnRenderer", "text")), ID: id})
	})
	return out
}

// artistVideos reads an artist page's Videos shelf and, when the shelf
// links to the full list, that list's first page: the shelf's videos
// first, then the list's others, each once, in YouTube Music's order.
func (s *Service) artistVideos(ctx context.Context, browseID string) ([]cand, error) {
	page, err := s.music(ctx, "browse", "US", "en", map[string]any{"browseId": browseID})
	if err != nil {
		return nil, err
	}
	shelf, more := videoShelf(page)
	cands := shelf
	if more != "" {
		list, err := s.music(ctx, "browse", "US", "en", map[string]any{"browseId": more})
		if err != nil {
			return nil, err
		}
		cands = append(cands, listVideos(list)...)
	}
	seen := map[string]bool{}
	out := cands[:0:0]
	for _, c := range cands {
		if !seen[c.ID] {
			seen[c.ID] = true
			c.Source = SourceYouTubeMusic
			out = append(out, c)
		}
	}
	return out, nil
}

// videoShelf reads the artist page's "Videos" shelf (pages are asked for
// in English) and the browse ID of its full list ("" when it has none).
func videoShelf(page any) ([]cand, string) {
	var out []cand
	more, found := "", false
	each(page, "musicCarouselShelfRenderer", func(sh any) {
		hdr := dig(sh, "header", "musicCarouselShelfBasicHeaderRenderer")
		if found || imgstore.FoldName(text(dig(hdr, "title"))) != "videos" {
			return
		}
		found = true
		more = str(dig(hdr, "moreContentButton", "buttonRenderer", "navigationEndpoint", "browseEndpoint", "browseId"))
		if more == "" {
			more = str(dig(hdr, "title", "runs", 0, "navigationEndpoint", "browseEndpoint", "browseId"))
		}
		items, _ := dig(sh, "contents").([]any)
		for _, x := range items {
			it := dig(x, "musicTwoRowItemRenderer")
			if it == nil {
				continue
			}
			c := itemVideo(it)
			c.Title = text(dig(it, "title"))
			c.Line = artistLine(text(dig(it, "subtitle")))
			if c.ID != "" {
				out = append(out, c)
			}
		}
	})
	return out, more
}

// listVideos reads the videos of a full list page, in order.
func listVideos(list any) []cand {
	var out []cand
	each(list, "musicResponsiveListItemRenderer", func(it any) {
		c := itemVideo(it)
		if id := str(dig(it, "playlistItemData", "videoId")); videoID.MatchString(id) {
			c.ID = id
		}
		if c.ID == "" {
			return
		}
		c.Title = text(dig(it, "flexColumns", 0, "musicResponsiveListItemFlexColumnRenderer", "text"))
		c.Line = artistLine(text(dig(it, "flexColumns", 1, "musicResponsiveListItemFlexColumnRenderer", "text")))
		c.Seconds = clock(text(dig(it, "fixedColumns", 0, "musicResponsiveListItemFixedColumnRenderer", "text")))
		out = append(out, c)
	})
	return out
}

// itemVideo reads an item's video ID and label from its watch links (the
// title's link may carry no label; the play button's does).
func itemVideo(it any) cand {
	var c cand
	each(it, "watchEndpoint", func(w any) {
		id := str(dig(w, "videoId"))
		if !videoID.MatchString(id) || c.ID != "" && id != c.ID {
			return
		}
		c.ID = id
		if c.Label == "" {
			c.Label = str(dig(w, "watchEndpointMusicSupportedConfigs", "watchEndpointMusicConfig", "musicVideoType"))
		}
	})
	return c
}

// artistLine is the artists part of a subtitle ("Drake & Future • 3M
// views" is "Drake & Future").
func artistLine(s string) string {
	s, _, _ = strings.Cut(s, " • ")
	return strings.TrimSpace(s)
}

var clockText = regexp.MustCompile(`^(?:(\d+):)?(\d+):(\d\d)$`)

// clock reads a duration shown as 4:05 or 1:02:03 (0 when unreadable).
func clock(s string) int {
	m := clockText.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	n := func(s string) int { v, _ := strconv.Atoi(s); return v }
	return n(m[1])*3600 + n(m[2])*60 + n(m[3])
}

// each calls fn on every value stored under key, depth first; lists keep
// their order (object keys are visited sorted, so the walk is stable). A
// value found is not searched further.
func each(v any, key string, fn func(any)) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if k == key {
				fn(t[k])
				continue
			}
			each(t[k], key, fn)
		}
	case []any:
		for _, x := range t {
			each(x, key, fn)
		}
	}
}

// dig follows object keys (string) and list positions (int).
func dig(v any, path ...any) any {
	for _, p := range path {
		switch k := p.(type) {
		case string:
			m, ok := v.(map[string]any)
			if !ok {
				return nil
			}
			v = m[k]
		case int:
			l, ok := v.([]any)
			if !ok || k < 0 || k >= len(l) {
				return nil
			}
			v = l[k]
		}
	}
	return v
}

func str(v any) string { s, _ := v.(string); return s }

// text reads a YouTube text: its runs joined, or its simpleText.
func text(v any) string {
	if s := str(dig(v, "simpleText")); s != "" {
		return s
	}
	runs, _ := dig(v, "runs").([]any)
	var b strings.Builder
	for _, r := range runs {
		b.WriteString(str(dig(r, "text")))
	}
	return b.String()
}
