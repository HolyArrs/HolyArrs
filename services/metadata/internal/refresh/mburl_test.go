package refresh

import "testing"

func TestMBEndpoints(t *testing.T) {
	if MBInfoURL != "https://metabrainz.org/api/musicbrainz/replication-info" {
		t.Fatalf("info url %q", MBInfoURL)
	}
	if MBPacketURL != "https://metabrainz.org/api/musicbrainz/replication-%d.tar.bz2" {
		t.Fatalf("packet url %q", MBPacketURL)
	}
}
