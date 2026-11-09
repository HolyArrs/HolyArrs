package music

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// API and New give the tests a music Handlers over a test Server.
type API = testAPI

func New(s Server) *API { return newTestAPI(s) }

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

func getJSON(t *testing.T, url string, want int) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("%s: %d, want %d", url, resp.StatusCode, want)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
