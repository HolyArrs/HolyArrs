package fakesite

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// The fakes below stand in for three subtitle sites that need no account: a
// movie site with an HTML table per movie (YIFY), a show site with show,
// season and episode pages (TVsubtitles), and a SOAP site that searches by
// file hash (BSPlayer). The page and reply shapes are our own, made for tests
// from the research notes. Every title and subtitle is made up by the test.

func zipOne(name, text string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create(name)
	_, _ = w.Write([]byte(text))
	_ = zw.Close()
	return buf.Bytes()
}

func htmlPage(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte("<!DOCTYPE html><html><head><title>Fake</title></head><body>" + body + "</body></html>"))
}

// YifyRow is one subtitle on the fake movie site.
type YifyRow struct {
	ID              string
	Rating          int
	Language        string // English name, as the site shows it
	Release         string
	HearingImpaired bool
	Uploader        string
	Text            string
	NoLink          bool // the subtitle page has no download link
}

// Yify fakes the movie subtitle site. Movies maps an IMDb id to its rows.
type Yify struct {
	mu       sync.Mutex
	Movies   map[string][]YifyRow
	Hostile  string // when set, the raw body of every movie page
	Requests int
	Referers []string
	Agents   []string
}

func (y *Yify) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	y.mu.Lock()
	defer y.mu.Unlock()
	y.Requests++
	y.Referers = append(y.Referers, r.Header.Get("Referer"))
	y.Agents = append(y.Agents, r.Header.Get("User-Agent"))
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/movie-imdb/"):
		rows, ok := y.Movies[strings.TrimPrefix(p, "/movie-imdb/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if y.Hostile != "" {
			_, _ = w.Write([]byte(y.Hostile))
			return
		}
		var sb strings.Builder
		sb.WriteString(`<table class="table other-subs"><thead><tr><th>Rating</th><th>Language</th><th>Release</th><th>HI</th><th>Uploader</th></tr></thead><tbody>`)
		sb.WriteString(`<tr><td>broken row</td></tr>`)
		for _, row := range rows {
			hi := ""
			if row.HearingImpaired {
				hi = `<span class="hi-subtitle"></span>`
			}
			fmt.Fprintf(&sb, `<tr><td class="rating-cell"><span class="label">%d</span></td><td class="flag-cell"><span class="sub-lang">%s</span></td><td><a href="/subtitles/%s"><span class="text-muted">subtitle</span> %s</a></td><td class="other-cell">%s</td><td class="uploader-cell"><a href="/user/x">%s</a></td></tr>`,
				row.Rating, html.EscapeString(row.Language), row.ID, html.EscapeString(row.Release), hi, html.EscapeString(row.Uploader))
		}
		sb.WriteString(`</tbody></table>`)
		htmlPage(w, sb.String())
	case strings.HasPrefix(p, "/subtitles/"):
		for _, rows := range y.Movies {
			for _, row := range rows {
				if p == "/subtitles/"+row.ID {
					if row.NoLink {
						htmlPage(w, "<p>nothing here</p>")
						return
					}
					htmlPage(w, `<a class="btn-icon download-subtitle" href="/subtitle/`+row.ID+`.zip">Download</a>`)
					return
				}
			}
		}
		http.NotFound(w, r)
	case strings.HasPrefix(p, "/subtitle/") && strings.HasSuffix(p, ".zip"):
		id := strings.TrimSuffix(strings.TrimPrefix(p, "/subtitle/"), ".zip")
		for _, rows := range y.Movies {
			for _, row := range rows {
				if row.ID == id {
					if row.Text == "notzip" {
						_, _ = w.Write([]byte("plain words"))
						return
					}
					_, _ = w.Write(zipOne(id+".srt", row.Text))
					return
				}
			}
		}
		http.NotFound(w, r)
	default:
		http.NotFound(w, r)
	}
}

// TVSub is one subtitle on the fake show site.
type TVSub struct {
	ID       int
	Season   int
	Episode  int
	Flag     string // flag code, such as en, br or gr
	Release  string
	Rip      string
	Text     string
	TwoFiles bool // the zip holds two files
}

