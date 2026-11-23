package sites

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// assrt reads Assrt (射手网 (伪)), a Chinese subtitle site, through its official
// REST API. Every call takes the person's free token as a query value. A
// search finds subtitle sets by keyword; a set's detail lists its files, each
// with its own download link, so the file for the right language and episode
// is picked at download time. The token allows a set number of calls a
// minute, so calls are spaced 3 seconds apart by default.
type assrt struct {
	c *client
}

var assrtMinGap = 3 * time.Second

func newAssrt(c *client) *assrt {
	if g := int(assrtMinGap / time.Millisecond); c.cfg.GapMS < g {
		c.cfg.GapMS = g
	}
	c.keyHeader = "" // the token goes in the query
	// A refused call comes back as HTTP 400 with the site's own status in the
	// body, which says more than the HTTP status does.
	c.status = func(resp *http.Response, body []byte) error {
		if resp.StatusCode == http.StatusOK {
			return nil
		}
		var st assrtStatus
		if json.Unmarshal(body, &st) != nil || st.Status == 0 {
			return nil
		}
		empty, err := st.err()
		if empty {
			return errAssrtEmpty
		}
		return err
	}
	return &assrt{c: c}
}

// errAssrtEmpty marks a refused keyword, which is an empty search.
var errAssrtEmpty = errors.New("empty search")

// assrtLangs maps the site's language keys (langlist entries without their
// "lang" prefix) to ours: simplified and traditional Chinese are both zh.
var assrtLangs = map[string]string{"chs": "zh", "cht": "zh", "chn": "zh", "twn": "zh", "eng": "en"}

// assrtWords are the language marks file names in a set carry.
var assrtWords = map[string][]string{
	"zh": {"chs", "cht", "简体", "繁体", "簡體", "繁體", "中文", "gb", "big5"},
	"en": {"eng", "英文", "english"},
}

// assrtStatus is the site's own error answer, sent with HTTP 200.
type assrtStatus struct {
	Status int    `json:"status"`
	ErrMsg string `json:"errmsg"`
}

// err reads the site's status: 0 is fine. 101 means the keyword is too short,
// which is an empty search. 20000s are token problems, 30900 is the per-minute
// quota, the other 30000s are the site's own failures.
func (s assrtStatus) err() (bool, error) {
	switch {
	case s.Status == 0:
		return false, nil
	case s.Status == 101:
		return true, nil
	case s.Status >= 20000 && s.Status < 30000:
		return false, &Error{Kind: KindLoginRefused, Msg: fmt.Sprintf("the site refused the token (%d)", s.Status)}
	case s.Status == 30900:
		return false, &Error{Kind: KindRateLimited, RetryAfter: time.Minute, Msg: "the token's calls for this minute are used up"}
	case s.Status >= 30000 && s.Status < 40000:
		return false, &Error{Kind: KindDown, Msg: fmt.Sprintf("the site failed (%d)", s.Status)}
	}
	return false, &Error{Kind: KindUnreadable, Msg: fmt.Sprintf("the site answered with status %d", s.Status)}
}

func (a *assrt) call(ctx context.Context, p string, v url.Values, out any) (bool, error) {
	v.Set("token", a.c.cfg.Key)
	body, err := a.c.get(ctx, p+"?"+v.Encode(), "application/json", maxSearchBody, false)
	if errors.Is(err, errAssrtEmpty) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	var st assrtStatus
	if err := json.Unmarshal(body, &st); err != nil {
		return false, unreadable("answer", err)
	}
	if empty, err := st.err(); empty || err != nil {
		return empty, err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return false, unreadable("answer", err)
	}
	return false, nil
}

type assrtSub struct {
	ID        int64  `json:"id"`
	VideoName string `json:"videoname"`
	Lang      struct {
		List map[string]any `json:"langlist"`
	} `json:"lang"`
}

// assrtCJK strips the Chinese title the site often puts before the release
// name, as in "瑞克和莫蒂.Rick.and.Morty.S07E10".
var assrtCJK = regexp.MustCompile(`^[\p{Han}\p{P}\s]*[\p{Han}][\p{Han}\p{P}\s]*?[.\s_]+`)

func assrtRelease(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == "不知道" { // "not known"
		return ""
	}
	if loc := assrtCJK.FindStringIndex(name); loc != nil && loc[1] < len(name) {
		name = name[loc[1]:]
	}
	// A name still mostly in Chinese says nothing the release parser reads.
	latin := 0
	for _, r := range name {
		if r < unicode.MaxASCII && unicode.IsLetter(r) {
			latin++
		}
	}
	if latin < 3 {
		return ""
	}
	return clip(name)
}

