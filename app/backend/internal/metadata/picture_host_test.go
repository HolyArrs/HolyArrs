package metadata

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type fixedAnswer struct{ ctype, body string }

func (f fixedAnswer) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, ContentLength: -1, Header: http.Header{"Content-Type": {f.ctype}}, Body: io.NopCloser(strings.NewReader(f.body))}, nil
}

// Picture addresses on the first address come back on the install's own;
// anything that is not JSON is left alone.
func TestPictureHostSwapsOldAddress(t *testing.T) {
	old, own := OldServiceURL+"/v1/img/", DefaultServiceURL+"/v1/img/"
	read := func(ctype, body string) string {
		resp, err := pictureHost{next: fixedAnswer{ctype, body}, old: old, own: own}.RoundTrip(&http.Request{})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		if resp.ContentLength >= 0 && int(resp.ContentLength) != len(b) {
			t.Fatalf("length says %d, body is %d", resp.ContentLength, len(b))
		}
		return string(b)
	}
	in := `{"logo":"` + old + `logo/603/500.png","site":"` + OldServiceURL + `/v1/other"}`
	want := `{"logo":"` + own + `logo/603/500.png","site":"` + OldServiceURL + `/v1/other"}`
	if got := read("application/json; charset=utf-8", in); got != want {
		t.Fatalf("got %s", got)
	}
	if got := read("image/webp", in); got != in {
		t.Fatalf("a picture was changed: %s", got)
	}
	if OldPictures(DefaultServiceURL) != old || strings.Contains(OldPictures("https://my.server"), "holyarrs") {
		t.Fatal("old pictures are only allowed on the official server")
	}
}
