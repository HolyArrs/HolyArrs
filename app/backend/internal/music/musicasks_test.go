package music

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// newMusicAskRig is the ask rig with a music Library folder and the fake
// MusicBrainz of the music tests.
func newMusicAskRig(t *testing.T) *askRig {
	t.Helper()
	r := newAskRig(t)
	r.asks.Library.Artists = apiMusic()
	r.owner.call("POST", "/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"music","label":"Music"}`, t.TempDir()), 201)
	return r
}

// route: POST /api/v1/asks
// route: GET /api/v1/asks
// route: GET /api/v1/artists/lookup
// route: POST /api/v1/asks/{id}/approve
// route: POST /api/v1/asks/{id}/turn-down
// route: PUT /api/v1/settings/asks
// route: GET /api/v1/asks/limit
func TestMusicAskOverHTTP(t *testing.T) {
	t.Parallel()
	r := newMusicAskRig(t)

	// Bad music asks are refused with the reason.
	r.ana.call("POST", "/api/v1/asks", `{"kind":"artist","id":"Lab Band"}`, 400)
	r.ana.call("POST", "/api/v1/asks", `{"kind":"artist","id":"mbid:`+apiArtist+`","albums":["nope"]}`, 400)
	r.ana.call("POST", "/api/v1/asks", `{"kind":"artist","id":"mbid:`+apiArtist+`","albums":["10000000-0000-0000-0000-0000000000ff"]}`, 400)
	r.ana.call("POST", "/api/v1/asks", `{"kind":"artist","id":"mbid:`+apiArtist+`","seasons":[1]}`, 400)
	r.ana.call("POST", "/api/v1/asks", `{"kind":"show","id":"tmdb:1","albums":["`+apiAlbum+`"]}`, 400)
	r.ana.call("POST", "/api/v1/asks", `{"kind":"artist","id":"mbid:a0000000-0000-0000-0000-0000000000ff"}`, 404)

	// The ask dialog lists the albums to ask for, oldest first.
	var look struct {
		Artists []ArtistResult `json:"artists"`
	}
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/artists/lookup?term=mbid:"+apiArtist+"&albums=true", "", 200), &look)
	if len(look.Artists) != 1 || len(look.Artists[0].AskAlbums) != 3 || look.Artists[0].AskAlbums[0].Name != "First Light" || look.Artists[0].Ask != nil {
		t.Fatalf("lookup %+v", look)
	}

	a := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"kind":"artist","id":"mbid:`+apiArtist+`","albums":["`+apiLive+`"]}`, 201))
	if a.Kind != "artist" || a.State != "asked" || a.AlbumWords != "Live Night" || len(a.Albums) != 1 || a.Ref != "mbid:"+apiArtist || !a.Mine {
		t.Fatalf("ask %+v", a)
	}
	var joined struct {
		Ask    requests.View `json:"ask"`
		Joined bool          `json:"joined"`
	}
	_ = json.Unmarshal(r.bo.call("POST", "/api/v1/asks", `{"kind":"artist","id":"mbid:`+apiArtist+`","albums":["`+apiLive+`"]}`, 200), &joined)
	// A member sees everyone who asked.
	if !joined.Joined || joined.Ask.ID != a.ID || len(joined.Ask.People) != 2 {
		t.Fatalf("join %+v", joined)
	}
	look.Artists = nil
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/artists/lookup?term=mbid:"+apiArtist, "", 200), &look)
	if look.Artists[0].Ask == nil || !look.Artists[0].Ask.Mine || len(look.Artists[0].Ask.Albums) != 1 || look.Artists[0].AskAlbums != nil {
		t.Fatalf("lookup with ask %+v", look)
	}
	var list struct {
		Asks []requests.View `json:"asks"`
	}
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/asks?kind=artist", "", 200), &list)
	if len(list.Asks) != 1 {
		t.Fatalf("artist asks %+v", list)
	}
	var lim requests.Limit
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/asks/limit", "", 200), &lim)
	if lim.Music.Used != 1 || lim.Movies.Used != 0 || lim.Music.Left != requests.DefaultPerWeek-1 {
		t.Fatalf("limit %+v", lim)
	}

	// A member may not approve; the owner approves and the artist is added
	// with only the asked-for album kept up to date.
	r.ana.call("POST", fmt.Sprintf("/api/v1/asks/%d/approve", a.ID), `{}`, 403)
	got := askOf(t, r.owner.call("POST", fmt.Sprintf("/api/v1/asks/%d/approve", a.ID), `{"note":"good one"}`, 200))
	if got.State != "approved" || got.TitleID == nil || got.DecidedBy == nil || got.DecidedBy.Name != "luis" {
		t.Fatalf("approved %+v", got)
	}
	var albums struct {
		Albums []struct {
			Name string `json:"name"`
			Keep bool   `json:"keep_up_to_date"`
		} `json:"albums"`
	}
	_ = json.Unmarshal(r.owner.call("GET", fmt.Sprintf("/api/v1/titles/%d/albums", *got.TitleID), "", 200), &albums)
	keeps := map[string]bool{}
	for _, al := range albums.Albums {
		keeps[al.Name] = al.Keep
	}
	if !keeps["Live Night"] || keeps["First Light"] || keeps["Soon"] {
		t.Fatalf("keeps %v", keeps)
	}
	var title struct {
		Kind string          `json:"kind"`
		Asks []requests.View `json:"asks"`
	}
	_ = json.Unmarshal(r.owner.call("GET", fmt.Sprintf("/api/v1/titles/%d", *got.TitleID), "", 200), &title)
	if title.Kind != "artist" || len(title.Asks) != 1 || len(title.Asks[0].People) != 2 {
		t.Fatalf("title %+v", title)
	}
	evs, _, err := r.st.Events(context.Background(), database.EventFilter{Kinds: []string{"ask_*"}, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	var b string
	for _, e := range evs {
		b += e.Summary + " | "
	}
	for _, w := range []string{"ana asked for Lab Band, Live Night", "bo also asked for", "approved by luis"} {
		if !strings.Contains(b, w) {
			t.Fatalf("history lacks %q: %s", w, b)
		}
	}
	if m := string(r.bo.call("GET", "/api/v1/messages", "", 200)); !strings.Contains(m, "Lab Band, Live Night was approved by an admin") {
		t.Fatalf("messages %s", m)
	}
	// The lookup now says which albums are kept.
	look.Artists = nil
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/artists/lookup?term=mbid:"+apiArtist+"&albums=true", "", 200), &look)
	for _, al := range look.Artists[0].AskAlbums {
		if al.Kept != (al.Name == "Live Night") || al.LeftOut {
			t.Fatalf("album %+v", al)
		}
	}

	// Turning down an ask for another album tells the member.
	b2 := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"kind":"artist","id":"mbid:`+apiArtist+`","albums":["`+apiAlbum+`"]}`, 201))
	r.owner.call("POST", fmt.Sprintf("/api/v1/asks/%d/turn-down", b2.ID), `{"note":"not now"}`, 200)
	if m := string(r.ana.call("GET", "/api/v1/messages", "", 200)); !strings.Contains(m, "Lab Band, First Light was turned down by an admin") {
		t.Fatalf("messages %s", m)
	}

	// Settings: the music fields save; a non-music folder is refused.
	var set requests.Settings
	_ = json.Unmarshal(r.owner.call("PUT", "/api/v1/settings/asks", `{"music_per_week":0}`, 200), &set)
	if set.MusicPerWeek != 0 || set.ShowsPerWeek != requests.DefaultPerWeek {
		t.Fatalf("settings %+v", set)
	}
	r.owner.call("PUT", "/api/v1/settings/asks", `{"music_library_folder_id":1}`, 400)
	r.ana.call("PUT", "/api/v1/settings/asks", `{"music_per_week":3}`, 403)
}

