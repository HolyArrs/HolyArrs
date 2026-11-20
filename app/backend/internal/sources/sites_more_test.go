package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// The fixtures under testdata/sites are trimmed copies of each site's real
// answers (pages saved by the Internet Archive for the sites blocked here).

// sz is a size as the site writes it, such as 1.5 GB.
func sz(f float64, shift uint) int64 { return int64(f * float64(uint64(1)<<shift)) }

func one(t *testing.T, items []Item, err error, n int) []Item {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != n {
		t.Fatalf("got %d items, want %d: %+v", len(items), n, items)
	}
	return items
}

func wantItem(t *testing.T, got Item, title, link string, size int64, seeders, peers int, hash string) {
	t.Helper()
	if got.Title != title || !strings.HasPrefix(got.Link, link) || got.Size != size || got.InfoHash != hash || got.Protocol != "torrent" {
		t.Errorf("item %+v\nwant title %q link %q size %d hash %q", got, title, link, size, hash)
	}
	if seeders >= 0 && (got.Seeders == nil || *got.Seeders != seeders || got.Peers == nil || *got.Peers != peers) {
		t.Errorf("%q: seeders %v peers %v, want %d %d", got.Title, got.Seeders, got.Peers, seeders, peers)
	}
}

func TestEverySiteHasASearch(t *testing.T) {
	for _, s := range Sites {
		switch s.ID {
		case "knaben", "torrentscsv", "subsplease", "iptorrents":
			continue
		}
		if moreSites[s.ID] == nil {
			t.Errorf("%s has no search", s.ID)
		}
	}
}

func TestThePirateBay(t *testing.T) {
	t.Parallel()
	var asked []string
	srv := siteServer(t, "thepiratebay.json", func(r *http.Request, _ string) { asked = append(asked, r.URL.RequestURI()) })
	items, err := siteClient(srv.URL, "thepiratebay", "").Search(context.Background(), Query{Title: "Severance", TV: true, Season: 1, Episode: "1"})
	items = one(t, items, err, 3)
	if len(asked) != 1 || asked[0] != "/q.php?cat=205%2C208%2C212&q=Severance+S01E01" {
		t.Fatalf("asked %q", asked)
	}
	wantItem(t, items[0], "Severance S01E01 Good News About Hell 1080p BluRay 10Bit DDP5 1 H265-d3g", "magnet:?xt=urn:btih:9353dfdad43e6f267e9d495a6838679c1ec15d8b&dn=Severance",
		2609557292, 113, 127, "9353dfdad43e6f267e9d495a6838679c1ec15d8b")
	if !items[0].Published.Equal(time.Unix(1735661076, 0)) || items[0].Files != 3 || items[0].GUID != "https://thepiratebay.org/description.php?id=77711156" {
		t.Errorf("item %+v", items[0])
	}
	if !strings.Contains(items[0].Link, "&tr=udp%3A%2F%2Ftracker.opentrackr.org") {
		t.Errorf("magnet has no trackers: %s", items[0].Link)
	}
}

func TestThePirateBay_MovieByIMDbThenWords(t *testing.T) {
	t.Parallel()
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Query().Get("q"))
		// The site says "no results" with one empty row.
		_, _ = w.Write([]byte(`[{"id":"0","name":"No results returned","info_hash":"0000000000000000000000000000000000000000","leechers":"0","seeders":"0","num_files":"0","size":"0","username":"","added":"0","status":"member","category":"0","imdb":""}]`))
	}))
	t.Cleanup(srv.Close)
	items, err := siteClient(srv.URL, "thepiratebay", "").Search(context.Background(), Query{Title: "The Descent", Year: 2005, IMDb: "tt0435625"})
	one(t, items, err, 0)
	if strings.Join(asked, "|") != "tt0435625|The Descent 2005" {
		t.Fatalf("asked %q", asked)
	}
}

