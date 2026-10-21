package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// route: PUT /api/v1/titles/{id}/follow
// route: DELETE /api/v1/titles/{id}/follow
// route: GET /api/v1/titles/{id}/follow
// route: GET /api/v1/my/follows
func TestFollowOverHTTP(t *testing.T) {
	r := newShowAskRig(t)
	tid, _, err := r.asks.Library.AddShow(context.Background(), "tmdb:100", library.ShowAddOptions{Seasons: "all"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/titles/" + itoa(tid) + "/follow"
	state := func(s *signedIn) bool {
		var x struct {
			Following bool `json:"following"`
		}
		_ = json.Unmarshal(s.call("GET", path, "", 200), &x)
		return x.Following
	}
	if state(r.ana) {
		t.Fatal("following before")
	}
	r.ana.call("PUT", path, "", 200)
	if !state(r.ana) || state(r.bo) {
		t.Fatal("follow is per person")
	}
	var list struct {
		Follows []struct {
			TitleID int64  `json:"title_id"`
			Name    string `json:"name"`
		} `json:"follows"`
	}
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/my/follows", "", 200), &list)
	if len(list.Follows) != 1 || list.Follows[0].TitleID != tid {
		t.Fatalf("list %+v", list)
	}
	r.ana.call("DELETE", path, "", 200)
	if state(r.ana) {
		t.Fatal("still following")
	}
	r.ana.call("PUT", "/api/v1/titles/999999/follow", "", 404)
}

// route: PUT /api/v1/metadata/show/{tmdb_id}/follow
// route: DELETE /api/v1/metadata/show/{tmdb_id}/follow
// route: GET /api/v1/metadata/show/{tmdb_id}/follow
func TestFollowShowByTMDBOverHTTP(t *testing.T) {
	r := newShowAskRig(t)
	path := "/api/v1/metadata/show/100/follow"
	state := func(s *signedIn) bool {
		var x struct {
			Following bool `json:"following"`
		}
		_ = json.Unmarshal(s.call("GET", path, "", 200), &x)
		return x.Following
	}
	if state(r.ana) {
		t.Fatal("following before")
	}
	r.ana.call("PUT", path, "", 200)
	if !state(r.ana) || state(r.bo) {
		t.Fatal("a TMDB follow is per person")
	}
	r.ana.call("PUT", "/api/v1/metadata/show/abc/follow", "", 400)

	// The show lands: the follow moves to the Library title and ana hears.
	tid, _, err := r.asks.Library.AddShow(context.Background(), "tmdb:100", library.ShowAddOptions{Seasons: "all"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := r.asks.LandTMDBFollows(context.Background(), tid); err != nil || n != 1 {
		t.Fatalf("landed %d %v", n, err)
	}
	var x struct {
		Following bool `json:"following"`
	}
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/titles/"+itoa(tid)+"/follow", "", 200), &x)
	if !x.Following || !state(r.ana) {
		t.Fatal("follow did not move to the Library title")
	}
	var msgs int
	if err := r.asks.Store.Read().QueryRow("SELECT count(*) FROM messages WHERE title_id = ? AND text LIKE '% is in the Library.'", tid).Scan(&msgs); err != nil || msgs != 1 {
		t.Fatalf("messages %d %v", msgs, err)
	}
	r.ana.call("DELETE", path, "", 200)
	if state(r.ana) {
		t.Fatal("still following")
	}
}

// route: PUT /api/v1/metadata/movie/{tmdb_id}/follow
// route: DELETE /api/v1/metadata/movie/{tmdb_id}/follow
// route: GET /api/v1/metadata/movie/{tmdb_id}/follow
func TestFollowMovieByTMDBOverHTTP(t *testing.T) {
	r := newAskRig(t)
	path := "/api/v1/metadata/movie/900005/follow"
	state := func(s *signedIn) bool {
		var x struct {
			Following bool `json:"following"`
		}
		_ = json.Unmarshal(s.call("GET", path, "", 200), &x)
		return x.Following
	}
	if state(r.ana) {
		t.Fatal("following before")
	}
	r.ana.call("PUT", path, "", 200)
	if !state(r.ana) {
		t.Fatal("not following")
	}
	r.ana.call("PUT", "/api/v1/metadata/movie/abc/follow", "", 400)
	r.ana.call("DELETE", path, "", 200)
	if state(r.ana) {
		t.Fatal("still following")
	}
	r.ana.call("PUT", path, "", 200)
	tid, _, err := r.asks.Library.Add(context.Background(), metadata.Movie{Name: "Quiet Orchard", Year: 2022, IDs: map[string]string{"tmdb": "900005"}},
		library.AddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := r.asks.LandTMDBFollows(context.Background(), tid); err != nil || n != 1 {
		t.Fatalf("landed %d %v", n, err)
	}
	if state(r.ana) {
		t.Fatal("the follow should end once the movie lands")
	}
	r.ana.call("PUT", path, "", 409)
}
