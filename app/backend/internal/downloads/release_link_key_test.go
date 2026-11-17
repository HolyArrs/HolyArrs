package downloads

import (
	"strings"
	"testing"
)

// A release link keeps the site's own download key for the Downloader, so any
// message that shows the link must hide it.
func TestRedactURLs_HidesSiteKeyInReleaseLink(t *testing.T) {
	link := "https://api.example.info/api?apikey=site99&id=abc&t=get"
	for _, msg := range []string{"fetch " + link + ": 403", "fetch api.example.info/api?apikey=site99&id=abc: 403"} {
		if got := RedactURLs(msg); strings.Contains(got, "site99") {
			t.Fatalf("shown message holds the key: %q", got)
		}
	}
}