// The recent list, which the Source check reads, writes its numbers as
// numbers, not strings as the search does.
func TestThePirateBay_RecentWithNumbers(t *testing.T) {
	t.Parallel()
	var asked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Path
		_, _ = w.Write([]byte(`[{"id":85103954,"info_hash":"78C9F19BEB1C21E6B9CF03D0B10686A35925ED2F","category":201,"name":"Steps (2026) iTA-ENG.WEBDL.1080p.x264-Dr4gon.mkv","status":"trusted","num_files":1,"size":2829072239,"seeders":7,"leechers":2,"username":"Dr4gon.","added":1795179505,"anon":0,"imdb":"tt14413964"}]`))
	}))
	t.Cleanup(srv.Close)
	items, err := siteClient(srv.URL, "thepiratebay", "").Search(context.Background(), Query{feed: true})
	items = one(t, items, err, 1)
	if asked != "/precompiled/data_top100_recent.json" {
		t.Fatalf("asked %q", asked)
	}
	wantItem(t, items[0], "Steps (2026) iTA-ENG.WEBDL.1080p.x264-Dr4gon.mkv", "magnet:?xt=urn:btih:78c9f19beb1c21e6b9cf03d0b10686a35925ed2f", 2829072239, 7, 9, "78c9f19beb1c21e6b9cf03d0b10686a35925ed2f")
	if items[0].Files != 1 || !items[0].Published.Equal(time.Unix(1795179505, 0)) || items[0].IMDb != "14413964" {
		t.Errorf("item %+v", items[0])
	}
}

func TestYTS(t *testing.T) {
	t.Parallel()
	srv := siteServer(t, "yts.json", func(r *http.Request, _ string) {
		if r.URL.Path != "/api/v2/list_movies.json" || r.URL.Query().Get("query_term") != "tt0435625" || r.URL.Query().Get("limit") != "50" {
			t.Errorf("asked %s", r.URL.RequestURI())
		}
	})
	items, err := siteClient(srv.URL, "yts", "").Search(context.Background(), Query{Title: "The Descent", Year: 2005, IMDb: "tt0435625"})
	items = one(t, items, err, 2)
	wantItem(t, items[0], "The Descent (2005) 720p BRRip x264 -YTS", "magnet:?xt=urn:btih:5a8f6f94cf7844439cb12747f331defb7de4bc5d", 962005565, 36, 49, "5a8f6f94cf7844439cb12747f331defb7de4bc5d")
	wantItem(t, items[1], "The Descent (2005) 1080p BRRip 5.1 x264 -YTS", "magnet:?xt=urn:btih:1b70c84bd49e7079b0347a6dd3045ebfaf0515e1", 1975684956, 100, 121, "1b70c84bd49e7079b0347a6dd3045ebfaf0515e1")
	if items[0].IMDb != "435625" || !items[0].Published.Equal(time.Unix(1620478315, 0)) {
		t.Errorf("item %+v", items[0])
	}
}

func TestYTS_TitleWithoutYear(t *testing.T) {
	t.Parallel()
	srv := siteServer(t, "yts.json", func(r *http.Request, _ string) {
		if r.URL.Query().Get("query_term") != "The Descent" {
			t.Errorf("asked %s", r.URL.RequestURI())
		}
	})
	_, err := siteClient(srv.URL, "yts", "").Search(context.Background(), Query{Title: "The Descent", Year: 2005})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBangumiMoe(t *testing.T) {
	t.Parallel()
	srv := siteServer(t, "bangumimoe.json", func(r *http.Request, body string) {
		var b map[string]string
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/torrent/search" || json.Unmarshal([]byte(body), &b) != nil || b["query"] != "Frieren S01" {
			t.Errorf("asked %s %s %s", r.Method, r.URL.Path, body)
		}
	})
	items, err := siteClient(srv.URL, "bangumimoe", "").Search(context.Background(), Query{Title: "Frieren", TV: true, Season: 1})
	items = one(t, items, err, 2)
	wantItem(t, items[0], "[7³ACG] 葬送的芙莉莲/Sousou no Frieren S01 | 01-28+SPx11 [简繁字幕] BDrip 1080p x265 OPUS 2.0", "magnet:?xt=urn:btih:f8651444a2c0b3ff7e3da3cbd75da58ef5f09e19",
		sz(52.02, 30), 0, 0, "f8651444a2c0b3ff7e3da3cbd75da58ef5f09e19")
	if items[0].GUID != srv.URL+"/torrent/6a90acf49605d5c690106883" || items[0].Published.Format(time.RFC3339) != "2026-08-27T21:32:36Z" {
		t.Errorf("item %+v", items[0])
	}
}

func TestBangumiMoe_Newest(t *testing.T) {
	t.Parallel()
	srv := siteServer(t, "bangumimoe.json", func(r *http.Request, _ string) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/torrent/latest" {
			t.Errorf("asked %s %s", r.Method, r.URL.Path)
		}
	})
	items, err := siteClient(srv.URL, "bangumimoe", "").Feed(context.Background(), nil)
	one(t, items, err, 2)
}

