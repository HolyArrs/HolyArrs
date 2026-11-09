package accounts

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// Sign-ins (good and bad), sign-outs and API keys made or deleted land in
// History with who and from where, never with a password or a key.
func TestAuthEvents(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"not the password"}`, nil)
	_, out := own.do(t, f, "POST", "/api/v1/auth/keys", `{"label":"tool key"}`)
	var k struct {
		ID  int64  `json:"id"`
		Key string `json:"key"`
	}
	_ = json.Unmarshal([]byte(out), &k)
	if resp, _ := own.do(t, f, "DELETE", "/api/v1/auth/keys/"+itoa(k.ID), ""); resp.StatusCode != 204 {
		t.Fatalf("delete key: %d", resp.StatusCode)
	}
	own.do(t, f, "POST", "/api/v1/auth/logout", "")
	es, _, err := f.s.Events(context.Background(), database.EventFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range es {
		seen[e.Kind] = true
		raw, _ := json.Marshal(e.Data)
		if strings.Contains(string(raw), pw) || strings.Contains(string(raw), "not the password") || (k.Key != "" && strings.Contains(string(raw), k.Key)) {
			t.Fatalf("%s holds a secret: %s", e.Kind, raw)
		}
		if e.Kind == "sign_in_failed" && (e.Data["name"] != "luis" || e.Data["ip"] == "") {
			t.Fatalf("failed sign-in data %v", e.Data)
		}
		if e.Kind == "api_key_deleted" && (e.Data["label"] != "tool key" || e.Actor.Kind != "user") {
			t.Fatalf("key deleted %+v", e)
		}
	}
	for _, kind := range []string{"signed_in", "sign_in_failed", "api_key_created", "api_key_deleted", "signed_out"} {
		if !seen[kind] {
			t.Errorf("no %s event (have %v)", kind, seen)
		}
	}
}

// A name that is no user is never stored (it may be a password), and
// repeats from one address within 10 minutes share one row with a count.
func TestFailedSignInUnknownNameAndCollapse(t *testing.T) {
	f := gated(t, Options{})
	ownerOf(t, f)
	c := client(t)
	for range 3 {
		call(t, c, "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"hunter2-secret","password":"x"}`, nil)
	}
	f.clock.add(11 * time.Minute)
	call(t, c, "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"wrong one"}`, nil)
	es, _, err := f.s.Events(context.Background(), database.EventFilter{Kinds: []string{"sign_in_failed"}})
	if err != nil || len(es) != 2 {
		t.Fatalf("rows %d %v", len(es), err)
	}
	if es[1].Data["name"] != "unknown name" || es[1].Data["count"] != float64(3) || strings.Contains(es[1].Summary, "hunter2") {
		t.Fatalf("collapsed row %+v", es[1])
	}
	if es[0].Data["name"] != "luis" || es[0].Data["count"] != float64(1) {
		t.Fatalf("new row %+v", es[0])
	}
}

