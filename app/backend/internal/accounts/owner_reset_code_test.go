package accounts

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

var codeRe = regexp.MustCompile(`[2-9A-Z]{4}(-[2-9A-Z]{4}){5}`)

// codeFromFile reads the live code from the config folder, as the owner would.
func codeFromFile(t *testing.T, f *fixture) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.s.Dir(), ResetCodeFile))
	if err != nil {
		t.Fatalf("no code file: %v", err)
	}
	c := codeRe.FindString(string(b))
	if c == "" {
		t.Fatalf("no code in %q", b)
	}
	return c
}

func askCode(t *testing.T, f *fixture, hdr map[string]string) (int, string) {
	t.Helper()
	resp, body := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/owner-reset/code", "{}", hdr)
	return resp.StatusCode, body
}

func finishReset(t *testing.T, f *fixture, code, name, pass string) (int, string) {
	t.Helper()
	b, _ := json.Marshal(resetBody{Code: code, Name: name, Password: pass})
	resp, body := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/owner-reset/finish", string(b), nil)
	return resp.StatusCode, body
}

// finishFrom finishes from a client address, through a trusted proxy.
func finishFrom(t *testing.T, f *fixture, ip, code, name, pass string) (int, string) {
	t.Helper()
	b, _ := json.Marshal(resetBody{Code: code, Name: name, Password: pass})
	resp, body := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/owner-reset/finish", string(b), map[string]string{"X-Forwarded-For": ip})
	return resp.StatusCode, body
}

func checkFrom(t *testing.T, f *fixture, ip, code string) int {
	t.Helper()
	b, _ := json.Marshal(resetBody{Code: code})
	resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/owner-reset/check", string(b), map[string]string{"X-Forwarded-For": ip})
	return resp.StatusCode
}

func codeFileGone(t *testing.T, f *fixture) bool {
	_, err := os.Stat(filepath.Join(f.s.Dir(), ResetCodeFile))
	return os.IsNotExist(err)
}

