package auth

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/store"
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
	es, _, err := f.s.Events(context.Background(), store.EventFilter{Limit: 100})
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
	es, _, err := f.s.Events(context.Background(), store.EventFilter{Kinds: []string{"sign_in_failed"}})
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
	es, _, err := f.s.Events(context.Background(), store.EventFilter{Kinds: []string{"sign_in_failed"}})
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
		old.Data["last_at"] != store.FormatTime(first.Add(6*time.Minute)) {
		t.Fatalf("first luis row %+v at %v", old.Data, old.At)
	}
	if es[0].Data["name"] != "luis" || es[0].Data["count"] != float64(1) {
		t.Fatalf("new window row %+v", es[0].Data)
	}
}
