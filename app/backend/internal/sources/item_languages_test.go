package sources

import (
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

func TestParseItemsReadsSourceLanguages(t *testing.T) {
	body := []byte(`<rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>
<item><title>Movie.2019.1080p.BluRay.x264.ENG-GRP</title><link>http://x/1</link>
<torznab:attr name="language" value="French"/><torznab:attr name="language" value="German"/><torznab:attr name="language" value="Klingon"/></item>
<item><title>Movie.2019.1080p.WEB-GRP</title><link>http://x/2</link>
<newznab:attr xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/" name="language" value="Spanish, English"/></item>
<item><title>Movie.2019.1080p.WEB-GRP</title><link>http://x/3</link><language>Italian</language></item>
<item><title>Movie.2019.FRENCH.1080p.WEB-GRP</title><link>http://x/4</link></item>
</channel></rss>`)
	items, err := parseItems(body, "torrent")
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"fr", "de"}, {"es", "en"}, {"it"}, nil}
	for i, w := range want {
		if !slices.Equal(items[i].Languages, w) {
			t.Errorf("item %d languages = %v, want %v", i, items[i].Languages, w)
		}
	}
	if got := parseItem(items[0], releasename.Hints{}).Languages; !slices.Equal(got, []string{"fr", "de"}) {
		t.Errorf("Source languages should win over the name, got %v", got)
	}
	if got := parseItem(items[3], releasename.Hints{}).Languages; !slices.Equal(got, []string{"fr"}) {
		t.Errorf("no Source languages should keep the name's, got %v", got)
	}
}

func TestStoredSourceLanguages(t *testing.T) {
	m := toMap(releasename.Result{})
	m[sourceLanguagesKey] = []string{"fr"}
	back := toMap(m)
	if got := storedSourceLanguages(back); !slices.Equal(got, []string{"fr"}) {
		t.Errorf("got %v", got)
	}
}
