package downloads

import (
	"encoding/json"
	"testing"
)

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
