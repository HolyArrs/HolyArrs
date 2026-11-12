package sites

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// tvsubs reads a show subtitle site in three steps: the show search, the
// season page with its episode ids, and the episode page with its subtitles.
// Show and episode ids rarely change, so they are kept in memory. Shows only.
type tvsubs struct {
	c *client

	mu    sync.Mutex
	shows map[string]int         // folded title and year -> show id
	eps   map[string]map[int]int // show id and season -> episode -> episode id
}

func newTVSubs(c *client) *tvsubs {
	c.keyHeader = ""
	c.userAgent = browserAgent
	return &tvsubs{c: c, shows: map[string]int{}, eps: map[string]map[int]int{}}
}

var (
	tvShowLink = regexp.MustCompile(`^/tvshow-([0-9]{1,9})\.html$`)
	tvEpLink   = regexp.MustCompile(`^/episode-([0-9]{1,9})\.html$`)
	tvSubLink  = regexp.MustCompile(`^/subtitle-([0-9]{1,9})\.html$`)
	tvYears    = regexp.MustCompile(`\(\s*([0-9]{4})\s*-\s*([0-9]{4})?\s*\)\s*$`)
	tvParen    = regexp.MustCompile(`\s*\([^()]*\)\s*$`)
	tvPiece    = regexp.MustCompile(`(?:^|[^A-Za-z0-9_])s[0-9]{1,2}\s*=\s*'([^'\\]{0,200})'`)
)

// tvShowName splits "Name (US) (2005-2013)" into the name and first year.
func tvShowName(s string) (string, int) {
	year := 0
	if m := tvYears.FindStringSubmatch(s); m != nil {
		year, _ = strconv.Atoi(m[1])
		s = s[:len(s)-len(m[0])]
	}
	for i := 0; i < 3; i++ {
		s = tvParen.ReplaceAllString(s, "")
	}
	return strings.TrimSpace(s), year
}

// tvFlags maps the site's flag codes to ours; the rest are our codes as is.
var tvFlags = map[string]string{"br": "pt-br", "ua": "uk", "gr": "el", "cn": "zh", "jp": "ja", "cz": "cs"}

var tvKnown = map[string]bool{"ar": true, "bg": true, "cs": true, "da": true, "de": true, "el": true, "en": true, "fi": true, "fr": true,
	"hu": true, "it": true, "ja": true, "ko": true, "nl": true, "pl": true, "pt": true, "pt-br": true, "ro": true, "ru": true, "es": true,
	"sv": true, "tr": true, "uk": true, "zh": true}

func tvLang(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if c, ok := tvFlags[code]; ok {
		code = c
	}
	if !tvKnown[code] {
		return ""
	}
	return code
}

func (s *tvsubs) Search(ctx context.Context, q Query) ([]Candidate, error) {
	if q.Kind != "episode" || q.Season <= 0 || q.Episode <= 0 || strings.TrimSpace(q.Title) == "" {
		return nil, nil
	}
	langs := wantedLangs(q.Languages)
	if len(langs) == 0 {
		return nil, nil
	}
	show, err := s.show(ctx, q)
	if err != nil || show == 0 {
		return nil, err
	}
	eps, err := s.season(ctx, show, q.Season)
	if err != nil {
		return nil, err
	}
	ep := eps[q.Episode]
	if ep == 0 {
		return nil, nil
	}
	b, err := s.c.getPage(ctx, "/episode-"+strconv.Itoa(ep)+".html", nil, true)
	if err != nil || b == nil {
		return nil, err
	}
	type item struct {
		id, flag, release, rip string
		block                  bool
	}
	var out []Candidate
	var cur *item
	var capture *strings.Builder
	var into *string
	emit := func() {
		if cur == nil || !cur.block || len(out) >= addic7edMaxRows {
			return
		}
		lang := tvLang(cur.flag)
		if lang == "" || !langWanted(langs, lang) {
			return
		}
		c := Candidate{Site: s.c.cfg.Name, Key: cur.id, Language: lang, Title: q.Title, Year: q.Year, Season: q.Season, Episode: q.Episode,
			Format: "srt", Link: "/download-" + cur.id + ".html"}
		if r := clip(cur.release); r != "" {
			c.ReleaseNames = append(c.ReleaseNames, r)
		}
		if r := clip(cur.rip); r != "" && (len(c.ReleaseNames) == 0 || r != c.ReleaseNames[0]) {
			c.ReleaseNames = append(c.ReleaseNames, r)
		}
		out = append(out, c)
	}
	tokens(b, func(t token) {
		switch {
		case t.tag == "a" && !t.end:
			emit()
			cur = nil
			if p, ok := sitePath(s.c, t.href); ok {
				if m := tvSubLink.FindStringSubmatch(p); m != nil {
					cur = &item{id: m[1]}
				}
			}
		case t.tag == "a" && t.end:
			emit()
			cur, capture, into = nil, nil, nil
		case cur == nil:
		case t.tag == "div" && !t.end && hasClass(t.attrs, "subtitlen"):
			cur.block = true
		case t.tag == "img" && !t.end && cur.flag == "":
			src := attr(t.attrs, "src")
			if i := strings.Index(src, "images/flags/"); i >= 0 {
				cur.flag = strings.TrimSuffix(path.Base(src[i:]), ".gif")
			}
		case t.tag == "h5" && !t.end:
			capture, into = &strings.Builder{}, &cur.release
		case t.tag == "p" && !t.end && strings.EqualFold(attr(t.attrs, "title"), "rip"):
			capture, into = &strings.Builder{}, &cur.rip
		case (t.tag == "h5" || t.tag == "p") && t.end && capture != nil:
			*into = capture.String()
			capture, into = nil, nil
		case t.tag == "" && capture != nil && capture.Len() < extraMaxText:
			capture.WriteString(t.text)
		}
	})
	emit()
	return out, nil
}

