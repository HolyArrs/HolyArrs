package discover

import (
	"context"
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

type fakeLogoer struct {
	logo  string
	err   error
	asked []string
}

func (f *fakeLogoer) TitleLogo(_ context.Context, kind, id, lang, orig string) (string, error) {
	f.asked = append(f.asked, kind+"/"+id+"/"+lang+"/"+orig)
	return f.logo, f.err
}

func TestReaderLogo(t *testing.T) {
	cases := []struct {
		name      string
		lang      string
		logos     map[string]string
		src       *fakeLogoer
		changed   bool
		want      string
		wantAsked int
	}{
		{"reads the reader's logo", "de", nil, &fakeLogoer{logo: "de.png"}, true, "de.png", 1},
		{"keeps none as asked", "ja", nil, &fakeLogoer{}, true, "", 1},
		{"asked before", "de", map[string]string{"de": ""}, &fakeLogoer{logo: "x"}, false, "", 0},
		{"English needs no read", "en", nil, &fakeLogoer{logo: "x"}, false, "", 0},
		{"error asks again later", "ko", nil, &fakeLogoer{err: errors.New("down")}, false, "", 1},
	}
	for _, c := range cases {
		orig := c.logos
		d := metadata.Details{Logos: c.logos}
		d.OriginalLanguage = "ko"
		got := readerLogo(context.Background(), c.src, &d, "movie", "496243", c.lang)
		if got != c.changed || d.Logos[c.lang] != c.want || len(c.src.asked) != c.wantAsked {
			t.Errorf("%s: changed %v logos %v asked %v", c.name, got, d.Logos, c.src.asked)
		}
		if c.changed && len(orig) != 0 {
			t.Errorf("%s: cached map changed", c.name)
		}
	}
	f := &fakeLogoer{logo: "de.png"}
	d := metadata.Details{}
	d.OriginalLanguage = "ko"
	readerLogo(context.Background(), f, &d, "movie", "496243", "de")
	if f.asked[0] != "movie/496243/de/ko" {
		t.Fatalf("asked %v", f.asked)
	}
}
