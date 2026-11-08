package downloads

import (
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

func TestClientRefusesMetadataAddresses(t *testing.T) {
	for _, u := range []string{"http://100.100.100.200/", "http://[fd00:ec2::254]/", "http://169.254.169.254/"} {
		_, err := Client(u, true).Get(u)
		if err == nil || !errors.Is(err, outbound.ErrAddressRefused) {
			t.Errorf("%s: %v", u, err)
		}
	}
}
