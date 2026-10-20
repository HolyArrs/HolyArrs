package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
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
