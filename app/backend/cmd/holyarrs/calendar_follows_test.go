package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

type calendarFollowAnswer struct {
	MyFollows int `json:"my_follows"`
	People    int `json:"people"`
	Dates     []struct {
		TitleID   int64 `json:"title_id"`
		Following bool  `json:"following"`
		Followers []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			Me   bool   `json:"me"`
		} `json:"followers"`
	} `json:"dates"`
}

// route: GET /api/v1/calendar
// Every Library date comes back, each with whether the viewer follows its
// title and who else does, and the answer says how many titles the viewer
// follows and how many people use the server.
func TestCalendarSaysWhoFollows(t *testing.T) {
	t.Parallel()
	srv, st, _ := libSetup(t)
	folder(t, srv.URL, "movie")
	a := addMovie(t, srv.URL, `{"id":"tmdb:900001"}`)
	if err := st.Write(context.Background(), func(tx *sql.Tx) error {
		for _, u := range []struct{ name, role, state string }{{"luis", "owner", "active"}, {"ana", "member", "active"}, {"gone", "member", "disabled"}} {
			if _, err := database.Insert(tx, "users", &domain.User{Name: u.name, Role: u.role, State: u.state}, domain.System); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var c calendarFollowAnswer
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/calendar?from=2031-01-01&to=2031-03-01", "", 200), &c)
	if len(c.Dates) != 1 || c.Dates[0].Following || len(c.Dates[0].Followers) != 0 || c.MyFollows != 0 || c.People != 2 {
		t.Fatalf("nobody follows yet: %+v", c)
	}
	execRow(t, st, `INSERT INTO follows (user_id, title_id, created_at) VALUES (2, ?, '2026-01-02T00:00:00Z'), (1, ?, '2026-01-03T00:00:00Z'), (3, ?, '2026-01-03T00:00:00Z')`,
		a.Title.ID, a.Title.ID, a.Title.ID)
	c = calendarFollowAnswer{}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/calendar?from=2031-01-01&to=2031-03-01", "", 200), &c)
	d := c.Dates[0]
	if !d.Following || c.MyFollows != 1 || len(d.Followers) != 2 || d.Followers[0].Name != "ana" || d.Followers[1].Name != "luis" || !d.Followers[1].Me {
		t.Fatalf("followers %+v", c)
	}
}