func TestNyaa(t *testing.T) {
	t.Parallel()
	srv := siteServer(t, "nyaasi.xml", func(r *http.Request, _ string) {
		q := r.URL.Query()
		if q.Get("page") != "rss" || q.Get("q") != "Frieren 05" || q.Get("c") != "0_0" || q.Get("f") != "0" {
			t.Errorf("asked %s", r.URL.RequestURI())
		}
	})
	items, err := siteClient(srv.URL, "nyaasi", "").Search(context.Background(), Query{Title: "Frieren 05", TV: true})
	items = one(t, items, err, 3)
	wantItem(t, items[0], "Frieren - Oltre la Fine del Viaggio (2023) [BluRay.1080p.x265.FLAC.ITA.JPN.Subs.Ita.Eng.Jpn] [42.6GB] [stress]",
		"https://nyaa.si/download/2180356.torrent", sz(42.6, 30), 5, 6, "444d6b7665bf616065d4c3793642bc41c17e767e")
	if items[0].GUID != "https://nyaa.si/view/2180356" || items[0].Published.Format(time.RFC3339) != "2026-10-30T15:32:00Z" {
		t.Errorf("item %+v", items[0])
	}
}

func TestTokyoToshokan(t *testing.T) {
	t.Parallel()
	srv := siteServer(t, "tokyotosho.xml", func(r *http.Request, _ string) {
		if r.URL.Path != "/rss.php" || r.URL.Query().Get("terms") != "Frieren" {
			t.Errorf("asked %s", r.URL.RequestURI())
		}
	})
	items, err := siteClient(srv.URL, "tokyotosho", "").Search(context.Background(), Query{Title: "Frieren", TV: true})
	items = one(t, items, err, 3)
	wantItem(t, items[0], "Lila Minx - Frieren.zip", "magnet:?xt=urn:btih:RVO3WDXD45Q3VY6YTNB2FLIR4HSRT3SN", sz(25.47, 20), -1, -1, "8d5dbb0ee3e761bae3d89b43a2ad11e1e519ee4d")
	if items[0].Seeders != nil || items[0].GUID != "https://www.tokyotosho.info/details.php?id=2148176" || items[0].Published.Format(time.RFC3339) != "2026-11-19T00:27:33Z" {
		t.Errorf("item %+v", items[0])
	}
}

func Test1337x(t *testing.T) {
	t.Parallel()
	srv := siteServer(t, "1337x.html", func(r *http.Request, _ string) {
		if r.URL.EscapedPath() != "/sort-category-search/Severance+S01E01/TV/seeders/desc/1/" {
			t.Errorf("asked %s", r.URL.EscapedPath())
		}
	})
	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	was := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = was })
	items, err := siteClient(srv.URL, "1337x", "").Search(context.Background(), Query{Title: "Severance", TV: true, Season: 1, Episode: "1"})
	items = one(t, items, err, 4)
	wantItem(t, items[0], "Young.Rock.S01E09.A.Lady.Named.Star.Search.HDTV.x264-CRiMSON[TGx]", srv.URL+"/torrent/4843796/Young-Rock-S01E09-A-Lady-Named-Star-Search-HDTV-x264-CRiMSON-TGx/",
		sz(172.2, 20), 66, 72, "")
	if items[0].Published.Format("2006-01-02") != "2021-04-21" {
		t.Errorf("date %v", items[0].Published)
	}
	// A cut-short name comes from the address; a time alone is today.
	wantItem(t, items[3], "Severance S01E01 Good News About Hell 1080p ATVP WEB DL DDP5 1 H 264 NTb", srv.URL+"/torrent/6012345/", sz(3.1, 30), 1204, 1292, "")
	if items[3].Published.Format(time.RFC3339) != "2026-11-20T09:40:00Z" {
		t.Errorf("date %v", items[3].Published)
	}
}

func TestDate1337x(t *testing.T) {
	at := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for in, want := range map[string]string{"Feb. 2nd": "2026-02-02", "Dec. 30th": "2025-12-30", "Dec. 30th '18": "2018-12-30", "11:05pm": "2026-03-01", "soon": "0001-01-01"} {
		if got := date1337x(in, at).Format("2006-01-02"); got != want {
			t.Errorf("%q: %s, want %s", in, got, want)
		}
	}
}

