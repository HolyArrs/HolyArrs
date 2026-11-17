package sites

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// bsplayer talks to a SOAP API that finds subtitles by the video file's hash
// and size, for movies and episodes. It opens an anonymous session, searches
// once and closes it. The site's servers share one domain; with the default
// address HolyArrs picks one that answers.
type bsplayer struct {
	c *client

	mu  sync.Mutex
	api *client // the server in use
}

// bspSearchWait is the pause before each search, which keeps the site from
// throttling. Tests set it to zero.
var bspSearchWait = 8 * time.Second

const (
	bspDomain  = ".api.bsplayer-subtitles.com"
	bspServers = 8
	bspTries   = 3
	bspNS      = "http://api.bsplayer-subtitles.com/v1.php"
	bspAgent   = "BSPlayer/2.x (HolyArrs)"
)

func newBSPlayer(c *client) *bsplayer {
	c.keyHeader = ""
	c.userAgent = bspAgent
	return &bsplayer{c: c}
}

// bspHosts returns the servers to try, in random order.
func bspHosts() []string {
	var all []string
	for i := 1; i <= 8; i++ {
		all = append(all, "s"+strconv.Itoa(i)+bspDomain)
	}
	for i := 101; i <= 109; i++ {
		all = append(all, "s"+strconv.Itoa(i)+bspDomain)
	}
	rand.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })
	return all[:bspServers]
}

// server returns the client for the server in use, picking one first when the
// address is the site's own domain.
func (b *bsplayer) server(ctx context.Context) (*client, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.api != nil {
		return b.api, nil
	}
	if !strings.HasSuffix(strings.ToLower(b.c.base.Hostname()), bspDomain) {
		b.api = b.c
		return b.api, nil
	}
	for _, h := range bspHosts() {
		cfg := b.c.cfg
		cfg.URL = b.c.base.Scheme + "://" + h
		cfg.Timeout = 3 * time.Second
		c, err := newClient(cfg)
		if err != nil {
			continue
		}
		c.keyHeader, c.userAgent = "", bspAgent
		if _, err := c.do(ctx, http.MethodGet, cfg.URL+"/v1.php", nil, nil, "", 64<<10, false); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		c.cfg.Timeout = b.c.cfg.Timeout
		b.api = c
		return c, nil
	}
	return nil, &Error{Kind: KindDown, Msg: "none of the site's servers answered"}
}

type bspItem struct {
	ID     string `xml:"subID"`
	Link   string `xml:"subDownloadLink"`
	Lang   string `xml:"subLang"`
	Name   string `xml:"subName"`
	Format string `xml:"subFormat"`
}

type bspReturn struct {
	Status string `xml:"status"`
	Result struct {
		Status string `xml:"status"`
	} `xml:"result"`
	Data struct {
		Text  string    `xml:",chardata"`
		Items []bspItem `xml:"item"`
	} `xml:"data"`
}

func (r *bspReturn) ok() bool {
	return strings.EqualFold(strings.TrimSpace(r.Status), "OK") || strings.EqualFold(strings.TrimSpace(r.Result.Status), "OK")
}

// call sends one SOAP call. fields are name and value pairs. A reply that is
// not XML is tried again a few times.
func (b *bsplayer) call(ctx context.Context, c *client, method string, fields ...string) (*bspReturn, error) {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?><SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ns1="` + bspNS + `"><SOAP-ENV:Body><ns1:` + method + `>`)
	for i := 0; i+1 < len(fields); i += 2 {
		sb.WriteString("<" + fields[i] + ">")
		_ = xml.EscapeText(&sb, []byte(fields[i+1]))
		sb.WriteString("</" + fields[i] + ">")
	}
	sb.WriteString(`</ns1:` + method + `></SOAP-ENV:Body></SOAP-ENV:Envelope>`)
	raw, err := c.ref("/v1.php")
	if err != nil {
		return nil, err
	}
	hdr := map[string]string{"Content-Type": "text/xml; charset=utf-8", "SOAPAction": `"` + bspNS + "#" + method + `"`}
	for try := 0; try < bspTries; try++ {
		body, err := c.do(ctx, http.MethodPost, raw, []byte(sb.String()), hdr, "text/xml", maxSearchBody, false)
		if err != nil {
			return nil, err
		}
		var env struct {
			Body struct {
				Resp struct {
					Return bspReturn `xml:"return"`
				} `xml:",any"`
			} `xml:"Body"`
		}
		if err := xml.Unmarshal(body, &env); err == nil {
			return &env.Body.Resp.Return, nil
		}
	}
	return nil, &Error{Kind: KindDown, Msg: "the site sent no readable answer after several tries; it seems busy"}
}

