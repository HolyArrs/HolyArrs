package model

import (
	"net/url"
	"strings"
)

// SameClient says two Downloaders reach the same download app: the same
// kind at the same host and port. The path after the port does not count,
// so http://sab:8080 and http://sab:8080/sabnzbd are one SABnzbd. When both
// carry a path and the paths differ (two apps behind one proxy, such as
// /qbit-movies and /qbit-tv), they are two apps.
func SameClient(a, b Downloader) bool {
	if !strings.EqualFold(a.Adapter, b.Adapter) {
		return false
	}
	ha, pa, oka := clientHost(a.URL)
	hb, pb, okb := clientHost(b.URL)
	if !oka || !okb || ha != hb {
		return false
	}
	return pa == "" || pb == "" || pa == pb
}

// clientHost is the lower-case host:port of a Downloader address, with the
// scheme's usual port filled in when none is written, and its lower-case
// path without the outer slashes.
func clientHost(raw string) (string, string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", false
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" {
		return "", "", false
	}
	port := u.Port()
	if port == "" {
		port = map[string]string{"https": "443"}[strings.ToLower(u.Scheme)]
		if port == "" {
			port = "80"
		}
	}
	return strings.ToLower(u.Hostname()) + ":" + port, strings.ToLower(strings.Trim(u.Path, "/")), true
}