func TestOwnerResetCodeWorksOnceAndSignsEveryoneOut(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	setupOwner(t, f, c)
	if code, _ := askCode(t, f, nil); code != http.StatusAccepted {
		t.Fatalf("ask: %d", code)
	}
	code := codeFromFile(t, f)
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(filepath.Join(f.s.Dir(), ResetCodeFile)); st.Mode().Perm() != 0o600 {
			t.Fatalf("code file mode %v, want 0600", st.Mode().Perm())
		}
	}
	// Check gives the owner name to fill in and does not use the code.
	b, _ := json.Marshal(resetBody{Code: strings.ToLower(strings.ReplaceAll(code, "-", " "))})
	resp, body := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/owner-reset/check", string(b), nil)
	if resp.StatusCode != 200 || !strings.Contains(body, `"luis"`) {
		t.Fatalf("check: %d %s", resp.StatusCode, body)
	}
	// A bad new password does not use the code up.
	if st, _ := finishReset(t, f, code, "luis", "x"); st != http.StatusBadRequest {
		t.Fatalf("short password: %d", st)
	}
	if st, body := finishReset(t, f, code, "boss", "a brand new passphrase"); st != 200 {
		t.Fatalf("finish: %d %s", st, body)
	}
	if !codeFileGone(t, f) {
		t.Fatal("code file left after use")
	}
	// The old session ended; the new login works; the code is spent.
	if resp, _ := call(t, c, "GET", f.srv.URL+"/api/v1/auth/me", "", nil); resp.StatusCode != 401 {
		t.Fatalf("old session still works: %d", resp.StatusCode)
	}
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"boss","password":"a brand new passphrase"}`, nil); resp.StatusCode != 200 {
		t.Fatalf("new login: %d", resp.StatusCode)
	}
	if st, _ := finishReset(t, f, code, "boss", "another passphrase"); st != 401 {
		t.Fatalf("code used twice: %d", st)
	}
	// The audit trail says how.
	var n int
	_ = f.s.ReadEvents().QueryRow(`SELECT count(*) FROM events WHERE summary LIKE '%one-time code from the log%'`).Scan(&n)
	if n != 1 {
		t.Fatalf("audit events: %d", n)
	}
}

func TestOwnerResetCodeExpires(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	askCode(t, f, nil)
	code := codeFromFile(t, f)
	f.clock.add(ResetCodeLife + time.Second)
	if st, _ := finishReset(t, f, code, "luis", "a brand new passphrase"); st != 401 {
		t.Fatalf("expired code worked: %d", st)
	}
	if !codeFileGone(t, f) {
		t.Fatal("code file left after expiry")
	}
	// Cleanup removes an expired file too.
	f.clock.add(2 * time.Minute)
	askCode(t, f, nil)
	f.clock.add(ResetCodeLife + time.Second)
	if err := f.a.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !codeFileGone(t, f) {
		t.Fatal("cleanup left an expired code file")
	}
}

func TestOwnerResetCodeEndsAfterFiveWrongTries(t *testing.T) {
	f := setup(t, Options{TrustedProxies: []string{"127.0.0.1"}})
	setupOwner(t, f, client(t))
	askCode(t, f, nil)
	code := codeFromFile(t, f)
	for i := 0; i < resetCodeTries; i++ {
		if st, _ := finishFrom(t, f, "10.0.1."+strconv.Itoa(i+1), "2222-2222-2222-2222-2222-2222", "luis", "a brand new passphrase"); st != 401 {
			t.Fatalf("wrong code %d: %d", i, st)
		}
	}
	if st, _ := finishFrom(t, f, "10.0.2.1", code, "luis", "a brand new passphrase"); st != 401 {
		t.Fatalf("right code worked after five wrong tries: %d", st)
	}
}

func TestOwnerResetOneLiveCode(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	askCode(t, f, nil)
	first := codeFromFile(t, f)
	f.clock.add(2 * time.Minute)
	askCode(t, f, nil)
	second := codeFromFile(t, f)
	if first == second {
		t.Fatal("same code twice")
	}
	if st, _ := finishReset(t, f, first, "luis", "a brand new passphrase"); st != 401 {
		t.Fatalf("replaced code worked: %d", st)
	}
	if st, _ := finishReset(t, f, second, "luis", "a brand new passphrase"); st != 200 {
		t.Fatalf("live code: %d", st)
	}
}

func TestOwnerResetRateLimitsAnswerTheSame(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	_, want := askCode(t, f, nil)
	first := codeFromFile(t, f)
	// Same address within a minute: no new code, same answer.
	if st, body := askCode(t, f, nil); st != http.StatusAccepted || body != want {
		t.Fatalf("limited answer differs: %d %s", st, body)
	}
	if codeFromFile(t, f) != first {
		t.Fatal("a second code within a minute from one address")
	}
	// Five an hour overall.
	l := &resetLimits{}
	now := time.Now()
	for i := 0; i < resetPerHour; i++ {
		if !l.allow("10.0.0."+string(rune('1'+i)), now) {
			t.Fatalf("request %d refused", i)
		}
	}
	if l.allow("10.0.0.9", now) {
		t.Fatal("sixth request in an hour allowed")
	}
	if !l.allow("10.0.0.9", now.Add(time.Hour)) {
		t.Fatal("still refused an hour later")
	}
}

func TestOwnerResetLinkSetting(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	csrf := setupOwner(t, f, c)
	_, body := call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/state", "", nil)
	if !strings.Contains(body, `"owner_reset":true`) {
		t.Fatalf("link not on by default: %s", body)
	}
	if resp, b := call(t, c, "PUT", f.srv.URL+"/api/v1/auth/settings", `{"owner_reset_link":false}`, map[string]string{CSRFHeader: csrf}); resp.StatusCode != 200 {
		t.Fatalf("turn off: %d %s", resp.StatusCode, b)
	}
	_, body = call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/state", "", nil)
	if strings.Contains(body, `"owner_reset"`) {
		t.Fatalf("link still shown: %s", body)
	}
	// Off: the page answers the same and makes no code.
	if st, _ := askCode(t, f, nil); st != http.StatusAccepted || !codeFileGone(t, f) {
		t.Fatalf("code made with the link off: %d", st)
	}
	// The command line still works while it is off.
	code, err := MakeOwnerResetCode(context.Background(), f.s, ResetCodeLife, f.clock.now())
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := finishReset(t, f, code, "luis", "a brand new passphrase"); st != 200 {
		t.Fatalf("command-line code with link off: %d", st)
	}
}

func TestOwnerResetNoOwnerMakesNoCode(t *testing.T) {
	f := setup(t, Options{})
	if st, _ := askCode(t, f, nil); st != http.StatusAccepted || !codeFileGone(t, f) {
		t.Fatalf("code made with no owner: %d", st)
	}
}

func TestOwnerResetEnvPrintsCodeAtStart(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	t.Setenv(EnvResetOwner, "1")
	a := New(f.s, Options{Now: f.clock.now, fastHash: true})
	t.Cleanup(a.Close)
	if !a.envReset {
		t.Fatal("env did not make a code")
	}
	code := codeFromFile(t, f)
	f.clock.add(ResetCodeLife + time.Minute) // the env code lasts 30 minutes
	if st, _ := finishReset(t, f, code, "luis", "a brand new passphrase"); st != 200 {
		t.Fatalf("env code: %d", st)
	}
}

// Wrong codes count per address too: an address that sent five is held off
// for a while, even with the right code, while others still get through.
func TestOwnerResetWrongTriesPerAddress(t *testing.T) {
	f := setup(t, Options{TrustedProxies: []string{"127.0.0.1"}})
	setupOwner(t, f, client(t))
	code, err := MakeOwnerResetCode(context.Background(), f.s, ResetCodeEnvLife, f.clock.now())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < resetWrongPerIP; i++ {
		if st := checkFrom(t, f, "10.0.0.5", "2222-2222-2222-2222-2222-2222"); st != 401 {
			t.Fatalf("wrong %d: %d", i, st)
		}
	}
	if st := checkFrom(t, f, "10.0.0.5", code); st != http.StatusTooManyRequests {
		t.Fatalf("held-off address: %d", st)
	}
	if st := checkFrom(t, f, "10.0.0.6", code); st != 200 {
		t.Fatalf("other address: %d", st)
	}
	f.clock.add(resetWrongWindow)
	if st := checkFrom(t, f, "10.0.0.5", code); st != 200 {
		t.Fatalf("after the window: %d", st)
	}
	// A forwarded header from an untrusted peer does not pick the address.
	g := setup(t, Options{})
	setupOwner(t, g, client(t))
	for i := 0; i < resetWrongPerIP; i++ {
		checkFrom(t, g, "10.0.9."+strconv.Itoa(i+1), "2222-2222-2222-2222-2222-2222")
	}
	if st := checkFrom(t, g, "10.0.9.99", "2222-2222-2222-2222-2222-2222"); st != http.StatusTooManyRequests {
		t.Fatalf("spoofed header dodged the limit: %d", st)
	}
}

// Check and finish are rate limited per address.
func TestOwnerResetCheckRateLimitPerAddress(t *testing.T) {
	f := setup(t, Options{TrustedProxies: []string{"127.0.0.1"}})
	setupOwner(t, f, client(t))
	code, err := MakeOwnerResetCode(context.Background(), f.s, ResetCodeLife, f.clock.now())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < resetTryPerMin; i++ {
		if st := checkFrom(t, f, "10.0.0.7", code); st != 200 {
			t.Fatalf("check %d: %d", i, st)
		}
	}
	if st := checkFrom(t, f, "10.0.0.7", code); st != http.StatusTooManyRequests {
		t.Fatalf("over the limit: %d", st)
	}
	if st, _ := finishFrom(t, f, "10.0.0.7", code, "luis", "a brand new passphrase"); st != http.StatusTooManyRequests {
		t.Fatalf("finish over the limit: %d", st)
	}
	if st := checkFrom(t, f, "10.0.0.8", code); st != 200 {
		t.Fatalf("other address: %d", st)
	}
	f.clock.add(time.Minute)
	if st, body := finishFrom(t, f, "10.0.0.7", code, "luis", "a brand new passphrase"); st != 200 {
		t.Fatalf("after a minute: %d %s", st, body)
	}
}

// A code asked for on the sign-in page never replaces or ends a code made on
// the host (reset-owner --code or HOLYARRS_RESET_OWNER), and strangers'
// wrong tries do not easily end it either.
func TestOwnerResetWebKeepsHostCode(t *testing.T) {
	f := setup(t, Options{TrustedProxies: []string{"127.0.0.1"}})
	setupOwner(t, f, client(t))
	host, err := MakeOwnerResetCode(context.Background(), f.s, ResetCodeEnvLife, f.clock.now())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		askCode(t, f, map[string]string{"X-Forwarded-For": "10.0.3." + strconv.Itoa(i+1)})
	}
	if codeFromFile(t, f) != host {
		t.Fatal("a sign-in page request replaced the host code in the file")
	}
	// Wrong tries from many addresses end the web code but not the host code.
	for i := 0; i < resetCodeTries+2; i++ {
		finishFrom(t, f, "10.0.4."+strconv.Itoa(i+1), "2222-2222-2222-2222-2222-2222", "luis", "a brand new passphrase")
	}
	if st, body := finishFrom(t, f, "10.0.5.1", host, "luis", "a brand new passphrase"); st != 200 {
		t.Fatalf("host code after web requests: %d %s", st, body)
	}
	if !codeFileGone(t, f) {
		t.Fatal("code file left after use")
	}
}

// A sign-in page code still works beside a live host code, and using either
// ends both.
func TestOwnerResetWebCodeBesideHostCode(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	host, err := MakeOwnerResetCode(context.Background(), f.s, ResetCodeEnvLife, f.clock.now())
	if err != nil {
		t.Fatal(err)
	}
	web, err := makeResetCode(context.Background(), f.s, keyResetCode, ResetCodeLife, f.clock.now())
	if err != nil {
		t.Fatal(err)
	}
	if st, body := finishReset(t, f, web, "luis", "a brand new passphrase"); st != 200 {
		t.Fatalf("web code: %d %s", st, body)
	}
	if st, _ := finishReset(t, f, host, "luis", "another passphrase!"); st != 401 {
		t.Fatalf("host code still worked after a reset: %d", st)
	}
}

func TestOwnerResetCodeTwoAtOnceOnlyOneWins(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	for round := 0; round < 5; round++ {
		code, err := MakeOwnerResetCode(context.Background(), f.s, ResetCodeLife, f.a.now())
		if err != nil {
			t.Fatal(err)
		}
		const n = 8
		wins := make(chan bool, n)
		start := make(chan struct{})
		for i := 0; i < n; i++ {
			go func() {
				<-start
				wins <- f.a.checkResetCode(context.Background(), code, true)
			}()
		}
		close(start)
		got := 0
		for i := 0; i < n; i++ {
			if <-wins {
				got++
			}
		}
		if got != 1 {
			t.Fatalf("round %d: %d requests used one code, want 1", round, got)
		}
	}
}
