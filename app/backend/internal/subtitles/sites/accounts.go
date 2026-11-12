package sites

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The two sites below need the person's own account. Their answer shapes are
// our reading of each site's public API and are checked only against fake
// servers in package fakesite, never against the live sites.

// openSubtitles talks to the OpenSubtitles.com REST API. Every call sends the
// API key in the Api-Key header and searches need nothing more. Downloads need
// a sign-in with username and password, which returns a bearer token. The
// token is kept for a day and renewed when the site refuses it.
type openSubtitles struct {
	c *client

	mu      sync.Mutex
	token   string
	tokenAt time.Time
	vip     bool
}

// osTokenLife is how long one sign-in is reused. The site's tokens last about
// a day, so this renews a little early.
const osTokenLife = 23 * time.Hour

func newOpenSubtitles(c *client) *openSubtitles {
	c.keyHeader = "Api-Key"
	c.userAgent = "HolyArrs v1"
	c.status = osStatus
	return &openSubtitles{c: c}
}

// osStatus reads the site's own error answers. A 406 means no downloads are
// left today and carries the time the count resets.
func osStatus(resp *http.Response, body []byte) error {
	if resp.StatusCode != http.StatusNotAcceptable {
		return nil
	}
	var q struct {
		ResetUTC string `json:"reset_time_utc"`
	}
	_ = json.Unmarshal(body, &q)
	e := &Error{Kind: KindDownloadLimit, Msg: "the account's downloads for today are used up"}
	e.RetryAfter = untilReset(q.ResetUTC)
	return e
}

// untilReset turns the site's reset time into a wait. It returns 0 when the
// time is unreadable, in the past or more than a day away.
func untilReset(at string) time.Duration {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(at))
	if err != nil {
		return 0
	}
	d := time.Until(t)
	if d <= 0 || d > 24*time.Hour {
		return 0
	}
	return d
}

type osFeature struct {
	Title         string `json:"title"`
	ParentTitle   string `json:"parent_title"`
	Year          int    `json:"year"`
	SeasonNumber  int    `json:"season_number"`
	EpisodeNumber int    `json:"episode_number"`
	IMDbID        int    `json:"imdb_id"`
	TMDBID        int    `json:"tmdb_id"`
	ParentIMDbID  int    `json:"parent_imdb_id"`
	ParentTMDBID  int    `json:"parent_tmdb_id"`
}

type osSub struct {
	ID         string `json:"id"`
	Attributes struct {
		Language          string  `json:"language"`
		HearingImpaired   bool    `json:"hearing_impaired"`
		ForeignPartsOnly  bool    `json:"foreign_parts_only"`
		FPS               float64 `json:"fps"`
		DownloadCount     int     `json:"download_count"`
		Release           string  `json:"release"`
		FromTrusted       bool    `json:"from_trusted"`
		AITranslated      bool    `json:"ai_translated"`
		MachineTranslated bool    `json:"machine_translated"`
		MovieHashMatch    bool    `json:"moviehash_match"`
		Uploader          struct {
			Name string `json:"name"`
		} `json:"uploader"`
		Feature osFeature `json:"feature_details"`
		Files   []struct {
			FileID   int64  `json:"file_id"`
			FileName string `json:"file_name"`
		} `json:"files"`
	} `json:"attributes"`
}

// idNumber turns "tt0012345" or "12345" into 12345, or 0 when it is neither.
func idNumber(id string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(id)), "tt"))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// baseLang returns a language code in lower case without its region.
func baseLang(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if i := strings.IndexAny(code, "-_"); i > 0 {
		code = code[:i]
	}
	return code
}