// TestMemberLookupsAreCapped: a member has at most two artist lookups in
// flight (a third is refused with 429), member lookups wait behind staff
// at MusicBrainz, and staff lookups are not marked as members'.
func TestMemberLookupsAreCapped(t *testing.T) {
	t.Parallel()
	r := newMusicAskRig(t)
	fa := r.asks.Library.Artists.(*fakeArtists)
	fa.mu.Lock()
	fa.block, fa.entered = make(chan struct{}), make(chan struct{}, 4)
	fa.mu.Unlock()
	var wg sync.WaitGroup
	// Two different names (one name asked twice shares one search).
	for _, term := range []string{"lab", "band"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.ana.call("GET", "/api/v1/artists/lookup?term="+term, "", 200)
		}()
	}
	<-fa.entered
	<-fa.entered
	body := r.ana.call("GET", "/api/v1/artists/lookup?term=lab", "", 429)
	if !strings.Contains(string(body), "two music lookups") {
		t.Fatalf("429 words %s", body)
	}
	close(fa.block)
	wg.Wait()
	fa.mu.Lock()
	fa.block = nil
	fa.members = nil
	fa.mu.Unlock()
	// Names not searched yet (searched ones are answered from the store).
	r.ana.call("GET", "/api/v1/artists/lookup?term=lab+one", "", 200)
	r.owner.call("GET", "/api/v1/artists/lookup?term=lab+two", "", 200)
	fa.mu.Lock()
	defer fa.mu.Unlock()
	if len(fa.members) != 2 || !fa.members[0] || fa.members[1] {
		t.Fatalf("member marks %v", fa.members)
	}
}

