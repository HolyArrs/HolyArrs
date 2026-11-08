package metadata

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// fixtureInstall is a made-up installation credential.
var fixtureInstall = strings.Join([]string{"hai", "fixture", "install", "credential"}, "_")

func testVault(t *testing.T) *secrets.Vault {
	t.Helper()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	v, err := secrets.Open(context.Background(), st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// fakeMetadataServer registers installs and answers TMDB reads at /v1/tmdb.
func fakeMetadataServer(t *testing.T, registered *atomic.Int32, tmdb *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/installations":
			registered.Add(1)
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"schema_version":1,"` + "tok" + `en":"` + fixtureInstall + `"}`))
		case strings.HasPrefix(r.URL.Path, "/v1/tmdb/"):
			if r.Header.Get("Authorization") != "Bearer "+fixtureInstall {
				w.WriteHeader(401)
				return
			}
			tmdb.Add(1)
			_, _ = w.Write([]byte(`{"page":1,"total_pages":1,"total_results":1,"results":[{"id":603,"title":"Fixture Film","release_date":"1999-03-31"}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFreshInstallRegistersOnceAndSearchesThroughServer(t *testing.T) {
	var registered, tmdb atomic.Int32
	srv := fakeMetadataServer(t, &registered, &tmdb)
	t.Setenv(EnvServiceURL, srv.URL)
	t.Setenv("HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL", "true")
	t.Setenv("HOLYARRS_METADATA_SERVICE_TOKEN_FILE", "")
	t.Setenv(EnvTMDBKey, "")
	t.Setenv(EnvTokenFile, "")
	t.Setenv("HOLYARRS_TMDB_URL", "")
	v := testVault(t)
	h, err := NewHosted(v)
	if err != nil {
		t.Fatal(err)
	}
	if h.OwnKey || h.TMDBBase != srv.URL+"/v1/tmdb" {
		t.Fatalf("base %q", h.TMDBBase)
	}
	for _, q := range []string{"Fixture", "Other"} {
		ms, err := h.TMDB().Search(context.Background(), q, 0)
		if err != nil || len(ms) != 1 || ms[0].Name != "Fixture Film" {
			t.Fatalf("%v %v", ms, err)
		}
	}
	if registered.Load() != 1 || tmdb.Load() != 2 {
		t.Fatalf("registered %d, tmdb %d", registered.Load(), tmdb.Load())
	}
	tok, _ := v.Setting(context.Background(), installationGroup, installationKey)
	if tok != fixtureInstall {
		t.Fatal("credential not kept")
	}
	// A restart reuses the kept credential.
	h, _ = NewHosted(v)
	if _, err := h.TMDB().Search(context.Background(), "Again", 0); err != nil || registered.Load() != 1 {
		t.Fatalf("registered again: %d %v", registered.Load(), err)
	}
}

func TestRefusedCredentialIsNotReplaced(t *testing.T) {
	var registered, tmdb atomic.Int32
	srv := fakeMetadataServer(t, &registered, &tmdb)
	t.Setenv(EnvServiceURL, srv.URL)
	t.Setenv("HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL", "true")
	t.Setenv("HOLYARRS_METADATA_SERVICE_TOKEN_FILE", "")
	t.Setenv(EnvTMDBKey, "")
	t.Setenv(EnvTokenFile, "")
	v := testVault(t)
	if err := v.SetSetting(context.Background(), installationGroup, installationKey, "hai_revoked_fixture", database.System); err != nil {
		t.Fatal(err)
	}
	h, err := NewHosted(v)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.TMDB().Search(context.Background(), "Fixture", 0); err == nil {
		t.Fatal("refused credential answered")
	}
	if registered.Load() != 0 {
		t.Fatal("re-registered after a refusal")
	}
}

func TestOwnTMDBKeyGoesDirect(t *testing.T) {
	t.Setenv(EnvServiceURL, "")
	t.Setenv(EnvTMDBKey, "own-fixture-key")
	t.Setenv(EnvTokenFile, "")
	t.Setenv("HOLYARRS_TMDB_URL", "")
	h, err := NewHosted(nil)
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := h.TMDBAccess(context.Background())
	if !h.OwnKey || h.TMDBBase != "" || tok != "own-fixture-key" {
		t.Fatalf("own key: %+v", h)
	}
	if h.Service.Base() != DefaultServiceURL {
		t.Fatal(h.Service.Base())
	}
}

func TestMetadataURLAndAddressPolicy(t *testing.T) {
	for _, ip := range []string{"169.254.169.254", "100.100.100.200", "fd00:ec2::254", "0.0.0.0", "::", "224.0.0.1", "ff02::1", "255.255.255.255"} {
		if outbound.MetadataServiceLocal(net.ParseIP(ip)) {
			t.Fatalf("unsafe local policy: %s", ip)
		}
	}
	for _, ip := range []string{"127.0.0.1", "::1", "192.168.1.10"} {
		if !outbound.MetadataServiceLocal(net.ParseIP(ip)) {
			t.Fatalf("private endpoint denied: %s", ip)
		}
	}
	t.Setenv(EnvServiceURL, "http://example.com")
	t.Setenv("HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL", "true")
	if _, err := NewHosted(nil); err == nil {
		t.Fatal("public bearer HTTP accepted")
	}
	t.Setenv("HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL", "yes-please")
	if _, err := NewHosted(nil); err == nil {
		t.Fatal("invalid opt-in accepted")
	}
}

func TestInstallationCredentialFileReadOnEachCall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installation.txt")
	if err := os.WriteFile(path, []byte("fixture-first"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOLYARRS_METADATA_SERVICE_TOKEN_FILE", path)
	access := installationAccess(nil, func(context.Context) (string, error) { t.Fatal("registered"); return "", nil })
	if tok, _ := access(context.Background()); tok != "fixture-first" {
		t.Fatal(tok)
	}
	if err := os.WriteFile(path, []byte("fixture-second"), 0600); err != nil {
		t.Fatal(err)
	}
	if tok, _ := access(context.Background()); tok != "fixture-second" {
		t.Fatal(tok)
	}
}
