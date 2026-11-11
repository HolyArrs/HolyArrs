package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// route: GET /api/v1/shows/lookup
// route: POST /api/v1/shows
// route: GET /api/v1/titles/{id}/seasons
// route: PUT /api/v1/titles/{id}/seasons/{number}
// route: GET /api/v1/titles/{id}/episodes
// route: PUT /api/v1/episodes/{id}
func TestShowsAPI(t *testing.T) {
	t.Parallel()
	srv, _, _, _ := showSetup(t)
	u := srv.URL + "/api/v1"

	do(t, "GET", u+"/shows/lookup", "", 400)
	do(t, "GET", u+"/shows/lookup?term=lab&source=musicbrainz", "", 400)
	do(t, "GET", u+"/shows/lookup?term=lab&source=tvdb", "", 400)
	var look struct {
		Shows  []shows.ShowResult `json:"shows"`
		Source string             `json:"source"`
	}
	_ = json.Unmarshal(do(t, "GET", u+"/shows/lookup?term=lab", "", 200), &look)
	if look.Source != "tmdb" || len(look.Shows) != 1 || look.Shows[0].IDs["tmdb"] != "100" || look.Shows[0].TitleID != 0 || look.Shows[0].Episodes != nil {
		t.Fatalf("lookup %+v", look)
	}

	do(t, "POST", u+"/shows", `{}`, 400)
	do(t, "POST", u+"/shows", `{"id":"tmdb:100","seasons":"pilot"}`, 400)
	do(t, "POST", u+"/shows", `{"id":"tmdb:999"}`, 404)
	do(t, "POST", u+"/shows", `{"id":"tvdb:200","details_source":"tvdb"}`, 400)
	var add addedShow
	_ = json.Unmarshal(do(t, "POST", u+"/shows", `{"id":"tmdb:100","seasons":"latest","new_seasons":"none"}`, 201), &add)
	sv := add.Title.Show
	if add.Title.Kind != "show" || add.JobID == 0 || len(add.Title.Slots) != 0 || sv.DetailsSource != "tmdb" || sv.EpisodeOrder != "aired" ||
		sv.SeasonChoice != "latest" || sv.NewSeasons != "none" || sv.Episodes != 4 || len(sv.Seasons) != 3 || sv.NotOutYet != 2 {
		t.Fatalf("added %+v", add)
	}
	id := add.Title.ID
	if b := string(do(t, "POST", u+"/shows", `{"id":"tvdb:200"}`, 409)); !strings.Contains(b, fmt.Sprintf(`"title_id":%d`, id)) {
		t.Fatalf("second add: %s", b)
	}
	_ = json.Unmarshal(do(t, "GET", u+"/shows/lookup?term=tmdb:100", "", 200), &look)
	if len(look.Shows) != 1 || look.Shows[0].TitleID != id {
		t.Fatalf("lookup by id after add %+v", look)
	}

	var seasons struct {
		Seasons []shows.SeasonSummary `json:"seasons"`
	}
	_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/titles/%d/seasons", u, id), "", 200), &seasons)
	if len(seasons.Seasons) != 3 || seasons.Seasons[1].KeepUpToDate || !seasons.Seasons[2].KeepUpToDate || seasons.Seasons[1].Aired != 2 {
		t.Fatalf("seasons %+v", seasons)
	}
	var one shows.SeasonSummary
	_ = json.Unmarshal(do(t, "PUT", fmt.Sprintf("%s/titles/%d/seasons/1", u, id), `{"keep_up_to_date":true}`, 200), &one)
	if !one.KeepUpToDate || one.Missing != 2 {
		t.Fatalf("season 1 on %+v", one)
	}
	do(t, "PUT", fmt.Sprintf("%s/titles/%d/seasons/1", u, id), `{}`, 400)
	do(t, "PUT", fmt.Sprintf("%s/titles/%d/seasons/9", u, id), `{"keep_up_to_date":true}`, 404)

	var eps struct {
		Episodes []EpisodeView `json:"episodes"`
		Order    string        `json:"episode_order"`
	}
	_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/titles/%d/episodes?season=1", u, id), "", 200), &eps)
	if len(eps.Episodes) != 2 || eps.Order != "aired" || eps.Episodes[0].Name != "Pilot" || len(eps.Episodes[0].Slots) != 1 ||
		eps.Episodes[0].Slots[0].State != "wanted" || eps.Episodes[0].Slots[0].StateLabel != "Missing" || eps.Episodes[0].IDs["tmdb"] != "e101" {
		t.Fatalf("episodes %+v", eps)
	}
	do(t, "GET", fmt.Sprintf("%s/titles/%d/episodes?season=x", u, id), "", 400)
	var ev EpisodeView
	_ = json.Unmarshal(do(t, "PUT", fmt.Sprintf("%s/episodes/%d", u, eps.Episodes[0].ID), `{"keep_up_to_date":false}`, 200), &ev)
	if ev.KeepUpToDate || ev.Season != 1 || ev.Episode != 1 {
		t.Fatalf("episode off %+v", ev)
	}
	do(t, "PUT", fmt.Sprintf("%s/episodes/%d", u, eps.Episodes[0].ID), `{}`, 400)
	do(t, "PUT", u+"/episodes/99999", `{"keep_up_to_date":true}`, 404)
	// Season and episode routes are for shows only.
	do(t, "GET", u+"/titles/99999/seasons", "", 404)
}

