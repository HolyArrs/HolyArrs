package main

import (
	"context"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

// Every picture-queue fetch queues its big size (made later at P3), so
// the fill never spends the CPU slot on 1200 px encodes.
func TestImageJobsQueueBigSize(t *testing.T) {
	for _, p := range []int{fetchq.P0Waiting, fetchq.P1Finish, fetchq.P2Shared, fetchq.P4Deep} {
		if !imgstore.IsBigLater(imageJobCtx(context.Background(), p)) {
			t.Fatalf("P%d makes the big size inline", p)
		}
	}
	if imgstore.SlotCount() != 1 {
		t.Fatalf("picture CPU slots %d, want 1 (one core stays for pages)", imgstore.SlotCount())
	}
}

// Deferred big sizes (1200 px) drain in the background lane: P4 keeps its
// small guaranteed share and every idle slot, and never runs ahead of
// P0 to P3 (audit 2026-10-29: 31,814 queued at P3).
func TestBigSizesInBackgroundLane(t *testing.T) {
	if bigPriority != fetchq.P4Deep {
		t.Fatalf("big sizes at P%d, want P4", bigPriority)
	}
}