func (o *openSubtitles) Search(ctx context.Context, q Query) ([]Candidate, error) {
	langs := make([]string, 0, len(q.Languages))
	for _, l := range q.Languages {
		if l = siteLang(l); l != "" && !slices.Contains(langs, l) {
			langs = append(langs, l)
		}
	}
	if len(langs) == 0 {
		return nil, nil
	}
	slices.Sort(langs)
	v := url.Values{}
	v.Set("languages", strings.Join(langs, ","))
	imdb, tmdb := idNumber(q.IMDb), idNumber(q.TMDB)
	if q.Hash != "" {
		v.Set("moviehash", strings.ToLower(q.Hash))
	}
	switch q.Kind {
	case "episode":
		if q.Season <= 0 || q.Episode <= 0 {
			return nil, nil
		}
		v.Set("type", "episode")
		v.Set("season_number", strconv.Itoa(q.Season))
		v.Set("episode_number", strconv.Itoa(q.Episode))
		switch {
		case imdb > 0:
			v.Set("parent_imdb_id", strconv.Itoa(imdb))
		case tmdb > 0:
			v.Set("parent_tmdb_id", strconv.Itoa(tmdb))
		case q.Title != "":
			v.Set("query", strings.ToLower(q.Title))
		default:
			return nil, nil
		}
	default:
		v.Set("type", "movie")
		switch {
		case imdb > 0:
			v.Set("imdb_id", strconv.Itoa(imdb))
		case tmdb > 0:
			v.Set("tmdb_id", strconv.Itoa(tmdb))
		case q.Title != "":
			v.Set("query", strings.ToLower(q.Title))
			if q.Year > 0 {
				v.Set("year", strconv.Itoa(q.Year))
			}
		default:
			return nil, nil
		}
	}
	body, err := o.c.get(ctx, "/subtitles?"+v.Encode(), "application/json", maxSearchBody, false)
	if err != nil {
		return nil, err
	}
	var res struct {
		Data []osSub `json:"data"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, unreadable("search", err)
	}
	out := make([]Candidate, 0, len(res.Data))
	for _, s := range res.Data {
		a := s.Attributes
		if len(a.Files) == 0 || a.Files[0].FileID <= 0 {
			continue
		}
		lang := siteLang(a.Language)
		if !langWanted(langs, lang) {
			continue // the site answered in another language
		}
		f := a.Feature
		c := Candidate{Site: o.c.cfg.Name, Key: strconv.FormatInt(a.Files[0].FileID, 10), Language: lang, HearingImpaired: a.HearingImpaired,
			Forced: a.ForeignPartsOnly, Title: f.Title, Year: f.Year, Uploader: a.Uploader.Name, Trusted: a.FromTrusted, FPS: a.FPS,
			Downloads: a.DownloadCount, Format: "srt", Link: strconv.FormatInt(a.Files[0].FileID, 10)}
		// Machine-made files are flagged; the site's switch decides whether they
		// are used at all.
		switch {
		case a.AITranslated:
			c.MachineMade = AITranslated
		case a.MachineTranslated:
			c.MachineMade = MachineTranslated
		}
		if r := strings.TrimSpace(a.Release); r != "" && len(r) <= 300 {
			c.ReleaseNames = []string{r}
		}
		if q.Kind == "episode" {
			c.Season, c.Episode = f.SeasonNumber, f.EpisodeNumber
			if f.ParentTitle != "" {
				c.Title = f.ParentTitle
			}
			c.MatchedByID = (imdb > 0 && f.ParentIMDbID == imdb) || (tmdb > 0 && f.ParentTMDBID == tmdb)
		} else {
			c.MatchedByID = (imdb > 0 && f.IMDbID == imdb) || (tmdb > 0 && f.TMDBID == tmdb)
		}
		if a.MovieHashMatch && q.Hash != "" {
			c.MatchedByID = true
		}
		out = append(out, c)
	}
	return out, nil
}

// signIn returns a usable token and signs in when there is none or it is old.
// fresh forces a new sign-in after the site refused the old token.
func (o *openSubtitles) signIn(ctx context.Context, fresh bool) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !fresh && o.token != "" && time.Since(o.tokenAt) < osTokenLife {
		return o.token, nil
	}
	o.token = ""
	if o.c.cfg.Username == "" || o.c.cfg.Password == "" {
		return "", &Error{Kind: KindLoginRefused, Msg: "downloads from OpenSubtitles.com need your username and password"}
	}
	raw, err := o.c.ref("/login")
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(struct {
		U string `json:"username"`
		P string `json:"password"`
	}{o.c.cfg.Username, o.c.cfg.Password})
	body, err := o.c.do(ctx, http.MethodPost, raw, b, nil, "application/json", maxSearchBody, false)
	if err != nil {
		var se *Error
		if errors.As(err, &se) && se.Kind == KindLoginRefused {
			return "", &Error{Kind: KindLoginRefused, Msg: "the site refused the username, password or API key"}
		}
		return "", err
	}
	var res struct {
		Token string `json:"token"`
		User  struct {
			VIP bool `json:"vip"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return "", unreadable("sign-in", err)
	}
	if res.Token == "" || len(res.Token) > 4096 || strings.ContainsAny(res.Token, " \r\n") {
		return "", &Error{Kind: KindUnreadable, Msg: "the sign-in answer held no usable token"}
	}
	o.token, o.tokenAt, o.vip = res.Token, time.Now(), res.User.VIP
	return o.token, nil
}

