package downloads

import (
	"errors"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestAria2CategoryMustBeAFolderName(t *testing.T) {
	for c, want := range map[string]error{"holyarrs": nil, "TV Shows 4k": nil, "films.x": nil, ".": ErrAria2Folder, "..": ErrAria2Folder,
		"a:b": ErrAria2Folder, "what?": ErrAria2Folder, "x*": ErrAria2Folder, `say"`: ErrAria2Folder, "<a>": ErrAria2Folder, "a|b": ErrAria2Folder,
		"tab\there": ErrAria2Folder, "a/b": ErrCategory} {
		cat := c
		var d domain.Downloader
		err := fill(Input{Name: "a", Adapter: "aria2", URL: "http://aria2:6800/jsonrpc", Category: &cat}, &d, true)
		if want != nil && !errors.Is(err, want) || want == nil && (errors.Is(err, ErrAria2Folder) || errors.Is(err, ErrCategory)) {
			t.Fatalf("%q: %v, want %v", c, err, want)
		}
		if want == nil && d.Protocol != "torrent" {
			t.Fatalf("%q: protocol %q", c, d.Protocol)
		}
	}
	cat := "a:b"
	var d domain.Downloader
	if err := fill(Input{Name: "q", Adapter: "qbittorrent", URL: "http://q:8080", Category: &cat}, &d, true); errors.Is(err, ErrAria2Folder) {
		t.Fatal("qBittorrent should keep a colon")
	}
}
