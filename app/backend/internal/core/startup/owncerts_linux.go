package startup

import (
	"encoding/pem"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// certsFolder is the folder inside the config folder where people drop the
// root certificates of their own certificate authority (PEM files). HolyArrs
// then trusts sites signed by it for every outgoing HTTPS call.
const certsFolder = "certs"

// systemCertDirs are the folders Go reads when SSL_CERT_DIR is not
// set; they stay in the list so the usual system certificates keep working.
var systemCertDirs = []string{"/etc/ssl/certs", "/etc/pki/tls/certs", "/system/etc/security/cacerts"}

// UseOwnCerts adds <config>/certs to the certificate folders Go trusts, when
// that folder exists and the person has not set SSL_CERT_DIR or SSL_CERT_FILE
// themselves. It must run before the first HTTPS call, because Go reads the
// trusted certificates once. It returns how many certificates it found.
func UseOwnCerts(configDir string) int {
	if os.Getenv("SSL_CERT_DIR") != "" || os.Getenv("SSL_CERT_FILE") != "" {
		return 0
	}
	dir := filepath.Join(configDir, certsFolder)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return 0
	}
	n := countCerts(dir)
	os.Setenv("SSL_CERT_DIR", strings.Join(append([]string{dir}, systemCertDirs...), ":"))
	if n == 0 {
		log.Printf("holyarrs: the folder %s has no PEM certificates in it; add your own root certificate there and restart", dir)
	} else {
		log.Printf("holyarrs: trusting %d of your own certificate(s) from %s", n, dir)
	}
	return n
}

// countCerts counts the PEM certificates in the files directly inside dir.
func countCerts(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		for {
			var b *pem.Block
			b, data = pem.Decode(data)
			if b == nil {
				break
			}
			if b.Type == "CERTIFICATE" {
				n++
			}
		}
	}
	return n
}
