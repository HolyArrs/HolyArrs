package accounts

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// limitOf reads max_rating and rating_country from a body.
func limitOf(t *testing.T, body string) (string, string) {
	t.Helper()
	var v struct {
		MaxRating     *string `json:"max_rating"`
		RatingCountry *string `json:"rating_country"`
	}
	jsonOf(t, body, &v)
	if v.MaxRating == nil || v.RatingCountry == nil {
		t.Fatalf("max_rating or rating_country missing: %s", body)
	}
	return *v.MaxRating, *v.RatingCountry
}

// userLimit reads one person's limit from GET /users.
func userLimit(t *testing.T, f *fixture, by person, name string) (string, string) {
	t.Helper()
	resp, out := by.do(t, f, "GET", "/api/v1/users", "")
	if resp.StatusCode != 200 {
		t.Fatalf("users: %d %s", resp.StatusCode, out)
	}
	var v struct {
		Users []struct {
			Name          string  `json:"name"`
			MaxRating     *string `json:"max_rating"`
			RatingCountry *string `json:"rating_country"`
		} `json:"users"`
	}
	jsonOf(t, out, &v)
	for _, u := range v.Users {
		if u.Name == name {
			if u.MaxRating == nil || u.RatingCountry == nil {
				t.Fatalf("users: no limit fields: %s", out)
			}
			return *u.MaxRating, *u.RatingCountry
		}
	}
	t.Fatalf("users: no %s in %s", name, out)
	return "", ""
}

func meLimit(t *testing.T, f *fixture, p person) (string, string) {
	t.Helper()
	resp, out := p.do(t, f, "GET", "/api/v1/auth/me", "")
	if resp.StatusCode != 200 {
		t.Fatalf("me: %d %s", resp.StatusCode, out)
	}
	return limitOf(t, out)
}

// route: GET /api/v1/users/invites
func TestInviteCarriesKidLimit(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	resp, out := own.do(t, f, "POST", "/api/v1/users/invites", `{"max_rating":"pg"}`)
	if resp.StatusCode != 201 {
		t.Fatalf("invite: %d %s", resp.StatusCode, out)
	}
	if max, country := limitOf(t, out); max != "PG" || country != "US" {
		t.Fatalf("invite answer: %q %q", max, country)
	}
	var v struct {
		Path string `json:"path"`
	}
	jsonOf(t, out, &v)
	tok := strings.TrimPrefix(v.Path, "/invite/")
	resp, out = call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/invites/"+tok, "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("look: %d %s", resp.StatusCode, out)
	}
	if max, country := limitOf(t, out); max != "PG" || country != "US" {
		t.Fatalf("look: %q %q", max, country)
	}
	_, out = own.do(t, f, "GET", "/api/v1/users/invites", "")
	if !strings.Contains(out, `"max_rating":"PG"`) || !strings.Contains(out, `"rating_country":"US"`) {
		t.Fatalf("invites list: %s", out)
	}
	kai := join(t, f, tok, "kai")
	if max, country := meLimit(t, f, kai); max != "PG" || country != "US" {
		t.Fatalf("me: %q %q", max, country)
	}
	if max, country := userLimit(t, f, own, "kai"); max != "PG" || country != "US" {
		t.Fatalf("users: %q %q", max, country)
	}
	// An invite without a limit makes a person without one.
	ana := join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	if max, country := meLimit(t, f, ana); max != "" || country != "US" {
		t.Fatalf("no-limit me: %q %q", max, country)
	}
	if max, _ := meLimit(t, f, own); max != "" {
		t.Fatalf("owner has a limit: %q", max)
	}
}

