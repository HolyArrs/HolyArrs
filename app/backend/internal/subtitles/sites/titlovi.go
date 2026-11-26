package sites

import (
	"archive/zip"
	"bytes"
	"context"
	"html"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/nwaples/rardecode/v2"
)

// titlovi reads Titlovi.com, the Croatian, Serbian, Bosnian, Slovenian and
// Macedonian subtitle site. It reads the public search page and needs no
// account. Movies and shows.
//
// A search page lists subtitles in <li class="subtitleContainer ..."> items:
// the heading (<h3 data-id="..."> with the title, the year and, for shows,
// "Sezona 1 Epizoda 2"), the release name (<h4>) and a flag image whose file
// name starts with the language's country code. A subtitle downloads from
// /download/?type=1&mediaid=<id> as a zip (some are rar).
//
// Titlovi sits behind Cloudflare and challenges many data-centre addresses
// with a 403, so HolyArrs has to run on a home connection to use it.
type titlovi struct{ c *client }

func newTitlovi(c *client) *titlovi {
	c.keyHeader = ""
	c.userAgent = browserAgent
	return &titlovi{c: c}
}

var (
	tlSeasonEp = regexp.MustCompile(`(?i)Sezona\s+(\d+)(?:\s+Epizoda\s+(\d+))?`)
	tlYear     = regexp.MustCompile(`\((\d{4})\)`)
	tlMediaID  = regexp.MustCompile(`^[0-9]{1,10}$`)
	tlSxxExx   = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])s(\d{1,2})[ ._-]?e(\d{1,3})(?:[^0-9]|$)`)
	tlNxNN     = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(\d{1,2})x(\d{2,3})(?:[^0-9]|$)`)
	tlSeasonOn = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:s|season[ ._-]?)(\d{1,2})(?:[^0-9a-z]|$)`)
)

// tlFlags maps the country code at the start of a flag file name to our
// language code.
var tlFlags = map[string]string{
	"hr": "hr", "rs": "sr", "sr": "sr", "ba": "bs", "bs": "bs", "en": "en", "gb": "en", "us": "en",
	"de": "de", "mk": "mk", "sl": "sl", "si": "sl",
}

// tlLang reads the language from a flag image address such as
// https://static.titlovi.com/.../flags/rs3.png. Serbian in Cyrillic ("cir" in
// the file name) comes back empty: it would answer a Serbian wish with a script
// the person did not ask for.
func tlLang(src string) string {
	name := strings.ToLower(path.Base(strings.TrimRight(strings.TrimSpace(src), "/")))
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		name = name[:i]
	}
	if strings.Contains(name, "cir") || len(name) < 2 {
		return ""
	}
	return tlFlags[name[:2]]
}

// tlItem is one result on the search page.
type tlItem struct {
	id, title, release, flag string
	year, season, episode    int
	fps                      float64
}

// tlParse reads the results of a search page.
func tlParse(b []byte) []tlItem {
	var out []tlItem
	var cur *tlItem
	var h3Text, yearText, titleText, relText, fpsText strings.Builder
	inH3, inA, inI, inH4, inFPS := false, false, false, false, false
	// Only an item's first heading and first release line count. The last item is
	// not closed before the page's sidebar, which has headings of its own.
	h3Done, h4Done := false, false
	flush := func() {
		if cur != nil && tlMediaID.MatchString(cur.id) {
			cur.title = strings.TrimSpace(html.UnescapeString(titleText.String()))
			cur.release = strings.TrimSpace(html.UnescapeString(relText.String()))
			if m := tlYear.FindStringSubmatch(yearText.String()); m != nil {
				cur.year, _ = strconv.Atoi(m[1])
			}
			if m := tlSeasonEp.FindStringSubmatch(h3Text.String()); m != nil {
				cur.season, _ = strconv.Atoi(m[1])
				if m[2] != "" {
					cur.episode, _ = strconv.Atoi(m[2])
				}
			}
			if f := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(fpsText.String())), "fps:")); f != "" {
				cur.fps, _ = strconv.ParseFloat(f, 64)
			}
			out = append(out, *cur)
		}
		cur = nil
		for _, sb := range []*strings.Builder{&h3Text, &yearText, &titleText, &relText, &fpsText} {
			sb.Reset()
		}
		inH3, inA, inI, inH4, inFPS = false, false, false, false, false
		h3Done, h4Done = false, false
	}
	tokens(b, func(t token) {
		if t.tag == "li" && !t.end && strings.HasPrefix(strings.TrimSpace(attr(t.attrs, "class")), "subtitleContainer") {
			flush()
			if len(out) >= addic7edMaxRows {
				return
			}
			cur = &tlItem{}
			return
		}
		if cur == nil {
			return
		}
		switch t.tag {
		case "h3":
			if t.end {
				h3Done = h3Done || inH3
				inH3, inA, inI = false, false, false
			} else if !h3Done {
				inH3 = true
				cur.id = strings.TrimSpace(attr(t.attrs, "data-id"))
			}
		case "h4":
			if t.end {
				h4Done = h4Done || inH4
				inH4, inFPS = false, false
			} else if !h4Done {
				inH4 = true
			}
		case "a":
			inA = inH3 && !t.end
		case "i":
			inI = inH3 && !t.end
		case "span":
			if inH4 && hasClass(t.attrs, "fps") {
				inFPS = !t.end
			} else if inFPS && t.end {
				inFPS = false
			}
		case "img":
			if !t.end && cur.flag == "" && hasClass(t.attrs, "lang") {
				cur.flag = attr(t.attrs, "src")
			}
		case "":
			if len(t.text) > extraMaxText*4 {
				return
			}
			switch {
			case inFPS:
				fpsText.WriteString(t.text)
			case inH4:
				relText.WriteString(t.text)
			case inH3:
				h3Text.WriteString(t.text)
				if inA {
					titleText.WriteString(t.text)
				}
				if inI {
					yearText.WriteString(t.text)
				}
			}
		}
	})
	flush()
	return out
}

// tlWho reads a season and episode out of a release or file name: "S02E05",
// "2x05" or, for a season pack, "S02" alone (episode 0). ok is false when the
// name says neither.
func tlWho(name string) (season, episode int, ok bool) {
	if m := tlSxxExx.FindStringSubmatch(name); m != nil {
		season, _ = strconv.Atoi(m[1])
		episode, _ = strconv.Atoi(m[2])
		return season, episode, true
	}
	if m := tlNxNN.FindStringSubmatch(name); m != nil {
		season, _ = strconv.Atoi(m[1])
		episode, _ = strconv.Atoi(m[2])
		return season, episode, true
	}
	if m := tlSeasonOn.FindStringSubmatch(name); m != nil {
		season, _ = strconv.Atoi(m[1])
		return season, 0, true
	}
	return 0, 0, false
}

func tlURL(term string, episode bool, season, ep int) string {
	p := "/titlovi/?prijevod=" + url.QueryEscape(term)
	if !episode {
		return p + "&t=1"
	}
	p += "&t=2&s=" + strconv.Itoa(season)
	if ep > 0 {
		p += "&e=" + strconv.Itoa(ep)
	}
	return p
}

func (s *titlovi) Search(ctx context.Context, q Query) ([]Candidate, error) {
	langs := wantedLangs(q.Languages)
	if len(langs) == 0 {
		return nil, nil
	}
	isEp := q.Kind == "episode"
	if isEp && q.Season < 0 {
		return nil, nil
	}
	var terms []string
	if imdb := strings.ToLower(strings.TrimSpace(q.IMDb)); imdbID.MatchString(imdb) {
		terms = append(terms, imdb)
	}
	if t := strings.TrimSpace(q.Title); t != "" {
		terms = append(terms, t)
	}
	if len(terms) == 0 {
		return nil, nil
	}
	attempts, failed := 0, 0
	var last error
	run := func(searchPack bool) []Candidate {
		for _, term := range terms {
			attempts++
			ep := q.Episode
			if searchPack {
				ep = 0
			}
			b, err := s.c.getPage(ctx, tlURL(term, isEp, q.Season, ep), map[string]string{"Referer": s.c.base.String() + "/"}, true)
			if err != nil {
				failed, last = failed+1, err
				if ctx.Err() != nil {
					return nil
				}
				continue
			}
			items := tlParse(b)
			// Titlovi lists only the film an IMDb id names, so a result of that search is the
			// title's own, even when the site calls the film by another name (F1 is listed as
			// "F1: The Movie"). A result of a title search is not known to be.
			if got := s.candidates(items, q, langs, isEp, searchPack, imdbID.MatchString(term)); len(got) > 0 {
				return got
			}
			if len(items) > 0 {
				// The search found the film. A search by its title would list the same
				// subtitles (or a mix with other films), so the next term would only
				// spend a request of the site's hourly limit.
				return nil
			}
		}
		return nil
	}
	if got := run(false); len(got) > 0 {
		return got, nil
	}
	if isEp && q.Episode > 0 {
		// A season pack holds the episode: the right file is cut out at download.
		if got := run(true); len(got) > 0 {
			return got, nil
		}
	}
	// Every call failed: say so, so a block or an outage rests the site
	// instead of reading as "no subtitles".
	if attempts > 0 && failed == attempts {
		return nil, last
	}
	return nil, nil
}

// candidates keeps the results that answer the query.
func (s *titlovi) candidates(items []tlItem, q Query, langs []string, isEp, searchPack, byID bool) []Candidate {
	var out []Candidate
	for _, it := range items {
		lang := tlLang(it.flag)
		if lang == "" || !langWanted(langs, lang) {
			continue
		}
		pack := false
		if isEp {
			if it.season > 0 {
				// The heading states the episode: trust it over the release name.
				// A season with no episode is a pack.
				if it.season != q.Season || (it.episode > 0 && it.episode != q.Episode) {
					continue
				}
				pack = it.episode == 0
			} else if sn, ep, ok := tlWho(it.release); ok {
				if sn != q.Season || (ep > 0 && ep != q.Episode) {
					continue
				}
				pack = ep == 0
			} else {
				pack = searchPack
			}
		}
		key := it.id
		if pack && q.Episode > 0 {
			key += ":e" + strconv.Itoa(q.Episode)
		}
		c := Candidate{Site: s.c.cfg.Name, Key: key, Language: lang, Title: it.title, Year: it.year, FPS: it.fps, Format: "srt",
			MatchedByID: byID, Link: "/download/?type=1&mediaid=" + it.id}
		if c.Title == "" {
			c.Title = q.Title
		}
		if isEp {
			c.Season = q.Season
			if !pack {
				c.Episode = q.Episode
			}
		}
		if it.release != "" {
			c.ReleaseNames = []string{it.release}
		}
		out = append(out, c)
	}
	return out
}

// tlKey splits a candidate key into the media id and, for a season pack, the
// episode to cut out of it.
func tlKey(key string) (id string, episode int, ok bool) {
	id, rest, packed := strings.Cut(key, ":e")
	if !tlMediaID.MatchString(id) {
		return "", 0, false
	}
	if !packed {
		return id, 0, true
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n <= 0 || n > 9999 {
		return "", 0, false
	}
	return id, n, true
}

const tlMaxEntry = 10 << 20

// A rar repack stops past this many subtitle files or this many bytes in all.
const (
	tlMaxFiles = 200
	tlMaxTotal = 20 << 20
)

var tlSubExt = map[string]bool{".srt": true, ".sub": true, ".ssa": true, ".ass": true, ".smi": true, ".vtt": true}

func (s *titlovi) Download(ctx context.Context, c Candidate) ([]byte, error) {
	id, episode, ok := tlKey(c.Key)
	if !ok {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site sent a link HolyArrs cannot follow"}
	}
	raw, err := s.c.ref("/download/?type=1&mediaid=" + id)
	if err != nil {
		return nil, err
	}
	b, err := s.c.do(ctx, http.MethodGet, raw, nil, map[string]string{"Referer": s.c.base.String() + "/"}, "", maxDownloadBody, false)
	if err != nil {
		return nil, err
	}
	z, err := tlAsZip(b)
	if err != nil {
		return nil, err
	}
	if episode > 0 {
		return tlPickEpisode(z, c.Season, episode)
	}
	return z, nil
}

// tlAsZip returns the download as a zip. A rar is unpacked and its subtitle
// files packed again, so the rest of HolyArrs reads one kind of archive.
func tlAsZip(b []byte) ([]byte, error) {
	switch {
	case bytes.HasPrefix(b, []byte("PK\x03\x04")):
		return b, nil
	case bytes.HasPrefix(b, []byte("Rar!")):
		rr, err := rardecode.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, &Error{Kind: KindUnreadable, Msg: "the site's rar could not be opened"}
		}
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		files, total := 0, 0
		for {
			h, err := rr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, &Error{Kind: KindUnreadable, Msg: "the site's rar could not be read"}
			}
			name := path.Base(strings.ReplaceAll(h.Name, `\`, "/"))
			if h.IsDir || !tlSubExt[strings.ToLower(path.Ext(name))] {
				continue
			}
			data, err := io.ReadAll(io.LimitReader(rr, tlMaxEntry+1))
			if err != nil || len(data) > tlMaxEntry {
				return nil, &Error{Kind: KindUnreadable, Msg: "the site's rar holds a file that is too big"}
			}
			files++
			total += len(data)
			if files > tlMaxFiles {
				return nil, &Error{Kind: KindUnreadable, Msg: "the site's rar holds too many files"}
			}
			if total > tlMaxTotal {
				return nil, &Error{Kind: KindUnreadable, Msg: "the site's rar is too big in all"}
			}
			w, err := zw.Create(name)
			if err != nil {
				return nil, &Error{Kind: KindUnreadable, Msg: "the site's rar could not be read"}
			}
			if _, err := w.Write(data); err != nil {
				return nil, &Error{Kind: KindUnreadable, Msg: "the site's rar could not be read"}
			}
		}
		if files == 0 || zw.Close() != nil {
			return nil, &Error{Kind: KindUnreadable, Msg: "the site's rar holds no subtitle"}
		}
		return buf.Bytes(), nil
	}
	return nil, &Error{Kind: KindUnreadable, Msg: "the site's download is not a zip or a rar"}
}

// tlPickEpisode cuts one episode's subtitle out of a season pack and packs it
// alone. A zip with a single subtitle is returned as it is.
func tlPickEpisode(z []byte, season, episode int) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(z), int64(len(z)))
	if err != nil {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site's download is not a zip"}
	}
	var subs []*zip.File
	for _, f := range zr.File {
		if !f.FileInfo().IsDir() && tlSubExt[strings.ToLower(path.Ext(f.Name))] {
			subs = append(subs, f)
		}
	}
	if len(subs) == 0 {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site's zip holds no subtitle"}
	}
	if len(subs) == 1 {
		return z, nil
	}
	var hit *zip.File
	for _, f := range subs {
		sn, ep, ok := tlWho(path.Base(strings.ReplaceAll(f.Name, `\`, "/")))
		if ok && ep == episode && (season <= 0 || sn == season) {
			hit = f
			break
		}
	}
	if hit == nil {
		return nil, &Error{Kind: KindUnreadable, Msg: "the season pack holds no file for this episode"}
	}
	if hit.UncompressedSize64 > tlMaxEntry {
		return nil, &Error{Kind: KindUnreadable, Msg: "the subtitle in the pack is too big"}
	}
	rc, err := hit.Open()
	if err != nil {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site's zip could not be read"}
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, tlMaxEntry+1))
	if err != nil || len(data) > tlMaxEntry {
		return nil, &Error{Kind: KindUnreadable, Msg: "the subtitle in the pack is too big"}
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(path.Base(strings.ReplaceAll(hit.Name, `\`, "/")))
	if err != nil {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site's zip could not be read"}
	}
	if _, err := w.Write(data); err != nil || zw.Close() != nil {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site's zip could not be read"}
	}
	return buf.Bytes(), nil
}