// TVSubs fakes the show subtitle site. Shows maps a show id to its listed
// name, such as "Quiet Harbour (2005-2013)".
type TVSubs struct {
	mu       sync.Mutex
	Shows    map[int]string
	Subs     map[int][]TVSub // by show id
	Requests int
	Searches []string
	Paths    []string
}

func (t *TVSubs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Requests++
	p := r.URL.Path
	t.Paths = append(t.Paths, p)
	switch {
	case p == "/search1.php" && r.Method == http.MethodPost:
		_ = r.ParseForm()
		q := r.PostForm.Get("qs")
		t.Searches = append(t.Searches, q)
		var sb strings.Builder
		sb.WriteString(`<div class="left_articles"><ul>`)
		for id, name := range t.Shows {
			if strings.Contains(strings.ToLower(name), strings.ToLower(q)) {
				fmt.Fprintf(&sb, `<li><div style=""><a href="/tvshow-%d.html">%s</a></div></li>`, id, html.EscapeString(name))
			}
		}
		sb.WriteString(`<li><a href="http://elsewhere.example/tvshow-9.html">Elsewhere</a></li></ul></div>`)
		htmlPage(w, sb.String())
	case strings.HasPrefix(p, "/tvshow-"):
		var show, season int
		if _, err := fmt.Sscanf(p, "/tvshow-%d-%d.html", &show, &season); err != nil {
			http.NotFound(w, r)
			return
		}
		var sb strings.Builder
		sb.WriteString(`<table id="table5"><tr><td>Episode</td><td>Name</td></tr>`)
		seen := map[int]bool{}
		for _, s := range t.Subs[show] {
			if s.Season != season || seen[s.Episode] {
				continue
			}
			seen[s.Episode] = true
			fmt.Fprintf(&sb, `<tr><td>%dx%02d</td><td><a href="episode-%d.html">Episode %d</a></td></tr>`, s.Season, s.Episode, epID(show, s.Season, s.Episode), s.Episode)
		}
		sb.WriteString(`</table>`)
		htmlPage(w, sb.String())
	case strings.HasPrefix(p, "/episode-"):
		id, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(p, "/episode-"), ".html"))
		var sb strings.Builder
		for show, subs := range t.Subs {
			for _, s := range subs {
				if epID(show, s.Season, s.Episode) != id {
					continue
				}
				fmt.Fprintf(&sb, `<a href="/subtitle-%d.html"><div class="subtitlen"><div class="subtitlen_left"><img src="images/flags/%s.gif" width="18"></div><h5><img src="images/icons/x.png">%s</h5><p title="rip"><img src="images/icons/rip.png">%s</p></div></a>`,
					s.ID, s.Flag, html.EscapeString(s.Release), html.EscapeString(s.Rip))
			}
		}
		htmlPage(w, sb.String())
	case strings.HasPrefix(p, "/download-"):
		id, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(p, "/download-"), ".html"))
		if id == 0 {
			http.NotFound(w, r)
			return
		}
		htmlPage(w, fmt.Sprintf(`<script type="text/javascript">var s1= 'fil'; var s2= 'es/Quiet'; var s3= '_%d'; var s4= '.zip'; document.location = s1+s2+s3+s4;</script>`, id))
	case strings.HasPrefix(p, "/files/Quiet_"):
		id, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(p, "/files/Quiet_"), ".zip"))
		for _, subs := range t.Subs {
			for _, s := range subs {
				if s.ID != id {
					continue
				}
				if s.TwoFiles {
					var buf bytes.Buffer
					zw := zip.NewWriter(&buf)
					for _, n := range []string{"a.srt", "b.srt"} {
						f, _ := zw.Create(n)
						_, _ = f.Write([]byte(s.Text))
					}
					_ = zw.Close()
					_, _ = w.Write(buf.Bytes())
					return
				}
				_, _ = w.Write(zipOne("sub.srt", s.Text))
				return
			}
		}
		http.NotFound(w, r)
	default:
		http.NotFound(w, r)
	}
}

func epID(show, season, episode int) int { return show*10000 + season*100 + episode }

// BSPItem is one subtitle on the fake hash site.
type BSPItem struct {
	ID   string
	Hash string
	Size int64
	Lang string // three-letter code, such as eng or pob
	Name string
	Text string
}