// bspCodes maps our codes to the site's three-letter codes.
var bspCodes = map[string]string{"ar": "ara", "bg": "bul", "cs": "cze", "da": "dan", "de": "ger", "el": "ell", "en": "eng", "fi": "fin",
	"fr": "fre", "hu": "hun", "it": "ita", "ja": "jpn", "ko": "kor", "nl": "dut", "pl": "pol", "pt": "por", "pt-br": "pob", "ro": "rum",
	"ru": "rus", "es": "spa", "sv": "swe", "tr": "tur", "uk": "ukr", "zh": "chi"}

func bspLang(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	for ours, theirs := range bspCodes {
		if theirs == code {
			return ours
		}
	}
	return ""
}

var hex16 = regexp.MustCompile(`^[0-9a-f]{16}$`)

func (b *bsplayer) Search(ctx context.Context, q Query) ([]Candidate, error) {
	hash := strings.ToLower(strings.TrimSpace(q.Hash))
	if (q.Kind != "movie" && q.Kind != "episode") || !hex16.MatchString(hash) || q.Size <= 0 {
		return nil, nil
	}
	langs := wantedLangs(q.Languages)
	var codes []string
	seen := map[string]bool{}
	for _, l := range langs {
		if c := bspCodes[l]; c != "" && !seen[c] {
			seen[c] = true
			codes = append(codes, c)
		}
	}
	if len(codes) == 0 {
		return nil, nil
	}
	c, err := b.server(ctx)
	if err != nil {
		return nil, err
	}
	login, err := b.call(ctx, c, "logIn", "username", "", "password", "", "AppID", "HolyArrs")
	if err != nil {
		return nil, err
	}
	handle := strings.TrimSpace(login.Data.Text)
	if !login.ok() || handle == "" || len(handle) > 200 {
		return nil, &Error{Kind: KindDown, Msg: "the site did not open a session"}
	}
	defer func() {
		lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_, _ = b.call(lctx, c, "logOut", "handle", handle)
	}()
	if bspSearchWait > 0 {
		t := time.NewTimer(bspSearchWait)
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return nil, ctx.Err()
		}
	}
	imdb := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(q.IMDb)), "tt")
	if _, err := strconv.ParseUint(imdb, 10, 64); err != nil {
		imdb = "*"
	}
	res, err := b.call(ctx, c, "searchSubtitles", "handle", handle, "movieHash", hash, "movieSize", strconv.FormatInt(q.Size, 10),
		"languageId", strings.Join(codes, ","), "imdbId", imdb)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(res.Result.Status), "OK") {
		return nil, nil
	}
	var out []Candidate
	for _, it := range res.Data.Items {
		if len(out) >= addic7edMaxRows {
			break
		}
		id, link := strings.TrimSpace(it.ID), strings.TrimSpace(it.Link)
		lang := bspLang(it.Lang)
		if id == "" || len(id) > 100 || lang == "" || !langWanted(langs, lang) || !b.linkOK(link, c.base.Host) {
			continue
		}
		name := clip(it.Name)
		if ext := path.Ext(name); ext != "" && len(ext) <= 5 {
			name = strings.TrimSuffix(name, ext)
		}
		format := strings.ToLower(strings.TrimSpace(it.Format))
		if format == "" || len(format) > 5 {
			format = "srt"
		}
		cand := Candidate{Site: b.c.cfg.Name, Key: id, Language: lang, Title: q.Title, Year: q.Year, MatchedByID: true, Format: format, Link: link}
		if q.Kind == "episode" {
			cand.Season, cand.Episode = q.Season, q.Episode
		}
		if name != "" {
			cand.ReleaseNames = []string{name}
		}
		out = append(out, cand)
	}
	return out, nil
}

// linkOK keeps download links on the server in use or the site's own domain.
func (b *bsplayer) linkOK(link, host string) bool {
	u, err := url.Parse(link)
	if err != nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	return u.Host == host || strings.HasSuffix(strings.ToLower(u.Hostname()), bspDomain)
}

func (b *bsplayer) Download(ctx context.Context, c Candidate) ([]byte, error) {
	b.mu.Lock()
	host := b.c.base.Host
	if b.api != nil {
		host = b.api.base.Host
	}
	b.mu.Unlock()
	if !b.linkOK(c.Link, host) {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site sent a link HolyArrs cannot follow"}
	}
	body, err := b.c.do(ctx, http.MethodGet, c.Link, nil, nil, "", maxDownloadBody, false)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(head(body, 16))) == "500" {
		return nil, &Error{Kind: KindDown, Msg: "the site failed to send the file"}
	}
	if !bytes.HasPrefix(body, []byte{0x1F, 0x8B}) {
		return body, nil
	}
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, unreadable("the file", err)
	}
	out, err := io.ReadAll(io.LimitReader(zr, maxDownloadBody+1))
	if err != nil {
		return nil, unreadable("the file", err)
	}
	if len(out) > maxDownloadBody {
		return nil, &Error{Kind: KindUnreadable, Msg: "the file is too large"}
	}
	return out, nil
}
