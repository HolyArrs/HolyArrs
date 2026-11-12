package sites

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// Three subtitle sites that need no account: YIFY Subtitles (movies, an HTML
// table per movie), TVsubtitles (shows, show, season and episode pages) and
// BSPlayer (a SOAP API searched by file hash). The page and reply shapes are
// our reading of the research notes and are checked only against the fakes in
// package fakesite, never the live sites.

const extraMaxText = 300

// sitePath keeps a link only when it points at the site's own host, and
// returns it as a path on the site. Relative links are taken from the root.
func sitePath(c *client, href string) (string, bool) {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "//") || strings.Contains(href, `\`) {
		return "", false
	}
	u, err := url.Parse(href)
	if err != nil || u.User != nil || u.Opaque != "" {
		return "", false
	}
	if u.IsAbs() && (u.Scheme != c.base.Scheme || u.Host != c.base.Host) {
		return "", false
	}
	if u.Host != "" && u.Host != c.base.Host {
		return "", false
	}
	p := u.Path
	if root := strings.TrimRight(c.base.Path, "/"); root != "" {
		if rest, ok := strings.CutPrefix(p, root+"/"); ok {
			p = rest
		}
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if strings.Contains(p, "..") || len(p) > 300 {
		return "", false
	}
	return p, true
}

// getPage fetches a path on the site with extra headers.
func (c *client) getPage(ctx context.Context, p string, hdr map[string]string, missingOK bool) ([]byte, error) {
	raw, err := c.ref(p)
	if err != nil {
		return nil, err
	}
	return c.do(ctx, http.MethodGet, raw, nil, hdr, "text/html, */*;q=0.5", addic7edMaxPage, missingOK)
}

func hasClass(attrs []byte, name string) bool {
	for _, f := range strings.Fields(attr(attrs, "class")) {
		if strings.EqualFold(f, name) {
			return true
		}
	}
	return false
}

func wantedLangs(asked []string) []string {
	var out []string
	for _, l := range asked {
		if l = siteLang(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// hrow is one table row: its cell texts, the first link in each cell, and the
// classes of every tag inside it.
type hrow struct {
	cells   []string
	hrefs   []string
	classes []string
}

// tableRows reads the rows of the first table whose attributes match.
func tableRows(b []byte, match func(attrs []byte) bool) []hrow {
	var out []hrow
	in, done, depth := false, false, 0
	var cur *hrow
	var cell *strings.Builder
	href := ""
	flush := func() {
		if cur != nil && cell != nil && len(cur.cells) < addic7edMaxCells {
			cur.cells = append(cur.cells, clip(cell.String()))
			cur.hrefs = append(cur.hrefs, href)
		}
		cell, href = nil, ""
	}
	end := func() {
		flush()
		if cur != nil && len(cur.cells) > 0 && len(out) < addic7edMaxRows {
			out = append(out, *cur)
		}
		cur = nil
	}
	tokens(b, func(t token) {
		if done {
			return
		}
		if t.tag == "table" {
			switch {
			case !t.end && !in && match(t.attrs):
				in = true
			case !t.end && in:
				depth++
			case t.end && in && depth > 0:
				depth--
			case t.end && in:
				end()
				done = true
			}
			return
		}
		if !in {
			return
		}
		switch {
		case t.tag == "tr":
			end()
			if !t.end {
				cur = &hrow{}
			}
		case t.tag == "td" || t.tag == "th":
			flush()
			if !t.end && cur != nil {
				cell = &strings.Builder{}
			}
		case t.tag == "" && cell != nil && cell.Len() < extraMaxText:
			cell.WriteString(t.text)
		case t.tag != "" && !t.end && cur != nil:
			if t.tag == "a" && cell != nil && href == "" {
				href = t.href
			}
			if c := attr(t.attrs, "class"); c != "" && len(cur.classes) < 100 {
				cur.classes = append(cur.classes, strings.Fields(c)...)
			}
		}
	})
	end()
	return out
}
