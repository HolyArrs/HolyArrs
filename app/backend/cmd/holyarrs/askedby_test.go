package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// askedByNames reads GET /asked-by for one title as one person.
func askedByNames(t *testing.T, s *signedIn, titleID int64) []string {
	t.Helper()
	var x struct {
		Titles map[string]struct {
			Askers []struct {
				Name string `json:"name"`
			} `json:"askers"`
		} `json:"titles"`
	}
	if err := json.Unmarshal(s.call("GET", "/api/v1/asked-by?title_id="+itoa(titleID), "", 200), &x); err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, p := range x.Titles[itoa(titleID)].Askers {
		out = append(out, p.Name)
	}
	sort.Strings(out)
	return out
}

func jarOf() http.CookieJar {
	j, _ := cookiejar.New(nil)
	return j
}

// route: GET /api/v1/asked-by
func TestAskedByEveryoneSees(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	join := func(name, invite string) *signedIn {
		var inv struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(r.owner.call("POST", "/api/v1/users/invites", invite, 201), &inv)
		b := &signedIn{t: t, base: r.srv.URL, c: &http.Client{Jar: jarOf()}}
		var x struct {
			CSRF string `json:"csrf"`
		}
		_ = json.Unmarshal(b.call("POST", "/api/v1/auth/invites/"+strings.TrimPrefix(inv.Path, "/invite/"), `{"name":"`+name+`","password":"a long test passphrase"}`, 201), &x)
		b.csrf = x.CSRF
		var me struct {
			ID int64 `json:"id"`
		}
		_ = json.Unmarshal(b.call("GET", "/api/v1/auth/me", "", 200), &me)
		b.id = me.ID
		return b
	}
	kai := join("kai", `{"max_rating":"PG"}`)
	mia := join("mia", `{}`)
	// mia becomes a manager: she approves asks.
	r.owner.call("PUT", "/api/v1/users/"+itoa(mia.id), `{"perms":["ask","auto_approve","approve","manage_library"]}`, 200)

	// One title, asked for by ana, bo and kai.
	var titleID int64
	ctx := context.Background()
	err := r.st.Write(ctx, func(tx *sql.Tx) error {
		title := mediafiles.Title{Kind: "movie", Name: "Paper Harbour", SortName: "paper harbour", SourceIDs: map[string]string{"tmdb": "900002"}, MetadataSource: "tmdb",
			Rating: "PG", WhyAdded: "by_hand", State: "active", NewPartsPolicy: "all", ReleaseGate: "released", GateDriver: "none", Status: "released", KeepUpToDate: true}
		if _, err := database.Insert(tx, "titles", &title, domain.System); err != nil {
			return err
		}
		titleID = title.ID
		now := time.Now().UTC()
		res, err := tx.Exec(`INSERT INTO asks (created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, title_id, asked_by, asked_at)
			VALUES (?, '{}', ?, '{}', 'movie', '900002', 'Paper Harbour', 'approved', ?, ?, ?)`, database.FormatTime(now), database.FormatTime(now), titleID, r.ana.id, database.FormatTime(now))
		if err != nil {
			return err
		}
		askID, _ := res.LastInsertId()
		for i, id := range []int64{r.ana.id, r.bo.id, kai.id} {
			if _, err := tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at) VALUES (?, ?, ?)", askID, id, database.FormatTime(now.Add(time.Duration(i)*time.Minute))); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	all := []string{"ana", "bo", "kai"}
	check := func(label string, s *signedIn) {
		t.Helper()
		if got := askedByNames(t, s, titleID); strings.Join(got, ",") != strings.Join(all, ",") {
			t.Fatalf("%s sees %v, want %v", label, got, all)
		}
	}
	// Everyone in the household sees every asker's name.
	check("owner", r.owner)
	check("manager", mia)
	check("member ana", r.ana)
	check("member bo", r.bo)
	check("kid", kai)

	// The Requests list names every asker too, for a member and a Kid.
	var list struct {
		Asks []requests.View `json:"asks"`
	}
	for _, s := range []*signedIn{r.bo, kai} {
		_ = json.Unmarshal(s.call("GET", "/api/v1/asks", "", 200), &list)
		if len(list.Asks) != 1 || len(list.Asks[0].People) != 3 || list.Asks[0].AskedBy.Name != "ana" {
			t.Fatalf("wishlist people %+v", list.Asks)
		}
	}

	// The per-person choice is gone.
	r.ana.call("GET", "/api/v1/my/ask-privacy", "", 404)
	r.ana.call("PUT", "/api/v1/my/ask-privacy", `{"show_my_name":true}`, 405)
	for _, k := range []string{"GET /api/v1/my/ask-privacy", "PUT /api/v1/my/ask-privacy"} {
		if _, ok := accounts.RoutePerms[k]; ok {
			t.Fatalf("%s is still in the route table", k)
		}
		if _, ok := parts.Routes()[k]; ok {
			t.Fatalf("%s is still in the parts table", k)
		}
	}

	// A saved old household choice and an old client field change nothing.
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
			VALUES ('this_app', 'ask_show_who_asked', 'approvers', '', '{}', '', '{}')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := string(r.owner.call("GET", "/api/v1/settings/asks", "", 200)); strings.Contains(got, "show_who_asked") {
		t.Fatalf("settings still carry show_who_asked: %s", got)
	}
	// Only people who may change settings change the household rules.
	mia.call("PUT", "/api/v1/settings/asks", `{"movies_per_week":9}`, 403)
	r.owner.call("PUT", "/api/v1/settings/asks", `{"show_who_asked":"nobody","movies_per_week":4}`, 200)
	if got := string(r.owner.call("GET", "/api/v1/settings/asks", "", 200)); !strings.Contains(got, `"movies_per_week":4`) {
		t.Fatalf("settings after save %s", got)
	}
	check("member ana, old choice saved", r.ana)

	r.ana.call("GET", "/api/v1/asked-by?title_id=x", "", 400)
}
