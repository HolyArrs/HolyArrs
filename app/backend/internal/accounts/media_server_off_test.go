package accounts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// offServer stands in for a media server that answers 403 for an account
// turned off on it, with the body the real servers send: Emby says so in
// words, and only with the right password ("off"); Jellyfin sends its plain
// error text for any password ("shut"). "proxy" gets a 403 page from
// something in front of the server instead. Everything else is 401.
func offServer(t *testing.T, kind string) *httptest.Server {
	refusal := "Error processing request."
	if kind == "emby" {
		refusal = "Your account has been disabled. Please contact your server administrator for more information."
	}
	m := http.NewServeMux()
	m.HandleFunc("POST /Users/AuthenticateByName", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Username, Pw string }
		_ = json.NewDecoder(r.Body).Decode(&b)
		switch {
		case b.Username == "off" && b.Pw == "right", b.Username == "shut":
			http.Error(w, refusal, http.StatusForbidden)
		case b.Username == "proxy":
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("<html><head><title>403 Forbidden</title></head><body><center><h1>403 Forbidden</h1></center><hr><center>nginx</center></body></html>"))
		case b.Username == "boss" && b.Pw == "right":
			_, _ = w.Write([]byte(`{"User":{"Id":"boss1","Name":"Boss","Policy":{"IsAdministrator":true,"IsDisabled":true}},"AccessToken":"tok"}`))
		default:
			http.Error(w, "refused", http.StatusUnauthorized)
		}
	})
	m.HandleFunc("POST /Sessions/Logout", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	s := httptest.NewServer(m)
	t.Cleanup(s.Close)
	return s
}

var offKinds = map[string]string{"jellyfin": "Jellyfin", "emby": "Emby"}

func offLogin(t *testing.T, kind string) (*fixture, func(name, pass string) (int, string)) {
	ms := offServer(t, kind)
	f := gated(t, Options{})
	own := ownerOf(t, f)
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"`+kind+`":{"on":true,"url":"`+ms.URL+`","join":"everyone"},"confirm_everyone":true}`); resp.StatusCode != 200 {
		t.Fatalf("way on: %d %s", resp.StatusCode, out)
	}
	c := client(t)
	return f, func(name, pass string) (int, string) {
		b, _ := json.Marshal(map[string]string{"name": name, "password": pass})
		resp, out := call(t, c, "POST", f.srv.URL+"/api/v1/auth/media-server/"+kind, string(b), nil)
		return resp.StatusCode, said(t, out)
	}
}

// Someone whose account is turned off on the Jellyfin or Emby server is told
// so, by the server's name and with what to do next. The try does not count
// against their name, but it still counts at their address.
func TestTurnedOffOnTheMediaServerIsSaid(t *testing.T) {
	for kind, word := range offKinds {
		t.Run(kind, func(t *testing.T) {
			want := "Your account is turned off on the " + word + " server. Ask the person who runs it to turn it back on."
			if !wordFileKnows(t, want) {
				t.Fatalf("the word file lacks %q", want)
			}
			for _, who := range [][2]string{{"off", "right"}, {"shut", "any"}, {"boss", "right"}} {
				f, login := offLogin(t, kind)
				if code, got := login(who[0], who[1]); code != 403 || got != want {
					t.Fatalf("%s: %d %q", who[0], code, got)
				}
				f.a.mu.Lock()
				byName := f.a.fails["name:"+kind+":"+who[0]]
				f.a.mu.Unlock()
				if byName != nil {
					t.Fatalf("%s: the turned-off try counted against the name", who[0])
				}
			}
		})
	}
}

// Turned-off tries are still limited per address: 20 in a row from one
// address run into the address limit.
func TestTurnedOffTriesHitTheAddressLimit(t *testing.T) {
	for kind := range offKinds {
		t.Run(kind, func(t *testing.T) {
			_, login := offLogin(t, kind)
			limited := false
			for i := 0; i < 20 && !limited; i++ {
				code, _ := login("off", "right")
				limited = code == 429
				if !limited && code != 403 {
					t.Fatalf("try %d: %d", i+1, code)
				}
			}
			if !limited {
				t.Fatal("20 turned-off tries from one address were never limited")
			}
		})
	}
}

// A wrong password still counts against the name.
func TestWrongPasswordStillCountsByName(t *testing.T) {
	for kind := range offKinds {
		f, login := offLogin(t, kind)
		if code, got := login("off", "wrong"); code != 401 || got != "wrong name or password" {
			t.Fatalf("%s: %d %q", kind, code, got)
		}
		f.a.mu.Lock()
		byName := f.a.fails["name:"+kind+":off"]
		f.a.mu.Unlock()
		if byName == nil {
			t.Fatalf("%s: a wrong password did not count against the name", kind)
		}
	}
}

// A 403 from something in front of the server (a proxy, a firewall page) is
// not taken as "turned off": it reads and counts as any failed answer.
func TestA403FromAProxyIsNotTurnedOff(t *testing.T) {
	for kind := range offKinds {
		f, login := offLogin(t, kind)
		if code, got := login("proxy", "x"); code != 401 || got != "wrong name or password" {
			t.Fatalf("%s: %d %q", kind, code, got)
		}
		f.a.mu.Lock()
		byName := f.a.fails["name:"+kind+":proxy"]
		f.a.mu.Unlock()
		if byName == nil {
			t.Fatalf("%s: the proxy's 403 did not count", kind)
		}
	}
}

// First run says the same when the admin account is turned off on the
// server, and a proxy's 403 reads as an address that did not answer.
func TestSetupSaysTurnedOffOnTheMediaServer(t *testing.T) {
	for kind, word := range offKinds {
		ms := offServer(t, kind)
		f := gated(t, Options{})
		cases := map[string]string{
			"off":   "Your account is turned off on the " + word + " server. Ask the person who runs it to turn it back on.",
			"boss":  "Your account is turned off on the " + word + " server. Ask the person who runs it to turn it back on.",
			"proxy": "that address did not answer like " + word + "; check it and try again",
		}
		for name, want := range cases {
			b, _ := json.Marshal(map[string]string{"url": ms.URL, "name": name, "password": "right"})
			_, out := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/setup/media-server/"+kind, string(b), nil)
			if got := said(t, out); got != want {
				t.Fatalf("%s %s: %q", kind, name, got)
			}
		}
	}
}
