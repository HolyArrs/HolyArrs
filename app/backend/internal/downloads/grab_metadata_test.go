package downloads

import (
	"context"
	"strings"
	"testing"
)

// TestGrabLinkMetadataText: a Release link that leads to a cloud metadata
// address says so, instead of asking to turn on the local network.
func TestGrabLinkMetadataText(t *testing.T) {
	for _, u := range []string{"http://169.254.169.254/latest/x.torrent", "http://metadata.google.internal/x.torrent", "http://168.63.129.16/x.torrent"} {
		_, _, err := fetchGrabLink(context.Background(), u, 1<<20, nil, "")
		if err == nil || !strings.Contains(err.Error(), "cloud metadata address") || strings.Contains(err.Error(), "May reach the local network") {
			t.Errorf("%s: %v", u, err)
		}
	}
	_, _, err := fetchGrabLink(context.Background(), "http://192.168.1.5/x.torrent", 1<<20, nil, "")
	if err == nil || !strings.Contains(err.Error(), "May reach the local network") {
		t.Errorf("local link: %v", err)
	}
}