func TestEditSetsAndClearsKidLimit(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	path := fmt.Sprintf("/api/v1/users/%d", userID(t, f, "ana"))
	resp, out := own.do(t, f, "PUT", path, `{"max_rating":"tv-14","rating_country":"us"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("set: %d %s", resp.StatusCode, out)
	}
	if max, country := limitOf(t, out); max != "TV-14" || country != "US" {
		t.Fatalf("set answer: %q %q", max, country)
	}
	if max, _ := userLimit(t, f, own, "ana"); max != "TV-14" {
		t.Fatalf("users after set: %q", max)
	}
	// A change that leaves max_rating out keeps it.
	if resp, out := own.do(t, f, "PUT", path, `{"perms":["ask"]}`); resp.StatusCode != 200 {
		t.Fatalf("perms: %d %s", resp.StatusCode, out)
	}
	if max, _ := userLimit(t, f, own, "ana"); max != "TV-14" {
		t.Fatalf("limit lost on a perms change: %q", max)
	}
	resp, out = own.do(t, f, "PUT", path, `{"max_rating":""}`)
	if resp.StatusCode != 200 {
		t.Fatalf("clear: %d %s", resp.StatusCode, out)
	}
	if max, _ := limitOf(t, out); max != "" {
		t.Fatalf("clear answer: %q", max)
	}
	if max, _ := userLimit(t, f, own, "ana"); max != "" {
		t.Fatalf("users after clear: %q", max)
	}
}

func TestKidLimitOnlyForMembers(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	const only = "Only members can have an age-rating limit."
	resp, out := own.do(t, f, "POST", "/api/v1/users/invites", `{"role":"admin","max_rating":"PG"}`)
	if resp.StatusCode != 400 || !strings.Contains(out, only) {
		t.Fatalf("admin invite with a limit: %d %s", resp.StatusCode, out)
	}
	join(t, f, invite(t, f, own, `{"role":"admin"}`), "ad")
	resp, out = own.do(t, f, "PUT", fmt.Sprintf("/api/v1/users/%d", userID(t, f, "ad")), `{"max_rating":"PG"}`)
	if resp.StatusCode != 400 || !strings.Contains(out, only) {
		t.Fatalf("admin edit with a limit: %d %s", resp.StatusCode, out)
	}
	join(t, f, invite(t, f, own, `{"role":"member","max_rating":"PG"}`), "kai")
	kaiPath := fmt.Sprintf("/api/v1/users/%d", userID(t, f, "kai"))
	resp, out = own.do(t, f, "PUT", kaiPath, `{"role":"admin","max_rating":"PG"}`)
	if resp.StatusCode != 400 || !strings.Contains(out, only) {
		t.Fatalf("promote with a limit: %d %s", resp.StatusCode, out)
	}
	resp, out = own.do(t, f, "PUT", kaiPath, `{"role":"admin"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("promote: %d %s", resp.StatusCode, out)
	}
	if max, _ := limitOf(t, out); max != "" {
		t.Fatalf("promote answer kept the limit: %q", max)
	}
	if max, _ := userLimit(t, f, own, "kai"); max != "" {
		t.Fatalf("an admin kept a limit: %q", max)
	}
	// Making them a member again does not bring the old limit back.
	if resp, out := own.do(t, f, "PUT", kaiPath, `{"role":"member"}`); resp.StatusCode != 200 {
		t.Fatalf("demote: %d %s", resp.StatusCode, out)
	}
	if max, _ := userLimit(t, f, own, "kai"); max != "" {
		t.Fatalf("old limit came back: %q", max)
	}
}

func TestKidLimitRejectsUnknownRating(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	resp, out := own.do(t, f, "POST", "/api/v1/users/invites", `{"max_rating":"X"}`)
	if resp.StatusCode != 400 || !strings.Contains(out, "G, PG, PG-13, R, NC-17, TV-Y, TV-Y7, TV-G, TV-PG, TV-14, TV-MA") {
		t.Fatalf("unknown rating invite: %d %s", resp.StatusCode, out)
	}
	join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	resp, out = own.do(t, f, "PUT", fmt.Sprintf("/api/v1/users/%d", userID(t, f, "ana")), `{"max_rating":"NR"}`)
	if resp.StatusCode != 400 || !strings.Contains(out, "TV-MA") {
		t.Fatalf("unknown rating edit: %d %s", resp.StatusCode, out)
	}
	if max, _ := userLimit(t, f, own, "ana"); max != "" {
		t.Fatalf("refused limit stored: %q", max)
	}
}

func TestKidLimitRejectsOtherCountries(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	const usOnly = "Only US age ratings are known for now."
	resp, out := own.do(t, f, "POST", "/api/v1/users/invites", `{"max_rating":"PG","rating_country":"GB"}`)
	if resp.StatusCode != 400 || !strings.Contains(out, usOnly) {
		t.Fatalf("GB invite: %d %s", resp.StatusCode, out)
	}
	join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	resp, out = own.do(t, f, "PUT", fmt.Sprintf("/api/v1/users/%d", userID(t, f, "ana")), `{"rating_country":"DE"}`)
	if resp.StatusCode != 400 || !strings.Contains(out, usOnly) {
		t.Fatalf("DE edit: %d %s", resp.StatusCode, out)
	}
}

func TestKidLimitTakesEffectWithoutSigningOut(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ana := join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	if max, _ := meLimit(t, f, ana); max != "" {
		t.Fatalf("start: %q", max)
	}
	path := fmt.Sprintf("/api/v1/users/%d", userID(t, f, "ana"))
	if resp, out := own.do(t, f, "PUT", path, `{"max_rating":"PG"}`); resp.StatusCode != 200 {
		t.Fatalf("set: %d %s", resp.StatusCode, out)
	}
	if max, _ := meLimit(t, f, ana); max != "PG" {
		t.Fatalf("same session after set: %q", max)
	}
	if resp, out := own.do(t, f, "PUT", path, `{"max_rating":"R"}`); resp.StatusCode != 200 {
		t.Fatalf("raise: %d %s", resp.StatusCode, out)
	}
	if max, _ := meLimit(t, f, ana); max != "R" {
		t.Fatalf("same session after raise: %q", max)
	}
}

func TestAPIKeyCarriesKidLimit(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	// Making a key needs settings, which a Kid may not hold, so the key is
	// made first and the limit set afterwards (it then narrows the key too).
	kai := join(t, f, invite(t, f, own, `{"role":"member","perms":["ask","settings"]}`), "kai")
	resp, out := kai.do(t, f, "POST", "/api/v1/auth/keys", `{"label":"tablet"}`)
	if resp.StatusCode != 201 {
		t.Fatalf("make key: %d %s", resp.StatusCode, out)
	}
	if r2, o2 := own.do(t, f, "PUT", fmt.Sprintf("/api/v1/users/%d", userID(t, f, "kai")), `{"perms":["ask"],"max_rating":"PG"}`); r2.StatusCode != 200 {
		t.Fatalf("set limit: %d %s", r2.StatusCode, o2)
	}
	var k struct {
		Key string `json:"key"`
	}
	jsonOf(t, out, &k)
	resp, out = call(t, &http.Client{}, "GET", f.srv.URL+"/api/v1/auth/me", "", map[string]string{APIKeyHeader: k.Key})
	if resp.StatusCode != 200 {
		t.Fatalf("me with key: %d %s", resp.StatusCode, out)
	}
	if max, country := limitOf(t, out); max != "PG" || country != "US" {
		t.Fatalf("me with key: %q %q", max, country)
	}
}

func TestKidMayOnlyAsk(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	const only = "A Kid account can only ask for titles."
	for _, perms := range []string{`["ask","manage_library"]`, `["ask","approve"]`, `["settings"]`} {
		resp, out := own.do(t, f, "POST", "/api/v1/users/invites", `{"role":"member","perms":`+perms+`,"max_rating":"PG"}`)
		if resp.StatusCode != 400 || !strings.Contains(out, only) {
			t.Fatalf("invite %s: %d %s", perms, resp.StatusCode, out)
		}
	}
	if resp, out := own.do(t, f, "POST", "/api/v1/users/invites", `{"role":"member","perms":["ask","auto_approve"],"max_rating":"PG"}`); resp.StatusCode != 201 {
		t.Fatalf("ask and auto_approve: %d %s", resp.StatusCode, out)
	}
	join(t, f, invite(t, f, own, `{"role":"member","perms":["ask","manage_library"]}`), "ana")
	anaPath := fmt.Sprintf("/api/v1/users/%d", userID(t, f, "ana"))
	resp, out := own.do(t, f, "PUT", anaPath, `{"max_rating":"PG"}`)
	if resp.StatusCode != 400 || !strings.Contains(out, only) {
		t.Fatalf("limit on a manager: %d %s", resp.StatusCode, out)
	}
	join(t, f, invite(t, f, own, `{"role":"member","max_rating":"PG"}`), "kai")
	kaiPath := fmt.Sprintf("/api/v1/users/%d", userID(t, f, "kai"))
	resp, out = own.do(t, f, "PUT", kaiPath, `{"perms":["ask","manage_library"]}`)
	if resp.StatusCode != 400 || !strings.Contains(out, only) {
		t.Fatalf("manage_library on a Kid: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "PUT", kaiPath, `{"perms":["ask","manage_library"],"max_rating":""}`); resp.StatusCode != 200 {
		t.Fatalf("clear limit and grant: %d %s", resp.StatusCode, out)
	}
}

// TestKidLimitAndLocalSkipExclude: with skipping sign-in at home on, a
// limit is refused (anyone at home is the owner); with a Kid, skipping
// sign-in cannot be turned on.
func TestKidLimitAndLocalSkipExclude(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	path := fmt.Sprintf("/api/v1/users/%d", userID(t, f, "ana"))
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"skip_login_local_network":true,"local_networks":["127.0.0.0/8","10.0.0.0/8"]}`); resp.StatusCode != 200 {
		t.Fatalf("skip on: %d %s", resp.StatusCode, out)
	}
	const refused = "Turn off skipping sign-in on the local network first: with it on, anyone at home signs in as the owner."
	for _, c := range []struct{ method, path, body string }{
		{"PUT", path, `{"max_rating":"PG"}`},
		{"POST", "/api/v1/users/invites", `{"max_rating":"PG"}`},
	} {
		resp, out := own.do(t, f, c.method, c.path, c.body)
		if resp.StatusCode != 400 || !strings.Contains(out, refused) {
			t.Fatalf("%s %s with skip on: %d %s", c.method, c.path, resp.StatusCode, out)
		}
	}
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"skip_login_local_network":false}`); resp.StatusCode != 200 {
		t.Fatalf("skip off: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "PUT", path, `{"max_rating":"PG"}`); resp.StatusCode != 200 {
		t.Fatalf("limit with skip off: %d %s", resp.StatusCode, out)
	}
	resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"skip_login_local_network":true,"local_networks":["127.0.0.0/8","10.0.0.0/8"]}`)
	if resp.StatusCode != 400 || !strings.Contains(out, "age limit") {
		t.Fatalf("skip on with a Kid: %d %s", resp.StatusCode, out)
	}
	if _, out := own.do(t, f, "GET", "/api/v1/auth/settings", ""); !strings.Contains(out, `"skip_login_local_network":false`) {
		t.Fatalf("skip turned on anyway: %s", out)
	}
}

// TestOutsideJoinOpen: open only once a way is on and lets new people join.
func TestOutsideJoinOpen(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	if f.a.OutsideJoinOpen(context.Background()) {
		t.Fatal("open with nothing set up")
	}
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"jellyfin":{"on":true,"url":"http://192.0.2.10:8096","join":"nobody"}}`); resp.StatusCode != 200 {
		t.Fatalf("jellyfin nobody: %d %s", resp.StatusCode, out)
	}
	if f.a.OutsideJoinOpen(context.Background()) {
		t.Fatal("open while nobody may join")
	}
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"jellyfin":{"on":true,"url":"http://192.0.2.10:8096","join":"list","allow":"kai"}}`); resp.StatusCode != 200 {
		t.Fatalf("jellyfin list: %d %s", resp.StatusCode, out)
	}
	if !f.a.OutsideJoinOpen(context.Background()) {
		t.Fatal("closed while a list may join")
	}
}

// TestOpenKidInviteBlocksLocalSkip: an open invite that carries an age
// limit makes a Kid once used, so skipping sign-in on the local network
// stays off while one is open; once it has run out, the skip may go on.
func TestOpenKidInviteBlocksLocalSkip(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	if resp, out := own.do(t, f, "POST", "/api/v1/users/invites", `{"max_rating":"PG","days":1}`); resp.StatusCode != 201 {
		t.Fatalf("kid invite: %d %s", resp.StatusCode, out)
	}
	resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"skip_login_local_network":true,"local_networks":["127.0.0.0/8","10.0.0.0/8"]}`)
	if resp.StatusCode != 400 || !strings.Contains(out, "invite") {
		t.Fatalf("skip on with an open Kid invite: %d %s", resp.StatusCode, out)
	}
	if _, out := own.do(t, f, "GET", "/api/v1/auth/settings", ""); !strings.Contains(out, `"skip_login_local_network":false`) {
		t.Fatalf("skip turned on anyway: %s", out)
	}
	f.clock.add(48 * time.Hour)
	resp, out = own.do(t, f, "POST", "/api/v1/auth/reauth", `{"password":"`+pw+`"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reauth: %d %s", resp.StatusCode, out)
	}
	var fresh struct {
		CSRF string `json:"csrf"`
	}
	jsonOf(t, out, &fresh)
	own.csrf = fresh.CSRF
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"skip_login_local_network":true,"local_networks":["127.0.0.0/8","10.0.0.0/8"]}`); resp.StatusCode != 200 {
		t.Fatalf("skip on after the invite ran out: %d %s", resp.StatusCode, out)
	}
}
