package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

var musicNewNow = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

func tmbid(n int) string { return fmt.Sprintf("00000000-0000-0000-0000-%012d", n) }

func rel(n int, kind, date string, artist int) metadata.NewRelease {
	return metadata.NewRelease{MBID: tmbid(n), Name: fmt.Sprint("R", n), PrimaryType: kind, ReleaseDate: date, Artist: fmt.Sprint("A", artist), ArtistMBID: tmbid(9000 + artist)}
}

func nday(n int) string { return musicNewNow.AddDate(0, 0, n).Format("2006-01-02") }

func TestMusicNewWindows(t *testing.T) {
	for _, c := range []struct {
		d    int
		want string
	}{{0, "week"}, {-7, "week"}, {-8, "recent"}, {-45, "recent"}, {-46, "library"}, {-91, ""}, {3, "coming"}, {61, ""}} {
		if got := musicNewWindow(nday(c.d), musicNewNow); got != c.want {
			t.Errorf("%d days: %q, want %q", c.d, got, c.want)
		}
	}
	if musicNewWindow("2026", musicNewNow) != "" {
		t.Error("a year-only date has a window")
	}
}

// Newer beats older at equal popularity; listens and a known artist lift a release.
func TestMusicNewScore(t *testing.T) {
	a := musicNewScore(MusicCard{Date: nday(-1)}, 0, musicNewNow)
	b := musicNewScore(MusicCard{Date: nday(-20)}, 0, musicNewNow)
	c := musicNewScore(MusicCard{Date: nday(-20), Listens: 100000}, 1, musicNewNow)
	if !(a > b && c > a) || musicNewScore(MusicCard{Date: nday(5)}, 3, musicNewNow) != 0 {
		t.Fatalf("scores %v %v %v", a, b, c)
	}
}

func musicNewFixture() musicNewInput {
	d := &musicNewData{at: musicNewNow}
	n := 1
	add := func(count int, kind string, from int) {
		for i := 0; i < count; i++ {
			d.releases = append(d.releases, rel(n, kind, nday(from-i%3), n%5))
			n++
		}
	}
	add(30, "album", -2)  // this week
	add(10, "ep", -15)    // recently out
	add(8, "single", -4)  // fresh tracks
	add(7, "album", 20)   // coming
	add(3, "album", -400) // too old
	live := rel(500, "album", nday(-1), 1)
	live.SecondaryTypes = []string{"live"}
	d.releases = append(d.releases, live)
	for i := 0; i < 8; i++ {
		d.albums.Items = append(d.albums.Items, metadata.MusicTrendItem{Rank: i + 1, MBID: tmbid(i + 1), Title: fmt.Sprint("R", i+1), Artist: "A", ArtistMBID: tmbid(9001), Listens: int64(1000 - i)})
		d.albums.Items = append(d.albums.Items, metadata.MusicTrendItem{Rank: i + 9, MBID: tmbid(700 + i), Title: fmt.Sprint("C", i), Artist: "B", ArtistMBID: tmbid(9002), Listens: 10})
	}
	d.artists.Items = []metadata.MusicTrendItem{{Rank: 1, MBID: tmbid(9001)}}
	return musicNewInput{data: d, owned: map[string]int64{tmbid(9003): 7}, images: map[string]string{}, follows: map[string]bool{tmbid(50): true}}
}

