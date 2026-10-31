package api

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

	"github.com/CoderLuii/holyarrs/internal/asks"
	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
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
// route: GET /api/v1/my/ask-privacy
// route: PUT /api/v1/my/ask-privacy
func TestAskedByPrivacyMatrix(t *testing.T) {
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
		title := domain.Title{Kind: "movie", Name: "Paper Harbour", SortName: "paper harbour", SourceIDs: map[string]string{"tmdb": "900002"}, MetadataSource: "tmdb",
			Rating: "PG", WhyAdded: "by_hand", State: "active", NewPartsPolicy: "all", ReleaseGate: "released", GateDriver: "none", Status: "released", KeepUpToDate: true}
		if _, err := store.Insert(tx, "titles", &title, domain.System); err != nil {
			return err
		}
		titleID = title.ID
		now := time.Now().UTC()
		res, err := tx.Exec(`INSERT INTO asks (created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, title_id, asked_by, asked_at)
			VALUES (?, '{}', ?, '{}', 'movie', '900002', 'Paper Harbour', 'approved', ?, ?, ?)`, store.FormatTime(now), store.FormatTime(now), titleID, r.ana.id, store.FormatTime(now))
		if err != nil {
			return err
		}
		askID, _ := res.LastInsertId()
		for i, id := range []int64{r.ana.id, r.bo.id, kai.id} {
			if _, err := tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at) VALUES (?, ?, ?)", askID, id, store.FormatTime(now.Add(time.Duration(i)*time.Minute))); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	all := []string{"ana", "bo", "kai"}
	check := func(label string, s *signedIn, want []string) {
		t.Helper()
		if got := askedByNames(t, s, titleID); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s: %s sees %v, want %v", label, label, got, want)
		}
	}
	setHousehold := func(v string) {
		r.owner.call("PUT", "/api/v1/settings/asks", `{"show_who_asked":"`+v+`"}`, 200)
	}
	optIn := func(s *signedIn, on bool) {
		body := `{"show_my_name":false}`
		if on {
			body = `{"show_my_name":true}`
		}
		r := s.call("PUT", "/api/v1/my/ask-privacy", body, 200)
		if strings.Contains(string(r), `"show_my_name":true`) != on {
			t.Fatalf("ask-privacy answer %s", r)
		}
	}

	// Default: admins only, nobody opted in.
	if !strings.Contains(string(r.ana.call("GET", "/api/v1/my/ask-privacy", "", 200)), `"show_my_name":false`) {
		t.Fatal("show_my_name should start off")
	}
	check("owner", r.owner, all)
	check("manager", mia, all)
	check("member ana", r.ana, []string{"ana"})
	check("member bo", r.bo, []string{"bo"})
	check("kid", kai, []string{"kai"})

	// Opt-in alone does nothing while the household says admins only.
	optIn(r.ana, true)
	check("member bo, ana opted in, admins only", r.bo, []string{"bo"})

	// Everyone in the household: members see askers who opted in.
	setHousehold("everyone")
	check("member bo, everyone", r.bo, []string{"ana", "bo"})
	check("kid, everyone", kai, []string{"kai"})
	check("owner, everyone", r.owner, all)
	optIn(r.bo, true)
	check("member ana, both opted in", r.ana, []string{"ana", "bo"})
	optIn(kai, true)
	check("member ana, kid opted in", r.ana, []string{"ana", "bo", "kai"})
	check("kid never sees others", kai, []string{"kai"})

	// Opting out hides the name again.
	optIn(r.bo, false)
	check("member ana, bo opted out", r.ana, []string{"ana", "kai"})

	// Requests and title answers are trimmed the same way.
	var list struct {
		Asks []asks.View `json:"asks"`
	}
	_ = json.Unmarshal(r.bo.call("GET", "/api/v1/asks", "", 200), &list)
	if len(list.Asks) != 1 || len(list.Asks[0].People) != 3 {
		t.Fatalf("bo's wishlist people %+v", list.Asks)
	}
	setHousehold("approvers")
	_ = json.Unmarshal(r.bo.call("GET", "/api/v1/asks", "", 200), &list)
	if len(list.Asks) != 1 || len(list.Asks[0].People) != 1 || list.Asks[0].People[0].Name != "bo" || list.Asks[0].AskedBy.Name != "bo" {
		t.Fatalf("bo's wishlist people, admins only %+v", list.Asks)
	}
	_ = json.Unmarshal(kai.call("GET", "/api/v1/asks", "", 200), &list)
	if len(list.Asks) != 1 || len(list.Asks[0].People) != 1 || list.Asks[0].People[0].Name != "kai" {
		t.Fatalf("kai's wishlist people %+v", list.Asks)
	}
	check("member ana, admins only again", r.ana, []string{"ana"})

	// Bad input.
	r.owner.call("PUT", "/api/v1/settings/asks", `{"show_who_asked":"nobody"}`, 400)
	r.ana.call("GET", "/api/v1/asked-by?title_id=x", "", 400)
	r.ana.call("PUT", "/api/v1/settings/asks", `{"show_who_asked":"everyone"}`, 403)
}