func (o *openSubtitles) Download(ctx context.Context, c Candidate) ([]byte, error) {
	id, err := strconv.ParseInt(c.Link, 10, 64)
	if err != nil || id <= 0 {
		return nil, &Error{Kind: KindUnreadable, Msg: "the subtitle has no file to download"}
	}
	raw, err := o.c.ref("/download")
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(map[string]int64{"file_id": id})
	var body []byte
	for try := 0; try < 2; try++ {
		token, err := o.signIn(ctx, try > 0)
		if err != nil {
			return nil, err
		}
		body, err = o.c.do(ctx, http.MethodPost, raw, b, map[string]string{"Authorization": "Bearer " + token}, "application/json", maxSearchBody, false)
		var se *Error
		if err != nil && errors.As(err, &se) && se.Kind == KindLoginRefused && try == 0 {
			continue // the token expired, so sign in again once
		}
		if err != nil {
			return nil, err
		}
		break
	}
	var res struct {
		Link      string `json:"link"`
		Remaining *int   `json:"remaining"`
		ResetUTC  string `json:"reset_time_utc"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, unreadable("download", err)
	}
	if res.Link == "" {
		if res.Remaining != nil && *res.Remaining <= 0 {
			return nil, &Error{Kind: KindDownloadLimit, RetryAfter: untilReset(res.ResetUTC), Msg: "the account's downloads for today are used up"}
		}
		return nil, &Error{Kind: KindUnreadable, Msg: "the download answer held no link"}
	}
	return o.c.fetch(ctx, res.Link, maxDownloadBody)
}

// subdl answers searches as JSON with the key in the query, and hands out zip
// links on its download host.
type subdl struct{ c *client }

// subdlDownloadHost is where the real site keeps its zip links. A test server
// serves them itself.
const subdlDownloadHost = "https://dl.subdl.com"

type sdSub struct {
	ReleaseName string `json:"release_name"`
	Name        string `json:"name"`
	Lang        string `json:"lang"`
	Language    string `json:"language"`
	Author      string `json:"author"`
	URL         string `json:"url"`
	Season      int    `json:"season"`
	Episode     *int   `json:"episode"`
	HI          bool   `json:"hi"`
	FullSeason  bool   `json:"full_season"`
}

func (s *subdl) Search(ctx context.Context, q Query) ([]Candidate, error) {
	var langs, codes []string
	for _, l := range q.Languages {
		if l = siteLang(l); l != "" && !slices.Contains(langs, l) {
			langs = append(langs, l)
		}
		if b := baseLang(l); b != "" && !slices.Contains(codes, b) {
			codes = append(codes, b)
		}
	}
	if len(langs) == 0 {
		return nil, nil
	}
	v := url.Values{}
	v.Set("api_key", s.c.cfg.Key)
	v.Set("languages", strings.ToUpper(strings.Join(codes, ",")))
	v.Set("subs_per_page", "30")
	v.Set("hi", "1")
	switch q.Kind {
	case "episode":
		if q.Season <= 0 || q.Episode <= 0 {
			return nil, nil
		}
		v.Set("type", "tv")
		v.Set("season_number", strconv.Itoa(q.Season))
		v.Set("episode_number", strconv.Itoa(q.Episode))
	default:
		v.Set("type", "movie")
	}
	byID := true
	switch {
	case idNumber(q.TMDB) > 0:
		v.Set("tmdb_id", strconv.Itoa(idNumber(q.TMDB)))
	case idNumber(q.IMDb) > 0:
		v.Set("imdb_id", "tt"+fmt.Sprintf("%07d", idNumber(q.IMDb)))
	case q.Title != "":
		byID = false
		v.Set("film_name", q.Title)
		if q.Year > 0 && q.Kind != "episode" {
			v.Set("year", strconv.Itoa(q.Year))
		}
	default:
		return nil, nil
	}
	body, err := s.c.get(ctx, "/subtitles?"+v.Encode(), "application/json", maxSearchBody, true)
	if err != nil || body == nil {
		return nil, err
	}
	var res struct {
		Status  *bool  `json:"status"`
		Error   string `json:"error"`
		Results []struct {
			Name string `json:"name"`
			Year int    `json:"year"`
		} `json:"results"`
		Subtitles []sdSub `json:"subtitles"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, unreadable("search", err)
	}
	if res.Status != nil && !*res.Status {
		msg := strings.ToLower(res.Error)
		switch {
		case strings.Contains(msg, "api") && (strings.Contains(msg, "key") || strings.Contains(msg, "invalid")):
			return nil, &Error{Kind: KindLoginRefused, Msg: "the site refused the API key"}
		case strings.Contains(msg, "limit"):
			return nil, &Error{Kind: KindRateLimited, Msg: "the site asked HolyArrs to slow down"}
		}
		return nil, nil // the site reports nothing found this way
	}
	title, year := q.Title, q.Year
	if len(res.Results) > 0 {
		title, year = res.Results[0].Name, res.Results[0].Year
	}
	out := make([]Candidate, 0, len(res.Subtitles))
	for _, x := range res.Subtitles {
		if !strings.HasPrefix(x.URL, "/") || strings.HasPrefix(x.URL, "//") || len(x.URL) > 300 || x.FullSeason {
			continue
		}
		lang := siteLang(x.Language)
		if !langWanted(langs, lang) {
			continue
		}
		c := Candidate{Site: s.c.cfg.Name, Key: strings.TrimSuffix(path.Base(x.URL), ".zip"), Language: lang, HearingImpaired: x.HI,
			Title: title, Year: year, Uploader: x.Author, MatchedByID: byID, Format: "srt", Link: x.URL}
		if r := strings.TrimSpace(x.ReleaseName); r != "" && len(r) <= 300 {
			c.ReleaseNames = []string{r}
		}
		if q.Kind == "episode" {
			c.Season = x.Season
			if x.Episode != nil {
				c.Episode = *x.Episode
			}
			if c.Season != q.Season || (c.Episode != 0 && c.Episode != q.Episode) {
				continue
			}
		}
		out = append(out, c)
	}
	return out, nil
}

