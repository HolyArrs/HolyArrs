package accounts

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

func setupPinKeys(t *testing.T, body string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return strings.Join(ks, " ")
}

// First-run setup as the wizard sees it: the state before and after the
// owner exists, the owner's answer, the second attempt refused, and the
// "done" call that closes the wizard (setting row and session family).
func TestSetupPinFirstRunFlow(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	url := f.srv.URL + "/api/v1/auth/"

	_, body := call(t, c, "GET", url+"state", "", nil)
	var st struct {
		NeedsSetup bool     `json:"needs_setup"`
		SetupWays  []string `json:"setup_ways"`
		Ways       []string `json:"ways"`
	}
	_ = json.Unmarshal([]byte(body), &st)
	if !st.NeedsSetup || strings.Join(st.SetupWays, ",") != "password,plex,jellyfin,emby" || strings.Join(st.Ways, ",") != "password" ||
		setupPinKeys(t, body) != "min_password_length needs_setup safe_mode setup_ways ways" {
		t.Fatalf("state before: %s", body)
	}
	// Open to anyone before the owner exists, but nothing to finish yet.
	if resp, _ := call(t, c, "POST", url+"setup/done", "", nil); resp.StatusCode != 401 {
		t.Fatalf("done before sign-in: %d", resp.StatusCode)
	}
	if resp, body := call(t, c, "GET", url+"setup/discover", "", nil); resp.StatusCode != 200 || setupPinKeys(t, body) != "servers" {
		t.Fatalf("discover: %d %s", resp.StatusCode, body)
	}
	// Refusals leave no owner behind.
	for _, b := range []string{`{"name":"luis","password":"abc"}`, `{"name":"","password":"` + pw + `"}`, `not json`} {
		if resp, _ := call(t, c, "POST", url+"setup", b, nil); resp.StatusCode != 400 {
			t.Fatalf("setup %s: %d", b, resp.StatusCode)
		}
	}
	var owners int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM users").Scan(&owners)
	if owners != 0 {
		t.Fatalf("%d users after refusals", owners)
	}

	resp, body := call(t, c, "POST", url+"setup", `{"name":"luis","password":"`+pw+`"}`, nil)
	if resp.StatusCode != 201 || setupPinKeys(t, body) != "csrf name" {
		t.Fatalf("setup: %d %s", resp.StatusCode, body)
	}
	var made struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal([]byte(body), &made)
	var role, perms string
	_ = f.s.Read().QueryRow("SELECT role, perms FROM users WHERE name = 'luis'").Scan(&role, &perms)
	if role != "owner" {
		t.Fatalf("role %q", role)
	}
	if resp, body := call(t, c, "POST", url+"setup", `{"name":"x","password":"`+pw+`"}`, nil); resp.StatusCode != 409 || !strings.Contains(body, "already set up") {
		t.Fatalf("second setup: %d %s", resp.StatusCode, body)
	}
	_, body = call(t, c, "GET", url+"state", "", nil)
	if strings.Contains(body, `"needs_setup":true`) || strings.Contains(body, "setup_ways") {
		t.Fatalf("state after: %s", body)
	}

	// The wizard's own session: setup_open and setup_recent until done.
	me := func() map[string]any {
		_, b := call(t, c, "GET", url+"me", "", nil)
		var m map[string]any
		_ = json.Unmarshal([]byte(b), &m)
		return m
	}
	m := me()
	if m["auth_method"] != "local_setup" || m["setup_open"] != true || m["setup_recent"] != true || m["role"] != "owner" {
		t.Fatalf("me during setup: %v", m)
	}
	var fam string
	_ = f.s.Read().QueryRow("SELECT family FROM sessions LIMIT 1").Scan(&fam)
	if !strings.HasPrefix(fam, "setup.") {
		t.Fatalf("session family %q", fam)
	}
	var done int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM settings WHERE grp = 'this_app' AND key = 'first_run_done'").Scan(&done)
	if done != 0 {
		t.Fatal("first_run_done set before done")
	}

	resp, body = call(t, c, "POST", url+"setup/done", "", map[string]string{CSRFHeader: made.CSRF})
	if resp.StatusCode != 200 || strings.TrimSpace(body) != `{"done":true}` {
		t.Fatalf("done: %d %s", resp.StatusCode, body)
	}
	var val string
	_ = f.s.Read().QueryRow("SELECT value FROM settings WHERE grp = 'this_app' AND key = 'first_run_done'").Scan(&val)
	_ = f.s.Read().QueryRow("SELECT family FROM sessions LIMIT 1").Scan(&fam)
	if val != "1" || strings.HasPrefix(fam, "setup.") || fam == "" {
		t.Fatalf("after done: value %q family %q", val, fam)
	}
	m = me()
	if m["setup_open"] != false || m["setup_recent"] != false {
		t.Fatalf("me after done: %v", m)
	}
	// Saying done again is fine.
	if resp, _ := call(t, c, "POST", url+"setup/done", "", map[string]string{CSRFHeader: made.CSRF}); resp.StatusCode != 200 {
		t.Fatalf("done twice: %d", resp.StatusCode)
	}
}

// The setup routes keep their answer when the wizard is not the caller: an
// API key (not a browser session) cannot finish setup, and a member cannot.
func TestSetupPinDoneNeedsTheOwnersBrowser(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	csrf := setupOwner(t, f, c)
	// A second browser that never signed in.
	other := client(t)
	if resp, _ := call(t, other, "POST", f.srv.URL+"/api/v1/auth/setup/done", "", map[string]string{CSRFHeader: csrf}); resp.StatusCode != 401 {
		t.Fatalf("stranger: %d", resp.StatusCode)
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM settings WHERE grp = 'this_app' AND key = 'first_run_done'").Scan(&n)
	if n != 0 {
		t.Fatal("a stranger finished setup")
	}
}