// TestArtistLookupListsEveryOpenAsk: with an approved ask and a waiting one
// for the same artist, the lookup lists both, and ask is the member's own.
func TestArtistLookupListsEveryOpenAsk(t *testing.T) {
	t.Parallel()
	r := newMusicAskRig(t)
	first := askOf(t, r.bo.call("POST", "/api/v1/asks", `{"kind":"artist","id":"mbid:`+apiArtist+`","albums":["`+apiLive+`"]}`, 201))
	r.owner.call("POST", fmt.Sprintf("/api/v1/asks/%d/approve", first.ID), `{}`, 200)
	second := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"kind":"artist","id":"mbid:`+apiArtist+`","albums":["`+apiAlbum+`"]}`, 201))
	var look struct {
		Artists []ArtistResult `json:"artists"`
	}
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/artists/lookup?term=mbid:"+apiArtist, "", 200), &look)
	got := look.Artists[0]
	if len(got.Asks) != 2 || got.Asks[0].ID != first.ID || got.Asks[1].ID != second.ID || got.Ask == nil || got.Ask.ID != second.ID || !got.Ask.Mine {
		t.Fatalf("asks %+v ask %+v", got.Asks, got.Ask)
	}
}

// Home's "Waiting for approval" shelf shows artist asks with a picture.
func TestArtistAskListHasPicture(t *testing.T) {
	t.Parallel()
	r := newMusicAskRig(t)
	r.ana.call("POST", "/api/v1/asks", `{"kind":"artist","id":"mbid:`+apiArtist+`"}`, 201)
	var out struct {
		Asks []requests.View `json:"asks"`
	}
	if err := json.Unmarshal(r.owner.call("GET", "/api/v1/asks?kind=artist", "", 200), &out); err != nil || len(out.Asks) != 1 {
		t.Fatalf("asks %+v %v", out, err)
	}
	v := out.Asks[0]
	if v.ArtistID != apiArtist || v.Poster != "/api/v1/music-trending/art/artist/"+apiArtist {
		t.Fatalf("artist ask %+v", v)
	}
	r.api.music.mu.Lock()
	_, known := r.api.music.names["artist|"+apiArtist]
	r.api.music.mu.Unlock()
	if !known {
		t.Fatal("the picture route does not know the asked artist")
	}
}
