package library

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

type countPrefetch struct{ metadata.Provider }

func (countPrefetch) Prefetch(context.Context, []metadata.TMDBRead) int { return 0 }

// Two folders with one name, windows apart, must not stall the scan.
func TestScanAheadDuplicateNamesDoNotStall(t *testing.T) {
	var folders []string
	for i := range 6 * scanWindow {
		folders = append(folders, fmt.Sprintf("Film %d (2001)", i))
	}
	folders[5*scanWindow] = "movies"
	folders[3] = "movies"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a := startAhead(ctx, countPrefetch{}, folders)
	defer a.close()
	for _, f := range folders {
		a.wait(ctx, f)
		if ctx.Err() != nil {
			t.Fatalf("scan stalled at %q", f)
		}
	}
}
