package fakesite

import (
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Addic7ed fakes a show subtitle site that serves HTML pages and needs a
// signed-in account to download. It has a sign-in form that sets a cookie, a
// show search page, a season page with one table row per subtitle, and
// downloads that need the cookie plus a Referer from the show page. The page
// shapes are our own, made for tests. Every name, password and subtitle is
// made up by the test that sets it, and it never contacts the outside.
type Addic7ed struct {
	mu   sync.Mutex
	User string
	Pass string
	// Shows maps a show id to its name.
	Shows map[int]string
	// Subs are the episode subtitles. ID is the numeric download id and Title the
	// show's name. Downloads == -1 marks a subtitle still in progress, shown as
	// 45.3% done.
	Subs []Sub
	// Challenge answers every page with an anti-bot check page.
	Challenge bool
	// Hostile, when set, is the raw body of every season page.
	Hostile string
	// LinkHost, when set, makes download links absolute on that host.
	LinkHost string
	// Expire ends every session once, as if the cookie had expired.
	Expire bool
	// LoginDelay keeps a fake login open so tests can overlap first calls.
	LoginDelay time.Duration
	// DownloadHTML, when set, answers downloads with this HTML page.
	DownloadHTML string
	// Base is the path the fake is served under, such as /addic7ed in the lab.
	// Links and redirects include it.
	Base string

	Logins      int
	Requests    int
	Downloads   int
	LastReferer string
	sessions    map[string]bool
	n           int
}

const a7Cookie = "a7session"

func (a *Addic7ed) page(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte("<!DOCTYPE html><html><head><title>Fake show subtitles</title></head><body>" + body + "</body></html>"))
}

const a7LoginForm = `<form action="dologin.php" method="post"><input name="username"><input type="password" name="password"></form>`

func (a *Addic7ed) signedIn(r *http.Request) bool {
	ck, err := r.Cookie(a7Cookie)
	return err == nil && a.sessions[ck.Value]
}

func (a *Addic7ed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Requests++
	if a.Challenge {
		a.page(w, http.StatusServiceUnavailable, `<h1>Just a moment...</h1><div class="cf-challenge">Please complete the captcha to continue.</div>`)
		return
	}
	if a.Expire {
		a.Expire = false
		a.sessions = nil
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/dologin.php":
		a.Logins++
		time.Sleep(a.LoginDelay)
		if err := r.ParseForm(); err != nil || r.PostForm.Get("username") != a.User || r.PostForm.Get("password") != a.Pass {
			a.page(w, http.StatusOK, `<div class="login-error">Wrong user name or password.</div>`+a7LoginForm)
			return
		}
		a.n++
		tok := fmt.Sprintf("fake-session-%d", a.n)
		if a.sessions == nil {
			a.sessions = map[string]bool{}
		}
		a.sessions[tok] = true
		http.SetCookie(w, &http.Cookie{Name: a7Cookie, Value: tok, Path: a.Base + "/", HttpOnly: true})
		http.Redirect(w, r, a.Base+"/index.php", http.StatusFound)
	case r.URL.Path == "/index.php":
		a.page(w, http.StatusOK, `<p>Welcome back.</p>`)
	case r.URL.Path == "/login.php":
		a.page(w, http.StatusOK, a7LoginForm)
	case r.URL.Path == "/search.php":
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
		var b strings.Builder
		b.WriteString(`<table class="results">`)
		for id, name := range a.Shows {
			if q != "" && strings.Contains(strings.ToLower(name), q) {
				fmt.Fprintf(&b, `<tr><td><a href="%s/show/%d">%s</a></td></tr>`, a.Base, id, html.EscapeString(name))
			}
		}
		b.WriteString(`</table>`)
		a.page(w, http.StatusOK, b.String())
	case strings.HasPrefix(r.URL.Path, "/show/"):
		id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/show/"))
		name, ok := a.Shows[id]
		if err != nil || !ok {
			http.NotFound(w, r)
			return
		}
		if a.Hostile != "" {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(a.Hostile))
			return
		}
		season := r.URL.Query().Get("season")
		var b strings.Builder
		b.WriteString(`<table class="season"><tr><th>S</th><th>E</th><th>Episode</th><th>Language</th><th>Version</th><th>Done</th><th>HI</th><th></th></tr>`)
		for _, s := range a.Subs {
			if s.Title != name || strconv.Itoa(s.Season) != season {
				continue
			}
			done, hi := "Completed", ""
			if s.Downloads == -1 {
				done = "45.3% Completed"
			}
			if s.HearingImpaired {
				hi = "yes"
			}
			link := a.Base + "/download/" + s.ID
			if a.LinkHost != "" {
				link = a.LinkHost + link
			}
			fmt.Fprintf(&b, `<tr class="sub"><td>%d</td><td>%d</td><td>Episode %d</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td><a href="%s">Download</a></td></tr>`,
				s.Season, s.Episode, s.Episode, html.EscapeString(s.Language), html.EscapeString(s.Release), done, hi, html.EscapeString(link))
		}
		b.WriteString(`</table>`)
		a.page(w, http.StatusOK, b.String())
	case strings.HasPrefix(r.URL.Path, "/download/"):
		if !a.signedIn(r) {
			http.Redirect(w, r, a.Base+"/login.php", http.StatusFound)
			return
		}
		a.LastReferer = r.Header.Get("Referer")
		if a.DownloadHTML != "" {
			a.page(w, http.StatusOK, a.DownloadHTML)
			return
		}
		if !strings.Contains(a.LastReferer, "/show/") {
			a.page(w, http.StatusForbidden, `<p>Open the show page first.</p>`)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/download/")
		for _, s := range a.Subs {
			if s.ID == id {
				a.Downloads++
				w.Header().Set("Content-Type", "text/srt; charset=utf-8")
				_, _ = w.Write([]byte(s.Text))
				return
			}
		}
		http.NotFound(w, r)
	default:
		http.NotFound(w, r)
	}
}
