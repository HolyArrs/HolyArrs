package downloads

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// qBittorrent signs in with an API key or with a user name and password;
// saving with neither is refused, the secret is masked, and a masked value
// sent back keeps it.
func TestQbitDownloaderKeyOrPassword(t *testing.T) {
	t.Parallel()
	srv, _ := dlServer(t)
	logins := 0
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch {
		case r.URL.Path == "/api/v2/auth/login":
			logins++
			if r.PostForm.Get("username") == "admin" && r.PostForm.Get("password") == "your-test-word" {
				http.SetCookie(w, &http.Cookie{Name: "QBT_SID_8090", Value: "s", Path: "/"})
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
		case r.Header.Get("Authorization") == "Bearer your-test-key":
			_, _ = w.Write([]byte("v5.2.4"))
		case strings.Contains(r.Header.Get("Cookie"), "QBT_SID_8090=s"):
			_, _ = w.Write([]byte("v5.1.2"))
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer qb.Close()
	base := srv.URL + "/api/v1/downloaders"
	do(t, "POST", base, `{"name":"None","kind":"qbittorrent","url":"`+qb.URL+`","allow_local_network":true}`, 400)
	do(t, "POST", base, `{"name":"NoPw","kind":"qbittorrent","url":"`+qb.URL+`","username":"admin","allow_local_network":true}`, 400)

	var key, pw DownloaderView
	_ = json.Unmarshal(do(t, "POST", base, `{"name":"Key","kind":"qbittorrent","url":"`+qb.URL+`","secret":"your-test-key","allow_local_network":true}`, 201), &key)
	_ = json.Unmarshal(do(t, "POST", base, `{"name":"Pw","kind":"qbittorrent","url":"`+qb.URL+`/","username":"admin","secret":"your-test-word","allow_local_network":true,"priority":2}`, 201), &pw)
	if key.Secret != secrets.Masked || pw.Secret != secrets.Masked || pw.Username != "admin" || key.Username != "" {
		t.Fatalf("views %+v %+v", key, pw)
	}
	for _, d := range []DownloaderView{key, pw} {
		var out map[string]any
		_ = json.Unmarshal(do(t, "POST", base+"/"+itoa(d.ID)+"/test", "", 200), &out)
		if out["ok"] != true {
			t.Fatalf("%s test: %v", d.Name, out)
		}
	}
	// A masked password sent back keeps the stored one.
	masked, _ := json.Marshal(map[string]any{"name": "Pw", "url": qb.URL + "/", "username": "admin", "secret": pw.Secret, "allow_local_network": true})
	do(t, "PUT", base+"/"+itoa(pw.ID), string(masked), 200)
	var out map[string]any
	_ = json.Unmarshal(do(t, "POST", base+"/"+itoa(pw.ID)+"/test", "", 200), &out)
	if out["ok"] != true {
		t.Fatalf("after masked edit: %v", out)
	}
	// Switching from key to password needs the password typed.
	do(t, "PUT", base+"/"+itoa(key.ID), `{"name":"Key","url":"`+qb.URL+`","username":"admin","allow_local_network":true}`, 400)
	// A wrong password says so in plain words.
	do(t, "PUT", base+"/"+itoa(pw.ID), `{"name":"Pw","url":"`+qb.URL+`/","username":"admin","secret":"wrong","allow_local_network":true}`, 200)
	_ = json.Unmarshal(do(t, "POST", base+"/"+itoa(pw.ID)+"/test", "", 200), &out)
	if out["ok"] != false || !strings.Contains(out["text"].(string), "turned down") {
		t.Fatalf("wrong password: %v", out)
	}
}