func TestTorrent9(t *testing.T) {
	t.Parallel()
	srv := siteServer(t, "torrent9.html", func(r *http.Request, _ string) {
		if r.URL.Path != "/search_torrent/severance-saison-2.html" {
			t.Errorf("asked %s", r.URL.Path)
		}
	})
	items, err := siteClient(srv.URL, "torrent9", "").Search(context.Background(), Query{Title: "Severance", TV: true, Season: 2})
	items = one(t, items, err, 3)
	wantItem(t, items[0], "Severance S02E02 2025 VOSTFR HDTV", srv.URL+"/torrent/123242/severance-s02e02-vostfr-hdtv-2025", sz(173.0, 20), 27, 53, "")
	if items[0].Published.Format("2006-01-02") != "2025-01-26" {
		t.Errorf("date %v", items[0].Published)
	}
}

func TestTorrent9_SeasonInFrench(t *testing.T) {
	page := `<table class="table-striped"><tbody><tr><td><i class="fa fa-tv"></i> <a title="Outlander.Saison.8.CUSTOM.MULTI.VFF.720p.WEBRip.AAC.2.0.x265-QC" href="/torrent/132070/outlander"></a></td>` +
		`<td>18/05/2026</td><td>5.3 Go</td><td>149</td><td>61</td></tr></tbody></table>`
	items := parseTorrent9([]byte(page), "https://t9")
	if len(items) != 1 || items[0].Title != "Outlander.S08.CUSTOM.MULTI.VFF.720p.WEBRip.AAC.2.0.x265-QC" || items[0].Size != sz(5.3, 30) {
		t.Fatalf("%+v", items)
	}
}

func TestTorrentDownload(t *testing.T) {
	t.Parallel()
	srv := siteServer(t, "torrentdownload.html", func(r *http.Request, _ string) {
		if r.URL.Path != "/search" || r.URL.Query().Get("q") != "Zoo Quest S03E05" {
			t.Errorf("asked %s", r.URL.RequestURI())
		}
	})
	items, err := siteClient(srv.URL, "torrentdownload", "").Search(context.Background(), Query{Title: "Zoo Quest", TV: true, Season: 3, Episode: "5"})
	// The sponsored rows at the top have no torrent and are left out.
	items = one(t, items, err, 1)
	wantItem(t, items[0], "Zoo Quest For A Dragon S03E05 Bali Part Two WEBRip MVGroup mkv", "magnet:?xt=urn:btih:772ac585685edf6528eefca9de1638a64bd52e3c",
		sz(317.46, 20), 0, 0, "772ac585685edf6528eefca9de1638a64bd52e3c")
	if items[0].Published.IsZero() || time.Since(items[0].Published) < 360*24*time.Hour {
		t.Errorf("a year+ old upload dated %v", items[0].Published)
	}
}

func TestAgo(t *testing.T) {
	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	for in, want := range map[string]string{"22 hours ago": "2026-11-19T14:00:00Z", "Yesterday": "2026-11-19T12:00:00Z", "Last Month": "2026-10-20T12:00:00Z",
		"1 Year+": "2025-11-20T12:00:00Z", "3 weeks ago": "2026-10-30T12:00:00Z", "a day ago": "2026-11-19T12:00:00Z"} {
		if got := ago(in, at).Format(time.RFC3339); got != want {
			t.Errorf("%q: %s, want %s", in, got, want)
		}
	}
	if !ago("whenever", at).IsZero() {
		t.Error("an unknown age got a date")
	}
}

func TestLimeTorrents(t *testing.T) {
	t.Parallel()
	srv := siteServer(t, "limetorrents.xml", func(r *http.Request, _ string) {
		if r.URL.EscapedPath() != "/searchrss/Zootopia%202016/" {
			t.Errorf("asked %s", r.URL.EscapedPath())
		}
	})
	items, err := siteClient(srv.URL, "limetorrents", "").Search(context.Background(), Query{Title: "Zootopia", Year: 2016})
	items = one(t, items, err, 3)
	wantItem(t, items[0], "Zootopia.2.2025.1080p.WEBRip.x265-KONTRAST", "magnet:?xt=urn:btih:56a196469bf004e7b3614c0d7aefac174971ab3f", 2654326354, 125, 160, "56a196469bf004e7b3614c0d7aefac174971ab3f")
	if items[0].Published.Format(time.RFC3339) != "2026-02-02T21:15:03Z" {
		t.Errorf("date %v", items[0].Published)
	}
}

func TestLimeTorrents_SeasonAloneSearchesTheTitle(t *testing.T) {
	t.Parallel()
	srv := siteServer(t, "limetorrents.xml", func(r *http.Request, _ string) {
		if r.URL.EscapedPath() != "/searchrss/Severance/" {
			t.Errorf("asked %s", r.URL.EscapedPath())
		}
	})
	if _, err := siteClient(srv.URL, "limetorrents", "").Search(context.Background(), Query{Title: "Severance", TV: true, Season: 2}); err != nil {
		t.Fatal(err)
	}
}

