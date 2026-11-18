package sources

import (
	"context"
	"slices"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A year right after the show's name must be the year the show began (the
// seeded show began in 2015, TVDB id 4242), unless the Source's own id for
// the release is the show's.
func TestJudgeShow_YearInName(t *testing.T) {
	f := seedShow(t, (&tvServer{}).start(t), nil)
	sr := &Searcher{Store: f.s}
	ctx := context.Background()
	set, err := sr.loadShow(ctx, mustTitle(t, f), mustVersion(t, f))
	if err != nil {
		t.Fatal(err)
	}
	set.target[f.slots["S01E01"]] = true
	cases := []struct {
		name string
		tvdb int
		imdb string
		want bool
	}{
		{"Copper.Valley.S01E01.1080p.WEB-DL.H.264-QUILL", 0, "", true},
		{"Copper.Valley.2015.S01E01.1080p.WEB-DL.H.264-QUILL", 0, "", true},
		{"Copper.Valley.2019.S01E01.1080p.WEB-DL.H.264-QUILL", 0, "", false},
		{"Copper.Valley.1998.S01E01.1080p.WEB-DL.H.264-QUILL", 0, "", false},
		{"Copper.Valley.2019.S01E01.1080p.WEB-DL.H.264-OXBOW", 4242, "", true},
		{"Valle.de.Cobre.S01E01.1080p.WEB-DL.H.264-QUILL", 4242, "", true},
		{"Copper.Valley.2019.S01E01.1080p.WEB-DL.H.264-MOSSFOX", 9999, "", false},
		{"Valle.de.Cobre.S01E01.1080p.WEB-DL.H.264-OXBOW", 0, "1234567", false},
	}
	var all []found
	for _, c := range cases {
		all = append(all, found{src: domain.Source{ID: f.srcID}, item: Item{Title: c.name, Size: 1500 << 20, Protocol: "torrent", Published: time.Now().Add(-time.Hour), TVDB: c.tvdb, IMDb: c.imdb}})
	}
	_, vs, err := sr.judgeShow(ctx, set, all, false, false, sr.showRules(ctx))
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range cases {
		if vs[i].Passed != c.want {
			t.Errorf("%s (tvdb %d): passed %v, want %v: %+v", c.name, c.tvdb, vs[i].Passed, c.want, vs[i].Reasons)
		}
	}
	if !slices.ContainsFunc(vs[2].Reasons, func(r mediafiles.Reason) bool {
		return r.Step == "match" && r.Effect == "fail" && r.Detail == "the name says 2019; this show began in 2015"
	}) {
		t.Errorf("2019 release: no year reason: %+v", vs[2].Reasons)
	}
}

// The ids a Source gives for a release are kept.
func TestParseItems_ShowIDs(t *testing.T) {
	items, err := parseItems([]byte(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0" xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>
<item><title>Copper.Valley.S01E01.1080p.WEB-DL.H.264-QUILL</title><link>http://src/1.torrent</link>
<enclosure url="http://src/1.torrent" length="100" type="application/x-bittorrent"/>
<torznab:attr name="tvdbid" value="4242"/><torznab:attr name="imdbid" value="tt0012345"/></item></channel></rss>`), "torrent")
	if err != nil || len(items) != 1 || items[0].TVDB != 4242 || items[0].IMDb != "12345" {
		t.Fatalf("ids: %+v %v", items, err)
	}
}
