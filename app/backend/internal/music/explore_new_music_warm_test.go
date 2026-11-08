package music

import (
	"context"
	"testing"
	"time"
)

// After the read-ahead has built New & Popular → Music, a cold visit (even
// after a restart wipes memory) answers at once with no building state.
func TestMusicNewPrebuildServesColdVisit(t *testing.T) {
	a, srv, _ := newRig(t)
	musicNew = musicNewStore{}
	t.Cleanup(func() { musicNew = musicNewStore{} })
	if !a.refreshMusicNew(context.Background()) {
		t.Fatal("prebuild did not run")
	}
	musicNew = musicNewStore{} // a restart: only the kept build is left
	body := getJSON(t, srv.URL+"/api/v1/explore/new/music", 200)
	if body["building"] != false {
		t.Fatalf("cold visit after prebuild still building: %v", body)
	}
	if _, ok := body["rows"].([]any); !ok {
		t.Fatalf("answer %v", body)
	}
	musicNew.mu.Lock()
	d := musicNew.data
	musicNew.mu.Unlock()
	if d == nil || time.Since(d.at) > musicNewTTL {
		t.Fatal("kept build not loaded")
	}
}

// With Music off, the read-ahead loop never runs, so nothing is prebuilt.
func TestMusicNewPrebuildOffWhenMusicOff(t *testing.T) {
	a, _, _ := newRig(t)
	musicNew = musicNewStore{}
	t.Cleanup(func() { musicNew = musicNewStore{} })
	pc := musicOffControl(t, a.s.Store)
	for _, l := range a.MusicLoops() {
		pc.Add(l, true)
	}
	time.Sleep(musicWarmDelay + 500*time.Millisecond)
	musicNew.mu.Lock()
	built := musicNew.data != nil || musicNew.building
	musicNew.mu.Unlock()
	if built {
		t.Fatal("Music is off but New & Popular → Music was prebuilt")
	}
	if a.warmMusicNew() != nil {
		t.Fatal("warmMusicNew answered while Music is off")
	}
}
