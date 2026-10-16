package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/secrets"
	"github.com/CoderLuii/holyarrs/internal/store"
)

func TestOptionalMetadataDisabledPreservesDirectProviders(t *testing.T) {
	t.Setenv("HOLYARRS_METADATA_SERVICE_URL", "")
	md := metadata.Lab()
	shows := metadata.ShowSources{"tmdb": &metadata.TMDB{}}
	artists := &metadata.MusicBrainz{}
	m, s, a, err := metadataServiceProviders(nil, md, shows, artists)
	if err != nil || m != md || s["tmdb"] != shows["tmdb"] || a != artists {
		t.Fatal("disabled service changed direct providers")
	}
}

func TestOptionalMetadataURLAndAddressPolicy(t *testing.T) {
	for _, ip := range []string{"169.254.169.254", "100.100.100.200", "fd00:ec2::254", "0.0.0.0", "::", "224.0.0.1", "ff02::1", "255.255.255.255"} {
		if metadataServiceLocal(net.ParseIP(ip)) {
			t.Fatalf("unsafe local policy: %s", ip)
		}
	}
	for _, ip := range []string{"127.0.0.1", "::1", "10.10.25.25"} {
		if !metadataServiceLocal(net.ParseIP(ip)) {
			t.Fatalf("private endpoint denied: %s", ip)
		}
	}
	t.Setenv("HOLYARRS_METADATA_SERVICE_URL", "http://example.com")
	t.Setenv("HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL", "true")
	if _, _, _, err := metadataServiceProviders(nil, metadata.Lab(), nil, nil); err == nil {
		t.Fatal("public bearer HTTP accepted")
	}
	t.Setenv("HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL", "yes-please")
	if _, _, _, err := metadataServiceProviders(nil, metadata.Lab(), nil, nil); err == nil {
		t.Fatal("invalid opt-in accepted")
	}
}

func TestOptionalMetadataReadsSeparateFileOnEachCall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installation.txt")
	if err := os.WriteFile(path, []byte("fixture-first"), 0600); err != nil {
		t.Fatal(err)
	}
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		want := "Bearer fixture-first"
		if calls > 1 {
			want = "Bearer fixture-second"
		}
		if r.Header.Get("Authorization") != want {
			t.Error("wrong separate installation credential")
		}
		w.WriteHeader(401)
	}))
	defer srv.Close()
	t.Setenv("HOLYARRS_METADATA_SERVICE_URL", srv.URL)
	t.Setenv("HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL", "true")
	t.Setenv("HOLYARRS_METADATA_SERVICE_TOKEN_FILE", path)
	md, _, _, err := metadataServiceProviders(nil, metadata.Lab(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = md.Search(context.Background(), "Fixture", 0)
	if err := os.WriteFile(path, []byte("fixture-second"), 0600); err != nil {
		t.Fatal(err)
	}
	_, _ = md.Search(context.Background(), "Fixture", 0)
	if calls != 2 {
		t.Fatalf("credential not checked per lookup: %d", calls)
	}
}

func TestOptionalMetadataVaultCredentialAndFilePrecedence(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	vault, err := secrets.Open(ctx, st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	if err := vault.SetSetting(ctx, "links", "metadata_service_installation_token", "METADATA_VAULT_TEST", domain.System); err != nil {
		t.Fatal(err)
	}
	want := "METADATA_VAULT_TEST"
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer "+want {
			t.Error("wrong credential source")
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	t.Setenv("HOLYARRS_METADATA_SERVICE_URL", srv.URL)
	t.Setenv("HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL", "true")
	t.Setenv("HOLYARRS_METADATA_SERVICE_TOKEN_FILE", "")
	md, _, _, err := metadataServiceProviders(vault, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = md.Search(ctx, "Fixture", 0)
	if err := vault.SetSetting(ctx, "links", "metadata_service_installation_token", "METADATA_VAULT_ROTATED", domain.System); err != nil {
		t.Fatal(err)
	}
	want = "METADATA_VAULT_ROTATED"
	_, _ = md.Search(ctx, "Fixture", 0)
	path := filepath.Join(t.TempDir(), "installation.txt")
	if err := os.WriteFile(path, []byte("METADATA_FILE_TEST"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOLYARRS_METADATA_SERVICE_TOKEN_FILE", path)
	want = "METADATA_FILE_TEST"
	md, _, _, err = metadataServiceProviders(vault, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = md.Search(ctx, "Fixture", 0)
	if calls != 3 {
		t.Fatalf("credential checks=%d, want3", calls)
	}
}
