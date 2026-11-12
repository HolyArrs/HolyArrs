package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

// localCSRF is the CSRF token a local visit (sign-in skipped) gets for the
// local cookie in c's jar, as the web app reads it from /auth/me.
func localCSRF(t *testing.T, base string, c *http.Client) string {
	t.Helper()
	resp, err := c.Get(base + "/api/v1/auth/me")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var v struct {
		CSRF string `json:"csrf"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&v) != nil || v.CSRF == "" {
		t.Fatalf("local visit: %d, no CSRF token", resp.StatusCode)
	}
	return v.CSRF
}