// Rows keep to their windows, nothing shows twice, small rows go.
func TestComposeMusicNew(t *testing.T) {
	hero, rows := composeMusicNew(musicNewFixture(), musicNewNow)
	if len(hero) < 3 || len(hero) > musicNewHeroMax {
		t.Fatalf("hero has %d", len(hero))
	}
	seen := map[string]string{}
	for _, c := range hero {
		seen[c.MBID] = "hero"
		if c.Badge != "New album" {
			t.Fatalf("hero badge %q", c.Badge)
		}
	}
	got := map[string]MusicRow{}
	for _, r := range rows {
		got[r.ID] = r
		if len(r.Items) < musicNewRowMin || len(r.Items) > musicNewRowMax {
			t.Fatalf("%s has %d", r.ID, len(r.Items))
		}
		for _, c := range r.Items {
			if where, ok := seen[c.MBID]; ok {
				t.Fatalf("%s in %s and %s", c.MBID, where, r.ID)
			}
			seen[c.MBID] = r.ID
			if c.MBID == tmbid(500) {
				t.Fatal("a live album shows")
			}
			w := musicNewWindow(c.Date, musicNewNow)
			switch r.ID {
			case "np_music_week":
				if w != "week" {
					t.Fatalf("week row has %s", c.Date)
				}
			case "np_music_recent":
				if w != "recent" {
					t.Fatalf("recent row has %s", c.Date)
				}
			case "np_music_coming":
				if !c.Upcoming || c.Badge != "Album coming" {
					t.Fatalf("coming row card %+v", c)
				}
			}
		}
	}
	for _, id := range []string{"np_music_week", "np_music_recent", "np_music_tracks", "np_music_coming", "np_music_climbing"} {
		if _, ok := got[id]; !ok {
			t.Errorf("no %s row (rows %v)", id, len(rows))
		}
	}
	// Coming soon: soonest first; a followed album says so.
	c := got["np_music_coming"].Items
	for i := 1; i < len(c); i++ {
		if c[i].Date < c[i-1].Date {
			t.Fatal("coming soon is not soonest first")
		}
	}
	following := false
	for _, x := range c {
		following = following || x.Following
	}
	if !following {
		t.Fatal("the followed album is not marked")
	}
	// The Library row: only Library artists' releases (one artist has 7 here).
	if r, ok := got["np_music_library"]; ok {
		for _, x := range r.Items {
			if x.TitleID != 7 {
				t.Fatalf("library row has %+v", x)
			}
		}
	}
}

// Nothing built yet: no hero and no rows, not a failure.
func TestComposeMusicNewEmpty(t *testing.T) {
	hero, rows := composeMusicNew(musicNewInput{owned: map[string]int64{}, follows: map[string]bool{}}, musicNewNow)
	if len(hero) != 0 || len(rows) != 0 {
		t.Fatal("rows from nothing")
	}
}

// route: GET /api/v1/explore/new/music
// The Music tab answers building first, then its rows; a Kid gets none.
func TestExploreNewMusicRoute(t *testing.T) {
	_, srv, _ := newRig(t)
	musicNew = musicNewStore{}
	body := getJSON(t, srv.URL+"/api/v1/explore/new/music", 200)
	if _, ok := body["rows"].([]any); !ok {
		t.Fatalf("answer %v", body)
	}
	for i := 0; i < 100 && body["building"] == true; i++ {
		time.Sleep(20 * time.Millisecond)
		body = getJSON(t, srv.URL+"/api/v1/explore/new/music", 200)
	}
	if body["building"] == true || body["kid"] != false {
		t.Fatalf("answer %v", body)
	}
	k := newKidRig(t)
	if code, b := k.kai.get("/api/v1/explore/new/music"); code == 200 && !strings.Contains(b, `"kid":true`) {
		t.Fatalf("kid answer %s", b)
	}
}

