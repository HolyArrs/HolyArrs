package sources

import "testing"

func TestRefusedData(t *testing.T) {
	for _, c := range []struct {
		found   int
		blocked string
		want    any
	}{
		{4, "", 4},
		{0, "", 0},
		{4, "no download client takes this", nil},
	} {
		got := refusedData(c.found, c.blocked)
		if c.want == nil {
			if got != nil {
				t.Fatalf("%+v: got %v", c, got)
			}
			continue
		}
		if got["found"] != c.want {
			t.Fatalf("%+v: got %v", c, got)
		}
	}
}