// Collapsed rows are per address and name, keep the first attempt's time
// (last_at holds the newest) and the 10 minutes run from the first attempt.
func TestFailedSignInCollapseWindow(t *testing.T) {
	f := gated(t, Options{})
	ownerOf(t, f)
	c := client(t)
	fail := func(name string) {
		call(t, c, "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"`+name+`","password":"wrong one"}`, nil)
	}
	first := f.clock.now().UTC()
	fail("luis")
	fail("nobody-here") // other name, same address: its own row
	f.clock.add(6 * time.Minute)
	fail("luis") // inside the window: counted
	f.clock.add(6 * time.Minute)
	fail("luis") // 12 minutes after the first: a new row, though only 6 after the last
	es, _, err := f.s.Events(context.Background(), database.EventFilter{Kinds: []string{"sign_in_failed"}})
	if err != nil || len(es) != 3 {
		t.Fatalf("rows %d %v", len(es), err)
	}
	var luis []map[string]any
	for _, e := range es {
		if e.Data["name"] == "luis" {
			luis = append(luis, e.Data)
		}
	}
	old := es[len(es)-1]
	if len(luis) != 2 || old.Data["name"] != "luis" || old.Data["count"] != float64(2) || !old.At.Equal(first) ||
		old.Data["last_at"] != database.FormatTime(first.Add(6*time.Minute)) {
		t.Fatalf("first luis row %+v at %v", old.Data, old.At)
	}
	if es[0].Data["name"] != "luis" || es[0].Data["count"] != float64(1) {
		t.Fatalf("new window row %+v", es[0].Data)
	}
}

// A wrong two-factor code, a turned-off member's sign-in and changes to a
// member's account all land on the audit trail with how and by whom, never
// with a password or a code.
func TestTwoFactorTurnedOffAndAccountEvents(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	turnOnTwoFactor(t, f, own)
	c := client(t)
	tk := ticketFor(t, f, c, "luis")
	call(t, c, "POST", f.srv.URL+"/api/v1/auth/login/two-factor", `{"ticket":"`+tk+`","code":"123456"}`, nil)

	join(t, f, invite(t, f, own, `{}`), "ana")
	var id int64
	_ = f.s.Read().QueryRow("SELECT id FROM users WHERE name = 'ana'").Scan(&id)
	if resp, out := own.do(t, f, "POST", "/api/v1/users/"+itoa(id)+"/password", `{"password":"another long test passphrase"}`); resp.StatusCode != 204 {
		t.Fatalf("set password %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "PUT", "/api/v1/users/"+itoa(id), `{"state":"disabled"}`); resp.StatusCode != 200 {
		t.Fatalf("turn off %d %s", resp.StatusCode, out)
	}
	call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"ana","password":"another long test passphrase"}`, nil)

	es, _, err := f.s.Events(context.Background(), database.EventFilter{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	fails := map[string]string{}
	actions := map[string]string{}
	for _, e := range es {
		raw, _ := json.Marshal(e.Data)
		if strings.Contains(string(raw), "123456") || strings.Contains(string(raw), "another long test passphrase") || strings.Contains(string(raw), pw) {
			t.Fatalf("%s holds a secret: %s", e.Kind, raw)
		}
		switch e.Kind {
		case "sign_in_failed":
			fails[e.Data["how"].(string)] = e.Data["name"].(string)
		case EventAccountChanged:
			if e.Actor.Kind != "user" {
				t.Fatalf("account change without a person %+v", e)
			}
			actions[e.Data["action"].(string)] = e.Summary
		}
	}
	if fails["two-factor"] != "luis" {
		t.Errorf("wrong code row %v", fails)
	}
	if fails["turned off"] != "ana" {
		t.Errorf("turned-off member not named %v", fails)
	}
	for _, a := range []string{"two_factor_on", "invite_made", "password_set", "edited"} {
		if actions[a] == "" {
			t.Errorf("no %s account event (have %v)", a, actions)
		}
	}
	if s := actions["edited"]; !strings.Contains(s, "ana") || !strings.Contains(s, "turned off") {
		t.Errorf("edit line %q", s)
	}
}

// Wrong passwords at the password check, new backup codes and a reset from
// the command line are on the audit trail; a save that changes nothing is not.
func TestReauthBackupCodesAndResetEvents(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	secret, _ := turnOnTwoFactor(t, f, own)
	own.do(t, f, "POST", "/api/v1/auth/reauth", `{"password":"not the password","code":"000000"}`)
	f.clock.add(totpStep * time.Second)
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/two-factor/backup-codes", `{"code":"`+codeNow(t, f, secret, 0)+`"}`); resp.StatusCode != 200 {
		t.Fatalf("new codes %d %s", resp.StatusCode, out)
	}
	join(t, f, invite(t, f, own, `{}`), "ana")
	var id int64
	_ = f.s.Read().QueryRow("SELECT id FROM users WHERE name = 'ana'").Scan(&id)
	if resp, out := own.do(t, f, "PUT", "/api/v1/users/"+itoa(id), `{"state":"active"}`); resp.StatusCode != 200 {
		t.Fatalf("same state %d %s", resp.StatusCode, out)
	}
	if err := ResetOwner(context.Background(), f.s, "luis", pw); err != nil {
		t.Fatal(err)
	}
	es, _, err := f.s.Events(context.Background(), database.EventFilter{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	fails := map[string]bool{}
	actions := map[string]int{}
	for _, e := range es {
		raw, _ := json.Marshal(e.Data)
		if strings.Contains(string(raw), "not the password") || strings.Contains(string(raw), pw) {
			t.Fatalf("%s holds a secret: %s", e.Kind, raw)
		}
		if e.Kind == "sign_in_failed" && e.Data["name"] == "luis" {
			fails[e.Data["how"].(string)] = true
		}
		if e.Kind == EventAccountChanged {
			actions[e.Data["action"].(string)]++
		}
	}
	if !fails["password check"] {
		t.Errorf("wrong password at the check not logged %v", fails)
	}
	if actions["backup_codes_new"] != 1 || actions["owner_reset"] != 1 {
		t.Errorf("actions %v", actions)
	}
	if actions["edited"] != 0 {
		t.Errorf("a save with no change was logged %v", actions)
	}
}