// route: GET /api/v1/metadata/album/{mbid}/follow
// route: PUT /api/v1/metadata/album/{mbid}/follow
// route: DELETE /api/v1/metadata/album/{mbid}/follow
// An album not out yet can be followed and unfollowed; out ones cannot;
// on its day the follow ends with a message.
func TestAlbumFollow(t *testing.T) {
	a, srv, _ := newRig(t)
	musicNew = musicNewStore{}
	u := srv.URL + "/api/v1/metadata/album/" + tmbid(42) + "/follow"
	send := func(method, body string) int {
		req, _ := http.NewRequest(method, u, strings.NewReader(body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	ahead := time.Now().UTC().AddDate(0, 0, 10).Format("2006-01-02")
	if c := send("PUT", `{"name":"Soon","artist":"X","release_date":"`+ahead+`"}`); c != 200 {
		t.Fatalf("follow %d", c)
	}
	if getJSON(t, u, 200)["following"] != true {
		t.Fatal("not following")
	}
	if c := send("PUT", `{"name":"Old","artist":"X","release_date":"2020-01-01"}`); c != 409 {
		t.Fatalf("out album %d", c)
	}
	if c := send("PUT", `{}`); c != 400 {
		t.Fatalf("empty body %d", c)
	}
	getJSON(t, srv.URL+"/api/v1/metadata/album/nope/follow", 400)
	// Its day comes: a message, and the follow ends.
	a.landAlbumFollows(t.Context(), time.Now().UTC().AddDate(0, 0, 11))
	var n int
	if err := a.s.Store.Read().QueryRow("SELECT count(*) FROM messages WHERE text LIKE 'Soon by X is out%'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("messages %d %v", n, err)
	}
	if getJSON(t, u, 200)["following"] != false {
		t.Fatal("still following after it landed")
	}
	send("PUT", `{"name":"Soon","artist":"X","release_date":"`+ahead+`"}`)
	if c := send("DELETE", ""); c != 200 || getJSON(t, u, 200)["following"] != false {
		t.Fatal("unfollow")
	}
}

// The metadata server's windows add releases, the explicit mark, Deezer
// popularity, and feed Fresh tracks from the track chart.
func TestComposeMusicNewFromServer(t *testing.T) {
	in := musicNewFixture()
	srv := map[string][]metadata.MusicNewItem{}
	for i := 0; i < 8; i++ {
		srv["tracks"] = append(srv["tracks"], metadata.MusicNewItem{MBID: tmbid(800 + i), Title: fmt.Sprint("Album", i), Artist: "Dz", Track: fmt.Sprint("Song", i), ReleaseDate: nday(-3), Explicit: i == 0, Popularity: i + 1})
	}
	srv["week"] = []metadata.MusicNewItem{{MBID: tmbid(900), Title: "Deezer Pick", Artist: "Dz", ArtistMBID: tmbid(9100), ReleaseDate: nday(-1), Type: "album", Explicit: true, Popularity: 1}}
	srv["upcoming"] = []metadata.MusicNewItem{{MBID: tmbid(901), Title: "Soon", Artist: "Dz", ReleaseDate: nday(10), Type: "ep"}, {MBID: tmbid(902), Title: "Too far", Artist: "Dz", ReleaseDate: nday(80), Type: "album"}}
	in.data.server = srv
	hero, rows := composeMusicNew(in, musicNewNow)
	all := map[string]MusicCard{}
	for _, c := range hero {
		all[c.MBID] = c
	}
	var tracks MusicRow
	for _, r := range rows {
		if r.ID == "np_music_tracks" {
			tracks = r
			continue
		}
		for _, c := range r.Items {
			all[c.MBID] = c
		}
	}
	if p, ok := all[tmbid(900)]; !ok || !p.Explicit || p.Badge != "New album" {
		t.Fatalf("server pick %+v", p)
	}
	inHero := false
	for _, c := range hero {
		inHero = inHero || c.MBID == tmbid(900)
	}
	if !inHero {
		t.Fatal("the week's top Deezer pick is not in the hero")
	}
	if _, ok := all[tmbid(902)]; ok {
		t.Fatal("a release past the Coming soon window shows")
	}
	if len(tracks.Items) != 8 || tracks.Items[0].Name != "Song0" || tracks.Items[0].Track != "Song0" || !tracks.Items[0].Explicit || tracks.Items[0].MBID != tmbid(800) || tracks.Items[0].Note != "From Album0" {
		t.Fatalf("tracks %+v", tracks.Items)
	}
}
