package sites

import (
	"bytes"
	"context"
	"errors"
	"html"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// addic7ed reads a show subtitle site that answers with HTML pages and needs
// the person's account to download. It signs in with the user name and
// password from the vault through a form post, and keeps the session cookie
// in memory per site. It finds the show by name on the search page, reads the
// season page rows (language, version, completeness, hearing impaired and
// download link) and downloads with the show page as Referer. Shows only.
// The page shapes are our reading of the site and are checked only against
// fake pages in fakesite.Addic7ed, never the live site. The site is used
// gently: at least addic7edMinGap between calls and at most addic7edDailyCap
// calls a day. An anti-bot check page makes it rest with a plain reason and is
// never worked around.
type addic7ed struct {
	c *client

	mu       sync.Mutex
	loginMu  sync.Mutex
	signedIn bool
	session  uint64
	day      string
	calls    int
}

type addic7edRedirectState struct{ err error }
type addic7edRedirectKey struct{}

// Limits that keep the addic7ed adapter gentle on the site.
var addic7edMinGap = 3 * time.Second

const (
	addic7edDailyCap = 200
	addic7edMaxPage  = 2 << 20
	addic7edMaxRows  = 2000
	addic7edMaxCells = 40
	addic7edMaxText  = 300
)

// Paths on the site.
const (
	a7Login  = "/dologin.php"
	a7Search = "/search.php"
	a7Show   = "/show/"
	a7Down   = "/download/"
)

func newAddic7ed(c *client) *addic7ed {
	if g := int(addic7edMinGap / time.Millisecond); c.cfg.GapMS < g {
		c.cfg.GapMS = max(g, 1)
	}
	jar, _ := cookiejar.New(nil)
	c.http.Jar = jar
	c.keyHeader = ""
	c.status = func(_ *http.Response, body []byte) error {
		if isChallenge(string(head(body, 64<<10))) {
			return challengeErr()
		}
		return nil
	}
	a := &addic7ed{c: c}
	checkRedirect := c.http.CheckRedirect
	c.http.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := checkRedirect(req, via); err != nil {
			return err
		}
		state, _ := req.Context().Value(addic7edRedirectKey{}).(*addic7edRedirectState)
		if err := a.count(); err != nil {
			if state != nil {
				state.err = err
			}
			return err
		}
		if err := c.wait(req.Context()); err != nil {
			if state != nil {
				state.err = err
			}
			return err
		}
		return nil
	}
	return a
}

// count enforces the daily cap. Once reached, the site rests until midnight UTC.
func (a *addic7ed) count() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now().UTC()
	if d := now.Format("2006-01-02"); d != a.day {
		a.day, a.calls = d, 0
	}
	if a.calls >= addic7edDailyCap {
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
		return &Error{Kind: KindRateLimited, RetryAfter: next.Sub(now), Msg: "HolyArrs's daily number of calls to this site is used up"}
	}
	a.calls++
	return nil
}

// page fetches one page of the site, by GET or by form POST when form is set,
// and checks it for an anti-bot check.
func (a *addic7ed) page(ctx context.Context, p string, form url.Values, referer string) ([]byte, error) {
	if err := a.count(); err != nil {
		return nil, err
	}
	raw, err := a.c.ref(p)
	if err != nil {
		return nil, err
	}
	method, hdr := http.MethodGet, map[string]string{}
	var body []byte
	if form != nil {
		method, body = http.MethodPost, []byte(form.Encode())
		hdr["Content-Type"] = "application/x-www-form-urlencoded"
	}
	if referer != "" {
		hdr["Referer"] = referer
	}
	state := &addic7edRedirectState{}
	ctx = context.WithValue(ctx, addic7edRedirectKey{}, state)
	out, err := a.c.do(ctx, method, raw, body, hdr, "text/html, */*;q=0.5", addic7edMaxPage, false)
	if err != nil && state.err != nil {
		return nil, state.err
	}
	return out, err
}

func head(b []byte, n int) []byte {
	if len(b) > n {
		return b[:n]
	}
	return b
}

func challengeErr() error {
	return &Error{Kind: KindBlocked, Msg: "the site showed an anti-bot check; HolyArrs never tries to get past one and rests instead"}
}

