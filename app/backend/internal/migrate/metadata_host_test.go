package migrate

import "testing"

// TestMetadataHostLinkLocal: an app moved from a rootless Podman setup may
// name the host as 169.254.1.2; that is the local network, while cloud
// metadata addresses and IPv6 link-local stay refused.
func TestMetadataHostLinkLocal(t *testing.T) {
	for raw, want := range map[string]bool{
		"http://169.254.1.2:8112": false, "http://192.168.1.5:8080": false, "http://qbit:8080": false,
		"http://169.254.169.254/": true, "http://169.254.170.2/": true, "http://169.254.0.23/": true,
		"http://[fe80::1]/": true, "http://[fd00:ec2::254]/": true, "http://100.100.100.200/": true,
		"http://[::ffff:169.254.169.254]/": true, "http://metadata.google.internal/": true,
		"http://168.63.129.16/": true, "http://[fd20:ce::254]/": true, "http://[64:ff9b::a9fe:a9fe]/": true,
	} {
		if got := metadataHost(raw); got != want {
			t.Errorf("metadataHost(%s) = %v, want %v", raw, got, want)
		}
	}
}