func (a *assrt) Search(ctx context.Context, q Query) ([]Candidate, error) {
	langs := wantedLangs(q.Languages)
	if !langWanted(langs, "zh") && !langWanted(langs, "en") {
		return nil, nil
	}
	words := strings.TrimSpace(q.Title)
	switch q.Kind {
	case "episode":
		if q.Season <= 0 || q.Episode <= 0 {
			return nil, nil
		}
		words += fmt.Sprintf(" S%02dE%02d", q.Season, q.Episode)
	case "movie":
		if q.Year > 0 {
			words += " " + strconv.Itoa(q.Year)
		}
	default:
		return nil, nil
	}
	if len([]rune(strings.TrimSpace(q.Title))) < 2 {
		return nil, nil
	}
	v := url.Values{}
	v.Set("q", words)
	v.Set("cnt", "15")
	v.Set("is_file", "1")
	var res struct {
		Sub struct {
			Subs []assrtSub `json:"subs"`
		} `json:"sub"`
	}
	if empty, err := a.call(ctx, "/sub/search", v, &res); empty || err != nil {
		return nil, err
	}
	var out []Candidate
	for _, s := range res.Sub.Subs {
		if s.ID <= 0 {
			continue
		}
		seen := map[string]bool{}
		for key := range s.Lang.List {
			lang := assrtLangs[strings.TrimPrefix(strings.ToLower(key), "lang")]
			if lang == "" || seen[lang] || !langWanted(langs, lang) {
				continue
			}
			seen[lang] = true
			c := Candidate{Site: a.c.cfg.Name, Key: strconv.FormatInt(s.ID, 10), Language: lang, Format: "srt"}
			if r := assrtRelease(s.VideoName); r != "" {
				c.ReleaseNames = []string{r}
			}
			if q.Kind == "episode" {
				// Kept for picking the episode's file out of a season set.
				c.Link = fmt.Sprintf("S%02dE%02d", q.Season, q.Episode)
			}
			out = append(out, c)
		}
	}
	return out, nil
}

type assrtFile struct {
	URL  string `json:"url"`
	Name string `json:"f"`
}

// assrtPick chooses the file in a set: the asked episode first, then the
// language, then a text subtitle, then the first.
func assrtPick(files []assrtFile, lang, episode string) (assrtFile, bool) {
	var subs []assrtFile
	for _, f := range files {
		ext := strings.ToLower(f.Name[strings.LastIndex(f.Name, ".")+1:])
		if f.URL != "" && (ext == "srt" || ext == "ass" || ext == "ssa" || ext == "vtt" || ext == "sub") {
			subs = append(subs, f)
		}
	}
	if episode != "" {
		var eps []assrtFile
		for _, f := range subs {
			if strings.Contains(strings.ToUpper(f.Name), episode) {
				eps = append(eps, f)
			}
		}
		if len(eps) > 0 {
			subs = eps
		}
	}
	for _, f := range subs {
		name := strings.ToLower(f.Name)
		for _, w := range assrtWords[lang] {
			if strings.Contains(name, w) {
				return f, true
			}
		}
	}
	if len(subs) > 0 {
		return subs[0], true
	}
	return assrtFile{}, false
}

var assrtEpisode = regexp.MustCompile(`^S[0-9]{2,3}E[0-9]{2,4}$`)

func (a *assrt) Download(ctx context.Context, c Candidate) ([]byte, error) {
	id, err := strconv.ParseInt(c.Key, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != c.Key {
		return nil, &Error{Kind: KindUnreadable, Msg: "the subtitle has no id HolyArrs can use"}
	}
	var res struct {
		Sub struct {
			Subs []struct {
				URL      string      `json:"url"`
				Filename string      `json:"filename"`
				Files    []assrtFile `json:"filelist"`
			} `json:"subs"`
		} `json:"sub"`
	}
	if empty, err := a.call(ctx, "/sub/detail", url.Values{"id": {c.Key}}, &res); err != nil || empty {
		if err == nil {
			err = &Error{Kind: KindUnreadable, Msg: "the site sent no details for this subtitle"}
		}
		return nil, err
	}
	if len(res.Sub.Subs) == 0 {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site no longer has this subtitle"}
	}
	d := res.Sub.Subs[0]
	ep := ""
	if assrtEpisode.MatchString(c.Link) {
		ep = c.Link
	}
	link := d.URL
	if f, ok := assrtPick(d.Files, c.Language, ep); ok {
		link = f.URL
	}
	if link == "" {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site sent no download link"}
	}
	return a.c.fetch(ctx, link, maxDownloadBody)
}