// isChallenge spots an anti-bot or captcha page.
func isChallenge(s string) bool {
	s = strings.ToLower(s)
	for _, w := range []string{"captcha", "cf-challenge", "challenge-platform", "just a moment", "attention required", "are you a robot", "verify you are human"} {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// hasLoginForm reports whether a page asks to sign in, judged by a password field.
func hasLoginForm(b []byte) bool {
	found := false
	tokens(head(b, 256<<10), func(t token) {
		if t.tag == "input" && !t.end && (strings.EqualFold(t.inputType, "password") || strings.EqualFold(t.name, "password")) {
			found = true
		}
	})
	return found
}

// signIn posts the sign-in form. The cookie it sets stays in the jar.
func (a *addic7ed) signIn(ctx context.Context) error {
	return a.login(ctx, false, 0)
}

func (a *addic7ed) refreshSignIn(ctx context.Context, seen uint64) error {
	return a.login(ctx, true, seen)
}

func (a *addic7ed) login(ctx context.Context, refresh bool, seen uint64) error {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	a.mu.Lock()
	ok := a.signedIn && (!refresh || a.session != seen)
	if refresh && !ok {
		a.signedIn = false
	}
	a.mu.Unlock()
	if ok {
		return nil
	}
	if a.c.cfg.Username == "" || a.c.cfg.Password == "" {
		return &Error{Kind: KindLoginRefused, Msg: "Addic7ed needs your user name and password"}
	}
	form := url.Values{"username": {a.c.cfg.Username}, "password": {a.c.cfg.Password}, "remember": {"true"}}
	b, err := a.page(ctx, a7Login, form, a.c.base.String()+"/login.php")
	if err != nil {
		var se *Error
		if errors.As(err, &se) && se.Kind == KindLoginRefused {
			return &Error{Kind: KindLoginRefused, Msg: "the site refused the user name or password"}
		}
		return err
	}
	home := *a.c.base
	home.Path = strings.TrimRight(home.Path, "/") + "/"
	if hasLoginForm(b) || len(a.c.http.Jar.Cookies(&home)) == 0 {
		return &Error{Kind: KindLoginRefused, Msg: "the site refused the user name or password"}
	}
	a.mu.Lock()
	a.signedIn = true
	a.session++
	a.mu.Unlock()
	return nil
}

func (a *addic7ed) sessionNumber() uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.session
}

func (a *addic7ed) Search(ctx context.Context, q Query) ([]Candidate, error) {
	if q.Kind != "episode" || q.Season <= 0 || q.Episode <= 0 || strings.TrimSpace(q.Title) == "" {
		return nil, nil
	}
	var langs []string
	for _, l := range q.Languages {
		if l = siteLang(l); l != "" {
			langs = append(langs, l)
		}
	}
	if len(langs) == 0 {
		return nil, nil
	}
	if err := a.signIn(ctx); err != nil {
		return nil, err
	}
	b, err := a.page(ctx, a7Search+"?"+url.Values{"search": {q.Title}, "Submit": {"Search"}}.Encode(), nil, "")
	if err != nil {
		return nil, err
	}
	showPath, fallbackPath := "", ""
	showYear := false
	want := fold(q.Title)
	for _, l := range links(b) {
		l.href = a.local(l.href)
		if !strings.HasPrefix(l.href, a7Show) {
			continue
		}
		id := strings.TrimPrefix(l.href, a7Show)
		if _, err := strconv.Atoi(id); err != nil {
			continue
		}
		t := fold(l.text)
		if q.Year > 0 && t == want+strconv.Itoa(q.Year) {
			showPath = a7Show + id
			showYear = true
			break
		}
		if t == want && fallbackPath == "" {
			fallbackPath = a7Show + id
		}
	}
	if showPath == "" {
		showPath = fallbackPath
	}
	if showPath == "" {
		return nil, nil
	}
	page := showPath + "?season=" + strconv.Itoa(q.Season)
	b, err = a.page(ctx, page, nil, "")
	if err != nil {
		return nil, err
	}
	var out []Candidate
	for _, r := range rows(b) {
		if len(r.cells) < 7 || r.href == "" {
			continue
		}
		s, e1 := strconv.Atoi(strings.TrimSpace(r.cells[0]))
		e, e2 := strconv.Atoi(strings.TrimSpace(r.cells[1]))
		if e1 != nil || e2 != nil || s != q.Season || e != q.Episode {
			continue
		}
		lang := langCode(r.cells[3])
		if lang == "" || !langWanted(langs, lang) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(r.cells[5]), "completed") {
			continue // still in progress on the site
		}
		dl, ok := a.pin(r.href)
		if !ok {
			continue
		}
		c := Candidate{Site: a.c.cfg.Name, Key: strings.TrimPrefix(dl, a7Down), Language: lang, Title: q.Title, Season: s, Episode: e,
			HearingImpaired: isYes(r.cells[6]), Format: "srt", Link: dl + "\n" + page}
		if showYear {
			c.Year = q.Year
		}
		for _, v := range strings.FieldsFunc(r.cells[4], func(r rune) bool { return r == ',' || r == '/' }) {
			if v = strings.TrimSpace(v); v != "" && len(v) <= 200 && len(c.ReleaseNames) < 10 {
				c.ReleaseNames = append(c.ReleaseNames, v)
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// pin keeps a download link only when it is on the site's own host, as a path.
func (a *addic7ed) pin(href string) (string, bool) {
	if strings.HasPrefix(href, "//") {
		return "", false
	}
	u, err := url.Parse(href)
	if err != nil || u.User != nil {
		return "", false
	}
	if u.IsAbs() && (u.Scheme != a.c.base.Scheme || u.Host != a.c.base.Host) {
		return "", false
	}
	if u.Host != "" && u.Host != a.c.base.Host {
		return "", false
	}
	p := a.local(u.Path)
	if !strings.HasPrefix(p, a7Down) || strings.Contains(p, "..") || len(p) > 200 {
		return "", false
	}
	return p, true
}

// local strips the site address's own path from a link path, for a site served
// under a path such as the lab's fakes.
func (a *addic7ed) local(p string) string {
	if root := strings.TrimRight(a.c.base.Path, "/"); root != "" {
		if rest, ok := strings.CutPrefix(p, root+"/"); ok {
			return "/" + rest
		}
	}
	return p
}

func (a *addic7ed) Download(ctx context.Context, c Candidate) ([]byte, error) {
	dl, page, _ := strings.Cut(c.Link, "\n")
	p, ok := a.pin(dl)
	if !ok || !strings.HasPrefix(page, a7Show) {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site sent a link HolyArrs cannot follow"}
	}
	ref, err := a.c.ref(page)
	if err != nil {
		return nil, err
	}
	if err := a.signIn(ctx); err != nil {
		return nil, err
	}
	for try := 0; try < 2; try++ {
		seen := a.sessionNumber()
		b, err := a.page(ctx, p, nil, ref)
		if err != nil {
			return nil, err
		}
		if !looksHTML(b) {
			return b, nil
		}
		if !hasLoginForm(b) {
			return nil, &Error{Kind: KindUnreadable, Msg: "the site answered with a page instead of a subtitle file"}
		}
		if try == 0 {
			if err := a.refreshSignIn(ctx, seen); err != nil {
				return nil, err
			}
		}
	}
	return nil, &Error{Kind: KindLoginRefused, Msg: "the site asked to sign in again after signing in"}
}

func looksHTML(b []byte) bool {
	h := bytes.ToLower(bytes.TrimSpace(head(b, 512)))
	if bytes.HasPrefix(h, []byte("<!doctype")) || hasLoginForm(h) {
		return true
	}
	for _, tag := range []string{"html", "head", "body", "form", "main", "div", "p", "h1", "title"} {
		if bytes.HasPrefix(h, []byte("<"+tag)) {
			return true
		}
	}
	return false
}

func isYes(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "no", "0", "-":
		return false
	}
	return true
}

// a7Langs maps the site's language names to our language codes.
var a7Langs = map[string]string{
	"english": "en", "french": "fr", "spanish": "es", "spanish (latin america)": "es", "spanish (spain)": "es", "german": "de",
	"italian": "it", "portuguese": "pt", "portuguese (brazilian)": "pt-br", "dutch": "nl", "swedish": "sv", "polish": "pl",
	"russian": "ru", "greek": "el", "hungarian": "hu", "romanian": "ro", "czech": "cs", "turkish": "tr", "arabic": "ar",
	"hebrew": "he", "croatian": "hr", "serbian (latin)": "sr", "serbian (cyrillic)": "sr", "danish": "da", "finnish": "fi",
	"norwegian": "no", "bulgarian": "bg", "catalan": "ca", "chinese (simplified)": "zh", "chinese (traditional)": "zh",
	"japanese": "ja", "korean": "ko", "indonesian": "id", "vietnamese": "vi", "persian": "fa", "slovenian": "sl",
	"slovak": "sk", "ukrainian": "uk", "bosnian": "bs", "macedonian": "mk", "estonian": "et", "lithuanian": "lt",
}

func langCode(name string) string { return a7Langs[strings.ToLower(strings.TrimSpace(name))] }

// A small bounded HTML reader.
// It reads tags and text in one forward pass, so the work grows linearly with
// the page size. It skips comments and script and style bodies, and keeps only
// what the adapter needs: links with their text, and table rows with their
// cell texts and first link. Nested or broken tags are taken as they come and
// nothing is ever run.

type link struct{ href, text string }

type row struct {
	cells []string
	href  string
}

type token struct {
	tag       string // lower case, or "" for text
	end       bool
	href      string
	inputType string
	name      string
	text      string
	valid     bool
	attrs     []byte // the raw attributes of a start tag, read with attr
}

// tokens calls fn for each tag and run of text in b.
func tokens(b []byte, fn func(token)) {
	i := 0
	for i < len(b) {
		if b[i] != '<' {
			j := bytes.IndexByte(b[i:], '<')
			if j < 0 {
				j = len(b) - i
			}
			fn(token{text: string(b[i : i+j]), valid: true})
			i += j
			continue
		}
		if bytes.HasPrefix(b[i:], []byte("<!--")) {
			j := bytes.Index(b[i+4:], []byte("-->"))
			if j < 0 {
				return
			}
			i += 4 + j + 3
			continue
		}
		j := tagEnd(b[i+1:])
		if j < 0 {
			return
		}
		inner := b[i+1 : i+1+j]
		i += j + 2
		t := token{valid: true}
		if len(inner) > 0 && inner[0] == '/' {
			t.end, inner = true, inner[1:]
		}
		n := 0
		for n < len(inner) && (inner[n] >= 'a' && inner[n] <= 'z' || inner[n] >= 'A' && inner[n] <= 'Z' || inner[n] >= '0' && inner[n] <= '9') {
			n++
		}
		if n == 0 {
			continue // doctype, processing instruction or a stray angle bracket
		}
		t.tag = strings.ToLower(string(inner[:n]))
		if !t.end {
			t.attrs = inner[n:]
		}
		if !t.end && (t.tag == "script" || t.tag == "style") {
			k := indexClosingTag(b[i:], t.tag)
			if k < 0 {
				return
			}
			i += k
			continue
		}
		if t.tag == "a" && !t.end {
			t.href = attr(inner[n:], "href")
		}
		if t.tag == "input" && !t.end {
			t.inputType = attr(inner[n:], "type")
			t.name = attr(inner[n:], "name")
		}
		fn(t)
	}
}

func tagEnd(b []byte) int {
	var quote byte
	for i, c := range b {
		switch {
		case quote != 0 && c == quote:
			quote = 0
		case quote == 0 && (c == '\'' || c == '"'):
			quote = c
		case quote == 0 && c == '>':
			return i
		}
	}
	return -1
}

// attr reads the value of one attribute from inside a tag.
func attr(in []byte, name string) string {
	for i := 0; i < len(in); {
		for i < len(in) && (isSpace(in[i]) || in[i] == '/') {
			i++
		}
		start := i
		for i < len(in) && isAttrName(in[i]) {
			i++
		}
		if start == i {
			i++
			continue
		}
		attrName := in[start:i]
		for i < len(in) && isSpace(in[i]) {
			i++
		}
		if i >= len(in) || in[i] != '=' {
			continue
		}
		i++
		for i < len(in) && isSpace(in[i]) {
			i++
		}
		valueStart, valueEnd := i, i
		if i < len(in) && (in[i] == '"' || in[i] == '\'') {
			q := in[i]
			i++
			valueStart = i
			for i < len(in) && in[i] != q {
				i++
			}
			if i >= len(in) {
				return ""
			}
			valueEnd = i
			i++
		} else {
			valueStart = i
			for i < len(in) && !isSpace(in[i]) {
				i++
			}
			valueEnd = i
		}
		if !strings.EqualFold(string(attrName), name) {
			continue
		}
		if valueEnd-valueStart > 500 {
			return ""
		}
		return strings.TrimSpace(html.UnescapeString(string(in[valueStart:valueEnd])))
	}
	return ""
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

func isAttrName(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == ':'
}

func clip(s string) string {
	s = strings.Join(strings.Fields(html.UnescapeString(s)), " ")
	if len(s) > addic7edMaxText {
		s = s[:addic7edMaxText]
	}
	return strings.ToValidUTF8(s, "")
}

// links returns every link with its text.
func links(b []byte) []link {
	var out []link
	var cur *link
	var text strings.Builder
	tokens(b, func(t token) {
		switch {
		case t.tag == "a" && !t.end:
			if cur != nil && len(out) < addic7edMaxRows {
				out = append(out, link{cur.href, clip(text.String())})
			}
			cur = &link{href: t.href}
			text.Reset()
		case t.tag == "a" && t.end:
			if cur != nil && len(out) < addic7edMaxRows {
				out = append(out, link{cur.href, clip(text.String())})
			}
			cur = nil
		case t.tag == "" && cur != nil && text.Len() < addic7edMaxText:
			text.WriteString(t.text)
		}
	})
	return out
}

// rows returns the table rows with their cell texts and first link.
func rows(b []byte) []row {
	var out []row
	var cur *row
	var cell *strings.Builder
	flush := func() {
		if cur != nil && cell != nil && len(cur.cells) < addic7edMaxCells {
			cur.cells = append(cur.cells, clip(cell.String()))
		}
		cell = nil
	}
	done := func() {
		flush()
		if cur != nil && len(cur.cells) > 0 && len(out) < addic7edMaxRows {
			out = append(out, *cur)
		}
		cur = nil
	}
	tokens(b, func(t token) {
		switch {
		case t.tag == "tr":
			done()
			if !t.end {
				cur = &row{}
			}
		case t.tag == "table" && t.end:
			done()
		case (t.tag == "td" || t.tag == "th") && cur != nil:
			flush()
			if !t.end {
				cell = &strings.Builder{}
			}
		case t.tag == "a" && !t.end && cur != nil && isDownloadHref(t.href):
			cur.href = t.href
		case t.tag == "" && cell != nil && cell.Len() < addic7edMaxText:
			cell.WriteString(t.text)
		}
	})
	done()
	return out
}

func isDownloadHref(href string) bool {
	u, err := url.Parse(href)
	return err == nil && (strings.HasPrefix(u.Path, a7Down) || strings.Contains(u.Path, a7Down))
}

// indexFold finds an ASCII needle in b in any case, in one pass.
func indexFold(b []byte, needle string) int {
	for i := 0; i+len(needle) <= len(b); {
		j := bytes.IndexByte(b[i:], needle[0])
		if j < 0 {
			return -1
		}
		i += j
		if i+len(needle) <= len(b) && strings.EqualFold(string(b[i:i+len(needle)]), needle) {
			return i
		}
		i++
	}
	return -1
}

func indexClosingTag(b []byte, tag string) int {
	needle := "</" + tag
	for at := 0; at < len(b); {
		i := indexFold(b[at:], needle)
		if i < 0 {
			return -1
		}
		i += at
		after := i + len(needle)
		if after == len(b) || b[after] == '>' || isSpace(b[after]) {
			return i
		}
		at = i + 1
	}
	return -1
}
