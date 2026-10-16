package downloader

import (
	"errors"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/source"
)

// The Downloader client with the local network allowed still refuses every
// cloud metadata address the Sources refuse, not only link-local ones.
func TestClientRefusesMetadataAddresses(t *testing.T) {
	for _, u := range []string{"http://100.100.100.200/", "http://[fd00:ec2::254]/", "http://169.254.169.254/"} {
		_, err := Client(u, true).Get(u)
		if err == nil || !errors.Is(err, source.ErrAddressRefused) {
			t.Errorf("%s: %v", u, err)
		}
	}
}
