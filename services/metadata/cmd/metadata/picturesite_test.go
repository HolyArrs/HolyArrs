package main

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
)

// A picture connection error names the site, plainly when it is one we use,
// and stays a fetchq.ErrConnection.
func TestConnSite(t *testing.T) {
	cause := &url.Error{Op: "Get", URL: "https://is1-ssl.mzstatic.com/image/thumb/x.jpg", Err: errors.New("read: connection reset")}
	err := connSite(errors.Join(errors.New("imgstore: ask again later"), cause))
	if !errors.Is(err, fetchq.ErrConnection) || !strings.HasSuffix(err.Error(), ": Apple Music covers (is1-ssl.mzstatic.com)") {
		t.Fatalf("%v", err)
	}
	if err := connSite(errors.New("no address")); err != fetchq.ErrConnection {
		t.Fatalf("bare: %v", err)
	}
	for host, want := range map[string]string{"coverartarchive.org": "Cover Art Archive (coverartarchive.org)", "assets.fanart.tv": "assets.fanart.tv", "example.com": "example.com", "notmzstatic.com": "notmzstatic.com"} {
		if got := siteName(host); got != want {
			t.Errorf("%s: %q", host, got)
		}
	}
}
