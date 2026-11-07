package main

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
)

// connSite is fetchq.ErrConnection naming the picture site that could not
// be reached, so the alarm says which one ("Apple Music covers
// (is1-ssl.mzstatic.com)"); the bare error when no address is in err.
func connSite(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		if u, e := url.Parse(ue.URL); e == nil && u.Hostname() != "" {
			return fmt.Errorf("%w: %s", fetchq.ErrConnection, siteName(u.Hostname()))
		}
	}
	return fetchq.ErrConnection
}

// siteName is a picture host with a plain name for the ones we use.
func siteName(host string) string {
	for _, s := range []struct{ suffix, name string }{
		{"mzstatic.com", "Apple Music covers"},
		{"coverartarchive.org", "Cover Art Archive"},
		{"archive.org", "Cover Art Archive"},
		{"fanart.tv", "fanart.tv"},
		{"theaudiodb.com", "TheAudioDB"},
		{"tmdb.org", "TMDB pictures"},
		{"wikimedia.org", "Wikimedia pictures"},
	} {
		if host == s.suffix || strings.HasSuffix(host, "."+s.suffix) {
			if s.name == s.suffix {
				return host
			}
			return s.name + " (" + host + ")"
		}
	}
	return host
}