// BSPlayer fakes the SOAP hash site at /v1.php. Its download links point at
// /dl/<id> on Host when set, else on the fake itself.
type BSPlayer struct {
	mu       sync.Mutex
	Items    []BSPItem
	Host     string // scheme and host for download links, such as http://s2.example
	NotXML   bool   // every SOAP call answers with a page that is not XML
	Fail500  bool   // downloads answer with the text 500
	Calls    []string
	Searches []BSPSearch
	Agents   []string
	Handles  []string
}

// BSPSearch is one search the fake saw.
type BSPSearch struct{ Hash, Size, Langs, IMDb string }

func (b *BSPlayer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Agents = append(b.Agents, r.Header.Get("User-Agent"))
	switch {
	case r.URL.Path == "/v1.php" && r.Method == http.MethodGet:
		_, _ = w.Write([]byte("ok"))
	case r.URL.Path == "/v1.php" && r.Method == http.MethodPost:
		action := strings.Trim(r.Header.Get("SOAPAction"), `"`)
		_, call, _ := strings.Cut(action, "#")
		b.Calls = append(b.Calls, call)
		if b.NotXML {
			_, _ = w.Write([]byte("<html>busy</html"))
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		var env struct {
			Body struct {
				Inner struct {
					Handle    string `xml:"handle"`
					MovieHash string `xml:"movieHash"`
					MovieSize string `xml:"movieSize"`
					Langs     string `xml:"languageId"`
					IMDb      string `xml:"imdbId"`
				} `xml:",any"`
			} `xml:"Body"`
		}
		_ = xml.Unmarshal(body, &env)
		in := env.Body.Inner
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		const open = `<?xml version="1.0" encoding="UTF-8"?><SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ns1="http://api.bsplayer-subtitles.com/v1.php"><SOAP-ENV:Body>`
		const end = `</SOAP-ENV:Body></SOAP-ENV:Envelope>`
		switch call {
		case "logIn":
			_, _ = w.Write([]byte(open + `<ns1:logInResponse><return><result>200</result><status>OK</status><data>made-up-handle</data></return></ns1:logInResponse>` + end))
		case "logOut":
			b.Handles = append(b.Handles, in.Handle)
			_, _ = w.Write([]byte(open + `<ns1:logOutResponse><return><status>OK</status></return></ns1:logOutResponse>` + end))
		case "searchSubtitles":
			b.Handles = append(b.Handles, in.Handle)
			b.Searches = append(b.Searches, BSPSearch{in.MovieHash, in.MovieSize, in.Langs, in.IMDb})
			var sb strings.Builder
			for _, it := range b.Items {
				if it.Hash != in.MovieHash || strconv.FormatInt(it.Size, 10) != in.MovieSize || !strings.Contains(","+in.Langs+",", ","+it.Lang+",") {
					continue
				}
				host := b.Host
				if host == "" {
					host = "http://" + r.Host
				}
				fmt.Fprintf(&sb, `<item><subID>%s</subID><subDownloadLink>%s/dl/%s</subDownloadLink><subLang>%s</subLang><subName>%s</subName><subFormat>srt</subFormat></item>`,
					it.ID, host, it.ID, it.Lang, html.EscapeString(it.Name))
			}
			_, _ = w.Write([]byte(open + `<ns1:searchSubtitlesResponse><return><result><result>200</result><status>OK</status></result><data>` + sb.String() + `</data></return></ns1:searchSubtitlesResponse>` + end))
		default:
			http.Error(w, "unknown call", http.StatusBadRequest)
		}
	case strings.HasPrefix(r.URL.Path, "/dl/"):
		if b.Fail500 {
			_, _ = w.Write([]byte("500"))
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/dl/")
		for _, it := range b.Items {
			if it.ID == id {
				var buf bytes.Buffer
				gz := gzip.NewWriter(&buf)
				_, _ = gz.Write([]byte(it.Text))
				_ = gz.Close()
				_, _ = w.Write(buf.Bytes())
				return
			}
		}
		http.NotFound(w, r)
	default:
		http.NotFound(w, r)
	}
}
