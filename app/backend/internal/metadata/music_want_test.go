package metadata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWantMusicPackageStatuses(t *testing.T) {
	mbid := "B8A7C51F-362C-4DCB-A259-BC6E0095F0A6"
	var code int
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/music/package-want/"+strings.ToLower(mbid) || r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("asked %s auth %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		w.WriteHeader(code)
	}))
	t.Cleanup(hs.Close)
	mk := func(tok string) *Service {
		s, err := NewService(ServiceOptions{URL: hs.URL, HTTP: hs.Client(), AllowLocal: true,
			Access: func(context.Context) (string, error) { return tok, nil }})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := mk("tok")
	for _, c := range []struct {
		code int
		want error
	}{{202, nil}, {200, ErrPackageCovered}, {404, ErrNotFound}, {429, ErrServiceBusy}, {403, ErrServiceAccess}, {500, ErrServiceUnavailable}} {
		code = c.code
		err := s.WantMusicPackage(context.Background(), mbid)
		if c.want == nil && err != nil || c.want != nil && !errors.Is(err, c.want) {
			t.Errorf("status %d: got %v want %v", c.code, err, c.want)
		}
	}

	// Refused calls must not reach the server: swap in a counting handler.
	var hits int
	hs2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	t.Cleanup(hs2.Close)
	mk2 := func(tok string) *Service {
		s, _ := NewService(ServiceOptions{URL: hs2.URL, HTTP: hs2.Client(), AllowLocal: true,
			Access: func(context.Context) (string, error) { return tok, nil }})
		return s
	}
	if err := mk2("tok").WantMusicPackage(context.Background(), "not-an-id"); !errors.Is(err, ErrNotFound) {
		t.Errorf("bad mbid: %v", err)
	}
	if err := mk2("to\nk").WantMusicPackage(context.Background(), mbid); !errors.Is(err, ErrServiceAccess) {
		t.Errorf("newline token: %v", err)
	}
	if hits != 0 {
		t.Errorf("%d requests sent for refused calls", hits)
	}
}
