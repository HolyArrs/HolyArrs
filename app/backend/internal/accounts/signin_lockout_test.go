package accounts

import "testing"

// Behind a shared proxy one person's typos never lock out another account;
// behind a trusted proxy each visitor's own address is limited.
func TestLockoutPerAccountBehindProxy(t *testing.T) {
	login := func(t *testing.T, f *fixture, name, word, xff string) int {
		t.Helper()
		resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"`+name+`","pass`+`word":"`+word+`"}`,
			map[string]string{"X-Forwarded-For": xff})
		return resp.StatusCode
	}
	t.Run("untrusted proxy", func(t *testing.T) {
		f := setup(t, Options{})
		setupOwner(t, f, client(t))
		for i := 0; i < freeTries+2; i++ {
			login(t, f, "mallory", "wrong-wrong", "203.0.113.7")
		}
		if got := login(t, f, "luis", pw, "203.0.113.9"); got != 200 {
			t.Fatalf("owner after someone else's typos: %d", got)
		}
		limited := false
		for i := 0; i < freeUserTries+2 && !limited; i++ {
			limited = login(t, f, "luis", "wrong-wrong", "203.0.113.9") == 429
		}
		if !limited {
			t.Fatal("the owner's own bad tries never slowed the owner")
		}
	})
	t.Run("trusted proxy", func(t *testing.T) {
		f := setup(t, Options{TrustedProxies: []string{"127.0.0.1"}})
		setupOwner(t, f, client(t))
		limited := false
		for i := 0; i < freeTries+2 && !limited; i++ {
			limited = login(t, f, "mallory", "wrong-wrong", "203.0.113.7") == 429
		}
		if !limited {
			t.Fatal("the visitor's address was never slowed")
		}
		if got := login(t, f, "someone", "wrong-wrong", "203.0.113.8"); got == 429 {
			t.Fatal("another visitor was slowed")
		}
	})
}
