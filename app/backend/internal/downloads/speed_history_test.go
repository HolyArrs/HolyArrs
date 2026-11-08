package downloads

import "testing"

func TestSpeedHistoryKeepsNewest(t *testing.T) {
	t.Parallel()
	var h SpeedHistory
	for i := 0; i < speedKeep+30; i++ {
		h.add(SpeedSample{Down: int64(i)})
	}
	all := h.last(0)
	if len(all) != speedKeep || all[0].Down != 30 || all[len(all)-1].Down != int64(speedKeep+29) {
		t.Fatalf("got %d samples, first %d", len(all), all[0].Down)
	}
	if speedKeep != 900 {
		t.Fatalf("keep %d, want 900 (15 minutes)", speedKeep)
	}
	if got := h.last(60); len(got) != 60 || got[59].Down != int64(speedKeep+29) {
		t.Fatalf("last(60) wrong: %d", len(got))
	}
}
