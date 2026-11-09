package main

import (
	"encoding/json"
	"testing"
)

type reviewsView struct {
	Source  string `json:"source"`
	Sources []struct {
		ID     string   `json:"id"`
		Orders []string `json:"orders"`
	} `json:"sources"`
	Order        string  `json:"order"`
	Score        float64 `json:"score"`
	Votes        int     `json:"votes"`
	Distribution []int   `json:"distribution"`
	Watching     *int    `json:"watching"`
	Reviews      []struct {
		Author string `json:"author"`
		Text   string `json:"text"`
	} `json:"reviews"`
	Pages  int  `json:"pages"`
	Hidden bool `json:"reviews_hidden"`
}

// A Kid keeps the score but gets no reviews; a title above the limit is 403.
func TestKidReviewsHidden(t *testing.T) {
	t.Parallel()
	k := newKidRig(t)
	code, body := k.kai.get("/api/v1/metadata/movie/4101/reviews")
	var g reviewsView
	if code != 200 || json.Unmarshal([]byte(body), &g) != nil || !g.Hidden || len(g.Reviews) != 0 {
		t.Fatalf("kid %d %s", code, body)
	}
	if code, _ := k.kai.get("/api/v1/metadata/movie/4204/reviews"); code != 403 {
		t.Fatalf("kid above limit %d", code)
	}
}
