package downloads

import (
	"net"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// TestExplainDownloaderMetadata: with the local network already allowed, a
// refused cloud metadata address must not tell the owner to turn that on.
func TestExplainDownloaderMetadata(t *testing.T) {
	for _, u := range []string{"http://169.254.169.254/", "http://metadata.google.internal/", "http://168.63.129.16/"} {
		_, err := Client(u, true).Get(u)
		if err == nil {
			t.Fatalf("%s: reached", u)
		}
		for _, e := range []error{err, outbound.Refusal(err)} {
			_, got := explainDownloader(e, u, true)
			if !strings.Contains(got, "cloud metadata") || strings.Contains(got, "Turn on") {
				t.Errorf("%s: %q", u, got)
			}
		}
	}
	_, got := explainDownloader(&net.OpError{Op: "dial", Err: outbound.ErrAddressRefused}, "http://192.168.1.10:8080/", false)
	if got != doctor.Line(doctor.Blocked) {
		t.Errorf("local address refused: %q", got)
	}
}
