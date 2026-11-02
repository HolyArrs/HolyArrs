package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigurationRejectsUnknownAndUnsafeBounds(t *testing.T) {
	for _, body := range []string{`{"data_dir":"/data","providers":{"tmdb":{}},"unknown":true}`, `{"data_dir":"/data","providers":{"tmdb":{}},"burst":-1}`, `{"data_dir":"/data","providers":{}}`, `{"data_dir":"/data","providers":{"tmdb":{}}} {}`} {
		p := filepath.Join(t.TempDir(), "config.json")
		os.WriteFile(p, []byte(body), 0600)
		if _, e := readConfig(p); e == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}
func TestCredentialIssueDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "token")
	os.WriteFile(p, []byte("preserve"), 0600)
	if e := run([]string{"token-issue", "--data-dir", filepath.Join(dir, "data"), "--token-file", p}); e == nil {
		t.Fatal("overwrote token")
	}
	b, _ := os.ReadFile(p)
	if string(b) != "preserve" {
		t.Fatal("changed credential")
	}
}

func TestPrivateModeRejectsOfficialEndpointsAndProviderCredentials(t *testing.T) {
	for _, body := range []string{`{"data_dir":"/data","private_fixtures":true,"providers":{"tmdb":{}}}`, `{"data_dir":"/data","private_fixtures":true,"providers":{"tmdb":{"base_url":"http://fixture:8081/tmdb/3","key_file":"/secret"}}}`} {
		p := filepath.Join(t.TempDir(), "config.json")
		os.WriteFile(p, []byte(body), 0600)
		if _, e := readConfig(p); e == nil {
			t.Fatal("accepted fixture outbound escape")
		}
	}
}

func TestCredentialFileRegistrationRecoveryAndDurableRevoke(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	token := filepath.Join(dir, "token")
	os.WriteFile(token, []byte("synthetic-recovery-credential-long-enough"), 0600)
	for _, operation := range []string{"token-register", "token-revoke", "check"} {
		if e := run([]string{operation, "--data-dir", data, "--token-file", token}); e != nil {
			t.Fatalf("%s: %v", operation, e)
		}
	}
}

func TestKeyEnvOnlyForTMDBAndHealthcheck(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(`{"data_dir":"/data","providers":{"musicbrainz":{"key_env":"X"}}}`), 0600)
	if _, e := readConfig(p); e == nil {
		t.Fatal("accepted music credential")
	}
	os.WriteFile(p, []byte(`{"data_dir":"/data","providers":{"tmdb":{"key_env":"HOLYARRS_TMDB_READ_TOKEN"}},"client_per_minute":120,"client_burst":30,"stats_token_env":"HOLYARRS_STATS_TOKEN","user_agent":"HolyArrsMetadata/0.2 (+https://example.invalid)"}`), 0600)
	if _, e := readConfig(p); e != nil {
		t.Fatal(e)
	}
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			w.WriteHeader(404)
		}
	}))
	defer ok.Close()
	if e := run([]string{"healthcheck", "--url", ok.URL + "/readyz"}); e != nil {
		t.Fatal(e)
	}
	if e := run([]string{"healthcheck", "--url", ok.URL + "/other"}); e == nil {
		t.Fatal("unhealthy accepted")
	}
}

func TestTrustedProxiesMustBeNetworks(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(`{"data_dir":"/data","providers":{"musicbrainz":{}},"trusted_proxies":["not-a-cidr"]}`), 0600)
	if _, e := readConfig(p); e == nil {
		t.Fatal("accepted invalid proxy network")
	}
	os.WriteFile(p, []byte(`{"data_dir":"/data","providers":{"musicbrainz":{}},"trusted_proxies":["172.18.0.0/16"]}`), 0600)
	if _, e := readConfig(p); e != nil {
		t.Fatal(e)
	}
}

func TestServiceTopKeys(t *testing.T) {
	st := &serviceTopConfig{KeyEnv: "P", BackupKeyEnv: "B", ActiveEnv: "A"}
	env := map[string]string{"P": "primary-k", "B": " backup-k\n"}
	get := func(k string) string { return env[k] }
	if k, err := serviceTopKeys(st, get); err != nil || len(k) != 1 || k[0] != "backup-k" {
		t.Fatalf("default %v %v", k, err)
	}
	env["A"] = "primary"
	if k, _ := serviceTopKeys(st, get); len(k) != 2 || k[0] != "primary-k" || k[1] != "backup-k" {
		t.Fatalf("primary %v", k)
	}
	env["A"] = "other"
	if _, err := serviceTopKeys(st, get); err == nil {
		t.Fatal("unknown choice accepted")
	}
}
