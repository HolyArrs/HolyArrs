package api

import (
	"net/http/httptest"
	"testing"
)

func TestSizedCoverPicksTheThumbnail(t *testing.T) {
	const rg = "https://coverartarchive.org/release-group/abc"
	cases := []struct{ src, size, want string }{
		{rg + "/front-250", "500", rg + "/front-500"},
		{rg + "/front", "1200", rg + "/front-1200"},
		{rg + "/front-500.jpg", "250", rg + "/front-250"},
		{"https://coverartarchive.org/release/x/123-250", "1200", "https://coverartarchive.org/release/x/123-1200"},
		{"https://e-cdns-images.dzcdn.net/cover/500x500.jpg", "1200", "https://e-cdns-images.dzcdn.net/cover/500x500.jpg"},
		{"https://cdn-images.dzcdn.net/images/cover/ab12/1000x1000-000000-80-0-0.jpg", "500", "https://cdn-images.dzcdn.net/images/cover/ab12/500x500-000000-80-0-0.jpg"},
		{"https://e-cdns-images.dzcdn.net/images/cover/ab12/250x250-000000-80-0-0.jpg", "1200", "https://e-cdns-images.dzcdn.net/images/cover/ab12/1200x1200-000000-80-0-0.jpg"},
		{"https://is1-ssl.mzstatic.com/image/thumb/Music/v4/x.png/600x600bb.jpg", "1200", "https://is1-ssl.mzstatic.com/image/thumb/Music/v4/x.png/1200x1200bb.jpg"},
		{"https://is1-ssl.mzstatic.com/image/thumb/Music/v4/x.png/100x100.jpg", "250", "https://is1-ssl.mzstatic.com/image/thumb/Music/v4/x.png/250x250bb.jpg"},
	}
	for _, c := range cases {
		if got := sizedCover(c.src, c.size); got != c.want {
			t.Errorf("sizedCover(%q, %s) = %q, want %q", c.src, c.size, got, c.want)
		}
	}
	for q, want := range map[string]string{"": "250", "?size=500": "500", "?size=1200": "1200", "?size=999": "250"} {
		if got := coverSize(httptest.NewRequest("GET", "/x"+q, nil)); got != want {
			t.Errorf("coverSize(%q) = %s, want %s", q, got, want)
		}
	}
}
