package main

import (
	"context"
	"encoding/json"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// route: PUT /api/v1/titles/{id}/follow
// route: DELETE /api/v1/titles/{id}/follow
// route: GET /api/v1/titles/{id}/follow
// route: GET /api/v1/my/follows
func TestFollowOverHTTP(t *testing.T) {
	t.Parallel()
	r := newShowAskRig(t)
	tid, _, err := shows.New(r.asks.Library).AddShow(context.Background(), "tmdb:100", shows.ShowAddOptions{Seasons: "all"}, domain.System)
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
	t.Parallel()
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
	tid, _, err := shows.New(r.asks.Library).AddShow(context.Background(), "tmdb:100", shows.ShowAddOptions{Seasons: "all"}, domain.System)
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
	t.Parallel()
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
	if !state(r.ana) {
		t.Fatal("the follow should go on as the Library title once the movie lands")
	}
	r.ana.call("DELETE", path, "", 200)
	if state(r.ana) {
		t.Fatal("unfollow by TMDB id ends the Library follow")
	}
	r.ana.call("PUT", path, "", 200) // a movie in the Library is followed as its title
	if !state(r.ana) {
		t.Fatal("not following the Library movie")
	}
}

// route: POST /api/v1/asks
// route: POST /api/v1/titles
// Asking follows by default (once approved; the asker, not the approver);
// "follow": false leaves it off. Adding by hand follows the adder.
func TestAskAndAddFollowByDefault(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	ctx := context.Background()
	a := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:900002"}`, 201))
	r.bo.call("POST", "/api/v1/asks", `{"id":"tmdb:900002","follow":false}`, 200)
	got := askOf(t, r.owner.call("POST", "/api/v1/asks/"+itoa(a.ID)+"/approve", `{}`, 200))
	if got.TitleID == nil {
		t.Fatalf("approved ask has no title: %+v", got)
	}
	tid := *got.TitleID
	if on, _ := r.asks.IsFollowing(ctx, r.ana.id, tid); !on {
		t.Fatal("the asker follows")
	}
	if on, _ := r.asks.IsFollowing(ctx, r.bo.id, tid); on {
		t.Fatal("follow false: no follow")
	}
	if on, _ := r.asks.IsFollowing(ctx, 1, tid); on {
		t.Fatal("the approver does not follow")
	}
	var add struct {
		Title struct {
			ID int64 `json:"id"`
		} `json:"title"`
	}
	_ = json.Unmarshal(r.owner.call("POST", "/api/v1/titles", `{"id":"tmdb:900003"}`, 201), &add)
	if on, _ := r.asks.IsFollowing(ctx, 1, add.Title.ID); !on {
		t.Fatal("adding by hand follows")
	}
}
