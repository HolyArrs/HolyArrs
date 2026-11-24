package sources

import (
	"net/url"
	"strings"
)

// infoURLKey keeps the link to the release's own page on its Source with a
// stored Release, so the search screens can open it.
const infoURLKey = "info_url"

// webLink returns s when it is an http or https address with a host, and ""
// for anything else, so no script or data link reaches a screen.
func webLink(s string) string {
	s = strings.TrimSpace(s)
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return ""
	}
	if sc := strings.ToLower(u.Scheme); sc != "http" && sc != "https" {
		return ""
	}
	return s
}

// itemInfoURL picks the release's page: the comments link, else the guid
// when that is a web address, as the old apps do.
func itemInfoURL(comments, guid string) string {
	if u := webLink(comments); u != "" {
		return u
	}
	return webLink(guid)
}

// keepInfoURL returns the link to store, or "" when it holds one of the
// Source's keys, which must never reach a screen.
func keepInfoURL(link string, keys []string) string {
	link = webLink(link)
	if link == "" || cleanLink(link, keys) != link {
		return ""
	}
	return link
}

// storedInfoURL reads back the link kept with a Release.
func storedInfoURL(parsed map[string]any) string {
	s, _ := parsed[infoURLKey].(string)
	return webLink(s)
}
