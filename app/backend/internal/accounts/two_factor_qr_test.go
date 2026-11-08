package accounts

import (
	"strings"
	"testing"
)

// route: POST /api/v1/auth/two-factor/setup
// Setup also gives a code to scan: a square of 0/1 rows with finder corners.
func TestTwoFactorSetupGivesAQRCode(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	resp, out := own.do(t, f, "POST", "/api/v1/auth/two-factor/setup", "")
	if resp.StatusCode != 200 {
		t.Fatalf("setup: %d %s", resp.StatusCode, out)
	}
	var s struct {
		Secret string   `json:"secret"`
		QR     []string `json:"qr"`
	}
	jsonOf(t, out, &s)
	n := len(s.QR)
	if n < 21 || (n-17)%4 != 0 {
		t.Fatalf("qr size %d", n)
	}
	for _, row := range s.QR {
		if len(row) != n || strings.Trim(row, "01") != "" {
			t.Fatalf("qr row %q", row)
		}
	}
	if !strings.HasPrefix(s.QR[0], "1111111") || !strings.HasSuffix(s.QR[0], "1111111") {
		t.Fatalf("finder corners missing: %q", s.QR[0])
	}
	if strings.Contains(strings.Join(s.QR, ""), s.Secret) {
		t.Fatal("the key leaked into the code rows as text")
	}
}
