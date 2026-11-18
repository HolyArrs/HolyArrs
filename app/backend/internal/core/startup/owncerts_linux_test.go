package startup

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ownCA makes a root certificate and a server certificate for 127.0.0.1
// signed by it. It returns the root as PEM and the server's TLS pair.
func ownCA(t *testing.T) ([]byte, tls.Certificate) {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Home Lab Root"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTpl, caTpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(caDER)
	srvKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	srvTpl := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "torrents.home.lan"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}
	srvDER, err := x509.CreateCertificate(rand.Reader, srvTpl, ca, &srvKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		tls.Certificate{Certificate: [][]byte{srvDER}, PrivateKey: srvKey}
}

// TestOwnCertsChild is run in a child process (Go reads trusted certificates
// once per process): it calls UseOwnCerts, then one HTTPS request.
func TestOwnCertsChild(t *testing.T) {
	cfg, url := os.Getenv("OWNCERT_CONFIG"), os.Getenv("OWNCERT_URL")
	if cfg == "" {
		t.Skip("run by TestOwnCerts")
	}
	UseOwnCerts(cfg)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
}

func TestOwnCerts(t *testing.T) {
	caPEM, pair := ownCA(t)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{pair}}
	srv.StartTLS()
	defer srv.Close()

	child := func(cfg string, env ...string) error {
		cmd := exec.Command(os.Args[0], "-test.run=^TestOwnCertsChild$", "-test.count=1")
		cmd.Env = append(os.Environ(), "OWNCERT_CONFIG="+cfg, "OWNCERT_URL="+srv.URL)
		cmd.Env = append(cmd.Env, env...)
		out, err := cmd.CombinedOutput()
		if err != nil && !strings.Contains(string(out), "certificate") {
			t.Logf("child output: %s", out)
		}
		return err
	}

	t.Run("no certs folder fails", func(t *testing.T) {
		if err := child(t.TempDir()); err == nil {
			t.Fatal("request to a site signed by an unknown CA worked")
		}
	})
	t.Run("root in certs folder works", func(t *testing.T) {
		cfg := t.TempDir()
		os.MkdirAll(filepath.Join(cfg, "certs"), 0o755)
		os.WriteFile(filepath.Join(cfg, "certs", "root_ca.crt"), caPEM, 0o644)
		if err := child(cfg); err != nil {
			t.Fatalf("request failed with the root in certs: %v", err)
		}
	})
	t.Run("own SSL_CERT_FILE wins", func(t *testing.T) {
		cfg := t.TempDir()
		os.MkdirAll(filepath.Join(cfg, "certs"), 0o755)
		os.WriteFile(filepath.Join(cfg, "certs", "root_ca.crt"), caPEM, 0o644)
		empty := filepath.Join(t.TempDir(), "none.pem")
		os.WriteFile(empty, nil, 0o644)
		if err := child(cfg, "SSL_CERT_FILE="+empty, "SSL_CERT_DIR="+t.TempDir()); err == nil {
			t.Fatal("certs folder was used although SSL_CERT_FILE was set")
		}
	})
}

func TestCountCerts(t *testing.T) {
	caPEM, _ := ownCA(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.crt"), append(append([]byte{}, caPEM...), caPEM...), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o644)
	if n := countCerts(dir); n != 2 {
		t.Fatalf("countCerts = %d, want 2", n)
	}
}
