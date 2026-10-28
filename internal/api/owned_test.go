package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// route: GET /api/v1/titles/owned
func TestTitlesOwned(t *testing.T) {
	srv, _, _ := setup(t)
	do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134"}`, 201)
	var got struct {
		Items []OwnedItem `json:"items"`
	}
	if err := json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/titles/owned", "", 200), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].Kind != "movie" || got.Items[0].TMDB != "693134" || got.Items[0].ID == 0 || got.Items[0].State != "wanted" {
		t.Fatalf("owned %+v", got)
	}
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/titles/owned", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	tag := resp.Header.Get("ETag")
	if tag == "" || resp.Header.Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("headers %v", resp.Header)
	}
	req.Header.Set("If-None-Match", tag)
	resp, err = srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotModified {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

// The SQL prefilter never drops a row foldMatch keeps.
func TestFoldPrefilterKeepsMatches(t *testing.T) {
	_, st, _ := setup(t)
	db := st.Read()
	for _, c := range []struct {
		name, q string
	}{{"Amélie", "amel"}, {"The Dark Knight", "dark kni"}, {"Ünïcode Song", "unicode"}, {"AC/DC Live", "ac dc"}} {
		words := foldWords(c.q)
		pre, args := foldPrefilter("n", words)
		var n int
		if err := db.QueryRow("SELECT count(*) FROM (SELECT ? AS n) WHERE 1 = 1"+pre, append([]any{c.name}, args...)...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if foldMatch(c.name, words) != (n == 1) {
			t.Fatalf("%q %q: sql %d", c.name, c.q, n)
		}
	}
}

// Kept outside answers survive a restart (they live in SQLite).
func TestKeptOnDisk(t *testing.T) {
	_, st, _ := setup(t)
	a := New(Server{Store: st, Version: "test"})
	keepOnDisk(context.Background(), a, "extras|movie:1", TitleExtras{Logo: "x", Scores: []OutsideScore{}})
	b := New(Server{Store: st, Version: "test"})
	k, ok := loadKept[TitleExtras](context.Background(), b, "extras|movie:1")
	if !ok || k.v.Logo != "x" || time.Since(k.at) > time.Minute {
		t.Fatalf("%+v %v", k, ok)
	}
}
