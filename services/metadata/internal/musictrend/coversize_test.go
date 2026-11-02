package musictrend

import (
	"encoding/json"
	"testing"
)

func TestSizedCover(t *testing.T) {
	const caa = "https://coverartarchive.org/release-group/abc"
	const dz = "https://e-cdns-images.dzcdn.net/images/cover/2e01/"
	const it = "https://is1-ssl.mzstatic.com/image/thumb/Music116/v4/08/x.png/"
	cases := []struct {
		in   string
		px   int
		want string
	}{
		{caa + "/front-250", 1200, caa + "/front-1200"},
		{caa + "/front", 500, caa + "/front-500"},
		{"https://coverartarchive.org/release/r/123-500.jpg", 250, "https://coverartarchive.org/release/r/123-250"},
		{dz + "500x500-000000-80-0-0.jpg", 1200, dz + "1200x1200-000000-80-0-0.jpg"},
		{dz + "1000x1000-000000-80-0-0.jpg", 250, dz + "250x250-000000-80-0-0.jpg"},
		{"https://api.deezer.com/album/42/image?size=big", 1200, "https://api.deezer.com/album/42/image?size=xl"},
		{it + "100x100bb.jpg", 1200, it + "1200x1200bb.jpg"},
		{it + "600x600bb.jpg", 250, it + "250x250bb.jpg"},
		{"https://example.com/x.jpg", 500, "https://example.com/x.jpg"},
	}
	for _, c := range cases {
		if got := SizedCover(c.in, c.px); got != c.want {
			t.Errorf("SizedCover(%q, %d) = %q, want %q", c.in, c.px, got, c.want)
		}
	}
	if CoverVariants("https://example.com/x.jpg") != nil {
		t.Error("unknown link got variants")
	}
}

func TestUpgradeWindowBodyAddsCoverSizes(t *testing.T) {
	old := []byte(`{"window":"week","items":[{"mbid":"m","cover":"https://e-cdns-images.dzcdn.net/images/cover/ab/1000x1000-000000-80-0-0.jpg"},{"mbid":"n"}]}`)
	var got struct{ Items []NewItem }
	if err := json.Unmarshal(upgradeWindowBody(old), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 2 || got.Items[0].CoverSizes["500"] != "https://e-cdns-images.dzcdn.net/images/cover/ab/500x500-000000-80-0-0.jpg" || got.Items[1].CoverSizes != nil {
		t.Errorf("items = %+v", got.Items)
	}
}
