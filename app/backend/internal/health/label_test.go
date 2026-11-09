package health

import "testing"

// Every check kind gets plain words in the "Fixed: ..." recovery event,
// never its internal key ("backups" is already a plain word).
func TestLabelHasPlainWordsForEveryKind(t *testing.T) {
	for _, k := range []string{"library_folder", "disk_space", "source", "downloader", "waiting_downloads", "update", "trailers", "config_folder", "config_disk"} {
		if got := label(k); got == k {
			t.Errorf("label(%q) returns the internal key", k)
		}
	}
}
