package sites

import (
	"bytes"
	"context"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// yify reads a movie subtitle site that lists a movie's subtitles in one HTML
// table found by the movie's IMDb id. Movies only.
type yify struct{ c *client }

const browserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"

var imdbID = regexp.MustCompile(`^tt[0-9]{1,10}$`)

func newYify(c *client) *yify {
	c.keyHeader = ""
	c.userAgent = browserAgent
	return &yify{c: c}
}

// yifyLangs maps the site's English language names to our codes.
var yifyLangs = map[string]string{
	"albanian": "sq", "arabic": "ar", "bengali": "bn", "brazilian portuguese": "pt-br", "bulgarian": "bg", "chinese": "zh",
	"croatian": "hr", "czech": "cs", "danish": "da", "dutch": "nl", "english": "en", "farsi/persian": "fa", "finnish": "fi",
	"french": "fr", "german": "de", "greek": "el", "hebrew": "he", "hungarian": "hu", "indonesian": "id", "italian": "it",
	"japanese": "ja", "korean": "ko", "lithuanian": "lt", "macedonian": "mk", "malay": "ms", "norwegian": "no", "polish": "pl",
	"portuguese": "pt", "romanian": "ro", "russian": "ru", "serbian": "sr", "slovenian": "sl", "spanish": "es", "swedish": "sv",
	"thai": "th", "turkish": "tr", "urdu": "ur", "vietnamese": "vi",
}

func (y *yify) Search(ctx context.Context, q Query) ([]Candidate, error) {
	imdb := strings.ToLower(strings.TrimSpace(q.IMDb))
	if q.Kind != "movie" || !imdbID.MatchString(imdb) {
		return nil, nil
	}
	langs := wantedLangs(q.Languages)
	if len(langs) == 0 {
		return nil, nil
	}
	b, err := y.c.getPage(ctx, "/movie-imdb/"+imdb, map[string]string{"Referer": y.c.base.String() + "/"}, true)
	if err != nil || b == nil {
		return nil, err
	}
	type scored struct {
		c      Candidate
		rating int
	}
	var found []scored
	for _, r := range tableRows(b, func(a []byte) bool { return hasClass(a, "other-subs") }) {
		if len(r.cells) < 5 {
			continue
		}
		rating, err := strconv.Atoi(strings.TrimSpace(r.cells[0]))
		if err != nil {
			continue
		}
		lang := yifyLangs[strings.ToLower(strings.TrimSpace(r.cells[1]))]
		if lang == "" || !langWanted(langs, lang) {
			continue
		}
		p, ok := sitePath(y.c, r.hrefs[2])
		if !ok || !strings.HasPrefix(p, "/subtitles/") || len(p) == len("/subtitles/") {
			continue
		}
		rel := strings.TrimSpace(r.cells[2])
		if len(rel) >= 9 && strings.EqualFold(rel[:9], "subtitle ") {
			rel = strings.TrimSpace(rel[9:])
		}
		c := Candidate{Site: y.c.cfg.Name, Key: strings.TrimPrefix(p, "/subtitles/"), Language: lang, Title: q.Title, Year: q.Year,
			MatchedByID: true, Uploader: strings.TrimSpace(r.cells[4]), Format: "srt", Link: p}
		for _, cl := range r.classes {
			if strings.EqualFold(cl, "hi-subtitle") {
				c.HearingImpaired = true
			}
		}
		if rel != "" {
			c.ReleaseNames = []string{rel}
		}
		found = append(found, scored{c, rating})
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].rating > found[j].rating })
	out := make([]Candidate, 0, len(found))
	for _, f := range found {
		out = append(out, f.c)
	}
	return out, nil
}

func (y *yify) Download(ctx context.Context, c Candidate) ([]byte, error) {
	p, ok := sitePath(y.c, c.Link)
	if !ok || !strings.HasPrefix(p, "/subtitles/") {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site sent a link HolyArrs cannot follow"}
	}
	ref, err := y.c.ref(p)
	if err != nil {
		return nil, err
	}
	b, err := y.c.getPage(ctx, p, map[string]string{"Referer": ref}, false)
	if err != nil {
		return nil, err
	}
	dl := ""
	tokens(b, func(t token) {
		if dl == "" && t.tag == "a" && !t.end && hasClass(t.attrs, "download-subtitle") {
			dl = t.href
		}
	})
	dp, ok := sitePath(y.c, dl)
	if !ok {
		return nil, &Error{Kind: KindUnreadable, Msg: "the subtitle page has no download link"}
	}
	raw, err := y.c.ref(dp)
	if err != nil {
		return nil, err
	}
	z, err := y.c.do(ctx, http.MethodGet, raw, nil, map[string]string{"Referer": ref}, "", maxDownloadBody, false)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(z, []byte("PK\x03\x04")) {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site's download is not a zip"}
	}
	return z, nil
}
