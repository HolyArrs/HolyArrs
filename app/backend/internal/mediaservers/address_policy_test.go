package mediaservers

import (
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// With the local network allowed, link-local and cloud
// metadata addresses are still refused.
func TestMetadataRefusedEvenWithLocalNetwork(t *testing.T) {
	c := client(domain.MediaServer{AllowLocal: true})
	for _, u := range []string{"http://169.254.169.254/latest/meta-data/", "http://[fe80::1]/", "http://[fd00:ec2::254]/", "http://100.100.100.200/"} {
		_, err := c.Get(u)
		if !errors.Is(err, outbound.ErrAddressRefused) {
			t.Fatalf("%s: %v", u, err)
		}
	}
}
