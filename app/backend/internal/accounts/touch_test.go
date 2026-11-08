package accounts

import (
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func TestSessionPollingForThirtyMinutesDoesNotWrite(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	setupOwner(t, f, c)
	before := f.s.WriteCount()
	first := c.Jar.Cookies(mustURL(f.srv.URL))[0].Value

	for range 30 {
		f.clock.add(time.Minute)
		if resp, _ := call(t, c, http.MethodGet, f.srv.URL+"/api/v1/titles", "", nil); resp.StatusCode != http.StatusOK {
			t.Fatalf("poll status %d", resp.StatusCode)
		}
	}

	if got := f.s.WriteCount(); got != before {
		t.Fatalf("30 minutes of authenticated polling committed %d writes", got-before)
	}
	if got := c.Jar.Cookies(mustURL(f.srv.URL))[0].Value; got != first {
		t.Fatal("session rotated before its hourly boundary")
	}
}

func TestAPIKeyUseIsRecordedAtFirstUseAndAfterAnHour(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	csrf := setupOwner(t, f, c)
	resp, body := call(t, c, http.MethodPost, f.srv.URL+"/api/v1/auth/keys", `{"label":"poller"}`, map[string]string{CSRFHeader: csrf})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("make key: %d %s", resp.StatusCode, body)
	}
	var made struct {
		ID  int64  `json:"id"`
		Key string `json:"key"`
	}
	jsonOf(t, body, &made)
	readLastUsed := func() sql.NullString {
		t.Helper()
		var used sql.NullString
		if err := f.s.Read().QueryRow("SELECT last_used_at FROM api_keys WHERE id = ?", made.ID).Scan(&used); err != nil {
			t.Fatal(err)
		}
		return used
	}
	if used := readLastUsed(); used.Valid {
		t.Fatalf("new key already used at %q", used.String)
	}

	hdr := map[string]string{APIKeyHeader: made.Key}
	if resp, _ := call(t, &http.Client{}, http.MethodGet, f.srv.URL+"/api/v1/titles", "", hdr); resp.StatusCode != http.StatusOK {
		t.Fatalf("first key use: %d", resp.StatusCode)
	}
	first := readLastUsed()
	if !first.Valid || first.String != database.FormatTime(f.clock.now()) {
		t.Fatalf("first key use = %v, want %s", first, database.FormatTime(f.clock.now()))
	}
	before := f.s.WriteCount()

	f.clock.add(30 * time.Minute)
	if resp, _ := call(t, &http.Client{}, http.MethodGet, f.srv.URL+"/api/v1/titles", "", hdr); resp.StatusCode != http.StatusOK {
		t.Fatalf("key use inside hour: %d", resp.StatusCode)
	}
	if got := f.s.WriteCount(); got != before {
		t.Fatalf("key use inside hour committed %d writes", got-before)
	}
	if used := readLastUsed(); used.String != first.String {
		t.Fatalf("key use inside hour changed last use from %s to %s", first.String, used.String)
	}

	f.clock.add(30*time.Minute + time.Second)
	if resp, _ := call(t, &http.Client{}, http.MethodGet, f.srv.URL+"/api/v1/titles", "", hdr); resp.StatusCode != http.StatusOK {
		t.Fatalf("key use after hour: %d", resp.StatusCode)
	}
	if got := f.s.WriteCount(); got != before+1 {
		t.Fatalf("key use after hour write count = %d, want one", got-before)
	}
	if used := readLastUsed(); !used.Valid || used.String != database.FormatTime(f.clock.now()) {
		t.Fatalf("key use after hour = %v, want %s", used, database.FormatTime(f.clock.now()))
	}
}
