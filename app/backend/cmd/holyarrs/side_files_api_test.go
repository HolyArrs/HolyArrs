package main

import (
	"strings"
	"testing"
)

// route: GET /api/v1/settings/side-files
// route: PUT /api/v1/settings/side-files
func TestSideFilesAPI(t *testing.T) {
	t.Parallel()
	srv, _, _ := lbLibSetup(t)
	url := srv.URL + "/api/v1/settings/side-files"
	if b := string(lbDo(t, "GET", url, "", 200)); !strings.Contains(b, `"on":false`) || !strings.Contains(b, `"types":["srt"]`) {
		t.Fatalf("default %s", b)
	}
	lbDo(t, "PUT", url, `{"on":true,"types":["mkv"]}`, 400)
	lbDo(t, "PUT", url, `{"on":true,"types":["srt","ass"]}`, 200)
	if b := string(lbDo(t, "GET", url, "", 200)); !strings.Contains(b, `"on":true`) || !strings.Contains(b, `"types":["srt","ass"]`) {
		t.Fatalf("saved %s", b)
	}
}
