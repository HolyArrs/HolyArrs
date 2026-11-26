package arrapi

import (
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A plugin's own download type reads as "unknown" to the old apps' clients,
// on Releases, the queue, history, the blocklist and indexers alike.
func TestPluginProtocolReadsUnknown(t *testing.T) {
	for in, want := range map[string]string{"torrent": "torrent", "usenet": "usenet", "soulseek": "unknown", "": "unknown"} {
		if got := arrProtocol(in); got != want {
			t.Errorf("arrProtocol(%q) = %q, want %q", in, got, want)
		}
	}
	h := &handler{side: "radarr"}
	if got := h.releaseRow(domain.Release{ID: 1, Name: "Film.2024.1080p.WEB-DL-X", Protocol: "soulseek"}, "Soulseek", time.Now()).Protocol; got != "unknown" {
		t.Errorf("release protocol %q", got)
	}
	src := domain.Source{ID: 4, Name: "Soulseek", Protocol: "soulseek", Format: "torznab", URL: "plugin://slskd", Extra: map[string]string{}}
	if got := h.indexerOf(src)["protocol"]; got != "unknown" {
		t.Errorf("indexer protocol %v", got)
	}
	src.Protocol = "torrent"
	if got := h.indexerOf(src)["protocol"]; got != "torrent" {
		t.Errorf("torrent indexer protocol %v", got)
	}
}
