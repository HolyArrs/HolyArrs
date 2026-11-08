package api

import (
	"encoding/json"
	"strings"
	"testing"
)

// myProfile is the answer of the profile routes.
type myProfile struct {
	DisplayName string `json:"display_name"`
	Username    string `json:"username"`
}

func TestMyProfileDisplayName(t *testing.T) {
	r := newAskRig(t)
	var p myProfile
	if err := json.Unmarshal(r.ana.call("GET", "/api/v1/my/profile", "", 200), &p); err != nil {
		t.Fatal(err)
	}
	if p.DisplayName != "" || p.Username != "ana" {
		t.Fatalf("fresh profile %+v", p)
	}
	if err := json.Unmarshal(r.ana.call("PUT", "/api/v1/my/profile", `{"display_name":"  Ana B.  "}`, 200), &p); err != nil {
		t.Fatal(err)
	}
	if p.DisplayName != "Ana B." || p.Username != "ana" {
		t.Fatalf("saved profile %+v", p)
	}
	// Each person edits only their own: bo still has none.
	_ = json.Unmarshal(r.bo.call("GET", "/api/v1/my/profile", "", 200), &p)
	if p.DisplayName != "" || p.Username != "bo" {
		t.Fatalf("bo's profile %+v", p)
	}
	// Too long or control characters are refused; extra fields (role, name) are ignored.
	r.ana.call("PUT", "/api/v1/my/profile", `{"display_name":"`+strings.Repeat("x", 65)+`"}`, 400)
	r.ana.call("PUT", "/api/v1/my/profile", `{"display_name":"a\u0007b"}`, 400)
	r.ana.call("PUT", "/api/v1/my/profile", `{"display_name":"Ana","role":"owner","name":"root"}`, 200)
	var me struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/auth/me", "", 200), &me)
	if me.Name != "ana" || me.Role == "owner" {
		t.Fatalf("profile edit changed sign-in name or role: %+v", me)
	}
	// Empty clears it back to the username.
	_ = json.Unmarshal(r.ana.call("PUT", "/api/v1/my/profile", `{"display_name":""}`, 200), &p)
	if p.DisplayName != "" {
		t.Fatalf("cleared profile %+v", p)
	}
}