func (s *subdl) Download(ctx context.Context, c Candidate) ([]byte, error) {
	if !strings.HasPrefix(c.Link, "/") || strings.HasPrefix(c.Link, "//") {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site sent a link HolyArrs cannot follow"}
	}
	if strings.EqualFold(s.c.base.Host, "api.subdl.com") {
		return s.c.fetch(ctx, subdlDownloadHost+c.Link, maxDownloadBody)
	}
	// On our own server, as in tests and the lab, the zip sits on the same host
	// beside the API path.
	u := *s.c.base
	u.Path, u.RawQuery = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/api/v1")+c.Link, ""
	return s.c.do(ctx, http.MethodGet, u.String(), nil, nil, "", maxDownloadBody, false)
}

// FileHash returns the OpenSubtitles-style hash of a video file: its size plus
// the 64-bit little-endian words of its first and last 64 KiB, as 16 hex
// digits. Files under 128 KiB get no hash.
func FileHash(name string) (string, error) {
	const chunk = 64 << 10
	f, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := st.Size()
	if !st.Mode().IsRegular() || size < 2*chunk {
		return "", fmt.Errorf("file too small to hash")
	}
	sum := uint64(size)
	buf := make([]byte, chunk)
	for _, off := range []int64{0, size - chunk} {
		if _, err := f.ReadAt(buf, off); err != nil {
			return "", err
		}
		for i := 0; i < chunk; i += 8 {
			var w uint64
			for j := 7; j >= 0; j-- {
				w = w<<8 | uint64(buf[i+j])
			}
			sum += w
		}
	}
	return fmt.Sprintf("%016x", sum), nil
}
