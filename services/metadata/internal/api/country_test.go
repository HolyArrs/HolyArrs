package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	"github.com/CoderLuii/holyarrs-metadata/internal/stats"
	"github.com/CoderLuii/holyarrs-metadata/internal/store"
)

// countryServer is a server on the real store with open registration.
func countryServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	rec, err := stats.Open(filepath.Join(t.TempDir(), "stats.sqlite"), stats.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rec.Close() })
	f := fetchFunc(func(context.Context, model.Query) (model.Result, error) {
		return model.Result{Data: json.RawMessage(`{"title":"Synthetic"}`), Complete: true}, nil
	})
	return New(st, f, Options{Registration: true, Burst: 100, TrustedProxies: testProxy(), Stats: rec, StatsToken: statsFixture})
}

// lookupFrom reads lookup with token, a country header and a count header,
// from a peer (the trusted proxy when "").
func lookupFrom(t *testing.T, s *Server, token, country, count, peer string) {
	t.Helper()
	r := httptest.NewRequest("GET", lookup, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("CF-Connecting-IP", "203.0.113.50")
	if country != "" {
		r.Header.Set("CF-IPCountry", country)
	}
	if count != "" {
		r.Header.Set(CountHeader, count)
	}
	if peer != "" {
		r.RemoteAddr = peer
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("lookup %d %s", w.Code, w.Body.String())
	}
}

type countryCount struct {
	InstallCountries struct {
		Days      int            `json:"days"`
		Total     int            `json:"total"`
		Countries map[string]int `json:"countries"`
	} `json:"install_countries"`
}

func countries(t *testing.T, s *Server) (countryCount, string) {
	t.Helper()
	w := req(s, "/v1/stats", statsFixture)
	if w.Code != 200 {
		t.Fatalf("stats %d %s", w.Code, w.Body.String())
	}
	var c countryCount
	if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	return c, w.Body.String()
}

// CF-IPCountry is read only from the trusted proxy; unknown and Tor are
// not countries; the statistics hold counts only.
func TestInstallCountryFromTrustedProxyOnly(t *testing.T) {
	s := countryServer(t)
	de, faked, unknown, tor := registerToken(t, s, "203.0.113.50"), registerToken(t, s, "203.0.113.50"), registerToken(t, s, "203.0.113.51"), registerToken(t, s, "203.0.113.52")
	lookupFrom(t, s, de, "de", "", "")
	lookupFrom(t, s, faked, "FR", "", "198.51.100.9:1234")
	lookupFrom(t, s, unknown, "XX", "", "")
	lookupFrom(t, s, tor, "T1", "", "")
	c, body := countries(t, s)
	if c.InstallCountries.Days != 30 || c.InstallCountries.Total != 1 || len(c.InstallCountries.Countries) != 1 || c.InstallCountries.Countries["DE"] != 1 {
		t.Fatalf("install countries %+v", c.InstallCountries)
	}
	for _, leak := range []string{de, faked, "203.0.113.", "198.51.100."} {
		if strings.Contains(body, leak) {
			t.Fatalf("statistics expose %q", leak)
		}
	}
}

// An install that sends the count header as off is cleared and not counted.
func TestInstallCountryOffSwitch(t *testing.T) {
	s := countryServer(t)
	a, b := registerToken(t, s, "203.0.113.60"), registerToken(t, s, "203.0.113.61")
	lookupFrom(t, s, a, "DE", "", "")
	lookupFrom(t, s, b, "SE", "off", "")
	if c, _ := countries(t, s); c.InstallCountries.Total != 1 || c.InstallCountries.Countries["DE"] != 1 {
		t.Fatalf("before off %+v", c.InstallCountries)
	}
	lookupFrom(t, s, a, "DE", "OFF", "")
	if c, _ := countries(t, s); c.InstallCountries.Total != 0 || len(c.InstallCountries.Countries) != 0 {
		t.Fatalf("after off %+v", c.InstallCountries)
	}
}
