package setup

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// TestConfirmIDsAreCapped: one person cannot fill memory with confirm ids.
func TestConfirmIDsAreCapped(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	a := &Handlers{Deps: Deps{Store: st, ExitApp: func(string) error { return nil }}}
	m := http.NewServeMux()
	a.Routes(m)
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	u := srv.URL + "/api/v1/"
	for i := 0; i < webserver.MaxConfirmsPerUser+50; i++ {
		do(t, "POST", u+"app/restart", "", 202)
	}
	n := a.ConfirmCount()
	if n > webserver.MaxConfirmsPerUser {
		t.Fatalf("%d confirm ids kept, cap %d", n, webserver.MaxConfirmsPerUser)
	}
	// The newest id still works.
	confirmTwice(t, u+"app/restart", 202)
	defer func() { webserver.ConfirmNow = time.Now }()
	webserver.ConfirmNow = func() time.Time { return time.Now().Add(2 * webserver.ConfirmFor) }
	do(t, "POST", u+"app/restart", "", 202) // purges the expired ones
	n = a.ConfirmCount()
	if n != 1 {
		t.Fatalf("expired ids not purged: %d left", n)
	}
}

func do(t *testing.T, method, url, body string, want int) []byte {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		t.Fatalf("%s %s: %d, want %d: %s", method, url, resp.StatusCode, want, b)
	}
	return b
}

// confirmTwice runs a two-step control: the first call answers 202 with a
// confirm id; the second, with the id, answers want.
func confirmTwice(t *testing.T, url string, want int) {
	t.Helper()
	var ask struct {
		Confirm string `json:"confirm"`
	}
	if err := json.Unmarshal(do(t, "POST", url, "", 202), &ask); err != nil || ask.Confirm == "" {
		t.Fatalf("no confirm id: %v", err)
	}
	do(t, "POST", url, `{"confirm":"`+ask.Confirm+`"}`, want)
}
