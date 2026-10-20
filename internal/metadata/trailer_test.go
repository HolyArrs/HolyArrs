package metadata

import "testing"

func TestPickTrailer(t *testing.T) {
	vs := []DetailVideo{
		{Key: "teaserEN01", Type: "Teaser", Official: true, Language: "en"},
		{Key: "fanTrailer1", Type: "Trailer", Official: false, Language: "en"},
		{Key: "officialEN1", Type: "Trailer", Official: true, Language: "en"},
		{Key: "officialDE1", Type: "Trailer", Official: true, Language: "de"},
	}
	cases := []struct {
		lang, want string
	}{{"en", "officialEN1"}, {"de", "officialDE1"}, {"fr", "officialEN1"}, {"", "officialEN1"}}
	for _, c := range cases {
		if got, ok := PickTrailer(vs, c.lang); !ok || got.Key != c.want {
			t.Errorf("%q: got %q", c.lang, got.Key)
		}
	}
	if got, ok := PickTrailer(vs[:1], "en"); !ok || got.Key != "teaserEN01" {
		t.Errorf("teaser only: %q", got.Key)
	}
	if _, ok := PickTrailer([]DetailVideo{{Key: "clip12345", Type: "Clip"}, {Key: "bad key!", Type: "Trailer"}}, "en"); ok {
		t.Error("clips and bad keys are never picked")
	}
	if _, ok := PickTrailer(nil, "en"); ok {
		t.Error("none")
	}
}
