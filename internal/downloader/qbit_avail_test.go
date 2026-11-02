package downloader

import (
	"encoding/json"
	"testing"
)

// qBittorrent's availability reaches the Job; its -1 and a missing field read unknown.
func TestQbitAvailability(t *testing.T) {
	for in, want := range map[string]float64{`{"availability":0.75}`: 0.75, `{"availability":-1}`: -1, `{}`: -1} {
		var q qbitTorrent
		if err := json.Unmarshal([]byte(in), &q); err != nil {
			t.Fatal(err)
		}
		got := q.job().Availability
		if (want < 0) != (got == nil) || (got != nil && *got != want) {
			t.Fatalf("%s: %v", in, got)
		}
	}
}
