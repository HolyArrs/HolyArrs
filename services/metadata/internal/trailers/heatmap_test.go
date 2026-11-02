package trailers

import "testing"

func TestParseHeatmap(t *testing.T) {
	p, d, ok, err := ParseHeatmap([]byte(`{"duration":200,"heatmap":[{"start_time":0,"value":0.2},{"start_time":62.5,"value":1},{"start_time":70,"value":0.9}]}`))
	if err != nil || !ok || p != 62.5 || d != 200 {
		t.Fatal(p, d, ok, err)
	}
	if _, d, ok, _ := ParseHeatmap([]byte(`{"duration":90}`)); ok || d != 90 {
		t.Fatal("no heatmap must be not found")
	}
}