// TestAddShowPerEpisodeOrder adds the show once per episode order through
// the API: the order is kept and the episodes carry what each order needs
// (season and episode, the air day, the absolute number).
func TestAddShowPerEpisodeOrder(t *testing.T) {
	t.Parallel()
	for _, order := range []string{"aired", "daily", "absolute"} {
		t.Run(order, func(t *testing.T) {
			srv, _, _, _ := showSetup(t)
			u := srv.URL + "/api/v1"
			var add addedShow
			_ = json.Unmarshal(do(t, "POST", u+"/shows", fmt.Sprintf(`{"id":"tmdb:100","episode_order":%q}`, order), 201), &add)
			if add.Title.Show.EpisodeOrder != order {
				t.Fatalf("order %q", add.Title.Show.EpisodeOrder)
			}
			var eps struct {
				Episodes []EpisodeView `json:"episodes"`
				Order    string        `json:"episode_order"`
			}
			_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/titles/%d/episodes", u, add.Title.ID), "", 200), &eps)
			if eps.Order != order || len(eps.Episodes) != 5 {
				t.Fatalf("episodes %+v", eps)
			}
			sp, pilot, ret, tba := eps.Episodes[0], eps.Episodes[1], eps.Episodes[3], eps.Episodes[4]
			if sp.Season != 0 || sp.Absolute != nil || pilot.Absolute == nil || *pilot.Absolute != 1 || *ret.Absolute != 3 ||
				pilot.AirDay != "2025-01-05" || tba.AirDay != "" || tba.AirDate != nil || tba.Slots[0].Reason != "no air date yet" {
				t.Fatalf("episode data %+v", eps.Episodes)
			}
		})
	}
	srv, _, _, _ := showSetup(t)
	do(t, "POST", srv.URL+"/api/v1/shows", `{"id":"tmdb:100","episode_order":"dvd"}`, 400)
}

// route: GET /api/v1/settings/shows
// route: PUT /api/v1/settings/shows
func TestShowSettingsAndEditAPI(t *testing.T) {
	t.Parallel()
	srv, _, _, _ := showSetup(t)
	u := srv.URL + "/api/v1"
	var d library.ShowDefaults
	_ = json.Unmarshal(do(t, "GET", u+"/settings/shows", "", 200), &d)
	if d.SeasonChoice != "all" || d.Source != "tmdb" || d.KeepSpecials || d.WaitMinutes != 0 {
		t.Fatalf("defaults %+v", d)
	}
	_ = json.Unmarshal(do(t, "PUT", u+"/settings/shows", `{"details_source":"tvmaze","keep_specials":true,"episode_wait_minutes":60}`, 200), &d)
	if d.Source != "tvmaze" || !d.KeepSpecials || d.WaitMinutes != 60 || d.SeasonChoice != "all" {
		t.Fatalf("saved %+v", d)
	}
	do(t, "PUT", u+"/settings/shows", `{"episode_wait_minutes":-5}`, 400)
	do(t, "PUT", u+"/settings/shows", `{"details_source":"imdb"}`, 400)

	// The default source is TVMaze now; a TMDB id still works.
	var add addedShow
	_ = json.Unmarshal(do(t, "POST", u+"/shows", `{"id":"tvdb:200"}`, 201), &add)
	id := add.Title.ID
	if add.Title.Show.DetailsSource != "tvmaze" || !add.Title.Show.Seasons[0].KeepUpToDate {
		t.Fatalf("added with settings %+v", add.Title.Show)
	}
	tu := fmt.Sprintf("%s/titles/%d", u, id)
	do(t, "PUT", tu, `{"release_gate":"announced"}`, 400)
	do(t, "PUT", tu, `{"episode_order":"dvd"}`, 400)
	do(t, "PUT", tu, `{"details_source":"tvdb"}`, 400)
	do(t, "PUT", tu, `{"details_source":"plex"}`, 400)
	var edited struct {
		Title struct {
			Show struct {
				DetailsSource string `json:"details_source"`
				EpisodeOrder  string `json:"episode_order"`
			} `json:"show"`
		} `json:"title"`
	}
	_ = json.Unmarshal(do(t, "PUT", tu, `{"details_source":"tmdb","episode_order":"absolute"}`, 200), &edited)
	if edited.Title.Show.DetailsSource != "tmdb" || edited.Title.Show.EpisodeOrder != "absolute" {
		t.Fatalf("edited %+v", edited)
	}
	// Searching a show queues a season search per season with an episode looking (3.2).
	do(t, "POST", tu+"/search", "", 202)
	var view TitleView
	_ = json.Unmarshal(do(t, "GET", tu, "", 200), &view)
	if view.Show == nil || view.Show.Episodes != 5 || view.Stage != "running" || len(view.Slots) != 0 {
		t.Fatalf("show view %+v", view)
	}
}
