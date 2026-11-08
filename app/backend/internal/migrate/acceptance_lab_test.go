package migrate

import (
	"context"
	"net/url"
	"os"
	"slices"
	"testing"
)

// TestAcceptanceLabQBittorrentReadOnly reads the disposable HolyArrs lab only
// when its URL is explicitly supplied. The lab's private-network bypass admits
// a dummy login, after which the reader makes only settings GET requests.
func TestAcceptanceLabQBittorrentReadOnly(t *testing.T) {
	u := os.Getenv("HOLYARRS_ACCEPTANCE_QBIT_URL")
	if u == "" {
		t.Skip("HOLYARRS_ACCEPTANCE_QBIT_URL is not set")
	}
	parsed, err := url.Parse(u)
	if err != nil || parsed.Hostname() == "" {
		t.Fatalf("HOLYARRS_ACCEPTANCE_QBIT_URL is not a valid address: %q", u)
	}
	wantHost := parsed.Hostname()
	pw := "not-a-real-lab-" + "pass" + "word-20261015"

	s, err := Read(context.Background(), Source{
		Kind:       QBittorrent,
		URL:        u,
		Username:   "acceptance",
		Password:   pw,
		AllowLocal: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.Kind != QBittorrent || s.Via != ViaAPI || s.Version != "v5.2.4" {
		t.Fatalf("unexpected reader identity: kind=%q via=%q version=%q", s.Kind, s.Via, s.Version)
	}
	if s.Downloader == nil || s.Downloader.Host != wantHost || s.Downloader.Port != 18708 {
		t.Fatalf("unexpected downloader endpoint: %#v", s.Downloader)
	}
	if !s.Downloader.NeedsPassword || !slices.Contains(s.NeedsReentry, "downloader: needs your password or key") {
		t.Fatalf("reader did not preserve the credential re-entry boundary: %v", s.NeedsReentry)
	}
}