func (s *tvsubs) show(ctx context.Context, q Query) (int, error) {
	key := fold(q.Title) + "|" + strconv.Itoa(q.Year)
	s.mu.Lock()
	id, ok := s.shows[key]
	s.mu.Unlock()
	if ok {
		return id, nil
	}
	raw, err := s.c.ref("/search1.php")
	if err != nil {
		return 0, err
	}
	form := url.Values{"qs": {q.Title}}
	b, err := s.c.do(ctx, http.MethodPost, raw, []byte(form.Encode()), map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		"text/html, */*;q=0.5", addic7edMaxPage, true)
	if err != nil {
		return 0, err
	}
	for _, l := range links(b) {
		p, ok := sitePath(s.c, l.href)
		if !ok {
			continue
		}
		m := tvShowLink.FindStringSubmatch(p)
		if m == nil {
			continue
		}
		name, year := tvShowName(l.text)
		if !sameName(name, q.Title) || (q.Year > 0 && year > 0 && year != q.Year) {
			continue
		}
		id, _ = strconv.Atoi(m[1])
		break
	}
	if id == 0 {
		return 0, nil // not kept, so a show the site adds later is found
	}
	s.mu.Lock()
	if len(s.shows) > 1000 {
		s.shows = map[string]int{}
	}
	s.shows[key] = id
	s.mu.Unlock()
	return id, nil
}

func (s *tvsubs) season(ctx context.Context, show, season int) (map[int]int, error) {
	key := strconv.Itoa(show) + "-" + strconv.Itoa(season)
	s.mu.Lock()
	eps, ok := s.eps[key]
	s.mu.Unlock()
	if ok {
		return eps, nil
	}
	b, err := s.c.getPage(ctx, "/tvshow-"+key+".html", nil, true)
	if err != nil {
		return nil, err
	}
	eps = map[int]int{}
	for _, r := range tableRows(b, func(a []byte) bool { return attr(a, "id") == "table5" }) {
		if len(r.cells) == 0 {
			continue
		}
		_, num, ok := strings.Cut(strings.ToLower(r.cells[0]), "x")
		n, err := strconv.Atoi(strings.TrimSpace(num))
		if !ok || err != nil || n <= 0 {
			continue
		}
		for _, h := range r.hrefs {
			if p, ok := sitePath(s.c, h); ok {
				if m := tvEpLink.FindStringSubmatch(p); m != nil {
					eps[n], _ = strconv.Atoi(m[1])
					break
				}
			}
		}
	}
	s.mu.Lock()
	if len(s.eps) > 1000 {
		s.eps = map[string]map[int]int{}
	}
	s.eps[key] = eps
	s.mu.Unlock()
	return eps, nil
}

// tvDownloadPath joins the quoted pieces the download page builds its file
// path from. The path must stay on the site.
func tvDownloadPath(b []byte) (string, bool) {
	var sb strings.Builder
	for _, m := range tvPiece.FindAllSubmatch(head(b, 256<<10), 20) {
		sb.Write(m[1])
	}
	p := sb.String()
	if p == "" || len(p) > 300 || strings.HasPrefix(p, "//") || strings.Contains(p, ":") || strings.Contains(p, "..") || strings.Contains(p, `\`) {
		return "", false
	}
	return "/" + strings.TrimLeft(p, "/"), true
}

func (s *tvsubs) Download(ctx context.Context, c Candidate) ([]byte, error) {
	id, err := strconv.Atoi(c.Key)
	if err != nil || id <= 0 || strconv.Itoa(id) != c.Key {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site sent a link HolyArrs cannot follow"}
	}
	b, err := s.c.getPage(ctx, "/download-"+c.Key+".html", nil, false)
	if err != nil {
		return nil, err
	}
	p, ok := tvDownloadPath(b)
	if !ok {
		return nil, &Error{Kind: KindUnreadable, Msg: "the download page has no file path HolyArrs can follow"}
	}
	raw, err := s.c.ref(p)
	if err != nil {
		return nil, err
	}
	z, err := s.c.do(ctx, http.MethodGet, raw, nil, nil, "", maxDownloadBody, false)
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(z), int64(len(z)))
	if err != nil {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site's download is not a zip"}
	}
	files := 0
	for _, f := range zr.File {
		if !f.FileInfo().IsDir() {
			files++
		}
	}
	if files != 1 {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site's zip should hold one file"}
	}
	return z, nil
}
