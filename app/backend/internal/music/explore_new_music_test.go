package music

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func tmbid(n int) string { return fmt.Sprintf("00000000-0000-0000-0000-%012d", n) }

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