func TestBigFANGroup(t *testing.T) {
	t.Parallel()
	srv := siteServer(t, "bigfangroup.html", func(r *http.Request, _ string) {
		q := r.URL.Query()
		if r.URL.Path != "/browse.php" || q.Get("search") != "Severance" || q.Get("ajax") != "1" || q.Get("incldead") != "1" {
			t.Errorf("asked %s", r.URL.RequestURI())
		}
	})
	items, err := siteClient(srv.URL, "bigfangroup", "").Search(context.Background(), Query{Title: "Severance", TV: true, Season: 1, Episode: "3"})
	items = one(t, items, err, 3)
	wantItem(t, items[0], "Разделение / Severance, S1-2E1-19 of 19 (2022-2025) WEB-DL-LostFilm", srv.URL+"/download.php?id=407324", sz(11.69, 30), 12, 112, "")
	if items[0].GUID != srv.URL+"/details.php?id=407324" || items[0].Published.Format(time.RFC3339) != "2025-03-26T11:51:34Z" {
		t.Errorf("item %+v", items[0])
	}
	wantItem(t, items[2], "Корпоративка / Severance (2006) BDRip 720p", srv.URL+"/download.php?id=74601", sz(4.63, 30), 0, 1, "")
}

func TestNoFeedSitesOnlySearch(t *testing.T) {
	t.Parallel()
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	t.Cleanup(srv.Close)
	for _, id := range []string{"1337x", "torrent9", "torrentdownload", "limetorrents"} {
		items, err := siteClient(srv.URL, id, "").Feed(context.Background(), nil)
		if err != nil || len(items) != 0 {
			t.Errorf("%s feed: %v %v", id, items, err)
		}
	}
	if called {
		t.Error("a site with no newest list was called for one")
	}
	var src domain.Source
	if _, _, err := fill(Input{Name: "1337x", Format: "site", Site: "1337x"}, &src, true); err != nil || src.UseFor.Feed || !src.UseFor.AutoSearch {
		t.Fatalf("use for %+v, %v", src.UseFor, err)
	}
}

// pageServer serves a search page and the torrent's own page.
func pageServer(t *testing.T, page string) *httptest.Server {
	data, err := os.ReadFile("testdata/sites/" + page)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/torrent/") {
			_, _ = w.Write(data)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDetailsMagnetReadAtGrab(t *testing.T) {
	ctx := context.Background()
	s, v := openVault(t)
	for _, c := range []struct{ site, page, hash string }{
		{"1337x", "1337x_page.html", "magnet:?xt=urn:btih:846F85D1CD1A70E21AC48B55EF52770DBA5D374D&dn=The+Inbetweeners+2+2014+BRRip+XviD-PsiX&tr="},
		{"torrent9", "torrent9_page.html", "magnet:?xt=urn:btih:XY4D2DNPWGH4DWAADPUOFVITPAVF3POU&tr=udp://tracker.opentrackr.org:1337/announce&tr="},
	} {
		srv := pageServer(t, c.page)
		local := true
		src, err := Add(ctx, s, v, Input{Name: c.site, Format: "site", Site: c.site, URL: srv.URL, AllowLocal: &local}, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		got, err := GrabLink(ctx, s, v, domain.Release{SourceID: src.ID, LinkRef: srv.URL + "/torrent/1/x/"})
		if err != nil || !strings.HasPrefix(got, c.hash) {
			t.Errorf("%s: %q, %v", c.site, got, err)
		}
		// A page on another host is never read.
		if _, err := GrabLink(ctx, s, v, domain.Release{SourceID: src.ID, LinkRef: "https://elsewhere.example/torrent/1/x/"}); err == nil {
			t.Errorf("%s: read a page on another host", c.site)
		}
		// A page with no magnet says so.
		if _, err := GrabLink(ctx, s, v, domain.Release{SourceID: src.ID, LinkRef: srv.URL + "/nothing"}); err == nil {
			t.Errorf("%s: no error for a page without a magnet", c.site)
		}
	}
	// Other links go as they are, with no call.
	got, err := GrabLink(ctx, s, v, domain.Release{SourceID: 999, LinkRef: "https://nyaa.si/download/1.torrent"})
	if err != nil || got != "https://nyaa.si/download/1.torrent" {
		t.Fatalf("%q %v", got, err)
	}
}
