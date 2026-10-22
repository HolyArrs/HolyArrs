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
