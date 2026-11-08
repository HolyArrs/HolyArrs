package main

import (
	"encoding/json"
	"testing"
	"time"
)

type calendarArt struct {
	Dates []struct {
		Kind        string `json:"kind"`
		Name        string `json:"name"`
		EpisodeName string `json:"episode_name"`
		Poster      string `json:"poster"`
		Backdrop    string `json:"backdrop"`
		Still       string `json:"still"`
	} `json:"dates"`
}

// Calendar entries carry the title's poster and backdrop and, for an
// episode with one, its still, so Home can show pictures.
func TestCalendarEntriesCarryArtwork(t *testing.T) {
	t.Parallel()
	srv, st, _ := libSetup(t)
	folder(t, srv.URL, "movie")
	a := addMovie(t, srv.URL, `{"id":"tmdb:900001"}`)
	// The add starts a background artwork refresh that can land after this
	// UPDATE and clear the backdrop, so set and read again until it holds.
	var c calendarArt
	for i := 0; i < 30; i++ {
		execRow(t, st, `UPDATE titles SET artwork = '["poster:lantern.jpg"]', backdrop = 'https://img.example/lantern-wide.jpg' WHERE id = ?`, a.Title.ID)
		c = calendarArt{}
		_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/calendar?from=2031-01-01&to=2031-03-01", "", 200), &c)
		if len(c.Dates) > 0 && c.Dates[0].Backdrop != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(c.Dates) == 0 || c.Dates[0].Poster != "/api/v1/artwork/lantern.jpg" || c.Dates[0].Backdrop != "https://img.example/lantern-wide.jpg" || c.Dates[0].Still != "" {
		t.Fatalf("movie entry %+v", c.Dates)
	}

	ssrv, sst, _, _ := showSetup(t)
	var add addAnswer
	_ = json.Unmarshal(do(t, "POST", ssrv.URL+"/api/v1/shows", `{"id":"tmdb:100"}`, 201), &add)
	execRow(t, sst, `UPDATE titles SET artwork = '["poster:lab.jpg"]', backdrop = 'https://img.example/lab-wide.jpg' WHERE id = ?`, add.Title.ID)
	execRow(t, sst, `UPDATE items SET still = 'https://img.example/pilot.jpg' WHERE title_id = ? AND name = 'Pilot'`, add.Title.ID)
	c = calendarArt{}
	_ = json.Unmarshal(do(t, "GET", ssrv.URL+"/api/v1/calendar?from=2025-01-01&to=2025-02-01", "", 200), &c)
	if len(c.Dates) != 2 {
		t.Fatalf("episodes %+v", c.Dates)
	}
	for _, e := range c.Dates {
		want := ""
		if e.EpisodeName == "Pilot" {
			want = "https://img.example/pilot.jpg"
		}
		if e.Kind != "episode" || e.Poster != "/api/v1/artwork/lab.jpg" || e.Backdrop != "https://img.example/lab-wide.jpg" || e.Still != want {
			t.Fatalf("episode entry %+v", e)
		}
	}
}
