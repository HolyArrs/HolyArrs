package main

import (
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
)

// Music-video jobs wait on YouTube's one-a-second pace; they may hold at
// most videoJobsMax picture workers, so covers and photos never queue
// behind them.
func TestVideoJobsHoldFewWorkers(t *testing.T) {
	sp := &speed{}
	for i := 0; i < videoJobsMax; i++ {
		if hold, _ := sp.holdImage("hv:a", fetchq.P1Finish); hold {
			t.Fatalf("video job %d held", i)
		}
	}
	if hold, d := sp.holdImage("hvsb:x", fetchq.P1Finish); !hold || d <= 0 {
		t.Fatal("video job over the cap not held")
	}
	if hold, _ := sp.holdImage("img:album:x", fetchq.P1Finish); hold {
		t.Fatal("picture job held")
	}
	sp.videoDone()
	if hold, _ := sp.holdImage("hv:b", fetchq.P1Finish); hold {
		t.Fatal("video job held after one finished")
	}
	if imageWorkers < 32 {
		t.Fatalf("image workers %d", imageWorkers)
	}
}

// Job starts are not the limit: each source keeps its own pace.
func TestImageStartRateDefault(t *testing.T) {
	if defaultImageRate < 20 {
		t.Fatalf("image start rate %v", defaultImageRate)
	}
}
